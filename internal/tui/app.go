package tui

import (
	"context"
	"fmt"
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

type paneID string
type appMode string

const (
	paneLog     paneID = "log"
	paneCommand paneID = "commands"
	paneEvents  paneID = "events"
	paneStreams paneID = "streams"

	modeMain        appMode = "main"
	modeConfig      appMode = "config"
	modeDetail      appMode = "detail"
	modeStream      appMode = "stream"
	modeHelp        appMode = "help"
	modeFields      appMode = "fields"
	modeFieldFilter appMode = "field-filter"
)

// App coordinates the TUI, pane focus, and background runner/watch events.
type App struct {
	cfg                  config.Config
	configPath           string
	commands             []detect.Command
	commandByID          map[string]detect.Command
	activeCmd            string
	buf                  *buffer.LogBuffer
	runner               *runner.Runner
	streams              *stream.Supervisor
	watcher              *watch.Watcher
	cancelWatch          context.CancelFunc
	fieldFilters         map[string]string
	streamLines          map[string][]string
	streamDetailID       string
	pendingOutput        []runner.Event
	outputFlushScheduled bool
	logWindowLimit       int

	width  int
	height int
	focus  paneID
	mode   appMode

	logPane     *LogPane
	commandPane *CommandPane
	eventsPane  *EventsPane
	streamsPane *StreamsPane
	fieldMenu   *fieldMenu
	filterMenu  *fieldFilterMenu
	editor      *ConfigEditor
	detail      *DetailView
	help        *HelpView

	processStatus string
	watchStatus   string
	statusDetail  string
	stale         bool
	escCount      int
	lastEsc       time.Time
	lastEnter     time.Time
	lastClick     time.Time
	lastClickLine int
}

type tickMsg time.Time
type outputFlushMsg time.Time
type runnerMsg runner.Event
type streamMsg stream.Event
type watchMsg watch.Event
type watchErrMsg struct{ err error }

// New creates the TUI model without starting the Bubble Tea program.
func New(cfg config.Config, configPath string, commands []detect.Command, defaultCommand string, buf *buffer.LogBuffer, run *runner.Runner, watcher *watch.Watcher) *App {
	return NewWithStreams(cfg, configPath, commands, defaultCommand, buf, run, nil, watcher)
}

// NewWithStreams creates the TUI model with an optional stream supervisor.
func NewWithStreams(cfg config.Config, configPath string, commands []detect.Command, defaultCommand string, buf *buffer.LogBuffer, run *runner.Runner, streams *stream.Supervisor, watcher *watch.Watcher) *App {
	commandByID := make(map[string]detect.Command, len(commands))
	for _, cmd := range commands {
		commandByID[cmd.ID] = cmd
	}

	focus := paneID(cfg.UI.FocusPane)
	if focus != paneCommand && focus != paneEvents && focus != paneStreams {
		focus = paneLog
	}

	app := &App{
		cfg:            cfg,
		configPath:     configPath,
		commands:       commands,
		commandByID:    commandByID,
		activeCmd:      defaultCommand,
		buf:            buf,
		runner:         run,
		streams:        streams,
		watcher:        watcher,
		fieldFilters:   make(map[string]string),
		streamLines:    make(map[string][]string),
		logWindowLimit: logShortMemoryLines,
		focus:          focus,
		mode:           modeMain,
		processStatus:  "idle",
		watchStatus:    "clean",
	}

	app.logPane = NewLogPane(cfg, buf)
	app.commandPane = NewCommandPane(commands, cfg.UI.OpenPanes)
	app.eventsPane = NewEventsPane(cfg.UI.OpenPanes)
	app.streamsPane = NewStreamsPane(cfg.UI.OpenPanes)
	app.fieldMenu = newFieldMenu()
	app.filterMenu = newFieldFilterMenu()
	app.editor = NewConfigEditor(configPath)
	app.detail = NewDetailView()
	app.help = NewHelpView()

	return app
}

// Init starts the background tick, runner, and optional watcher integration.
func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd(), a.startInitialRun()}
	a.applyActiveStreams()
	if a.watcher != nil {
		ctx, cancel := context.WithCancel(context.Background())
		a.cancelWatch = cancel
		go a.watcher.Run(ctx, 250*time.Millisecond)
		cmds = append(cmds, waitWatchEvent(a.watcher), waitWatchError(a.watcher))
	}
	cmds = append(cmds, waitRunnerEvent(a.runner))
	if a.streams != nil {
		cmds = append(cmds, waitStreamEvent(a.streams))
	}
	return tea.Batch(cmds...)
}

