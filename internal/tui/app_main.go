package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stackriot/iwatch/internal/runner"
	"github.com/stackriot/iwatch/internal/stream"
)

const mouseRowOffset = 2
const doubleClickWindow = 600 * time.Millisecond

func (a *App) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch a.mode {
	case modeDetail:
		switch {
		case msg.Button == tea.MouseButtonWheelUp && msg.Action == tea.MouseActionPress:
			a.detail.scroll = max(0, a.detail.scroll-3)
		case msg.Button == tea.MouseButtonWheelDown && msg.Action == tea.MouseActionPress:
			a.detail.scroll += 3
		}
		return a, nil
	case modeConfig, modeHelp:
		return a, nil
	}

	switch {
	case msg.Button == tea.MouseButtonWheelUp && msg.Action == tea.MouseActionPress:
		lines := a.snapshotLines()
		a.logPane.moveCursor(-3)
		a.logPane.syncAutoScroll(lines)
		a.syncLogViewport(lines)
		return a, nil
	case msg.Button == tea.MouseButtonWheelDown && msg.Action == tea.MouseActionPress:
		lines := a.snapshotLines()
		a.logPane.moveCursor(3)
		a.logPane.syncAutoScroll(lines)
		a.syncLogViewport(lines)
		return a, nil
	case msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress:
		return a, nil
	}

	bodyHeight := a.bodyHeight()
	adjustedY := msg.Y + mouseRowOffset
	if adjustedY < 0 || adjustedY >= bodyHeight {
		return a, nil
	}

	logWidth := a.logPaneWidth()
	if msg.X <= 0 || msg.X >= logWidth-1 {
		return a, nil
	}

	lines := a.snapshotLines()
	lineIndex, ok := a.logPane.lineIndexAt(adjustedY, logWidth, bodyHeight, lines, a.buf.ObservedFields(), a.cfg.UI.LogView)
	if !ok {
		return a, nil
	}

	if a.lastClickLine == lineIndex && time.Since(a.lastClick) <= doubleClickWindow {
		a.lastClick = time.Time{}
		a.lastClickLine = -1
		a.logPane.cursor = lineIndex
		a.logPane.selecting = true
		a.logPane.syncAutoScroll(lines)
		a.syncLogViewport(lines)
		a.openLineDetail()
		return a, nil
	}

	a.focus = paneLog
	a.logPane.cursor = lineIndex
	a.logPane.selecting = true
	a.logPane.syncAutoScroll(lines)
	a.syncLogViewport(lines)
	a.lastClick = time.Now()
	a.lastClickLine = lineIndex
	return a, nil
}

func (a *App) handleMainKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.logPane.queryInput.Focused() {
		switch msg.String() {
		case "esc", "enter":
			a.logPane.queryInput.Blur()
			a.focus = paneLog
			if msg.String() == "esc" {
				a.escCount = 0
			}
			return a, nil
		}

		var cmd tea.Cmd
		a.logPane.queryInput, cmd = a.logPane.queryInput.Update(msg)
		a.resetLogWindow()
		a.logPane.refreshQueryState(false, a.snapshotLines())
		return a, cmd
	}

	if msg.String() == "esc" {
		if a.logPane.selecting {
			a.moveLogToTail()
			a.escCount = 0
			return a, nil
		}
		if time.Since(a.lastEsc) > 2*time.Second {
			a.escCount = 0
		}
		a.lastEsc = time.Now()
		a.escCount++
		if a.escCount >= 3 {
			return a, a.quitCmd()
		}
	} else {
		a.escCount = 0
	}

	switch msg.String() {
	case "q":
		return a, a.quitCmd()
	case "?":
		a.mode = modeHelp
	case "up", "k":
		if a.focus == paneStreams {
			a.streamsPane.Move(-1, len(a.streamStatuses()))
			return a, nil
		}
		a.moveLogCursor(-1)
	case "down", "j":
		if a.focus == paneStreams {
			a.streamsPane.Move(1, len(a.streamStatuses()))
			return a, nil
		}
		a.moveLogCursor(1)
	case "pgup", "ctrl+u":
		a.pageLogCursor(-1)
	case "pgdown", "ctrl+d":
		a.pageLogCursor(1)
	case "home":
		a.logWindowLimit = a.buf.Len()
		lines := a.snapshotLines()
		a.logPane.cursor = 0
		a.logPane.syncAutoScroll(lines)
		a.syncLogViewport(lines)
	case "end", "G":
		a.moveLogToTail()
	case "tab":
		a.focus = a.nextPane()
	case "r":
		return a, a.restartActive()
	case "t":
		a.truncateLogs()
	case "c":
		a.togglePane(paneCommand)
		a.focus = paneCommand
	case "/", "f":
		a.focus = paneLog
		a.logPane.queryInput.Focus()
	case "w":
		a.togglePane(paneEvents)
		a.focus = paneEvents
	case "l":
		a.togglePane(paneStreams)
		a.focus = paneStreams
	case "v":
		a.mode = modeFields
		a.fieldMenu.ensureVisible(len(a.fieldMenu.FilteredFields(orderedObservedFields(a.cfg.UI.LogView, a.buf.ObservedFields()))), a.fieldMenu.visibleRows(a.bodyHeight()))
	case "F":
		a.mode = modeFieldFilter
		a.filterMenu.ensureVisible(len(a.filterMenu.FilteredFields(a.buf.ObservedFields())), a.filterMenu.visibleRows(a.bodyHeight()))
	case "S":
		if a.cfg.UI.SplitDirection == "horizontal" {
			a.cfg.UI.SplitDirection = "vertical"
		} else {
			a.cfg.UI.SplitDirection = "horizontal"
		}
	case "enter":
		if a.focus == paneStreams {
			a.toggleSelectedStream()
			return a, nil
		}
		if a.focus == paneLog {
			if a.logPane.selecting {
				a.openLineDetail()
				return a, nil
			}
			if time.Since(a.lastEnter) <= 600*time.Millisecond {
				a.buf.Append("system", strings.Repeat("-", 48))
			}
			a.lastEnter = time.Now()
			a.moveLogToTail()
			return a, nil
		}
		if a.focus == paneCommand {
			if command, ok := a.commandPane.SelectedCommand(); ok {
				a.activeCmd = command.ID
				a.cfg.DefaultCommand = command.ID
				return a, a.restartActive()
			}
		}
	case "o":
		if a.focus == paneStreams {
			a.openSelectedStreamDetail()
		}
	case "n":
		lines := a.snapshotLines()
		a.logPane.jumpMatch(1, lines)
		a.syncLogViewport(lines)
	case "N":
		lines := a.snapshotLines()
		a.logPane.jumpMatch(-1, lines)
		a.syncLogViewport(lines)
	case "[", "left":
		a.switchPreset(-1)
	case "]", "right":
		a.switchPreset(1)
	case "g":
		a.editor.Open(a.cfg)
		a.mode = modeConfig
	}
	return a, nil
}

