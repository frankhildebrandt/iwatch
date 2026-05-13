package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/stackriot/iwatch/internal/buffer"
	"github.com/stackriot/iwatch/internal/config"
)

var (
	timeFormats = []string{"time", "date-short", "relative"}
	wrapModes   = []string{"off", "simple", "field"}
)

type logPaneContext struct {
	CommandTitle  string
	PresetTitle   string
	ProcessStatus string
	WatchStatus   string
	StatusDetail  string
	BufferLen     int
	BufferCap     int
	StreamCount   int
}

// LogPane owns log rendering, query input, selection, and cursor state.
type LogPane struct {
	buf         *buffer.LogBuffer
	queryInput  textinput.Model
	query       string
	matchCursor int
	cursor      int
	viewportTop int
	selecting   bool
	autoScroll  bool
}

// NewLogPane creates the primary log pane state from the current config.
func NewLogPane(cfg config.Config, buf *buffer.LogBuffer) *LogPane {
	queryInput := textinput.New()
	queryInput.Placeholder = "Filter/Search log... e.g. level=info heartbeat"
	queryInput.CharLimit = 256

	return &LogPane{
		buf:         buf,
		queryInput:  queryInput,
		matchCursor: -1,
		autoScroll:  true,
	}
}

// ID returns the pane identifier.
func (p *LogPane) ID() paneID {
	return paneLog
}

// IsOpen reports whether the log pane is visible.
func (p *LogPane) IsOpen() bool {
	return true
}

// SetOpen keeps the log pane open because it is always visible.
func (p *LogPane) SetOpen(_ bool) {}

// View renders the main log pane.
func (p *LogPane) View(width, height int, focused bool, lines []buffer.ViewLine, observed []string, view config.LogViewConfig, ctx logPaneContext) string {
	header := p.renderHeader(ctx)
	if len(lines) == 0 {
		return logPaneStyle(width, height).Render(header + "\nNo output yet.")
	}

	if p.cursor >= len(lines) {
		p.cursor = len(lines) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	start, end := p.visibleRange(width, height, lines, observed, view)

	rendered := make([]string, 0, end-start)
	for idx := start; idx < end; idx++ {
		rendered = append(rendered, p.renderStyledLine(lines[idx], width-4, p.selecting && idx == p.cursor, observed, view))
	}
	content := header + "\n" + strings.Join(rendered, "\n")
	return logPaneStyle(width, height).Render(content)
}

// InputBar renders the current help and query bar below the panes.
func (p *LogPane) InputBar() string {
	label := "log"
	keys := "[j/k, arrows, pgup/pgdown, ctrl+u/d, home/end/G] nav [t] truncate [v] fields [F] field filter [r] rebuild [l] streams [p] cmd output [/] query [g] config [S] split [[]/[]] preset [tab] focus [n/N] hit [?] help [q|esc x3] quit"
	if p.queryInput.Focused() {
		label = "query"
		keys = "[esc] close [enter] close"
	} else if p.selecting {
		label = "select"
		keys = "[j/k, arrows, pgup/pgdown, home/end/G] choose [esc] tail [?] help [q] quit"
	}

	value := p.queryInput.View()
	if !p.queryInput.Focused() && p.query == "" {
		value = p.queryInput.Placeholder
	}
	if !p.autoScroll && label == "log" {
		label = "log paused"
	}
	bar := fmt.Sprintf("%s> %s | %s", label, value, keys)

	return lipgloss.NewStyle().
		Padding(0, 1).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("255")).
		Render(bar)
}

