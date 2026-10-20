// Command check-dependencies checks Go import direction without loading business packages.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const modulePrefix = "gowms/internal/modules/"

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if err := check(*root, os.Stdout); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

// check scans every internal Go file, including files excluded by the current build tags.
// Tests may assemble real cross-module services as fixtures; their production files remain guarded.
func check(root string, out io.Writer) error {
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return fmt.Errorf("repository root must contain go.mod: %w", err)
	}
	var violations []error
	files, imports := 0, 0
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		positions := token.NewFileSet()
		file, err := parser.ParseFile(positions, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		files++
		for _, dependency := range file.Imports {
			name, err := strconv.Unquote(dependency.Path.Value)
			if err != nil {
				return err
			}
			imports++
			if reason := forbidden(relative, name); reason != "" {
				position := positions.Position(dependency.Pos())
				violations = append(violations, fmt.Errorf("%s:%d: %s: %s", filepath.ToSlash(relative), position.Line, reason, name))
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("dependency scan failed: %w", err)
	}
	if files == 0 {
		return errors.New("dependency scan found no Go files")
	}
	if len(violations) > 0 {
		return errors.Join(violations...)
	}
	_, err = fmt.Fprintf(out, "Go dependency direction passed: %d files, %d imports checked (full internal tree).\n", files, imports)
	return err
}

func forbidden(filename, dependency string) string {
	filename = strings.ReplaceAll(filename, "\\", "/")
	if strings.HasSuffix(filename, "_test.go") {
		return ""
	}
	if strings.HasPrefix(filename, "internal/pkg/") && strings.HasPrefix(dependency, modulePrefix) {
		return "基础包不能依赖业务模块"
	}
	source := strings.Split(filename, "/")
	if len(source) < 4 || source[0] != "internal" || source[1] != "modules" {
		return "" // app/bootstrap/testutil are explicit assembly or test infrastructure.
	}
	owner, layer := source[2], ""
	if len(source) >= 5 {
		layer = source[3]
	}
	if layer == "handler" && (dependency == "database/sql" || strings.HasPrefix(dependency, "gorm.io/")) {
		return "Handler 应通过 Service 访问数据库"
	}
	if !strings.HasPrefix(dependency, modulePrefix) {
		return ""
	}
	target := strings.Split(strings.TrimPrefix(dependency, modulePrefix), "/")
	if target[0] == "demo" && owner != "demo" {
		return "业务模块不能反向依赖 Demo"
	}
	if len(target) < 2 {
		return ""
	}
	targetLayer := target[1]
	if targetLayer == "handler" {
		return "业务实现不能依赖 Handler，路由装配归 internal/app"
	}
	if targetLayer == "repository" && (layer == "handler" || owner != target[0]) {
		return "不能绕过 Service/API 直接调用 Repository"
	}
	if (layer == "api" || layer == "model" || layer == "dto") && (targetLayer == "service" || targetLayer == "repository") {
		return "API/model/DTO 不能依赖实现层"
	}
	if layer == "repository" && targetLayer == "service" {
		return "Repository 不能反向依赖 Service"
	}
	if targetLayer == "service" && owner != target[0] && (owner != "demo" || layer != "service") {
		return "跨模块调用应依赖现有 API，Demo Service 编排除外"
	}
	if layer == "model" && targetLayer == "dto" {
		return "model 不能反向依赖请求 DTO"
	}
	return ""
}
