package tui

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/stackriot/iwatch/internal/buffer"
	"github.com/stackriot/iwatch/internal/config"
	"github.com/stackriot/iwatch/internal/detect"
	"github.com/stackriot/iwatch/internal/runner"
	"github.com/stackriot/iwatch/internal/watch"
)

type paneID string

const (
	paneLog     paneID = "log"
	paneCommand paneID = "commands"
	paneEvents  paneID = "events"
)

type App struct {
	cfg         config.Config
	commands    []detect.Command
	commandByID map[string]detect.Command
	activeCmd   string
	buf         *buffer.LogBuffer
	runner      *runner.Runner
	watcher     *watch.Watcher
	cancelWatch context.CancelFunc

	width     int
	height    int
	openPanes map[paneID]bool
	focus     paneID
	splitDir  string

	commandList list.Model
	events      []string
	queryInput  textinput.Model

	query       string
	matchCursor int
	cursor      int

	processStatus string
	watchStatus   string
	statusDetail  string
	stale         bool
	escCount      int
	lastEsc       time.Time
}

type tickMsg time.Time
type runnerMsg runner.Event
type watchMsg watch.Event
type watchErrMsg struct{ err error }

func New(cfg config.Config, commands []detect.Command, defaultCommand string, buf *buffer.LogBuffer, run *runner.Runner, watcher *watch.Watcher) *App {
	items := make([]list.Item, 0, len(commands))
	byID := make(map[string]detect.Command, len(commands))
	for _, cmd := range commands {
		items = append(items, commandItem{command: cmd})
		byID[cmd.ID] = cmd
	}

	commandList := list.New(items, commandDelegate{}, 30, 10)
	commandList.Title = "Commands"
	commandList.SetShowHelp(false)
	commandList.SetFilteringEnabled(true)

	queryInput := textinput.New()
	queryInput.Placeholder = "Filter/Search log... e.g. level=info heartbeat"
	queryInput.CharLimit = 256

	openPanes := map[paneID]bool{paneLog: true}
	for _, pane := range cfg.UI.OpenPanes {
		switch paneID(pane) {
		case paneCommand, paneEvents:
			openPanes[paneID(pane)] = true
		}
	}
	openPanes[paneLog] = true

	focus := paneID(cfg.UI.FocusPane)
	if focus != paneCommand && focus != paneEvents {
		focus = paneLog
	}

	return &App{
		cfg:           cfg,
		commands:      commands,
		commandByID:   byID,
		activeCmd:     defaultCommand,
		buf:           buf,
		runner:        run,
		watcher:       watcher,
		openPanes:     openPanes,
		focus:         focus,
		splitDir:      cfg.UI.SplitDirection,
		commandList:   commandList,
		queryInput:    queryInput,
		matchCursor:   -1,
		processStatus: "idle",
		watchStatus:   "clean",
	}
}

func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd(), a.startInitialRun()}
	if a.watcher != nil {
		ctx, cancel := context.WithCancel(context.Background())
		a.cancelWatch = cancel
		go a.watcher.Run(ctx, 250*time.Millisecond)
		cmds = append(cmds, waitWatchEvent(a.watcher), waitWatchError(a.watcher))
	}
	cmds = append(cmds, waitRunnerEvent(a.runner))
	return tea.Batch(cmds...)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil
	case tea.KeyMsg:
		return a.handleKey(msg)
	case tickMsg:
		if time.Since(a.lastEsc) > 2*time.Second {
			a.escCount = 0
		}
		return a, tickCmd()
	case runnerMsg:
		return a.handleRunner(runner.Event(msg))
	case watchMsg:
		a.stale = true
		a.watchStatus = "rebuild possible"
		a.statusDetail = msg.Path
		a.events = appendTrimmed(a.events, fmt.Sprintf("%s %s", msg.Op.String(), msg.Path), 200)
		return a, waitWatchEvent(a.watcher)
	case watchErrMsg:
		a.events = appendTrimmed(a.events, "watch error: "+msg.err.Error(), 200)
		return a, waitWatchError(a.watcher)
	}

	if a.openPanes[paneCommand] {
		var cmd tea.Cmd
		a.commandList, cmd = a.commandList.Update(msg)
		return a, cmd
	}
	return a, nil
}

