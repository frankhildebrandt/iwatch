package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/stackriot/iwatch/internal/buffer"
	"github.com/stackriot/iwatch/internal/config"
	"github.com/stackriot/iwatch/internal/detect"
	"github.com/stackriot/iwatch/internal/runner"
	"github.com/stackriot/iwatch/internal/stream"
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

func TestHelpOpensAndCloses(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	app = model.(*App)
	if app.mode != modeHelp {
		t.Fatalf("expected help mode, got %s", app.mode)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(*App)
	if app.mode != modeMain {
		t.Fatalf("expected main mode, got %s", app.mode)
	}
}

func TestHelpQuestionMarkCloses(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.mode = modeHelp

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	app = model.(*App)
	if app.mode != modeMain {
		t.Fatalf("expected main mode, got %s", app.mode)
	}
}

func TestShareOpensFromLogAndCloses(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="hello"`)
	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.height = 20

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	app = model.(*App)
	if app.mode != modeShare {
		t.Fatalf("expected share mode, got %s", app.mode)
	}
	if app.share.contents == "" {
		t.Fatal("expected share contents to be set")
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(*App)
	if app.mode != modeMain {
		t.Fatalf("expected main mode, got %s", app.mode)
	}
}

func TestLogInputBarKeepsSecondaryKeysOut(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)

	bar := app.inputBar()
	for _, key := range []string{"[s]", "[w]", "[enter]"} {
		if strings.Contains(bar, key) {
			t.Fatalf("expected input bar not to contain %s: %q", key, bar)
		}
	}
	if !strings.Contains(bar, "[?] help") {
		t.Fatalf("expected input bar to contain help key: %q", bar)
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

func TestRenderLineHonorsHiddenFieldsAndShowsNewFields(t *testing.T) {
	cfg := config.Default()
	cfg.UI.LogView.ShowRawMessage = boolPtr(false)
	cfg.UI.LogView.VisibleFields = []string{"msg"}
	cfg.UI.LogView.HiddenFields = []string{"level"}

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="hello" component=api`)
	app := New(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)

	rendered := app.renderLine(app.snapshotLines()[0], 120)
	if strings.Contains(rendered, "level=info") {
		t.Fatalf("expected hidden level field to be absent, got %q", rendered)
	}
	if !strings.Contains(rendered, "msg=hello") {
		t.Fatalf("expected configured msg field to render, got %q", rendered)
	}
	if !strings.Contains(rendered, "component=api") {
		t.Fatalf("expected new unconfigured component field to render, got %q", rendered)
	}
	if strings.Index(rendered, "msg=hello") > strings.Index(rendered, "component=api") {
		t.Fatalf("expected visibleFields ordering before discovered fields, got %q", rendered)
	}
}

func TestFieldMenuOpensNavigatesTogglesAndCloses(t *testing.T) {
	cfg := config.Default()
	cfg.UI.LogView.ShowRawMessage = boolPtr(false)

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="hello" component=api`)

	app := New(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 100
	app.height = 30

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	app = model.(*App)
	if app.mode != modeFields {
		t.Fatalf("expected field menu mode, got %s", app.mode)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = model.(*App)
	if app.fieldMenu.cursor != 1 {
		t.Fatalf("field menu cursor = %d", app.fieldMenu.cursor)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	if !sameTUIStrings(app.cfg.UI.LogView.HiddenFields, []string{"msg"}) {
		t.Fatalf("HiddenFields after toggle = %#v", app.cfg.UI.LogView.HiddenFields)
	}

	rendered := app.renderLine(app.snapshotLines()[0], 120)
	if strings.Contains(rendered, "msg=hello") {
		t.Fatalf("expected toggled msg field to be hidden, got %q", rendered)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(*App)
	if app.mode != modeMain {
		t.Fatalf("expected main mode after esc, got %s", app.mode)
	}
}

func TestFieldMenuShowsNewKeysWhileOpen(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="hello"`)

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 100
	app.height = 30

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	app = model.(*App)
	buf.Append("stdout", `component=api status=200`)

	rendered := app.View()
	for _, field := range []string{"level", "msg", "component", "status"} {
		if !strings.Contains(rendered, field) {
			t.Fatalf("expected field menu to contain %q, got %q", field, rendered)
		}
	}
}

