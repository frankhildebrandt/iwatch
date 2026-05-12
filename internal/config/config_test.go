package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigPrecedence(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(base, ".iwatch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".iwatch"), 0o755); err != nil {
		t.Fatal(err)
	}

	mustWrite(t, filepath.Join(home, ".iwatch", "config.json"), `{"watchPath":"home","bufferLines":10,"ui":{"focusPane":"events"}}`)
	mustWrite(t, filepath.Join(base, ".iwatch", "config.json"), `{"watchPath":"project","bufferLines":20}`)
	mustWrite(t, filepath.Join(base, ".iwatch.config.json"), `{"watchPath":"root","bufferLines":30}`)

	cfg, path, err := ResolveConfig(base, CLIOverrides{WatchPath: "flag", BufferLines: 44, CommandID: "make:run"})
	if err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	if path != filepath.Join(base, ".iwatch.config.json") {
		t.Fatalf("used path = %s", path)
	}
	if cfg.WatchPath != "flag" || cfg.BufferLines != 44 || cfg.DefaultCommand != "make:run" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.UI.FocusPane != "events" {
		t.Fatalf("expected inherited UI focus pane from home config, got %+v", cfg.UI)
	}
}

func TestResolveConfigExplicitPath(t *testing.T) {
	base := t.TempDir()
	explicit := filepath.Join(base, "custom.json")
	mustWrite(t, explicit, `{"watchPath":"explicit","bufferLines":64}`)

	cfg, path, err := ResolveConfig(base, CLIOverrides{ConfigPath: explicit})
	if err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	if path != explicit {
		t.Fatalf("path = %s", path)
	}
	if cfg.WatchPath != "explicit" || cfg.BufferLines != 64 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
