// Command check-vulns runs govulncheck and applies the repository's vulnerability policy.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	scanner := flag.String("scanner", "govulncheck", "govulncheck executable")
	report := flag.String("report", "vuln.json", "raw JSON report path")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return runScan(ctx, *scanner, *report, os.Stdout, os.Stderr)
}

func runScan(ctx context.Context, scanner, reportPath string, out, stderr io.Writer) error {
	// 输出位置由运行此本地 CLI 的开发者/CI 显式指定，不接受网络请求参数。
	report, err := os.OpenFile(reportPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600) // #nosec G304 -- intentional developer-selected report output path.
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, scanner, "-format=json", "-scan=symbol", "./...")
	cmd.Stdout, cmd.Stderr = report, stderr
	runErr := cmd.Run()
	if runErr != nil {
		// JSON 模式的扫描失败不是“没有漏洞”；即使已输出部分 finding，也不能放行。
		return fmt.Errorf("漏洞扫描执行失败，不能判定为安全（原始报告保留在 %s）: %w", reportPath, errors.Join(runErr, report.Close()))
	}
	if _, err := report.Seek(0, io.SeekStart); err != nil {
		return errors.Join(err, report.Close())
	}
	result, parseErr := parseReport(report)
	if err := errors.Join(parseErr, report.Close()); err != nil {
		return fmt.Errorf("漏洞扫描报告无效，不能判定为安全: %w", err)
	}
	for _, item := range result.unfixed {
		if _, err := fmt.Fprintln(out, "::warning::已调用漏洞尚无上游修复版本："+item); err != nil {
			return err
		}
	}
	for _, item := range result.fixable {
		if _, err := fmt.Fprintln(out, "::error::已调用漏洞需要升级："+item); err != nil {
			return err
		}
	}
	if len(result.fixable) > 0 {
		return errors.New("漏洞门禁未通过：存在已调用且已有修复版本的漏洞")
	}
	_, err = fmt.Fprintf(out, "漏洞扫描执行完成：无已调用且已有修复版本的漏洞；无修复版本告警 %d 项。\n", len(result.unfixed))
	return err
}

type scanResult struct {
	fixable []string
	unfixed []string
}

// parseReport 消费完整 JSON 对象流；空输出、截断、未知协议或非符号级报告都不能放行。
func parseReport(r io.Reader) (*scanResult, error) {
	decoder := json.NewDecoder(r)
	var configs, sboms int
	fixable, unfixed := map[string]bool{}, map[string]bool{}
	for {
		var message map[string]json.RawMessage
		if err := decoder.Decode(&message); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		if len(message) != 1 {
			return nil, errors.New("每条记录必须包含一个消息")
		}
		for kind, raw := range message {
			if string(raw) == "null" {
				return nil, errors.New("消息不能为空")
			}
			switch kind {
			case "config":
				var config struct {
					ProtocolVersion string `json:"protocol_version"`
					ScanLevel       string `json:"scan_level"`
					ScanMode        string `json:"scan_mode"`
				}
				if err := json.Unmarshal(raw, &config); err != nil {
					return nil, err
				}
				if config.ProtocolVersion != "v1.0.0" || config.ScanLevel != "symbol" || config.ScanMode != "source" {
					return nil, errors.New("不支持的协议、扫描级别或扫描模式")
				}
				configs++
			case "SBOM":
				var sbom struct {
					Roots []string `json:"roots"`
				}
				if err := json.Unmarshal(raw, &sbom); err != nil {
					return nil, err
				}
				if len(sbom.Roots) == 0 {
					return nil, errors.New("没有实际扫描目标")
				}
				sboms++
			case "finding":
				var finding struct {
					OSV          string `json:"osv"`
					FixedVersion string `json:"fixed_version"`
					Trace        []struct {
						Function string `json:"function"`
					} `json:"trace"`
				}
				if err := json.Unmarshal(raw, &finding); err != nil {
					return nil, err
				}
				if finding.OSV == "" || len(finding.Trace) == 0 {
					return nil, errors.New("finding 缺少漏洞编号或调用信息")
				}
				for _, frame := range finding.Trace {
					if frame.Function == "" {
						continue
					}
					if strings.TrimSpace(finding.FixedVersion) == "" {
						unfixed[finding.OSV] = true
					} else {
						fixable[finding.OSV+" 修复版本 "+finding.FixedVersion] = true
					}
					break
				}
			case "osv", "progress":
				// 保留在原始报告中，不参与现有“已调用且有修复版本”的阻断策略。
			default:
				return nil, fmt.Errorf("未知消息类型 %q", kind)
			}
		}
	}
	if configs != 1 || sboms != 1 {
		return nil, errors.New("报告缺少或重复配置/扫描目标记录")
	}
	result := &scanResult{}
	for item := range fixable {
		result.fixable = append(result.fixable, item)
	}
	for item := range unfixed {
		result.unfixed = append(result.unfixed, item)
	}
	slices.Sort(result.fixable)
	slices.Sort(result.unfixed)
	return result, nil
}
