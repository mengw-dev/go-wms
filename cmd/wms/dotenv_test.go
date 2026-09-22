package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	const existing = "WMS_TEST_DOTENV_EXISTING"
	const quoted = "WMS_TEST_DOTENV_QUOTED"
	t.Setenv(existing, "process")
	t.Setenv(quoted, "")
	if err := os.Unsetenv(quoted); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), ".env")
	content := "# comment\n" + existing + "=file\n" + quoted + "=\"value=with'quote\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv(existing) != "process" {
		t.Fatal("existing environment overwritten")
	}
	if got := os.Getenv(quoted); got != "value=with'quote" {
		t.Fatalf("quoted value=%q", got)
	}
	if err := loadDotEnv(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDotEnvReportsReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 128*1024)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadDotEnv(path); err == nil {
		t.Fatal("scanner error was ignored")
	}
}
