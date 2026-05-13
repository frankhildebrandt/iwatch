package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/stackriot/iwatch/internal/buffer"
	"github.com/stackriot/iwatch/internal/config"
	"github.com/stackriot/iwatch/internal/runner"
	"github.com/stackriot/iwatch/internal/watch"
)

func (a *App) renderSidePanes(width, height int) string {
	var parts []string
	if a.commandPane.IsOpen() {
		parts = append(parts, a.commandPane.View(width, height, a.focus == paneCommand))
	}
	if a.eventsPane.IsOpen() {
		parts = append(parts, a.eventsPane.View(width, height, a.focus == paneEvents))
	}
	return strings.Join(parts, "\n")
}

func (a *App) logPaneContext() logPaneContext {
	commandTitle := a.activeCmd
	if cmd, ok := a.commandByID[a.activeCmd]; ok {
		commandTitle = cmd.Title
	}
	preset := activePreset(a.cfg)
	return logPaneContext{
		CommandTitle:  commandTitle,
		PresetTitle:   preset.Title,
		ProcessStatus: a.processStatus,
		WatchStatus:   a.watchStatus,
		StatusDetail:  a.statusDetail,
		BufferLen:     a.buf.Len(),
		BufferCap:     a.cfg.BufferLines,
	}
}

func (a *App) quitCmd() tea.Cmd {
	if a.cancelWatch != nil {
		a.cancelWatch()
	}
	_ = a.runner.Stop(1500 * time.Millisecond)
	return tea.Quit
}

func (a *App) logPageSize() int {
	bodyHeight := a.bodyHeight()
	return max(1, bodyHeight-3)
}

func (a *App) bodyHeight() int {
	return max(5, a.height-a.toolbarHeight())
}

func (a *App) toolbarHeight() int {
	return max(1, lipgloss.Height(a.logPane.InputBar()))
}

func (a *App) openLineDetail() {
	line, ok := a.logPane.currentLine(a.snapshotLines())
	if !ok {
		return
	}
	a.detail.Open(line)
	a.mode = modeDetail
}

func (a *App) openLineDetailAt(index int, lines []buffer.ViewLine) {
	if index < 0 || index >= len(lines) {
		return
	}
	a.logPane.cursor = index
	a.syncLogViewport(lines)
	a.detail.Open(lines[index])
	a.mode = modeDetail
}

func (a *App) logPaneWidth() int {
	side := a.renderSidePanes(max(30, a.width/3), a.bodyHeight())
	if side == "" || a.cfg.UI.SplitDirection == "horizontal" {
		return a.width
	}
	return a.width - max(30, a.width/3)
}

func (a *App) nextPane() paneID {
	var panes []string
	for _, pane := range []Pane{a.logPane, a.commandPane, a.eventsPane} {
		if pane.IsOpen() {
			panes = append(panes, string(pane.ID()))
		}
	}
	sortStrings(panes)
	if len(panes) == 0 {
		return paneLog
	}
	current := string(a.focus)
	for idx, pane := range panes {
		if pane == current {
			return paneID(panes[(idx+1)%len(panes)])
		}
	}
	return paneID(panes[0])
}

func (a *App) togglePane(id paneID) {
	switch id {
	case paneLog:
		return
	case paneCommand:
		a.commandPane.SetOpen(!a.commandPane.IsOpen())
	case paneEvents:
		a.eventsPane.SetOpen(!a.eventsPane.IsOpen())
	}
}

func (a *App) snapshotLines() []buffer.ViewLine {
	preset := activePreset(a.cfg)
	highlights := preset.HighlightRules
	if len(highlights) == 0 {
		highlights = a.cfg.HighlightRules
	}
	return a.buf.Snapshot(buffer.SnapshotOptions{
		Preset:     preset,
		Query:      a.logPane.query,
		Highlights: highlights,
	})
}

func (a *App) renderLine(line buffer.ViewLine, width int) string {
	return a.logPane.renderStyledLine(line, width, false, a.cfg.UI.LogView)
}