func (a *App) View() string {
	if a.width == 0 || a.height == 0 {
		return "loading..."
	}

	inputHeight := 1
	bodyHeight := max(5, a.height-inputHeight)

	main := a.renderLogPane(a.width, bodyHeight)
	side := a.renderSidePanes(max(30, a.width/3), bodyHeight)

	var body string
	if side == "" {
		body = main
	} else if a.splitDir == "horizontal" {
		body = lipgloss.JoinVertical(lipgloss.Left, main, side)
	} else {
		mainWidth := a.width - max(30, a.width/3)
		body = lipgloss.JoinHorizontal(lipgloss.Top, a.renderLogPane(mainWidth, bodyHeight), side)
	}

	return lipgloss.JoinVertical(lipgloss.Left, body, a.renderInputBar())
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.queryInput.Focused() {
		switch msg.String() {
		case "esc":
			a.queryInput.Blur()
			a.focus = paneLog
			a.escCount = 0
			return a, nil
		case "enter":
			a.queryInput.Blur()
			a.focus = paneLog
			return a, nil
		}

		var cmd tea.Cmd
		a.queryInput, cmd = a.queryInput.Update(msg)
		a.refreshQueryState(false)
		return a, cmd
	}

	if msg.String() == "esc" {
		if time.Since(a.lastEsc) > 2*time.Second {
			a.escCount = 0
		}
		a.lastEsc = time.Now()
		a.escCount++
		if a.escCount >= 3 {
			if a.cancelWatch != nil {
				a.cancelWatch()
			}
			_ = a.runner.Stop(1500 * time.Millisecond)
			return a, tea.Quit
		}
	} else {
		a.escCount = 0
	}

	switch msg.String() {
	case "up":
		a.cursor = max(0, a.cursor-1)
		return a, nil
	case "down":
		a.cursor++
		return a, nil
	case "pgup":
		a.cursor = max(0, a.cursor-10)
		return a, nil
	case "pgdown":
		a.cursor += 10
		return a, nil
	case "tab":
		a.focus = a.nextPane()
		return a, nil
	case "r":
		return a, a.restartActive()
	case "c":
		a.togglePane(paneCommand)
		a.focus = paneCommand
		return a, nil
	case "/", "f":
		a.focus = paneLog
		a.queryInput.Focus()
		return a, nil
	case "w":
		a.togglePane(paneEvents)
		a.focus = paneEvents
		return a, nil
	case "s":
		if a.splitDir == "horizontal" {
			a.splitDir = "vertical"
		} else {
			a.splitDir = "horizontal"
		}
		return a, nil
	case "enter":
		if a.focus == paneCommand {
			if item, ok := a.commandList.SelectedItem().(commandItem); ok {
				a.activeCmd = item.command.ID
				return a, a.restartActive()
			}
		}
		return a, nil
	case "n":
		a.jumpMatch(1)
		return a, nil
	case "N":
		a.jumpMatch(-1)
		return a, nil
	}
	return a, nil
}

func (a *App) handleRunner(ev runner.Event) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case runner.EventStarted:
		a.processStatus = fmt.Sprintf("running pid=%d", ev.PID)
		a.watchStatus = "clean"
		a.stale = false
		a.statusDetail = ""
	case runner.EventOutput:
		a.buf.Append(ev.Source, ev.Text)
		a.refreshQueryState(true)
	case runner.EventExited:
		if ev.Err != nil {
			a.processStatus = fmt.Sprintf("exited (%d)", ev.Code)
			a.buf.Append("system", fmt.Sprintf("process exited with code %d", ev.Code))
		} else {
			a.processStatus = "completed"
			a.buf.Append("system", "process completed successfully")
		}
		a.refreshQueryState(true)
	case runner.EventError:
		a.buf.Append("system", "runner error: "+ev.Err.Error())
		a.refreshQueryState(true)
	}
	return a, waitRunnerEvent(a.runner)
}

func (a *App) renderLogPane(width, height int) string {
	lines := a.buf.Snapshot(a.query)
	header := a.renderLogHeader()
	if len(lines) == 0 {
		return paneStyle(a.focus == paneLog, width, height).Render(header + "\nNo output yet.")
	}

	maxVisible := max(1, height-3)
	if a.cursor >= len(lines) {
		a.cursor = len(lines) - 1
	}
	if a.cursor < 0 {
		a.cursor = 0
	}
	start := max(0, a.cursor-maxVisible+1)
	end := min(len(lines), start+maxVisible)

	rendered := make([]string, 0, end-start)
	for _, line := range lines[start:end] {
		rendered = append(rendered, a.renderLine(line, width-4))
	}
	content := header + "\n" + strings.Join(rendered, "\n")
	return paneStyle(a.focus == paneLog, width, height).Render(content)
}

func (a *App) renderLine(line buffer.ViewLine, width int) string {
	text := line.Text
	if width > 0 {
		text = lipgloss.NewStyle().MaxWidth(width).Render(text)
	}

	style := lipgloss.NewStyle()
	switch line.HighlightRule {
	case "error":
		style = style.Foreground(lipgloss.Color("9")).Bold(true)
	case "warn":
		style = style.Foreground(lipgloss.Color("11"))
	case "success":
		style = style.Foreground(lipgloss.Color("10"))
	}
	if line.Matched {
		style = style.Background(lipgloss.Color("238")).Bold(true)
	}

	prefix := " "
	if line.Source == "stderr" {
		prefix = "!"
	} else if line.Source == "system" {
		prefix = ">"
	}
	return style.Render(prefix + " " + text)
}

