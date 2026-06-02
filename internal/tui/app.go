package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/stackriot/iwatch/internal/buffer"
	"github.com/stackriot/iwatch/internal/config"
	"github.com/stackriot/iwatch/internal/detect"
	"github.com/stackriot/iwatch/internal/runner"
	"github.com/stackriot/iwatch/internal/stream"
)

type paneID string
type appMode string
type shutdownAction string

const (
	paneLog     paneID = "log"
	paneEvents  paneID = "events"
	paneStreams paneID = "streams"

	modeMain        appMode = "main"
	modeConfig      appMode = "config"
	modeDetail      appMode = "detail"
	modeStream      appMode = "stream"
	modeShare       appMode = "share"
	modeHelp        appMode = "help"
	modeFields      appMode = "fields"
	modeFieldFilter appMode = "field-filter"

	shutdownQuit    shutdownAction = "quit"
	shutdownRebuild shutdownAction = "rebuild"
)

// App coordinates the TUI, pane focus, and background runner events.
type App struct {
	cfg                  config.Config
	configPath           string
	commands             []detect.Command
	commandByID          map[string]detect.Command
	activeCmd            string
	buf                  *buffer.LogBuffer
	runner               *runner.Runner
	streams              *stream.Supervisor
	fieldFilters         map[string]string
	streamLines          map[string][]string
	streamDetailID       string
	runtimeStreams       map[string]config.StreamConfig
	runtimeStreamOrder   []string
	pendingOutput        []runner.Event
	outputFlushScheduled bool

	width  int
	height int
	focus  paneID
	mode   appMode

	logPane     *LogPane
	eventsPane  *EventsPane
	streamsPane *StreamsPane
	fieldMenu   *fieldMenu
	filterMenu  *fieldFilterMenu
	editor      *ConfigEditor
	detail      *DetailView
	help        *HelpView
	share       *ShareView
	automations *AutomationEngine

	processStatus    string
	appStatus        string
	statusDetail     string
	shutdownState    shutdownAction
	restartStreamIDs []string
	escCount         int
	lastEsc          time.Time
	lastEnter        time.Time
	lastClick        time.Time
	lastClickLine    int

	backendURL string
	viteURL    string
}

type tickMsg time.Time
type outputFlushMsg time.Time
type runnerMsg runner.Event
type streamMsg stream.Event
type shutdownDoneMsg struct{ action shutdownAction }
type shareResultMsg struct {
	copied   bool
	path     string
	err      error
	contents string
}

// New creates the TUI model without starting the Bubble Tea program.
func New(cfg config.Config, configPath string, commands []detect.Command, defaultCommand string, buf *buffer.LogBuffer, run *runner.Runner) *App {
	return NewWithStreams(cfg, configPath, commands, defaultCommand, buf, run, nil)
}

// NewWithStreams creates the TUI model with an optional stream supervisor.
func NewWithStreams(cfg config.Config, configPath string, commands []detect.Command, defaultCommand string, buf *buffer.LogBuffer, run *runner.Runner, streams *stream.Supervisor) *App {
	commandByID := make(map[string]detect.Command, len(commands))
	for _, cmd := range commands {
		commandByID[cmd.ID] = cmd
	}

	focus := paneID(cfg.UI.FocusPane)
	if focus != paneEvents && focus != paneStreams {
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
		fieldFilters:   make(map[string]string),
		streamLines:    make(map[string][]string),
		runtimeStreams: make(map[string]config.StreamConfig),
		focus: focus,
		mode:           modeMain,
		processStatus:  "idle",
		appStatus:      "clean",
	}

	app.logPane = NewLogPane(cfg, buf)
	app.eventsPane = NewEventsPane(cfg.UI.OpenPanes)
	app.streamsPane = NewStreamsPane(cfg.UI.OpenPanes)
	app.fieldMenu = newFieldMenu()
	app.filterMenu = newFieldFilterMenu()
	app.editor = NewConfigEditor(configPath)
	app.detail = NewDetailView()
	app.help = NewHelpView()
	app.share = NewShareView()
	app.automations = NewAutomationEngine(cfg.UI.Automations)

	return app
}

// Init starts the background tick, runner, and stream integration.
func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd(), a.startInitialRun()}
	a.applyActiveStreams()
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
		case modeShare:
			return a.handleShareKey(msg)
		case modeHelp:
			return a.handleHelpKey(msg)
		case modeFields:
			return a.handleFieldKey(msg)
		case modeFieldFilter:
			return a.handleFieldFilterKey(msg)
		default:
			return a.handleMainKey(msg)
		}
	case shareResultMsg:
		a.share.ApplyResult(msg)
		return a, nil
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
	case shutdownDoneMsg:
		return a.handleShutdownDone(msg)
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
	case modeShare:
		return a.share.View(a.width, a.height)
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

	return lipgloss.JoinVertical(lipgloss.Left, body, a.inputBar())
}

// Run starts the Bubble Tea program for the configured app model.
func Run(cfg config.Config, configPath string, commands []detect.Command, defaultCommand string, buf *buffer.LogBuffer, run *runner.Runner, streams *stream.Supervisor) error {
	app := NewWithStreams(cfg, configPath, commands, defaultCommand, buf, run, streams)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