func (a *App) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "?", "q":
		a.mode = modeMain
	}
	return a, nil
}

func (a *App) handleFieldKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	fields := orderedObservedFields(a.cfg.UI.LogView, a.buf.ObservedFields())
	filtered := a.fieldMenu.FilteredFields(fields)
	a.fieldMenu.ensureVisible(len(filtered), a.fieldMenu.visibleRows(a.bodyHeight()))

	switch msg.String() {
	case "esc":
		a.mode = modeMain
		return a, nil
	case "backspace":
		a.fieldMenu.Backspace()
	case "ctrl+u":
		a.fieldMenu.ClearFilter()
	case "up", "k":
		a.fieldMenu.Move(-1, len(filtered), a.fieldMenu.visibleRows(a.bodyHeight()))
	case "down", "j":
		a.fieldMenu.Move(1, len(filtered), a.fieldMenu.visibleRows(a.bodyHeight()))
	case " ", "enter":
		field, ok := a.fieldMenu.CurrentField(fields)
		if !ok {
			return a, nil
		}
		a.toggleHiddenField(field)
	default:
		if msg.Type == tea.KeyRunes {
			a.fieldMenu.Type(string(msg.Runes))
		}
	}
	return a, nil
}

func (a *App) handleFieldFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	fields := a.buf.ObservedFields()
	filtered := a.filterMenu.FilteredFields(fields)
	a.filterMenu.ensureVisible(len(filtered), a.filterMenu.visibleRows(a.bodyHeight()))

	if a.filterMenu.Editing() {
		switch msg.String() {
		case "esc":
			a.filterMenu.StopEditing()
			return a, nil
		case "enter":
			a.filterMenu.StopEditing()
			return a, nil
		case "backspace":
			a.backspaceFieldFilterValue(a.filterMenu.editingField)
		case "ctrl+u":
			a.setFieldFilter(a.filterMenu.editingField, "")
		default:
			if msg.Type == tea.KeyRunes {
				a.appendFieldFilterValue(a.filterMenu.editingField, string(msg.Runes))
			}
		}
		a.resetLogWindow()
		a.logPane.refreshQueryState(a.logPane.autoScroll, a.snapshotLines())
		return a, nil
	}

	switch msg.String() {
	case "esc":
		a.mode = modeMain
		return a, nil
	case "backspace":
		a.filterMenu.BackspaceFieldFilter()
	case "ctrl+u":
		a.filterMenu.ClearFieldFilter()
	case "up", "k":
		a.filterMenu.Move(-1, len(filtered), a.filterMenu.visibleRows(a.bodyHeight()))
	case "down", "j":
		a.filterMenu.Move(1, len(filtered), a.filterMenu.visibleRows(a.bodyHeight()))
	case "enter":
		field, ok := a.filterMenu.CurrentField(fields)
		if !ok {
			return a, nil
		}
		a.filterMenu.StartEditing(field)
	default:
		if msg.Type == tea.KeyRunes {
			a.filterMenu.TypeFieldFilter(string(msg.Runes))
		}
	}
	return a, nil
}

