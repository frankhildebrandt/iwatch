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
	"github.com/stackriot/iwatch/internal/detect"
	"github.com/stackriot/iwatch/internal/runner"
	"github.com/stackriot/iwatch/internal/stream"
	"github.com/stackriot/iwatch/internal/watch"
)

const (
	logShortMemoryLines = 1000
	gracefulStopTimeout = 30 * time.Second
)

func (a *App) renderSidePanes(width, height int) string {
	var parts []string
	if a.commandPane.IsOpen() {
		parts = append(parts, a.commandPane.View(width, height, a.focus == paneCommand))
	}
	if a.commandOutputPane.IsOpen() {
		status, _ := a.streamStatus(a.commandOutputPane.StreamID())
		parts = append(parts, a.commandOutputPane.View(width, height, a.focus == paneCommandOutput, status, a.streamLines[a.commandOutputPane.StreamID()]))
	}
	if a.eventsPane.IsOpen() {
		parts = append(parts, a.eventsPane.View(width, height, a.focus == paneEvents))
	}
	if a.streamsPane.IsOpen() {
		parts = append(parts, a.streamsPane.View(width, height, a.focus == paneStreams, a.streamStatuses()))
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
		StreamCount:   a.streamCount(),
	}
}

func (a *App) quitCmd() tea.Cmd {
	if a.shutdownState != "" {
		return a.forceShutdownCmd()
	}
	if !a.hasRunningChildren() {
		if a.cancelWatch != nil {
			a.cancelWatch()
		}
		return tea.Quit
	}
	return a.beginShutdown(shutdownQuit)
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
	for _, pane := range []Pane{a.logPane, a.commandPane, a.commandOutputPane, a.eventsPane, a.streamsPane} {
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
	case paneCommandOutput:
		open := !a.commandOutputPane.IsOpen()
		a.commandOutputPane.SetOpen(open)
		if !open {
			a.closeCommandOutputPane()
		}
	case paneEvents:
		a.eventsPane.SetOpen(!a.eventsPane.IsOpen())
	case paneStreams:
		a.streamsPane.SetOpen(!a.streamsPane.IsOpen())
	}
}

func (a *App) snapshotLines() []buffer.ViewLine {
	if a.logWindowLimit <= 0 {
		a.logWindowLimit = logShortMemoryLines
	}
	return a.snapshotLinesWithLimit(a.logWindowLimit)
}

func (a *App) snapshotLinesWithLimit(limit int) []buffer.ViewLine {
	preset := activePreset(a.cfg)
	highlights := preset.HighlightRules
	if len(highlights) == 0 {
		highlights = a.cfg.HighlightRules
	}
	return a.buf.Snapshot(buffer.SnapshotOptions{
		Preset:       preset,
		Query:        a.logPane.query,
		FieldFilters: cloneFieldFilters(a.fieldFilters),
		Highlights:   highlights,
		Limit:        limit,
	})
}

func (a *App) renderLine(line buffer.ViewLine, width int) string {
	observed := a.buf.ObservedFields()
	if len(observed) == 0 {
		observed = fieldsFromLine(line)
	}
	return a.logPane.renderStyledLine(line, width, false, observed, a.cfg.UI.LogView)
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
	a.applyActiveStreams()
	a.resetLogWindow()
	lines := a.snapshotLines()
	a.logPane.refreshQueryState(true, lines)
	a.syncLogViewport(lines)
}

func (a *App) syncLogViewport(lines []buffer.ViewLine) {
	a.logPane.ensureCursorVisible(max(10, a.logPaneWidth()), a.bodyHeight(), lines, a.buf.ObservedFields(), a.cfg.UI.LogView)
}

func (a *App) moveLogCursor(delta int) {
	lines := a.snapshotLines()
	if delta < 0 && a.logPane.cursor <= 0 {
		lines = a.expandLogWindow(lines)
	}
	a.logPane.selecting = true
	a.logPane.moveCursor(delta)
	a.logPane.syncAutoScroll(lines)
	a.syncLogViewport(lines)
}

