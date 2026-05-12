package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stackriot/iwatch/internal/config"
)

func TestDiscoverPrefersPackageScripts(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "package.json"), `{"scripts":{"build":"vite build","dev":"vite","start":"vite preview"}}`)
	write(t, filepath.Join(dir, "Makefile"), "build:\n\tgo build\nrun:\n\tgo run .\n")

	res, err := Discover(dir, config.Config{})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if res.Default != "npm:dev" {
		t.Fatalf("default = %s", res.Default)
	}
	if len(res.Commands) < 5 {
		t.Fatalf("expected merged commands, got %d", len(res.Commands))
	}
}

func TestDiscoverFallsBackToConfigCommands(t *testing.T) {
	dir := t.TempDir()
	res, err := Discover(dir, config.Config{
		DefaultCommand: "custom",
		Commands: []config.CommandConfig{
			{ID: "custom", Title: "Custom", Cmd: "echo hi"},
		},
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(res.Commands) != 1 || res.Default != "custom" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
