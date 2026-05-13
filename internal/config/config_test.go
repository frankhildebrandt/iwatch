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
	if loaded.UI.LogView.Palette != DefaultLogPalette {
		t.Fatalf("palette = %q, want %q", loaded.UI.LogView.Palette, DefaultLogPalette)
	}
}

func TestLogViewPaletteDefaults(t *testing.T) {
	cfg := DefaultMerge(Config{})

	if cfg.UI.LogView.Palette != DefaultLogPalette {
		t.Fatalf("palette = %q, want %q", cfg.UI.LogView.Palette, DefaultLogPalette)
	}
}

func TestLogViewPaletteNormalizesInvalidValues(t *testing.T) {
	cfg := DefaultMerge(Config{
		UI: UIConfig{
			LogView: LogViewConfig{Palette: "unknown"},
		},
	})

	if cfg.UI.LogView.Palette != DefaultLogPalette {
		t.Fatalf("palette = %q, want %q", cfg.UI.LogView.Palette, DefaultLogPalette)
	}
}

func TestResolveConfigLoadsValidPalette(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".iwatch", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, `{"ui":{"logView":{"palette":"ocean"}}}`)

	cfg, _, err := ResolveConfig(base, CLIOverrides{})
	if err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	if cfg.UI.LogView.Palette != "ocean" {
		t.Fatalf("palette = %q, want ocean", cfg.UI.LogView.Palette)
	}
}

func TestLogViewHiddenFieldsNormalizeAndClone(t *testing.T) {
	cfg := DefaultMerge(Config{
		UI: UIConfig{
			LogView: LogViewConfig{
				VisibleFields: []string{" Level ", "msg", "level"},
				HiddenFields:  []string{" Component ", "msg", "component"},
			},
		},
	})

	if got, want := cfg.UI.LogView.VisibleFields, []string{"level", "msg"}; !sameStrings(got, want) {
		t.Fatalf("VisibleFields = %#v, want %#v", got, want)
	}
	if got, want := cfg.UI.LogView.HiddenFields, []string{"component", "msg"}; !sameStrings(got, want) {
		t.Fatalf("HiddenFields = %#v, want %#v", got, want)
	}

	cloned := Clone(cfg)
	cloned.UI.LogView.HiddenFields[0] = "changed"
	if cfg.UI.LogView.HiddenFields[0] != "component" {
		t.Fatalf("Clone() shared hidden fields backing array: %#v", cfg.UI.LogView.HiddenFields)
	}
}

func TestStreamsNormalizeAndClone(t *testing.T) {
	cfg := DefaultMerge(Config{
		Streams: []StreamConfig{
			{ID: " app ", Title: " App ", Type: "PROCESS", Cmd: "tail -f app.log", AutoStart: boolPtr(false)},
			{ID: "app", Source: "other.log"},
		},
		UI: UIConfig{
			Presets: []FilterPreset{
				{ID: "ops", Title: "Ops", Streams: []string{" app ", "app", "app", ""}},
			},
		},
	})

	if got, want := cfg.Streams[0].ID, "app"; got != want {
		t.Fatalf("stream id = %q, want %q", got, want)
	}
	if got, want := cfg.Streams[0].Type, "process"; got != want {
		t.Fatalf("stream type = %q, want %q", got, want)
	}
	if boolValue(cfg.Streams[0].AutoStart) {
		t.Fatal("expected explicit autoStart=false to be preserved")
	}
	if got, want := cfg.Streams[1].ID, "app-2"; got != want {
		t.Fatalf("duplicate stream id = %q, want %q", got, want)
	}
	if got, want := cfg.UI.Presets[0].Streams, []string{"app"}; !sameStrings(got, want) {
		t.Fatalf("preset streams = %#v, want %#v", got, want)
	}

	cloned := Clone(cfg)
	cloned.Streams[0].Enabled = boolPtr(false)
	cloned.UI.Presets[0].Streams[0] = "changed"
	if !boolValue(cfg.Streams[0].Enabled) {
		t.Fatal("Clone() shared stream enabled pointer")
	}
	if cfg.UI.Presets[0].Streams[0] != "app" {
		t.Fatal("Clone() shared preset stream backing array")
	}
}

func TestResolveConfigLoadsStreams(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".iwatch", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, `{"streams":[{"id":"app","title":"App","type":"file","source":"app.log"}],"ui":{"presets":[{"id":"ops","title":"Ops","streams":["app"]}]}}`)

	cfg, _, err := ResolveConfig(base, CLIOverrides{})
	if err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	if len(cfg.Streams) != 1 || cfg.Streams[0].ID != "app" {
		t.Fatalf("streams = %#v", cfg.Streams)
	}
	if got, want := cfg.UI.Presets[0].Streams, []string{"app"}; !sameStrings(got, want) {
		t.Fatalf("preset streams = %#v, want %#v", got, want)
	}
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func TestResolveConfigLoadsHiddenFields(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".iwatch", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, `{"ui":{"logView":{"visibleFields":["level"],"hiddenFields":["msg"]}}}`)

	cfg, _, err := ResolveConfig(base, CLIOverrides{})
	if err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	if got, want := cfg.UI.LogView.VisibleFields, []string{"level"}; !sameStrings(got, want) {
		t.Fatalf("VisibleFields = %#v, want %#v", got, want)
	}
	if got, want := cfg.UI.LogView.HiddenFields, []string{"msg"}; !sameStrings(got, want) {
		t.Fatalf("HiddenFields = %#v, want %#v", got, want)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}