func (a *App) renderSidePanes(width, height int) string {
	var parts []string
	if a.openPanes[paneCommand] {
		a.commandList.SetSize(width-2, max(5, height/3))
		parts = append(parts, paneStyle(a.focus == paneCommand, width, max(5, height/3)).Render(a.commandList.View()))
	}
	if a.openPanes[paneEvents] {
		events := strings.Join(lastN(a.events, max(3, height/3-2)), "\n")
		if events == "" {
			events = "No watch events yet."
		}
		parts = append(parts, paneStyle(a.focus == paneEvents, width, max(5, height/3)).Render("Watch events\n"+events))
	}
	return strings.Join(parts, "\n")
}

func (a *App) renderLogHeader() string {
	commandTitle := a.activeCmd
	if cmd, ok := a.commandByID[a.activeCmd]; ok {
		commandTitle = cmd.Title
	}
	info := fmt.Sprintf("cmd: %s | run: %s | watch: %s | buffer: %d/%d", commandTitle, a.processStatus, a.watchStatus, a.buf.Len(), a.cfg.BufferLines)
	if a.statusDetail != "" {
		info += " | " + a.statusDetail
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(info)
}

func (a *App) renderInputBar() string {
	label := "log"
	keys := "[r] rebuild [c] commands [/] query [f] query [w] events [tab] focus [n/N] next/prev [esc x3] quit"
	if a.queryInput.Focused() {
		label = "query"
		keys = "[esc] close [enter] close [n/N] edit text"
	}

	value := a.queryInput.View()
	if !a.queryInput.Focused() && a.query == "" {
		value = a.queryInput.Placeholder
	}
	bar := fmt.Sprintf("%s> %s | %s", label, value, keys)

	return lipgloss.NewStyle().
		Padding(0, 1).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("255")).
		Render(bar)
}

func (a *App) nextPane() paneID {
	var panes []string
	for pane, open := range a.openPanes {
		if open {
			panes = append(panes, string(pane))
		}
	}
	sort.Strings(panes)
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
	if id == paneLog {
		return
	}
	a.openPanes[id] = !a.openPanes[id]
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

func Run(cfg config.Config, commands []detect.Command, defaultCommand string, buf *buffer.LogBuffer, run *runner.Runner, watcher *watch.Watcher) error {
	app := New(cfg, commands, defaultCommand, buf, run, watcher)
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err := p.Run()
	return err
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

func (a *App) refreshQueryState(followTail bool) {
	a.query = a.queryInput.Value()
	lines := a.buf.Snapshot(a.query)

	if len(lines) == 0 {
		a.cursor = 0
		a.matchCursor = -1
		return
	}

	if followTail {
		a.cursor = len(lines) - 1
		if a.query != "" {
			a.matchCursor = a.cursor
		}
		return
	}

	if a.cursor >= len(lines) {
		a.cursor = len(lines) - 1
	}
	if a.cursor < 0 {
		a.cursor = 0
	}
	if a.query != "" {
		a.matchCursor = a.cursor
	} else {
		a.matchCursor = -1
	}
}

func (a *App) jumpMatch(direction int) {
	lines := a.buf.Snapshot(a.query)
	if len(lines) == 0 || a.query == "" {
		return
	}

	if a.matchCursor < 0 || a.matchCursor >= len(lines) {
		a.matchCursor = a.cursor
	}

	if direction >= 0 {
		a.matchCursor++
		if a.matchCursor >= len(lines) {
			a.matchCursor = 0
		}
	} else {
		a.matchCursor--
		if a.matchCursor < 0 {
			a.matchCursor = len(lines) - 1
		}
	}
	a.cursor = a.matchCursor
}

type commandItem struct {
	command detect.Command
}

func (i commandItem) Title() string       { return i.command.Title }
func (i commandItem) Description() string { return i.command.Cmd + " [" + i.command.Source + "]" }
func (i commandItem) FilterValue() string { return i.command.Title + " " + i.command.Cmd }

type commandDelegate struct{}

func (commandDelegate) Height() int                             { return 2 }
func (commandDelegate) Spacing() int                            { return 0 }
func (commandDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (commandDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ci := item.(commandItem)
	line := ci.Title()
	if index == m.Index() {
		line = "> " + line
	}
	fmt.Fprint(w, line+"\n"+ci.Description())
}
