package app

import (
	"testing"

	"github.com/stackriot/iwatch/internal/detect"
)

func TestResolvePositionalCommandKeepsDefaultWithoutArgs(t *testing.T) {
	t.Parallel()

	commands := []detect.Command{
		{ID: "make:run", Title: "make run", Cmd: "make run"},
	}

	gotCommands, gotDefault := resolvePositionalCommand("/tmp/project", commands, "make:run", nil)
	if len(gotCommands) != len(commands) {
		t.Fatalf("command count = %d, want %d", len(gotCommands), len(commands))
	}
	if gotDefault != "make:run" {
		t.Fatalf("default command = %q, want %q", gotDefault, "make:run")
	}
}

func TestResolvePositionalCommandUsesMatchingMakeTarget(t *testing.T) {
	t.Parallel()

	commands := []detect.Command{
		{ID: "make:run", Title: "make run", Cmd: "make run"},
		{ID: "make:multipass-run", Title: "make multipass-run", Cmd: "make multipass-run"},
	}

	gotCommands, gotDefault := resolvePositionalCommand("/tmp/project", commands, "make:run", []string{"make", "multipass-run"})
	if len(gotCommands) != len(commands) {
		t.Fatalf("command count = %d, want %d", len(gotCommands), len(commands))
	}
	if gotDefault != "make:multipass-run" {
		t.Fatalf("default command = %q, want %q", gotDefault, "make:multipass-run")
	}
}

func TestResolvePositionalCommandAppendsAdHocCommand(t *testing.T) {
	t.Parallel()

	commands := []detect.Command{
		{ID: "make:run", Title: "make run", Cmd: "make run"},
	}

	gotCommands, gotDefault := resolvePositionalCommand("/tmp/project", commands, "make:run", []string{"go", "test", "./..."})
	if len(gotCommands) != len(commands)+1 {
		t.Fatalf("command count = %d, want %d", len(gotCommands), len(commands)+1)
	}
	if gotDefault != "cli:go test ./..." {
		t.Fatalf("default command = %q, want %q", gotDefault, "cli:go test ./...")
	}

	gotCommand := gotCommands[len(gotCommands)-1]
	if gotCommand.Title != "go test ./..." {
		t.Fatalf("title = %q, want %q", gotCommand.Title, "go test ./...")
	}
	if gotCommand.Cmd != "go test ./..." {
		t.Fatalf("cmd = %q, want %q", gotCommand.Cmd, "go test ./...")
	}
	if gotCommand.CWD != "/tmp/project" {
		t.Fatalf("cwd = %q, want %q", gotCommand.CWD, "/tmp/project")
	}
	if gotCommand.Source != "cli" {
		t.Fatalf("source = %q, want %q", gotCommand.Source, "cli")
	}
}