func (a *App) pageLogCursor(direction int) {
	lines := a.snapshotLines()
	if direction < 0 && a.logPane.cursor < max(1, a.logPageSize()/2) {
		lines = a.expandLogWindow(lines)
	}
	a.logPane.selecting = true
	a.logPane.pageCursor(direction, a.logPageSize())
	a.logPane.syncAutoScroll(lines)
	a.syncLogViewport(lines)
}

func (a *App) moveLogToTail() {
	a.resetLogWindow()
	lines := a.snapshotLines()
	a.logPane.selecting = false
	a.logPane.refreshQueryState(true, lines)
	a.syncLogViewport(lines)
}

func (a *App) truncateLogs() {
	a.resetLogWindow()
	a.buf.Truncate()
	a.buf.Append("system", "logs truncated")
	a.moveLogToTail()
}

func (a *App) flushPendingOutput() {
	if len(a.pendingOutput) == 0 {
		return
	}
	for _, ev := range a.pendingOutput {
		a.buf.Append(ev.Source, ev.Text)
	}
	a.pendingOutput = a.pendingOutput[:0]
	if a.logPane.autoScroll {
		a.resetLogWindow()
	}
	lines := a.snapshotLines()
	a.logPane.refreshQueryState(a.logPane.autoScroll, lines)
	a.syncLogViewport(lines)
}

func (a *App) resetLogWindow() {
	a.logWindowLimit = logShortMemoryLines
}