func TestFieldMenuFiltersWhileTyping(t *testing.T) {
	cfg := config.Default()
	cfg.UI.LogView.ShowRawMessage = boolPtr(false)

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="hello" component=api request_id=abc`)

	app := New(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 100
	app.height = 30

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	app = model.(*App)
	for _, r := range []rune("req") {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = model.(*App)
	}

	rendered := app.View()
	if !strings.Contains(rendered, "Filter: req") || !strings.Contains(rendered, "request_id") {
		t.Fatalf("expected filtered request_id menu, got %q", rendered)
	}
	for _, hidden := range []string{"level", "component"} {
		if strings.Contains(rendered, hidden) {
			t.Fatalf("expected %q to be filtered out, got %q", hidden, rendered)
		}
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	if !sameTUIStrings(app.cfg.UI.LogView.HiddenFields, []string{"request_id"}) {
		t.Fatalf("HiddenFields after filtered toggle = %#v", app.cfg.UI.LogView.HiddenFields)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	app = model.(*App)
	if app.fieldMenu.filter != "re" {
		t.Fatalf("filter after backspace = %q", app.fieldMenu.filter)
	}
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	app = model.(*App)
	if app.fieldMenu.filter != "" {
		t.Fatalf("filter after ctrl+u = %q", app.fieldMenu.filter)
	}
}

func TestFieldMenuScrollsWhenManyFieldsExist(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	for idx := 0; idx < 20; idx++ {
		buf.Append("stdout", fmt.Sprintf("field_%02d=value", idx))
	}

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 100
	app.height = 14

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	app = model.(*App)
	for idx := 0; idx < 10; idx++ {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
		app = model.(*App)
	}

	if app.fieldMenu.viewportTop == 0 {
		t.Fatalf("expected viewport to scroll after moving down, got %d", app.fieldMenu.viewportTop)
	}
	rendered := app.View()
	if !strings.Contains(rendered, "field_10") {
		t.Fatalf("expected scrolled menu to show field_10, got %q", rendered)
	}
	if !strings.Contains(rendered, "/20") {
		t.Fatalf("expected scroll position indicator, got %q", rendered)
	}
}

func TestFieldFilterDialogFiltersLiveWithANDLogic(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO component=api msg="ready"`)
	buf.Append("stdout", `level=ERROR component=api msg="bad"`)
	buf.Append("stdout", `level=ERROR component=worker msg="bad"`)

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 100
	app.height = 30

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	app = model.(*App)
	if app.mode != modeFieldFilter {
		t.Fatalf("expected field filter mode, got %s", app.mode)
	}

	for _, r := range []rune("level") {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = model.(*App)
	}
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	for _, r := range []rune("err") {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = model.(*App)
	}
	if got := len(app.snapshotLines()); got != 2 {
		t.Fatalf("expected level filter to leave 2 lines, got %d", got)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	app = model.(*App)
	for _, r := range []rune("component") {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = model.(*App)
	}
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	for _, r := range []rune("api") {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = model.(*App)
	}

	lines := app.snapshotLines()
	if len(lines) != 1 {
		t.Fatalf("expected AND field filters to leave 1 line, got %d", len(lines))
	}
	if got := lines[0].Fields["component"]; got != "api" {
		t.Fatalf("component = %q", got)
	}
	if !sameTUIMap(app.fieldFilters, map[string]string{"level": "err", "component": "api"}) {
		t.Fatalf("field filters = %#v", app.fieldFilters)
	}
}

