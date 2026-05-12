package tui

import (
	"strings"
	"testing"
	"time"

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
	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.togglePane(paneCommand)
	if !app.commandPane.IsOpen() {
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
	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	app = model.(*App)
	if !app.logPane.queryInput.Focused() {
		t.Fatal("expected query input to be focused")
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(*App)
	if app.logPane.queryInput.Focused() {
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

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	app = model.(*App)

	for _, r := range []rune("level=info") {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = model.(*App)
	}

	if got := app.logPane.query; got != "level=info" {
		t.Fatalf("query = %q", got)
	}
	lines := buf.Snapshot(buffer.SnapshotOptions{Query: app.logPane.query})
	if len(lines) != 1 {
		t.Fatalf("len = %d", len(lines))
	}
	if lines[0].Fields["level"] != "info" {
		t.Fatalf("unexpected level field: %q", lines[0].Fields["level"])
	}
}

func TestPresetSwitchChangesVisibleLines(t *testing.T) {
	cfg := config.Default()
	cfg.UI.Presets = []config.FilterPreset{
		{
			ID:    "all",
			Title: "All",
		},
		{
			ID:    "errors",
			Title: "Errors",
			Clauses: []config.FilterClause{
				{Conditions: []config.FilterCondition{{Field: "level", Value: "error"}}},
			},
		},
	}
	cfg.UI.ActivePreset = "all"

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="ok"`)
	buf.Append("stdout", `level=ERROR msg="bad"`)

	app := New(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	if got := len(app.snapshotLines()); got != 2 {
		t.Fatalf("before switch len = %d", got)
	}

	app.switchPreset(1)

	if got := len(app.snapshotLines()); got != 1 {
		t.Fatalf("after switch len = %d", got)
	}
}

func TestArrowLeftRightSwitchPresets(t *testing.T) {
	cfg := config.Default()
	cfg.UI.Presets = []config.FilterPreset{
		{ID: "all", Title: "All"},
		{ID: "errors", Title: "Errors"},
		{ID: "debug", Title: "Debug"},
	}
	cfg.UI.ActivePreset = "errors"

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}

	app := New(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRight})
	app = model.(*App)
	if got := app.cfg.UI.ActivePreset; got != "debug" {
		t.Fatalf("active preset after right = %q", got)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
	app = model.(*App)
	if got := app.cfg.UI.ActivePreset; got != "errors" {
		t.Fatalf("active preset after left = %q", got)
	}
}

func TestPresetSwitchJumpsToTail(t *testing.T) {
	cfg := config.Default()
	cfg.UI.Presets = []config.FilterPreset{
		{ID: "all", Title: "All"},
		{ID: "errors", Title: "Errors"},
	}
	cfg.UI.ActivePreset = "all"

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"one", "two", "three"} {
		buf.Append("stdout", line)
	}

	app := New(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.height = 20
	app.logPane.cursor = 0
	app.logPane.autoScroll = false

	app.switchPreset(1)

	if got := app.cfg.UI.ActivePreset; got != "errors" {
		t.Fatalf("active preset = %q", got)
	}
	if !app.logPane.autoScroll {
		t.Fatal("expected preset switch to resume tail follow")
	}
	if got := app.logPane.cursor; got != 2 {
		t.Fatalf("expected cursor at tail, got %d", got)
	}
}

func TestRenderLineHidesTimeColumnForNonTimeFormats(t *testing.T) {
	cfg := config.Default()
	cfg.UI.LogView.TimeFormat = "relative"
	cfg.UI.LogView.VisibleFields = []string{"level"}

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)

	rendered := app.renderLine(buffer.ViewLine{
		Line: buffer.Line{
			Source:    "stdout",
			Text:      `level=INFO msg="hello"`,
			Fields:    map[string]string{"level": "info"},
			Timestamp: time.Now().Add(-5 * time.Second),
		},
	}, 80)

	if strings.Contains(rendered, "ago") || strings.Contains(rendered, "5s") {
		t.Fatalf("expected no time column in rendered line, got %q", rendered)
	}
	if !strings.Contains(rendered, "level=info") {
		t.Fatalf("expected log field to remain visible, got %q", rendered)
	}
}

func TestRenderLineWrapsAfterFixedTimeColumn(t *testing.T) {
	cfg := config.Default()
	cfg.UI.LogView.VisibleFields = []string{"level", "component"}
	cfg.UI.LogView.WrapMode = "field"

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)

	rendered := app.renderLine(buffer.ViewLine{
		Line: buffer.Line{
			Source:    "stdout",
			Text:      `message words that should wrap after the time column`,
			Fields:    map[string]string{"level": "info", "component": "service"},
			Timestamp: time.Date(2026, 5, 12, 15, 4, 5, 0, time.UTC),
		},
	}, 36)

	lines := strings.Split(rendered, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapped output, got %q", rendered)
	}
	if !strings.Contains(lines[0], "15:04:05 ") {
		t.Fatalf("expected fixed time column in first line, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "           ") {
		t.Fatalf("expected wrapped continuation to be indented after time column, got %q", lines[1])
	}
}

func TestViNavigationMovesCursor(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		buf.Append("stdout", "line")
	}

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.height = 20

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app = model.(*App)
	if app.logPane.cursor != 1 {
		t.Fatalf("cursor after j = %d", app.logPane.cursor)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	app = model.(*App)
	if app.logPane.cursor != 0 {
		t.Fatalf("cursor after k = %d", app.logPane.cursor)
	}
}

func TestEnterMovesToEndAndDoubleEnterAddsSeparator(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", "first")
	buf.Append("stdout", "second")

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	if app.logPane.cursor != 1 {
		t.Fatalf("cursor after enter = %d", app.logPane.cursor)
	}

	app.lastEnter = time.Now()
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)

	lines := app.snapshotLines()
	if got := len(lines); got != 3 {
		t.Fatalf("len(lines) = %d", got)
	}
	if got := lines[len(lines)-1].Source; got != "system" {
		t.Fatalf("separator source = %q", got)
	}
	if got := lines[len(lines)-1].Text; got != "------------------------------------------------" {
		t.Fatalf("separator text = %q", got)
	}
	if app.logPane.cursor != 2 {
		t.Fatalf("cursor after double enter = %d", app.logPane.cursor)
	}
	if !app.logPane.autoScroll {
		t.Fatal("expected enter to enable auto scroll")
	}
}

func TestManualScrollPausesAutoScrollUntilEnd(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", "first")
	buf.Append("stdout", "second")
	buf.Append("stdout", "third")

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.height = 20
	app.logPane.cursor = 2
	app.logPane.autoScroll = true

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	app = model.(*App)
	if app.logPane.autoScroll {
		t.Fatal("expected manual scroll to disable auto scroll")
	}

	_, _ = app.handleRunner(runner.Event{Type: runner.EventOutput, Source: "stdout", Text: "fourth"})
	if app.logPane.cursor != 1 {
		t.Fatalf("expected cursor to stay put while paused, got %d", app.logPane.cursor)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	app = model.(*App)
	if !app.logPane.autoScroll {
		t.Fatal("expected reaching the end to re-enable auto scroll")
	}
	if app.logPane.cursor != 3 {
		t.Fatalf("expected cursor at end, got %d", app.logPane.cursor)
	}

	_, _ = app.handleRunner(runner.Event{Type: runner.EventOutput, Source: "stdout", Text: "fifth"})
	if app.logPane.cursor != 4 {
		t.Fatalf("expected cursor to follow tail again, got %d", app.logPane.cursor)
	}
}

func TestMouseWheelScrollPausesAutoScroll(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"first", "second", "third"} {
		buf.Append("stdout", line)
	}

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 100
	app.height = 20
	app.logPane.cursor = 2
	app.logPane.autoScroll = true

	model, _ := app.Update(tea.MouseMsg{
		X:      4,
		Y:      4,
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
	})
	app = model.(*App)

	if app.logPane.autoScroll {
		t.Fatal("expected mouse wheel scroll to disable auto scroll")
	}
}

func TestSelectionModeOpensDetailView(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO payload="{\"ok\":true,\"count\":2}" msg="hello world"`)

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	app = model.(*App)
	if !app.logPane.selecting {
		t.Fatal("expected log selection mode to be active")
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	if app.mode != modeDetail {
		t.Fatalf("mode = %s", app.mode)
	}
	if app.detail.line == nil {
		t.Fatal("expected selected detail line")
	}

	lines := app.renderDetailLines()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "payload:") {
		t.Fatalf("expected payload field in detail view, got %q", joined)
	}
	if !strings.Contains(joined, "\"ok\": true") {
		t.Fatalf("expected pretty printed json in detail view, got %q", joined)
	}
	if !strings.Contains(joined, "msg:") || !strings.Contains(joined, "hello world") {
		t.Fatalf("expected msg field in detail view, got %q", joined)
	}
}

func TestEscCancelsSelectionMode(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="hello world"`)

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	app = model.(*App)
	if !app.logPane.selecting {
		t.Fatal("expected selection mode to be active")
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(*App)
	if app.logPane.selecting {
		t.Fatal("expected esc to cancel selection mode")
	}
	if app.escCount != 0 {
		t.Fatalf("expected esc counter reset, got %d", app.escCount)
	}
}

func TestMouseClickSelectsLine(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="first"`)
	buf.Append("stdout", `level=ERROR msg="second"`)

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 100
	app.height = 20

	model, _ := app.Update(tea.MouseMsg{
		X:      4,
		Y:      0,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	app = model.(*App)

	if app.mode != modeMain {
		t.Fatalf("mode = %s", app.mode)
	}
	if !app.logPane.selecting {
		t.Fatal("expected click to enable selection mode")
	}
	if got := app.logPane.cursor; got != 0 {
		t.Fatalf("unexpected selected cursor: %d", got)
	}
	if app.logPane.autoScroll {
		t.Fatal("expected selecting an older line to pause auto scroll")
	}
}

func TestMouseClickUsesVisibleViewport(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		buf.Append("stdout", "line "+string(rune('a'+i)))
	}

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 100
	app.height = 8
	lines := app.snapshotLines()
	app.logPane.moveToEnd(lines)
	app.logPane.syncAutoScroll(lines)
	app.syncLogViewport(lines)

	model, _ := app.Update(tea.MouseMsg{
		X:      4,
		Y:      0,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	app = model.(*App)

	if got := app.logPane.cursor; got != 8 {
		t.Fatalf("expected first visible viewport row to select line 8, got %d", got)
	}
	if !app.logPane.selecting {
		t.Fatal("expected click to keep selection mode active")
	}
	start, _ := app.logPane.visibleRange(app.logPaneWidth(), app.bodyHeight(), app.snapshotLines(), app.cfg.UI.LogView)
	if start != 8 {
		t.Fatalf("expected viewport to stay pinned to visible page start 8, got %d", start)
	}
}

func TestMouseClickHandlesWrappedViewportRows(t *testing.T) {
	cfg := config.Default()
	cfg.UI.LogView.WrapMode = "field"
	cfg.UI.LogView.VisibleFields = []string{}

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		buf.Append("stdout", "this is a deliberately long wrapped log line that should span multiple viewport rows")
	}

	app := New(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 50
	app.height = 10
	lines := app.snapshotLines()
	app.logPane.moveToEnd(lines)
	app.logPane.syncAutoScroll(lines)
	app.syncLogViewport(lines)

	model, _ := app.Update(tea.MouseMsg{
		X:      4,
		Y:      0,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	app = model.(*App)

	start, end := app.logPane.visibleRange(app.logPaneWidth(), app.bodyHeight(), app.snapshotLines(), app.cfg.UI.LogView)
	if app.logPane.cursor < start || app.logPane.cursor >= end {
		t.Fatalf("expected clicked row to map into current wrapped viewport, got cursor %d outside [%d,%d)", app.logPane.cursor, start, end)
	}
}

func TestMouseDoubleClickOpensDetailView(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO payload="{\"ok\":true}" msg="double click"`)

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 100
	app.height = 20

	click := tea.MouseMsg{
		X:      4,
		Y:      0,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}

	model, _ := app.Update(click)
	app = model.(*App)
	if app.mode != modeMain {
		t.Fatalf("expected first click to stay in main mode, got %s", app.mode)
	}

	app.lastClick = time.Now()
	model, _ = app.Update(click)
	app = model.(*App)
	if app.mode != modeDetail {
		t.Fatalf("expected double click to open detail view, got %s", app.mode)
	}
	if app.detail.line == nil {
		t.Fatal("expected selected detail line")
	}
}

func TestDetailViewRendersStyledSections(t *testing.T) {
	view := NewDetailView()
	view.Open(buffer.ViewLine{
		Line: buffer.Line{
			Source:    "stdout",
			Text:      `level=INFO payload="{\"ok\":true}"`,
			RawFields: map[string]string{"level": "info", "payload": `{"ok":true}`},
			Timestamp: time.Date(2026, 5, 12, 15, 4, 5, 0, time.UTC),
			Session:   2,
			Index:     7,
		},
	})

	rendered := view.View(100, 40)
	for _, fragment := range []string{"Log Details", "Metadata", "Raw", "Fields", "Session", "\"ok\": true"} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("expected %q in rendered detail view, got %q", fragment, rendered)
		}
	}
}