func (a *App) renderDetailLines() []string {
	return a.detail.lines()
}

func (a *App) switchPreset(direction int) {
	if len(a.cfg.UI.Presets) == 0 {
		return
	}
	ids := make([]string, 0, len(a.cfg.UI.Presets))
	current := 0
	for idx, preset := range a.cfg.UI.Presets {
		ids = append(ids, preset.ID)
		if preset.ID == a.cfg.UI.ActivePreset {
			current = idx
		}
	}
	next := (current + direction + len(ids)) % len(ids)
	a.cfg.UI.ActivePreset = ids[next]
	lines := a.snapshotLines()
	a.logPane.refreshQueryState(true, lines)
	a.syncLogViewport(lines)
}

func (a *App) syncLogViewport(lines []buffer.ViewLine) {
	a.logPane.ensureCursorVisible(max(10, a.logPaneWidth()), a.bodyHeight(), lines, a.cfg.UI.LogView)
}

func (a *App) switchDraftPreset(direction int) {
	if len(a.editor.draft.UI.Presets) == 0 {
		return
	}
	current := 0
	for idx, preset := range a.editor.draft.UI.Presets {
		if preset.ID == a.editor.draft.UI.ActivePreset {
			current = idx
			break
		}
	}
	next := (current + direction + len(a.editor.draft.UI.Presets)) % len(a.editor.draft.UI.Presets)
	a.editor.draft.UI.ActivePreset = a.editor.draft.UI.Presets[next].ID
}

func (a *App) startInitialRun() tea.Cmd {
	return a.restartActive()
}

func (a *App) restartActive() tea.Cmd {
	cmd, ok := a.commandByID[a.activeCmd]
	if !ok {
		return func() tea.Msg {
			return runnerMsg(runner.Event{Type: runner.EventError, Err: fmt.Errorf("unknown command %q", a.activeCmd)})
		}
	}
	a.buf.StartSession(cmd.Title)
	return func() tea.Msg {
		if a.runner.Running() {
			if err := a.runner.Restart(cmd); err != nil {
				return runnerMsg(runner.Event{Type: runner.EventError, Err: err})
			}
			return nil
		}
		if err := a.runner.Start(cmd); err != nil {
			return runnerMsg(runner.Event{Type: runner.EventError, Err: err})
		}
		return nil
	}
}

func waitRunnerEvent(run *runner.Runner) tea.Cmd {
	return func() tea.Msg {
		ev := <-run.Events()
		return runnerMsg(ev)
	}
}

func waitWatchEvent(w *watch.Watcher) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-w.Events()
		if !ok {
			return nil
		}
		return watchMsg(ev)
	}
}