func (a *App) expandLogWindow(current []buffer.ViewLine) []buffer.ViewLine {
	if a.logWindowLimit >= a.buf.Len() {
		return current
	}

	oldLen := len(current)
	a.logWindowLimit = min(a.buf.Len(), max(logShortMemoryLines, a.logWindowLimit+logShortMemoryLines))
	expanded := a.snapshotLines()
	a.logPane.cursor += max(0, len(expanded)-oldLen)
	a.logPane.viewportTop += max(0, len(expanded)-oldLen)
	return expanded
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

func (a *App) toggleHiddenField(field string) {
	current := a.cfg.UI.LogView.HiddenFields
	for idx, value := range current {
		if value != field {
			continue
		}
		a.cfg.UI.LogView.HiddenFields = append(current[:idx], current[idx+1:]...)
		return
	}
	a.cfg.UI.LogView.HiddenFields = append(current, field)
}

func (a *App) setFieldFilter(field string, value string) {
	field = strings.ToLower(strings.TrimSpace(field))
	value = strings.ToLower(strings.TrimSpace(value))
	if field == "" {
		return
	}
	if value == "" {
		delete(a.fieldFilters, field)
		return
	}
	a.fieldFilters[field] = value
}

func (a *App) appendFieldFilterValue(field string, value string) {
	if value == "" {
		return
	}
	a.setFieldFilter(field, a.fieldFilters[field]+strings.ToLower(value))
}

func (a *App) backspaceFieldFilterValue(field string) {
	value := a.fieldFilters[field]
	if value == "" {
		return
	}
	a.setFieldFilter(field, value[:len(value)-1])
}

func cloneFieldFilters(filters map[string]string) map[string]string {
	if len(filters) == 0 {
		return nil
	}
	out := make(map[string]string, len(filters))
	for field, value := range filters {
		if field == "" || value == "" {
			continue
		}
		out[field] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func countActiveFieldFilters(filters map[string]string) int {
	count := 0
	for _, value := range filters {
		if value != "" {
			count++
		}
	}
	return count
}

func fieldsFromLine(line buffer.ViewLine) []string {
	fields := make([]string, 0, len(line.Fields))
	for field := range line.Fields {
		fields = append(fields, field)
	}
	sortStrings(fields)
	return fields
}

func (a *App) startInitialRun() tea.Cmd {
	return a.startActiveCommandCmd()
}

func (a *App) restartActive() tea.Cmd {
	if a.shutdownState != "" {
		return a.forceShutdownCmd()
	}
	if a.hasRunningChildren() {
		return a.beginShutdown(shutdownRebuild)
	}
	return a.startActiveCommandCmd()
}

func (a *App) startActiveCommandCmd() tea.Cmd {
	cmd, ok := a.commandByID[a.activeCmd]
	if !ok {
		return func() tea.Msg {
			return runnerMsg(runner.Event{Type: runner.EventError, Err: fmt.Errorf("unknown command %q", a.activeCmd)})
		}
	}
	a.buf.StartSession(cmd.Title)
	return func() tea.Msg {
		if err := a.runner.Start(cmd); err != nil {
			return runnerMsg(runner.Event{Type: runner.EventError, Err: err})
		}
		return nil
	}
}

func (a *App) beginShutdown(action shutdownAction) tea.Cmd {
	a.shutdownState = action
	if a.cancelWatch != nil && action == shutdownQuit {
		a.cancelWatch()
	}
	if a.streams != nil {
		a.restartStreamIDs = a.streams.RunningIDs()
	} else {
		a.restartStreamIDs = nil
	}
	a.processStatus = "stopping gracefully"
	switch action {
	case shutdownQuit:
		a.watchStatus = "quitting"
		a.buf.Append("system", "quitting: stopping child processes gracefully (press q again to force)")
	case shutdownRebuild:
		a.watchStatus = "rebuilding"
		a.buf.Append("system", "rebuild: stopping child processes gracefully (press r again to force)")
	}
	return func() tea.Msg {
		done := make(chan struct{})
		go func() {
			defer close(done)
			if a.streams != nil {
				<-a.streams.RequestStopAll(gracefulStopTimeout)
			}
		}()
		if err := a.runner.Stop(gracefulStopTimeout); err != nil {
			return runnerMsg(runner.Event{Type: runner.EventError, Err: err})
		}
		<-done
		return shutdownDoneMsg{action: action}
	}
}

func (a *App) forceShutdownCmd() tea.Cmd {
	a.buf.Append("system", "forcing child process shutdown")
	a.processStatus = "forcing shutdown"
	if a.streams != nil {
		a.streams.ForceStopAll()
	}
	return func() tea.Msg {
		if err := a.runner.ForceStop(); err != nil {
			return runnerMsg(runner.Event{Type: runner.EventError, Err: err})
		}
		return nil
	}
}

func (a *App) hasRunningChildren() bool {
	if a.runner.Running() {
		return true
	}
	return a.streams != nil && a.streams.ActiveCount() > 0
}

func (a *App) handleShutdownDone(msg shutdownDoneMsg) (tea.Model, tea.Cmd) {
	restartStreamIDs := append([]string(nil), a.restartStreamIDs...)
	a.shutdownState = ""
	a.restartStreamIDs = nil
	a.processStatus = "idle"

	if msg.action == shutdownQuit {
		return a, tea.Quit
	}

	a.watchStatus = "clean"
	for _, id := range restartStreamIDs {
		if err := a.streams.Start(id); err != nil {
			a.eventsPane.Append(fmt.Sprintf("stream %s restart failed: %v", id, err))
		}
	}
	return a, a.startActiveCommandCmd()
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

func waitStreamEvent(streams *stream.Supervisor) tea.Cmd {
	return func() tea.Msg {
		ev := <-streams.Events()
		return streamMsg(ev)
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func outputFlushCmd() tea.Cmd {
	return tea.Tick(75*time.Millisecond, func(t time.Time) tea.Msg {
		return outputFlushMsg(t)
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

func (a *App) activeStreamIDs() []string {
	ids := make([]string, 0, len(a.runtimeStreamOrder)+len(a.cfg.Streams))
	preset := activePreset(a.cfg)
	if len(preset.Streams) > 0 {
		ids = append(ids, preset.Streams...)
	} else {
		hasPresetStreams := false
		for _, candidate := range a.cfg.UI.Presets {
			if len(candidate.Streams) > 0 {
				hasPresetStreams = true
				break
			}
		}
		if !hasPresetStreams {
			for _, cfg := range a.cfg.Streams {
				if cfg.ID == "" {
					continue
				}
				ids = append(ids, cfg.ID)
			}
		}
	}
	for _, id := range a.runtimeStreamOrder {
		ids = append(ids, id)
	}
	return ids
}

func (a *App) applyActiveStreams() {
	if a.streams == nil {
		return
	}
	a.streams.Configure(a.allStreamConfigs())
	a.streams.Apply(a.activeStreamIDs())
}

func (a *App) streamStatuses() []stream.Status {
	if a.streams == nil {
		return nil
	}
	return a.streams.Statuses()
}

func (a *App) streamStatus(id string) (stream.Status, bool) {
	if id == "" {
		return stream.Status{}, false
	}
	for _, status := range a.streamStatuses() {
		if status.ID == id {
			return status, true
		}
	}
	return stream.Status{}, false
}

func (a *App) streamCount() int {
	if a.streams == nil {
		return 0
	}
	return a.streams.ActiveCount()
}

func (a *App) toggleSelectedStream() {
	if a.streams == nil {
		return
	}
	status, ok := a.streamsPane.Selected(a.streamStatuses())
	if !ok {
		return
	}
	if status.Running {
		_ = a.streams.Stop(status.ID)
		return
	}
	if err := a.streams.Start(status.ID); err != nil {
		a.eventsPane.Append("stream " + status.ID + ": " + err.Error())
	}
}

func (a *App) allStreamConfigs() []config.StreamConfig {
	streams := append([]config.StreamConfig(nil), a.cfg.Streams...)
	for _, id := range a.runtimeStreamOrder {
		cfg, ok := a.runtimeStreams[id]
		if !ok {
			continue
		}
		streams = append(streams, cfg)
	}
	return streams
}

func (a *App) startCommandAsStream(command detect.Command, streamID string, title string) {
	if a.streams == nil {
		a.eventsPane.Append("command streams unavailable")
		return
	}
	if _, ok := a.runtimeStreams[streamID]; ok {
		_ = a.streams.Stop(streamID)
	}

	cfg := config.StreamConfig{
		ID:        streamID,
		Title:     title,
		Type:      "process",
		Enabled:   boolPtr(true),
		CWD:       command.CWD,
		Cmd:       command.Cmd,
		AutoStart: boolPtr(true),
	}
	a.runtimeStreams[streamID] = cfg
	if !containsString(a.runtimeStreamOrder, streamID) {
		a.runtimeStreamOrder = append(a.runtimeStreamOrder, streamID)
	}
	a.streamLines[streamID] = nil
	a.applyActiveStreams()
}

func (a *App) startCommandInStream(command detect.Command) {
	a.startCommandAsStream(command, "cmd-stream:"+command.ID, command.Title)
	a.streamsPane.SetOpen(true)
	a.focus = paneStreams
}

func (a *App) startCommandInOutputPane(command detect.Command) {
	streamID := "cmd-panel:" + command.ID
	if a.commandOutputID != "" && a.commandOutputID != streamID {
		a.removeRuntimeStream(a.commandOutputID)
	}
	a.commandOutputID = streamID
	a.commandOutputPane.SetCommand(streamID, command.Title)
	a.commandOutputPane.SetOpen(true)
	a.startCommandAsStream(command, streamID, command.Title)
	a.focus = paneCommandOutput
}

func (a *App) closeCommandOutputPane() {
	if a.commandOutputID == "" {
		a.commandOutputPane.Clear()
		return
	}
	a.removeRuntimeStream(a.commandOutputID)
	a.commandOutputID = ""
	a.commandOutputPane.Clear()
	if a.focus == paneCommandOutput {
		a.focus = paneLog
	}
}

func (a *App) removeRuntimeStream(id string) {
	if id == "" {
		return
	}
	delete(a.runtimeStreams, id)
	delete(a.streamLines, id)
	a.runtimeStreamOrder = removeString(a.runtimeStreamOrder, id)
	if a.streams != nil {
		_ = a.streams.Stop(id)
		a.applyActiveStreams()
	}
}

func (a *App) openSelectedStreamDetail() {
	status, ok := a.streamsPane.Selected(a.streamStatuses())
	if !ok {
		return
	}
	a.streamDetailID = status.ID
	a.mode = modeStream
}

func (a *App) renderStreamDetail() string {
	title := "Stream: " + a.streamDetailID
	lines := a.streamLines[a.streamDetailID]
	content := title
	if len(lines) == 0 {
		content += "\nNo stream output yet."
	} else {
		start := max(0, len(lines)-max(1, a.bodyHeight()-3))
		content += "\n" + strings.Join(lines[start:], "\n")
	}
	help := "[esc/enter] close [q] quit"
	return lipgloss.JoinVertical(
		lipgloss.Left,
		paneStyle(true, a.width, a.bodyHeight()).Render(content),
		lipgloss.NewStyle().Padding(0, 1).Background(lipgloss.Color("236")).Foreground(lipgloss.Color("255")).Render(help),
	)
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

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func removeString(values []string, target string) []string {
	out := values[:0]
	for _, value := range values {
		if value == target {
			continue
		}
		out = append(out, value)
	}
	return out
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

func formatStream(stream config.StreamConfig) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%t|%t", stream.ID, stream.Title, stream.Type, stream.Source, stream.Cmd, stream.CWD, boolValue(stream.Enabled), boolValue(stream.AutoStart))
}

func parseStream(value string) config.StreamConfig {
	parts := strings.SplitN(value, "|", 8)
	for len(parts) < 8 {
		parts = append(parts, "")
	}
	enabled := parseBoolDefault(parts[6], true)
	autoStart := parseBoolDefault(parts[7], true)
	return config.StreamConfig{
		ID:        strings.TrimSpace(parts[0]),
		Title:     strings.TrimSpace(parts[1]),
		Type:      strings.TrimSpace(parts[2]),
		Source:    strings.TrimSpace(parts[3]),
		Cmd:       strings.TrimSpace(parts[4]),
		CWD:       strings.TrimSpace(parts[5]),
		Enabled:   boolPtr(enabled),
		AutoStart: boolPtr(autoStart),
	}
}

func parseBoolDefault(value string, fallback bool) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
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

func orderedVisibleFields(view config.LogViewConfig, observed []string) []string {
	ordered := orderedObservedFields(view, observed)
	if len(ordered) == 0 {
		return nil
	}

	hidden := hiddenFieldSet(view)
	out := make([]string, 0, len(ordered))
	for _, field := range ordered {
		if _, ok := hidden[field]; ok {
			continue
		}
		out = append(out, field)
	}
	return out
}

func orderedObservedFields(view config.LogViewConfig, observed []string) []string {
	if len(observed) == 0 {
		return nil
	}

	observedSet := make(map[string]struct{}, len(observed))
	for _, field := range observed {
		observedSet[field] = struct{}{}
	}

	out := make([]string, 0, len(observed))
	seen := make(map[string]struct{}, len(observed))
	for _, field := range view.VisibleFields {
		if _, ok := observedSet[field]; !ok {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	for _, field := range observed {
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func hiddenFieldSet(view config.LogViewConfig) map[string]struct{} {
	set := make(map[string]struct{}, len(view.HiddenFields))
	for _, field := range view.HiddenFields {
		set[field] = struct{}{}
	}
	return set
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
