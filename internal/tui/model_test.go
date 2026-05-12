package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stackriot/iwatch/internal/buffer"
	"github.com/stackriot/iwatch/internal/config"
	"github.com/stackriot/iwatch/internal/detect"
	"github.com/stackriot/iwatch/internal/runner"
)

func TestToggleAndFocusPanes(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(config.Default(), []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.togglePane(paneCommand)
	if !app.openPanes[paneCommand] {
		t.Fatal("expected command pane to be open")
	}
	next := app.nextPane()
	if next != paneLog && next != paneCommand {
		t.Fatalf("unexpected next pane: %s", next)
	}
}

func TestQueryInputOpensAndCloses(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(config.Default(), []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	app = model.(*App)
	if !app.queryInput.Focused() {
		t.Fatal("expected query input to be focused")
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(*App)
	if app.queryInput.Focused() {
		t.Fatal("expected query input to be closed")
	}
}

func TestQueryUpdatesLive(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="thread example heartbeat"`)
	buf.Append("stdout", `level=ERROR msg="other event"`)

	app := New(config.Default(), []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	app = model.(*App)

	for _, r := range []rune("level=info") {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = model.(*App)
	}

	if got := app.query; got != "level=info" {
		t.Fatalf("query = %q", got)
	}
	lines := buf.Snapshot(app.query)
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	if lines[0].Fields["level"] != "info" {
		t.Fatalf("unexpected level field: %q", lines[0].Fields["level"])
	}
}
