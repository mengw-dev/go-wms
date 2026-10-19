package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cleanReport = `{"config":{"protocol_version":"v1.0.0","scan_level":"symbol","scan_mode":"source"}}
{"SBOM":{"roots":["gowms/cmd/wms"]}}
`

const fixableFinding = `{"finding":{"osv":"GO-FIXABLE","fixed_version":"v1.2.3","trace":[{"module":"example.com/lib","function":"Vulnerable"}]}}
`

const unfixedFinding = `{"finding":{"osv":"GO-NOFIX","trace":[{"module":"example.com/lib","function":"Vulnerable"}]}}
`

func TestMain(m *testing.M) {
	// 当前测试可执行文件充当独立扫描器，真实验证进程退出状态与残缺 stdout 的处理。
	if mode := os.Getenv("WMS_VULN_GATE_TEST_SCANNER"); mode != "" {
		if strings.Join(os.Args[1:], " ") != "-format=json -scan=symbol ./..." {
			os.Exit(9)
		}
		body, code := cleanReport, 0
		switch mode {
		case "failure":
			code = 2
			body = ""
		case "partial failure":
			code = 2
		case "empty":
			body = ""
		case "truncated":
			body += `{"finding":`
		case "fixable":
			body += fixableFinding
		case "unfixed":
			body += unfixedFinding
		}
		if _, err := io.WriteString(os.Stdout, body); err != nil {
			os.Exit(9)
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestScannerExecutionAndPolicy(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ mode, errorPart string }{
		{"clean", ""}, {"unfixed", ""}, {"fixable", "漏洞门禁未通过"},
		{"failure", "扫描执行失败"}, {"partial failure", "扫描执行失败"},
		{"empty", "报告无效"}, {"truncated", "报告无效"},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			t.Setenv("WMS_VULN_GATE_TEST_SCANNER", tt.mode)
			report := filepath.Join(t.TempDir(), "vuln.json")
			var output, stderr bytes.Buffer
			err := runScan(context.Background(), executable, report, &output, &stderr)
			if tt.errorPart == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.errorPart) {
				t.Fatalf("error=%v want %q", err, tt.errorPart)
			}
			if tt.errorPart != "" && strings.Contains(output.String(), "扫描执行完成") {
				t.Fatal("failure reported success")
			}
			if tt.mode == "unfixed" && !strings.Contains(output.String(), "::warning::") {
				t.Fatal("unfixed called vulnerability lost its warning")
			}
			if _, err := os.Stat(report); err != nil {
				t.Fatalf("diagnostic report missing: %v", err)
			}
		})
	}
}

func TestMissingScannerFails(t *testing.T) {
	var output bytes.Buffer
	err := runScan(context.Background(), filepath.Join(t.TempDir(), "missing-scanner"), filepath.Join(t.TempDir(), "report.json"), &output, io.Discard)
	if err == nil || output.Len() != 0 {
		t.Fatalf("missing scanner accepted: %v %s", err, output.String())
	}
}

func TestCanceledScannerFails(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runScan(ctx, executable, filepath.Join(t.TempDir(), "report.json"), io.Discard, io.Discard); err == nil {
		t.Fatal("canceled scan accepted")
	}
}

func TestReportRejectsMalformedOrIncompleteData(t *testing.T) {
	for name, body := range map[string]string{
		"empty": "", "array": "[]", "null": "null", "empty object": "{}", "missing SBOM": strings.Split(cleanReport, "\n")[0],
		"missing config":      `{"SBOM":{"roots":["app"]}}`,
		"duplicate config":    cleanReport + cleanReport,
		"wrong protocol":      strings.ReplaceAll(cleanReport, "v1.0.0", "v2.0.0"),
		"wrong level":         strings.ReplaceAll(cleanReport, "symbol", "package"),
		"wrong mode":          strings.ReplaceAll(cleanReport, "source", "binary"),
		"no targets":          strings.ReplaceAll(cleanReport, `["gowms/cmd/wms"]`, `[]`),
		"unknown message":     cleanReport + `{"unexpected":{}}`,
		"null message":        cleanReport + `{"finding":null}`,
		"broken finding":      cleanReport + `{"finding":{"osv":"GO-1"}}`,
		"wrong trace type":    cleanReport + `{"finding":{"osv":"GO-1","trace":{}}}`,
		"trailing truncation": cleanReport + `{"finding":`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseReport(strings.NewReader(body)); err == nil {
				t.Fatal("invalid report accepted")
			}
		})
	}
}

func TestReportPreservesCalledOnlyPolicyAndDeduplicates(t *testing.T) {
	moduleFinding := `{"finding":{"osv":"GO-MODULE","fixed_version":"v1.0.0","trace":[{"module":"example.com/lib"}]}}`
	packageFinding := `{"finding":{"osv":"GO-PACKAGE","fixed_version":"v1.0.0","trace":[{"module":"example.com/lib","package":"example.com/lib/p"}]}}`
	result, err := parseReport(strings.NewReader(cleanReport + moduleFinding + packageFinding + fixableFinding + fixableFinding + unfixedFinding + unfixedFinding))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.fixable) != 1 || result.fixable[0] != "GO-FIXABLE 修复版本 v1.2.3" || len(result.unfixed) != 1 || result.unfixed[0] != "GO-NOFIX" {
		t.Fatalf("policy or deduplication drift: %+v", result)
	}
}