func (a *App) handleConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := a.editor.rows()
	if len(rows) == 0 {
		return a, nil
	}
	if a.editor.selected >= len(rows) {
		a.editor.selected = len(rows) - 1
	}

	if a.editor.editing {
		switch msg.String() {
		case "esc":
			a.stopEditing()
			return a, nil
		case "enter":
			a.applyEditorInput(rows[a.editor.selected])
			a.stopEditing()
			return a, nil
		}
		var cmd tea.Cmd
		a.editor.input, cmd = a.editor.input.Update(msg)
		return a, cmd
	}

	switch msg.String() {
	case "q":
		return a, a.quitCmd()
	case "esc":
		a.mode = modeMain
	case "up", "k":
		a.editor.selected = max(0, a.editor.selected-1)
	case "down", "j":
		a.editor.selected = min(len(rows)-1, a.editor.selected+1)
	case "pgup", "ctrl+u":
		a.editor.selected = max(0, a.editor.selected-max(1, a.logPageSize()/2))
	case "pgdown", "ctrl+d":
		a.editor.selected = min(len(rows)-1, a.editor.selected+max(1, a.logPageSize()/2))
	case "enter", " ":
		return a.activateEditorRow(rows[a.editor.selected])
	case "e":
		return a.startEditingRow(rows[a.editor.selected])
	case "a":
		return a.addEditorItem(rows[a.editor.selected])
	case "d":
		return a.deleteEditorItem(rows[a.editor.selected])
	case "y":
		return a.duplicatePreset()
	case "[", "left":
		a.switchDraftPreset(-1)
	case "]", "right":
		a.switchDraftPreset(1)
	}
	return a, nil
}

func (a *App) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return a, a.quitCmd()
	case "esc", "enter":
		a.mode = modeMain
	case "up", "k":
		a.detail.scroll = max(0, a.detail.scroll-1)
	case "down", "j":
		a.detail.scroll++
	case "pgup", "ctrl+u":
		a.detail.scroll = max(0, a.detail.scroll-max(1, a.logPageSize()/2))
	case "pgdown", "ctrl+d":
		a.detail.scroll += max(1, a.logPageSize()/2)
	case "home", "g":
		a.detail.scroll = 0
	}
	return a, nil
}

func (a *App) handleStreamDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return a, a.quitCmd()
	case "esc", "enter":
		a.mode = modeMain
	}
	return a, nil
}

func (a *App) handleRunner(ev runner.Event) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case runner.EventStarted:
		a.flushPendingOutput()
		a.processStatus = fmt.Sprintf("running pid=%d", ev.PID)
		a.watchStatus = "clean"
		a.stale = false
		a.statusDetail = ""
	case runner.EventOutput:
		a.pendingOutput = append(a.pendingOutput, ev)
		cmds := []tea.Cmd{waitRunnerEvent(a.runner)}
		if !a.outputFlushScheduled {
			a.outputFlushScheduled = true
			cmds = append(cmds, outputFlushCmd())
		}
		return a, tea.Batch(cmds...)
	case runner.EventExited:
		a.flushPendingOutput()
		if ev.Err != nil {
			a.processStatus = fmt.Sprintf("exited (%d)", ev.Code)
			a.buf.Append("system", fmt.Sprintf("process exited with code %d", ev.Code))
		} else {
			a.processStatus = "completed"
			a.buf.Append("system", "process completed successfully")
		}
		lines := a.snapshotLines()
		a.logPane.refreshQueryState(a.logPane.autoScroll, lines)
		a.syncLogViewport(lines)
	case runner.EventError:
		a.flushPendingOutput()
		a.buf.Append("system", "runner error: "+ev.Err.Error())
		lines := a.snapshotLines()
		a.logPane.refreshQueryState(a.logPane.autoScroll, lines)
		a.syncLogViewport(lines)
	}
	return a, waitRunnerEvent(a.runner)
}

func (a *App) handleStream(ev stream.Event) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case stream.EventStarted:
		a.eventsPane.Append(fmt.Sprintf("stream %s started pid=%d", ev.StreamID, ev.PID))
	case stream.EventOutput:
		a.streamLines[ev.StreamID] = appendTrimmed(a.streamLines[ev.StreamID], ev.Text, 1000)
		a.buf.Append(ev.Source, ev.Text)
		if a.logPane.autoScroll {
			a.resetLogWindow()
		}
		lines := a.snapshotLines()
		a.logPane.refreshQueryState(a.logPane.autoScroll, lines)
		a.syncLogViewport(lines)
	case stream.EventExited:
		if ev.Err != nil {
			a.eventsPane.Append(fmt.Sprintf("stream %s exited with code %d", ev.StreamID, ev.Code))
		} else {
			a.eventsPane.Append("stream " + ev.StreamID + " completed")
		}
	case stream.EventError:
		if ev.Err != nil {
			a.eventsPane.Append("stream " + ev.StreamID + ": " + ev.Err.Error())
		}
	}
	return a, waitStreamEvent(a.streams)
}
