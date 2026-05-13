package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/stackriot/iwatch/internal/detect"
)

// CommandPane owns the command list and selection UI.
type CommandPane struct {
	open bool
	list list.Model
}

// NewCommandPane builds the command list pane.
func NewCommandPane(commands []detect.Command, openPanes []string) *CommandPane {
	items := make([]list.Item, 0, len(commands))
	for _, cmd := range commands {
		items = append(items, commandItem{command: cmd})
	}

	commandList := list.New(items, commandDelegate{}, 30, 10)
	commandList.Title = "Commands [enter] panel [o] stream"
	commandList.SetShowHelp(false)
	commandList.SetFilteringEnabled(true)

	pane := &CommandPane{list: commandList}
	for _, paneName := range openPanes {
		if paneID(paneName) == paneCommand {
			pane.open = true
			break
		}
	}
	return pane
}

// ID returns the pane identifier.
func (p *CommandPane) ID() paneID {
	return paneCommand
}

// IsOpen reports whether the pane is visible.
func (p *CommandPane) IsOpen() bool {
	return p.open
}

// SetOpen toggles the pane visibility.
func (p *CommandPane) SetOpen(open bool) {
	p.open = open
}

// Update forwards Bubble Tea messages to the list model.
func (p *CommandPane) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return cmd
}

// View renders the pane with the current dimensions.
func (p *CommandPane) View(width, height int, focused bool) string {
	p.list.SetSize(width-2, max(5, height/3))
	return paneStyle(focused, width, max(5, height/3)).Render(p.list.View())
}

// SelectedCommand returns the currently highlighted command, if any.
func (p *CommandPane) SelectedCommand() (detect.Command, bool) {
	item, ok := p.list.SelectedItem().(commandItem)
	if !ok {
		return detect.Command{}, false
	}
	return item.command, true
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