// Update routes Bubble Tea messages to the active screen and background handlers.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil
	case tea.MouseMsg:
		return a.handleMouse(msg)
	case tea.KeyMsg:
		switch a.mode {
		case modeConfig:
			return a.handleConfigKey(msg)
		case modeDetail:
			return a.handleDetailKey(msg)
		case modeStream:
			return a.handleStreamDetailKey(msg)
		case modeHelp:
			return a.handleHelpKey(msg)
		case modeFields:
			return a.handleFieldKey(msg)
		case modeFieldFilter:
			return a.handleFieldFilterKey(msg)
		default:
			return a.handleMainKey(msg)
		}
	case tickMsg:
		if time.Since(a.lastEsc) > 2*time.Second {
			a.escCount = 0
		}
		return a, tickCmd()
	case outputFlushMsg:
		a.outputFlushScheduled = false
		a.flushPendingOutput()
		return a, nil
	case runnerMsg:
		return a.handleRunner(runner.Event(msg))
	case streamMsg:
		return a.handleStream(stream.Event(msg))
	case watchMsg:
		a.stale = true
		a.watchStatus = "rebuild possible"
		a.statusDetail = msg.Path
		a.eventsPane.Append(fmt.Sprintf("%s %s", msg.Op.String(), msg.Path))
		return a, waitWatchEvent(a.watcher)
	case watchErrMsg:
		a.eventsPane.Append("watch error: " + msg.err.Error())
		return a, waitWatchError(a.watcher)
	}

	if a.commandPane.IsOpen() {
		var cmd tea.Cmd
		cmd = a.commandPane.Update(msg)
		return a, cmd
	}
	return a, nil
}

// View renders the current screen.
func (a *App) View() string {
	if a.width == 0 || a.height == 0 {
		return "loading..."
	}

	switch a.mode {
	case modeConfig:
		return a.editor.View(a.width, a.height, a.logPageSize())
	case modeDetail:
		return a.detail.View(a.width, a.height)
	case modeStream:
		return a.renderStreamDetail()
	case modeHelp:
		return a.help.View(a.width, a.height)
	case modeFields:
		return lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.Place(
				a.width,
				a.bodyHeight(),
				lipgloss.Center,
				lipgloss.Center,
				a.fieldMenu.View(a.width, a.bodyHeight(), a.buf.ObservedFields(), a.cfg.UI.LogView),
			),
			a.fieldMenu.InputBar(),
		)
	case modeFieldFilter:
		return lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.Place(
				a.width,
				a.bodyHeight(),
				lipgloss.Center,
				lipgloss.Center,
				a.filterMenu.View(a.width, a.bodyHeight(), a.buf.ObservedFields(), a.fieldFilters),
			),
			a.filterMenu.InputBar(),
		)
	}

	bodyHeight := a.bodyHeight()
	lines := a.snapshotLines()
	observed := a.buf.ObservedFields()

	main := a.logPane.View(a.width, bodyHeight, a.focus == paneLog, lines, observed, a.cfg.UI.LogView, a.logPaneContext())
	side := a.renderSidePanes(max(30, a.width/3), bodyHeight)

	var body string
	if side == "" {
		body = main
	} else if a.cfg.UI.SplitDirection == "horizontal" {
		body = lipgloss.JoinVertical(lipgloss.Left, main, side)
	} else {
		mainWidth := a.width - max(30, a.width/3)
		body = lipgloss.JoinHorizontal(lipgloss.Top, a.logPane.View(mainWidth, bodyHeight, a.focus == paneLog, lines, observed, a.cfg.UI.LogView, a.logPaneContext()), side)
	}

	return lipgloss.JoinVertical(lipgloss.Left, body, a.logPane.InputBar())
}

// Run starts the Bubble Tea program for the configured app model.
func Run(cfg config.Config, configPath string, commands []detect.Command, defaultCommand string, buf *buffer.LogBuffer, run *runner.Runner, streams *stream.Supervisor, watcher *watch.Watcher) error {
	app := NewWithStreams(cfg, configPath, commands, defaultCommand, buf, run, streams, watcher)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
