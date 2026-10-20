package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDependencyDirection(t *testing.T) {
	for _, test := range []struct{ file, dependency, reason string }{
		{"internal/pkg/lock/lock.go", modulePrefix + "inventory/model", "基础包"},
		{"internal/modules/inbound/service/order.go", modulePrefix + "demo/service", "Demo"},
		{"internal/modules/future/module.go", modulePrefix + "demo/service", "Demo"},
		{"internal/modules/future/handler/http.go", modulePrefix + "future/repository/nested", "Repository"},
		{"internal/modules/inbound/handler/http.go", "gorm.io/gorm", "数据库"},
		{"internal/modules/inbound/handler/http.go", "database/sql", "数据库"},
		{"internal/modules/inbound/api/api.go", modulePrefix + "inbound/service", "实现层"},
		{"internal/modules/inbound/model/model.go", modulePrefix + "inbound/dto", "DTO"},
		{"internal/modules/inbound/dto/dto.go", modulePrefix + "inbound/repository", "实现层"},
		{"internal/modules/inventory/repository/query.go", modulePrefix + "inventory/service", "Repository"},
		{"internal/modules/inbound/service/order.go", modulePrefix + "outbound/repository", "Repository"},
		{"internal/modules/inbound/service/order.go", modulePrefix + "inventory/service", "跨模块"},
		{"internal/modules/inbound/service/order.go", modulePrefix + "inbound/handler", "Handler"},
		{`internal\modules\future\api\api.go`, modulePrefix + "future/service", "实现层"},
	} {
		t.Run(test.file+"->"+test.dependency, func(t *testing.T) {
			if reason := forbidden(test.file, test.dependency); !strings.Contains(reason, test.reason) {
				t.Fatalf("forbidden import was not rejected: %q", reason)
			}
		})
	}
}

func TestAcceptedAssemblyAndCrossModelDependencies(t *testing.T) {
	for _, test := range []struct{ file, dependency string }{
		{"internal/app/app.go", modulePrefix + "inbound/service"},
		{"internal/bootstrap/seed.go", modulePrefix + "system/model"},
		{"internal/testutil/fixture.go", modulePrefix + "inbound/service"},
		{"internal/modules/demo/service/service.go", modulePrefix + "inbound/service"},
		{"internal/modules/inventory/repository/stock.go", modulePrefix + "basic/model"},
		{"internal/modules/ai/repository/query.go", modulePrefix + "inventory/model"},
		{"internal/modules/inbound/dto/dto.go", modulePrefix + "task/model"},
		{"internal/modules/inbound/service/order.go", modulePrefix + "inbound/repository"},
		{"internal/modules/system/repository/user.go", "gorm.io/gorm"},
		{"internal/modules/inbound/service/order.go", modulePrefix + "inventory/api"},
		{"internal/modules/inbound/service/order_test.go", modulePrefix + "inventory/service"},
		{"internal/pkg/tenant/gorm_test.go", modulePrefix + "basic/model"},
	} {
		if reason := forbidden(test.file, test.dependency); reason != "" {
			t.Errorf("legal dependency %s -> %s rejected: %s", test.file, test.dependency, reason)
		}
	}
}

func fixtureFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScanRejectsAliasedBlankAndTaggedImportsAcrossWholeTree(t *testing.T) {
	root := t.TempDir()
	fixtureFile(t, root, "go.mod", "module gowms\n")
	fixtureFile(t, root, "internal/pkg/early/file.go", "package early\nimport renamed \"gowms/internal/modules/basic/model\"\n")
	fixtureFile(t, root, "internal/modules/new/handler/file.go", "//go:build never_enabled\n\npackage handler\nimport _ `gowms/internal/modules/new/repository`\n")
	fixtureFile(t, root, "internal/modules/new/service/fixture_test.go", "package service\nimport _ \"gowms/internal/modules/basic/service\"\n")
	err := check(root, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "early/file.go:2") || !strings.Contains(err.Error(), "handler/file.go:4") {
		t.Fatalf("whole tree violations were lost: %v", err)
	}
}

func TestScanFailsClosedForMissingEmptyAndMalformedSources(t *testing.T) {
	root := t.TempDir()
	if err := check(root, io.Discard); err == nil {
		t.Fatal("missing root accepted")
	}
	fixtureFile(t, root, "go.mod", "module gowms\n")
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := check(root, io.Discard); err == nil {
		t.Fatal("empty tree accepted")
	}
	fixtureFile(t, root, "internal/pkg/bad/file.go", "package bad\nimport \"unterminated\n")
	if err := check(root, io.Discard); err == nil {
		t.Fatal("malformed import accepted")
	}
}

// This is the same complete scan used by CI and verify; it never requires MySQL/Redis.
func TestRepositoryDependencyDirection(t *testing.T) {
	var output bytes.Buffer
	if err := check(filepath.Join("..", ".."), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "full internal tree") {
		t.Fatal(output.String())
	}
}