func waitWatchError(w *watch.Watcher) tea.Cmd {
	return func() tea.Msg {
		err, ok := <-w.Errors()
		if !ok {
			return nil
		}
		return watchErrMsg{err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func activePreset(cfg config.Config) config.FilterPreset {
	for _, preset := range cfg.UI.Presets {
		if preset.ID == cfg.UI.ActivePreset {
			return preset
		}
	}
	if len(cfg.UI.Presets) > 0 {
		return cfg.UI.Presets[0]
	}
	return config.FilterPreset{ID: config.DefaultPresetID, Title: "Default"}
}

func activePresetPtr(cfg *config.Config) *config.FilterPreset {
	for idx := range cfg.UI.Presets {
		if cfg.UI.Presets[idx].ID == cfg.UI.ActivePreset {
			return &cfg.UI.Presets[idx]
		}
	}
	if len(cfg.UI.Presets) == 0 {
		cfg.UI.Presets = []config.FilterPreset{{ID: config.DefaultPresetID, Title: "Default"}}
		cfg.UI.ActivePreset = config.DefaultPresetID
	}
	return &cfg.UI.Presets[0]
}

func paneStyle(focused bool, width, height int) lipgloss.Style {
	border := lipgloss.RoundedBorder()
	style := lipgloss.NewStyle().Border(border).Width(width).Height(height)
	if focused {
		return style.BorderForeground(lipgloss.Color("12"))
	}
	return style.BorderForeground(lipgloss.Color("240"))
}

func appendTrimmed(values []string, value string, maxLen int) []string {
	values = append(values, value)
	if len(values) <= maxLen {
		return values
	}
	return values[len(values)-maxLen:]
}

func lastN(values []string, n int) []string {
	if len(values) <= n {
		return values
	}
	return values[len(values)-n:]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func boolPtr(value bool) *bool {
	return &value
}

func cycleValue(values []string, current string, direction int) string {
	if len(values) == 0 {
		return current
	}
	index := 0
	for idx, value := range values {
		if value == current {
			index = idx
			break
		}
	}
	index = (index + direction + len(values)) % len(values)
	return values[index]
}

func formatClause(clause config.FilterClause) string {
	parts := make([]string, 0, len(clause.Conditions))
	for _, cond := range clause.Conditions {
		if cond.Field != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", cond.Field, cond.Value))
		} else {
			parts = append(parts, cond.Value)
		}
	}
	return strings.Join(parts, " ")
}

func parseClause(value string) config.FilterClause {
	tokens := splitTokens(value)
	conditions := make([]config.FilterCondition, 0, len(tokens))
	for _, token := range tokens {
		if key, raw, ok := strings.Cut(token, "="); ok && key != "" {
			conditions = append(conditions, config.FilterCondition{Field: key, Value: raw})
			continue
		}
		if token != "" {
			conditions = append(conditions, config.FilterCondition{Value: token})
		}
	}
	return config.FilterClause{Conditions: conditions}
}

func formatRule(rule config.HighlightRule) string {
	return fmt.Sprintf("%s|%s|%d|%s", rule.ID, rule.Style, rule.Priority, rule.Pattern)
}

func parseRule(value string) config.HighlightRule {
	parts := strings.SplitN(value, "|", 4)
	for len(parts) < 4 {
		parts = append(parts, "")
	}
	priority, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil {
		priority = 0
	}
	return config.HighlightRule{
		ID:       strings.TrimSpace(parts[0]),
		Style:    strings.TrimSpace(parts[1]),
		Priority: priority,
		Pattern:  strings.TrimSpace(parts[3]),
	}
}

func parseFields(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			fields = append(fields, part)
		}
	}
	return fields
}

func wrapWithIndent(text string, width int, indent string) string {
	if width <= 0 || lipgloss.Width(text) <= width {
		return text
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	lines := []string{words[0]}
	for _, word := range words[1:] {
		current := lines[len(lines)-1]
		candidate := current + " " + word
		if lipgloss.Width(candidate) <= width {
			lines[len(lines)-1] = candidate
			continue
		}
		if lipgloss.Width(word) > width {
			chunks := splitLongWord(word, width)
			if lipgloss.Width(current) < width {
				lines = append(lines, chunks[0])
				lines = append(lines, chunks[1:]...)
			} else {
				lines = append(lines, chunks...)
			}
			continue
		}
		lines = append(lines, word)
	}

	for i := 1; i < len(lines); i++ {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}

func splitLongWord(word string, width int) []string {
	if width <= 0 || lipgloss.Width(word) <= width {
		return []string{word}
	}

	var chunks []string
	var current strings.Builder
	currentWidth := 0
	for _, r := range word {
		rw := lipgloss.Width(string(r))
		if currentWidth+rw > width && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
			currentWidth = 0
		}
		current.WriteRune(r)
		currentWidth += rw
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func truncateWithEllipsis(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}

	var out strings.Builder
	currentWidth := 0
	for _, r := range text {
		rw := lipgloss.Width(string(r))
		if currentWidth+rw > width-1 {
			break
		}
		out.WriteRune(r)
		currentWidth += rw
	}
	return out.String() + "…"
}

func padRight(value string, width int) string {
	padding := width - lipgloss.Width(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func splitTokens(value string) []string {
	var tokens []string
	var current strings.Builder
	quote := rune(0)
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}

	for _, r := range value {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && quote != 0:
			escaped = true
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			current.WriteRune(r)
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t':
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
