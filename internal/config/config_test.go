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

func TestResolveProjectConfigPathPrefersProjectFiles(t *testing.T) {
	base := t.TempDir()
	projectPath := filepath.Join(base, ".iwatch", "config.json")
	if err := os.MkdirAll(filepath.Dir(projectPath), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, projectPath, `{}`)

	got := ResolveProjectConfigPath(base, filepath.Join(t.TempDir(), "config.json"))
	if got != projectPath {
		t.Fatalf("project path = %s", got)
	}
}

func TestSaveWritesNormalizedProjectConfig(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".iwatch", "config.json")
	cfg := Config{
		UI: UIConfig{
			Presets: []FilterPreset{{ID: "ops", Title: "Ops"}},
		},
	}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, ok, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile() error = %v", err)
	}
	if !ok {
		t.Fatal("expected saved file to exist")
	}
	loaded = DefaultMerge(loaded)
	if loaded.UI.ActivePreset != "ops" {
		t.Fatalf("active preset = %q", loaded.UI.ActivePreset)
	}
	if loaded.UI.LogView.TimeFormat != "time" || loaded.UI.LogView.WrapMode != "off" {
		t.Fatalf("unexpected log view defaults: %+v", loaded.UI.LogView)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