func (p *LogPane) renderHeader(ctx logPaneContext) string {
	info := fmt.Sprintf("cmd: %s | preset: %s | run: %s | watch: %s | streams: %d | buffer: %d/%d", ctx.CommandTitle, ctx.PresetTitle, ctx.ProcessStatus, ctx.WatchStatus, ctx.StreamCount, ctx.BufferLen, ctx.BufferCap)
	if ctx.StatusDetail != "" {
		info += " | " + ctx.StatusDetail
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(info)
}

func (p *LogPane) renderStyledLine(line buffer.ViewLine, width int, selected bool, observed []string, view config.LogViewConfig) string {
	style := lipgloss.NewStyle()
	palette := paletteForLogView(view)
	switch line.HighlightRule {
	case "error":
		style = style.Foreground(lipgloss.Color("9")).Bold(true)
	case "warn":
		style = style.Foreground(lipgloss.Color("11"))
	case "success":
		style = style.Foreground(lipgloss.Color("10"))
	}

	prefixMarker := " "
	if line.Source == "stderr" {
		prefixMarker = "!"
	} else if line.Source == "system" {
		prefixMarker = ">"
	}

	timeColumn := ""
	if boolValue(view.ShowTimestamp) && view.TimeFormat == "time" {
		timeColumn = palette.renderTime(padRight(formatTimestamp(line.Timestamp, view.TimeFormat), 8))
	}

	fields := orderedVisibleFields(view, observed)
	contentParts := make([]string, 0, 1+len(fields))
	if boolValue(view.ShowSource) {
		contentParts = append(contentParts, line.Source)
	}
	for _, field := range fields {
		if value, ok := line.Fields[strings.ToLower(field)]; ok {
			contentParts = append(contentParts, renderStyledLogfmtPair(field, value, palette))
		}
	}

	body := ""
	if boolValue(view.ShowRawMessage) {
		body = line.Text
	}
	if body == "" && len(contentParts) == 0 {
		body = line.Text
	}
	if body != "" {
		contentParts = append(contentParts, styleLogfmtText(body, palette))
	}

	content := strings.Join(contentParts, " | ")
	leftPrefix := prefixMarker + " "
	if timeColumn != "" {
		leftPrefix += timeColumn + " "
	}
	if selected {
		leftPrefix = "> " + leftPrefix
	} else {
		leftPrefix = "  " + leftPrefix
	}

	rendered := content
	switch view.WrapMode {
	case "simple", "field":
		contentWidth := width - lipgloss.Width(leftPrefix)
		rendered = wrapWithIndent(content, max(10, contentWidth), strings.Repeat(" ", lipgloss.Width(leftPrefix)))
	default:
		if width > 0 {
			maxWidth := max(0, width-lipgloss.Width(leftPrefix))
			rendered = truncateWithEllipsis(content, maxWidth)
		}
	}

	if selected {
		style = style.Background(lipgloss.Color("238")).Bold(true)
	}
	return style.Render(leftPrefix + rendered)
}

func (p *LogPane) refreshQueryState(followTail bool, lines []buffer.ViewLine) {
	p.query = p.queryInput.Value()
	if len(lines) == 0 {
		p.cursor = 0
		p.matchCursor = -1
		p.viewportTop = 0
		p.autoScroll = true
		return
	}
	if followTail {
		p.autoScroll = true
		p.cursor = len(lines) - 1
		if p.query != "" {
			p.matchCursor = p.cursor
		}
		return
	}
	if p.cursor >= len(lines) {
		p.cursor = len(lines) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.query != "" {
		p.matchCursor = p.cursor
	} else {
		p.matchCursor = -1
	}
	p.syncAutoScroll(lines)
}

func (p *LogPane) moveCursor(delta int) {
	p.cursor += delta
	if p.cursor < 0 {
		p.cursor = 0
	}
}

func (p *LogPane) pageCursor(direction int, pageSize int) {
	step := max(1, pageSize/2)
	p.moveCursor(direction * step)
}

func (p *LogPane) visibleRange(width, height int, lines []buffer.ViewLine, observed []string, view config.LogViewConfig) (int, int) {
	if len(lines) == 0 {
		return 0, 0
	}

	contentRows := max(1, height-3)
	start := max(0, min(p.viewportTop, len(lines)-1))
	usedRows := 0
	end := start
	for idx := start; idx < len(lines); idx++ {
		lineRows := p.lineHeight(lines[idx], width, observed, view)
		if idx > start && usedRows+lineRows > contentRows {
			break
		}
		usedRows += lineRows
		end = idx + 1
	}
	if end == start {
		end = min(len(lines), start+1)
	}
	return start, end
}

func (p *LogPane) lineIndexAt(y, width, height int, lines []buffer.ViewLine, observed []string, view config.LogViewConfig) (int, bool) {
	if len(lines) == 0 || width < 4 || height < 3 {
		return 0, false
	}

	if y < 2 || y >= height-1 {
		return 0, false
	}
	contentRow := y - 2

	start, end := p.visibleRange(width, height, lines, observed, view)
	row := 0
	for idx := start; idx < end; idx++ {
		renderedHeight := p.lineHeight(lines[idx], width, observed, view)
		if contentRow >= row && contentRow < row+renderedHeight {
			return idx, true
		}
		row += renderedHeight
	}

	return 0, false
}

func (p *LogPane) moveToEnd(lines []buffer.ViewLine) {
	if len(lines) == 0 {
		p.cursor = 0
		p.viewportTop = 0
		return
	}
	p.cursor = len(lines) - 1
}

func (p *LogPane) currentLine(lines []buffer.ViewLine) (buffer.ViewLine, bool) {
	if len(lines) == 0 {
		return buffer.ViewLine{}, false
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(lines) {
		p.cursor = len(lines) - 1
	}
	return lines[p.cursor], true
}

func (p *LogPane) jumpMatch(direction int, lines []buffer.ViewLine) {
	if len(lines) == 0 || p.query == "" {
		return
	}
	if p.matchCursor < 0 || p.matchCursor >= len(lines) {
		p.matchCursor = p.cursor
	}
	if direction >= 0 {
		p.matchCursor++
		if p.matchCursor >= len(lines) {
			p.matchCursor = 0
		}
	} else {
		p.matchCursor--
		if p.matchCursor < 0 {
			p.matchCursor = len(lines) - 1
		}
	}
	p.cursor = p.matchCursor
	p.syncAutoScroll(lines)
}

func (p *LogPane) syncAutoScroll(lines []buffer.ViewLine) {
	if len(lines) == 0 {
		p.autoScroll = true
		return
	}
	p.autoScroll = p.cursor >= len(lines)-1
}

func (p *LogPane) ensureCursorVisible(width, height int, lines []buffer.ViewLine, observed []string, view config.LogViewConfig) {
	if len(lines) == 0 {
		p.cursor = 0
		p.viewportTop = 0
		return
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(lines) {
		p.cursor = len(lines) - 1
	}

	if p.autoScroll && p.cursor >= len(lines)-1 {
		p.viewportTop = p.tailViewportTop(width, height, lines, observed, view)
		return
	}

	start, end := p.visibleRange(width, height, lines, observed, view)
	if p.cursor < start {
		p.viewportTop = p.cursor
		return
	}
	if p.cursor < end {
		return
	}

	for top := start + 1; top < len(lines); top++ {
		p.viewportTop = top
		start, end = p.visibleRange(width, height, lines, observed, view)
		if p.cursor >= start && p.cursor < end {
			return
		}
	}

	p.viewportTop = p.cursor
}

func (p *LogPane) tailViewportTop(width, height int, lines []buffer.ViewLine, observed []string, view config.LogViewConfig) int {
	if len(lines) == 0 {
		return 0
	}

	contentRows := max(1, height-3)
	start := len(lines) - 1
	usedRows := p.lineHeight(lines[start], width, observed, view)
	for start > 0 {
		nextRows := p.lineHeight(lines[start-1], width, observed, view)
		if usedRows+nextRows > contentRows {
			break
		}
		start--
		usedRows += nextRows
	}
	return start
}

func (p *LogPane) lineHeight(line buffer.ViewLine, width int, observed []string, view config.LogViewConfig) int {
	return lipgloss.Height(p.renderStyledLine(line, width-4, p.selecting && line.Index == p.cursor, observed, view))
}

func logPaneStyle(width, height int) lipgloss.Style {
	return lipgloss.NewStyle().
		Padding(1, 1).
		Width(width).
		Height(height)
}

func styleLogfmtText(value string, palette logPalette) string {
	tokens := splitTokens(value)
	if len(tokens) == 0 {
		return value
	}

	styled := make([]string, 0, len(tokens))
	for _, token := range tokens {
		key, rawValue, ok := strings.Cut(token, "=")
		if !ok || key == "" {
			styled = append(styled, token)
			continue
		}
		styled = append(styled, renderStyledLogfmtPair(key, rawValue, palette))
	}
	return strings.Join(styled, " ")
}

func renderStyledLogfmtPair(key, value string, palette logPalette) string {
	return palette.renderKey(key) + "=" + palette.renderValue(value)
}

func formatTimestamp(ts time.Time, mode string) string {
	switch mode {
	case "date-short":
		return ts.Format("02.01. 15:04:05")
	case "relative":
		return humanDuration(time.Since(ts))
	default:
		return ts.Format("15:04:05")
	}
}

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