func TestFieldFilterDialogBackspaceRemovesEmptyValue(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=ERROR msg="bad"`)
	buf.Append("stdout", `level=INFO msg="ok"`)

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 100
	app.height = 30

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	app = model.(*App)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	for _, r := range []rune("err") {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = model.(*App)
	}
	if got := len(app.snapshotLines()); got != 1 {
		t.Fatalf("expected active field filter, got %d lines", got)
	}

	for idx := 0; idx < 3; idx++ {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		app = model.(*App)
	}
	if len(app.fieldFilters) != 0 {
		t.Fatalf("expected empty value to remove field filter, got %#v", app.fieldFilters)
	}
	if got := len(app.snapshotLines()); got != 2 {
		t.Fatalf("expected removed filter to restore all lines, got %d", got)
	}
}

func TestFieldFilterDialogScrollsWhenManyFieldsExist(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	for idx := 0; idx < 20; idx++ {
		buf.Append("stdout", fmt.Sprintf("field_%02d=value", idx))
	}

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 100
	app.height = 14

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	app = model.(*App)
	for idx := 0; idx < 10; idx++ {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
		app = model.(*App)
	}

	if app.filterMenu.viewportTop == 0 {
		t.Fatalf("expected field filter viewport to scroll, got %d", app.filterMenu.viewportTop)
	}
	rendered := app.View()
	if !strings.Contains(rendered, "field_10") {
		t.Fatalf("expected scrolled filter menu to show field_10, got %q", rendered)
	}
	if !strings.Contains(rendered, "/20") {
		t.Fatalf("expected scroll position indicator, got %q", rendered)
	}
}

func TestFieldFilterDialogEscLeavesEditBeforeClosing(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=ERROR msg="bad"`)

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.width = 100
	app.height = 30

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	app = model.(*App)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	if !app.filterMenu.Editing() {
		t.Fatal("expected field filter value editing")
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(*App)
	if app.mode != modeFieldFilter || app.filterMenu.Editing() {
		t.Fatalf("expected esc to leave edit mode only, mode=%s editing=%t", app.mode, app.filterMenu.Editing())
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(*App)
	if app.mode != modeMain {
		t.Fatalf("expected second esc to close dialog, got %s", app.mode)
	}
}

func TestConfigEditorReceivesLiveHiddenFields(t *testing.T) {
	cfg := config.Default()
	cfg.UI.LogView.ShowRawMessage = boolPtr(false)

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="hello"`)

	app := New(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	app = model.(*App)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(*App)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	app = model.(*App)

	if app.mode != modeConfig {
		t.Fatalf("expected config mode, got %s", app.mode)
	}
	if !sameTUIStrings(app.editor.draft.UI.LogView.HiddenFields, []string{"level"}) {
		t.Fatalf("editor hidden fields = %#v", app.editor.draft.UI.LogView.HiddenFields)
	}
}

func TestRenderLineUsesDefaultPaletteForStructuralLogText(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(previous)

	cfg := config.Default()
	cfg.UI.LogView.VisibleFields = []string{"level"}
	cfg.UI.LogView.ShowRawMessage = boolPtr(false)

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)

	rendered := app.renderLine(buffer.ViewLine{
		Line: buffer.Line{
			Source:    "stdout",
			Fields:    map[string]string{"level": "info"},
			Timestamp: time.Date(2026, 5, 12, 15, 4, 5, 0, time.UTC),
		},
	}, 80)

	for _, want := range []string{
		"\x1b[38;5;240m15:04:05\x1b[0m",
		"\x1b[38;5;245mlevel\x1b[0m=",
		"=\x1b[38;5;255minfo\x1b[0m",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered line missing %q: %q", want, rendered)
		}
	}
}

func TestConfigEditorCyclesLogPalette(t *testing.T) {
	cfg := config.Default()
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.editor.Open(cfg)

	row, ok := findEditorRow(app.editor.rows(), editorPalette)
	if !ok {
		t.Fatal("expected palette row")
	}

	model, _ := app.activateEditorRow(row)
	app = model.(*App)

	if got := app.editor.draft.UI.LogView.Palette; got != "contrast" {
		t.Fatalf("palette after cycle = %q, want contrast", got)
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
	if !app.logPane.selecting {
		t.Fatal("expected cursor navigation to enter selection mode")
	}
	if app.logPane.autoScroll {
		t.Fatal("expected manual scroll to disable auto scroll")
	}

	_, _ = app.handleRunner(runner.Event{Type: runner.EventOutput, Source: "stdout", Text: "fourth"})
	app.flushPendingOutput()
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
	app.flushPendingOutput()
	if app.logPane.cursor != 4 {
		t.Fatalf("expected cursor to follow tail again, got %d", app.logPane.cursor)
	}
}

func TestPageUpEntersSelectionMode(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	for idx := 0; idx < 10; idx++ {
		buf.Append("stdout", fmt.Sprintf("line-%d", idx))
	}

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.height = 20
	app.logPane.cursor = 9
	app.logPane.autoScroll = true

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	app = model.(*App)
	if !app.logPane.selecting {
		t.Fatal("expected page up to enter selection mode")
	}
	if app.logPane.autoScroll {
		t.Fatal("expected page up to pause tail follow")
	}
}

func TestScrollingPastShortMemoryExpandsLogWindow(t *testing.T) {
	buf, err := buffer.New(2000, nil)
	if err != nil {
		t.Fatal(err)
	}
	for idx := 0; idx < 1505; idx++ {
		buf.Append("stdout", fmt.Sprintf("line-%04d", idx))
	}

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.height = 20

	lines := app.snapshotLines()
	if len(lines) != logShortMemoryLines {
		t.Fatalf("initial window len = %d, want %d", len(lines), logShortMemoryLines)
	}
	if lines[0].Text != "line-0505" {
		t.Fatalf("initial window first line = %q", lines[0].Text)
	}

	app.logPane.cursor = 0
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyUp})
	app = model.(*App)
	lines = app.snapshotLines()
	if len(lines) != 1505 {
		t.Fatalf("expanded window len = %d, want 1505", len(lines))
	}
	if lines[0].Text != "line-0000" {
		t.Fatalf("expanded window first line = %q", lines[0].Text)
	}
	if !app.logPane.selecting {
		t.Fatal("expected expanding scroll to keep selection mode")
	}
}

func TestRunnerOutputBatchesUntilFlush(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)

	_, _ = app.handleRunner(runner.Event{Type: runner.EventOutput, Source: "stdout", Text: "one"})
	_, _ = app.handleRunner(runner.Event{Type: runner.EventOutput, Source: "stdout", Text: "two"})
	if got := buf.Len(); got != 0 {
		t.Fatalf("expected output to remain pending before flush, got buffer len %d", got)
	}
	if !app.outputFlushScheduled {
		t.Fatal("expected output flush to be scheduled")
	}

	app.flushPendingOutput()
	lines := app.snapshotLines()
	if len(lines) != 2 || lines[0].Text != "one" || lines[1].Text != "two" {
		t.Fatalf("unexpected flushed lines: %+v", lines)
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
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyDown})
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
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyDown})
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

func TestEscSelectionExitReturnsToTail(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"first", "second", "third"} {
		buf.Append("stdout", line)
	}

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.height = 20
	app.logPane.cursor = 0
	app.logPane.selecting = true
	app.logPane.autoScroll = false

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(*App)
	if app.logPane.selecting {
		t.Fatal("expected esc to leave selection mode")
	}
	if !app.logPane.autoScroll || app.logPane.cursor != 2 {
		t.Fatalf("expected selection exit to tail, cursor=%d auto=%t", app.logPane.cursor, app.logPane.autoScroll)
	}
}

func TestSKeyDoesNotToggleSelection(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", "first")

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	app = model.(*App)
	if app.logPane.selecting {
		t.Fatal("expected s to leave selection unchanged")
	}
}

func TestTruncateLogsKeyClearsBufferAndTails(t *testing.T) {
	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf.Append("stdout", `level=INFO msg="hello"`)
	buf.Append("stdout", `level=ERROR msg="bad"`)

	app := New(config.Default(), "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), nil)
	app.height = 20
	app.logPane.cursor = 0
	app.logPane.autoScroll = false

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	app = model.(*App)

	lines := app.snapshotLines()
	if len(lines) != 1 {
		t.Fatalf("expected only truncate marker, got %+v", lines)
	}
	if lines[0].Source != "system" || lines[0].Text != "logs truncated" {
		t.Fatalf("unexpected truncate marker: %+v", lines[0])
	}
	if !app.logPane.autoScroll || app.logPane.cursor != 0 {
		t.Fatalf("expected truncate to tail, cursor=%d auto=%t", app.logPane.cursor, app.logPane.autoScroll)
	}
	if got := app.buf.ObservedFields(); len(got) == 0 {
		t.Fatal("expected truncate to keep observed fields")
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
	start, _ := app.logPane.visibleRange(app.logPaneWidth(), app.bodyHeight(), app.snapshotLines(), app.buf.ObservedFields(), app.cfg.UI.LogView)
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

	start, end := app.logPane.visibleRange(app.logPaneWidth(), app.bodyHeight(), app.snapshotLines(), app.buf.ObservedFields(), app.cfg.UI.LogView)
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

func TestPresetSwitchAppliesActiveStreams(t *testing.T) {
	cfg := config.Default()
	cfg.Streams = []config.StreamConfig{{ID: "app", Title: "App", Type: "process", Cmd: "sleep 10"}}
	cfg.UI.Presets = []config.FilterPreset{
		{ID: "main", Title: "Main"},
		{ID: "logs", Title: "Logs", Streams: []string{"app"}},
	}
	cfg.UI.ActivePreset = "main"
	cfg = config.DefaultMerge(cfg)

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	streams := stream.New(cfg.Streams, t.TempDir())
	defer streams.StopAll()
	app := NewWithStreams(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), streams, nil)
	app.applyActiveStreams()

	if got := streams.ActiveCount(); got != 0 {
		t.Fatalf("active streams before preset switch = %d, want 0", got)
	}
	app.switchPreset(1)
	if got := streams.ActiveCount(); got != 1 {
		t.Fatalf("active streams after preset switch = %d, want 1", got)
	}
}

func TestStreamOutputUsesGlobalSnapshotSearch(t *testing.T) {
	cfg := config.Default()
	cfg.Streams = []config.StreamConfig{{ID: "app", Title: "App", Type: "file", Source: "app.log"}}
	cfg.UI.Presets = []config.FilterPreset{{ID: "logs", Title: "Logs", Streams: []string{"app"}}}
	cfg.UI.ActivePreset = "logs"
	cfg = config.DefaultMerge(cfg)

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := NewWithStreams(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), stream.New(cfg.Streams, t.TempDir()), nil)
	app.logPane.query = "level=error"

	model, _ := app.handleStream(stream.Event{Type: stream.EventOutput, StreamID: "app", Source: "app", Text: `level=ERROR msg="from stream"`})
	app = model.(*App)
	lines := app.snapshotLines()
	if len(lines) != 1 {
		t.Fatalf("snapshot lines = %d, want 1", len(lines))
	}
	if lines[0].Source != "app" {
		t.Fatalf("line source = %q, want app", lines[0].Source)
	}
}

func TestOnDemandProcessStreamStartsFromPane(t *testing.T) {
	autoStart := false
	cfg := config.Default()
	cfg.Streams = []config.StreamConfig{{ID: "proc", Title: "Proc", Type: "process", Cmd: "sleep 10", AutoStart: &autoStart}}
	cfg.UI.Presets = []config.FilterPreset{{ID: "logs", Title: "Logs", Streams: []string{"proc"}}}
	cfg.UI.ActivePreset = "logs"
	cfg = config.DefaultMerge(cfg)

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	streams := stream.New(cfg.Streams, t.TempDir())
	defer streams.StopAll()
	app := NewWithStreams(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), streams, nil)
	app.applyActiveStreams()
	if got := streams.ActiveCount(); got != 0 {
		t.Fatalf("active streams before on-demand start = %d, want 0", got)
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	app = model.(*App)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)

	if got := streams.ActiveCount(); got != 1 {
		t.Fatalf("active streams after on-demand start = %d, want 1", got)
	}
}

func TestAutoStartProcessStreamStartsWithoutPresetStreamList(t *testing.T) {
	cfg := config.Default()
	cfg.Streams = []config.StreamConfig{{ID: "dev", Title: "React Dev", Type: "process", Cmd: "sleep 10"}}
	cfg = config.DefaultMerge(cfg)

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}

	streams := stream.New(cfg.Streams, t.TempDir())
	defer streams.StopAll()

	app := NewWithStreams(cfg, "", []detect.Command{{ID: "cmd", Title: "Cmd", Cmd: "echo hi"}}, "cmd", buf, runner.New(), streams, nil)
	app.applyActiveStreams()

	if got := streams.ActiveCount(); got != 1 {
		t.Fatalf("active streams = %d, want 1", got)
	}
}

func TestCommandPaneEnterStartsCommandOutputPane(t *testing.T) {
	cfg := config.Default()

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	streams := stream.New(nil, t.TempDir())
	defer streams.StopAll()

	commands := []detect.Command{
		{ID: "first", Title: "First", Cmd: "echo first"},
		{ID: "second", Title: "Second", Cmd: "echo second"},
	}

	app := NewWithStreams(cfg, "", commands, "first", buf, runner.New(), streams, nil)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	app = model.(*App)
	if app.focus != paneCommand {
		t.Fatalf("focus = %s, want %s", app.focus, paneCommand)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = model.(*App)
	if command, ok := app.commandPane.SelectedCommand(); !ok || command.ID != "second" {
		t.Fatalf("selected command = %#v, ok=%v, want second", command, ok)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	if got, want := app.focus, paneCommandOutput; got != want {
		t.Fatalf("focus = %s, want %s", got, want)
	}
	if !app.commandOutputPane.IsOpen() {
		t.Fatal("expected command output pane to be open")
	}
	if got, want := app.commandOutputPane.StreamID(), "cmd-panel:second"; got != want {
		t.Fatalf("command output stream id = %q, want %q", got, want)
	}
	if got, want := streams.ActiveCount(), 1; got != want {
		t.Fatalf("active stream count = %d, want %d", got, want)
	}
	if got, want := app.activeCmd, "first"; got != want {
		t.Fatalf("active command = %q, want %q", got, want)
	}
}

func TestCommandPaneOStartsCommandAsStream(t *testing.T) {
	cfg := config.Default()

	buf, err := buffer.New(100, nil)
	if err != nil {
		t.Fatal(err)
	}
	streams := stream.New(nil, t.TempDir())
	defer streams.StopAll()

	commands := []detect.Command{
		{ID: "first", Title: "First", Cmd: "echo first"},
		{ID: "second", Title: "Second", Cmd: "echo second"},
	}

	app := NewWithStreams(cfg, "", commands, "first", buf, runner.New(), streams, nil)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	app = model.(*App)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = model.(*App)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	app = model.(*App)

	if got, want := app.focus, paneStreams; got != want {
		t.Fatalf("focus = %s, want %s", got, want)
	}
	if !app.streamsPane.IsOpen() {
		t.Fatal("expected streams pane to be open")
	}
	if got, want := streams.ActiveCount(), 1; got != want {
		t.Fatalf("active stream count = %d, want %d", got, want)
	}
	if _, ok := app.runtimeStreams["cmd-stream:second"]; !ok {
		t.Fatal("expected runtime command stream to be registered")
	}
}

func sameTUIStrings(left, right []string) bool {
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

func sameTUIMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func findEditorRow(rows []editorRow, kind editorKind) (editorRow, bool) {
	for _, row := range rows {
		if row.kind == kind {
			return row, true
		}
	}
	return editorRow{}, false
}
