package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/stackriot/iwatch/internal/stream"
)

// CommandOutputPane renders output from a command launched into a dedicated side pane.
type CommandOutputPane struct {
	open     bool
	streamID string
	title    string
	scroll   int
}

// NewCommandOutputPane creates the command output pane state.
func NewCommandOutputPane(openPanes []string) *CommandOutputPane {
	pane := &CommandOutputPane{scroll: -1}
	for _, paneName := range openPanes {
		if paneID(paneName) == paneCommandOutput {
			pane.open = true
			break
		}
	}
	return pane
}

// ID returns the pane identifier.
func (p *CommandOutputPane) ID() paneID {
	return paneCommandOutput
}

// IsOpen reports whether the pane is visible.
func (p *CommandOutputPane) IsOpen() bool {
	return p.open
}

// SetOpen toggles the pane visibility.
func (p *CommandOutputPane) SetOpen(open bool) {
	p.open = open
	if !open {
		p.scroll = -1
	}
}

// SetCommand binds the pane to one launched command stream.
func (p *CommandOutputPane) SetCommand(streamID string, title string) {
	p.streamID = streamID
	p.title = title
	p.scroll = -1
}

// Clear removes the active command binding from the pane.
func (p *CommandOutputPane) Clear() {
	p.streamID = ""
	p.title = ""
	p.scroll = -1
}

// StreamID returns the bound stream identifier, if any.
func (p *CommandOutputPane) StreamID() string {
	return p.streamID
}

// Move scrolls the visible output region.
func (p *CommandOutputPane) Move(delta int, total int, visible int) {
	if total <= 0 || visible <= 0 {
		p.scroll = -1
		return
	}
	maxScroll := max(0, total-visible)
	if p.scroll < 0 {
		p.scroll = maxScroll
	}
	p.scroll += delta
	if p.scroll < 0 {
		p.scroll = 0
	}
	if p.scroll > maxScroll {
		p.scroll = maxScroll
	}
}

// View renders the pane with the latest output and runtime status.
func (p *CommandOutputPane) View(width, height int, focused bool, status stream.Status, lines []string) string {
	boxHeight := max(5, height/3)
	title := "Command Output"
	if p.title != "" {
		title += ": " + p.title
	}
	state := "idle"
	if status.ID != "" {
		state = "inactive"
		if status.Active {
			state = "active"
		}
		if status.Running {
			state = "running"
		}
		if status.Error != "" {
			state = "error: " + status.Error
		}
	}
	header := lipgloss.NewStyle().Bold(true).Render(title) + "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(state)

	if len(lines) == 0 {
		return paneStyle(focused, width, boxHeight).Render(header + "\n\nNo command output yet.")
	}

	visibleRows := max(1, boxHeight-4)
	maxScroll := max(0, len(lines)-visibleRows)
	if p.scroll < 0 {
		p.scroll = maxScroll
	}
	if p.scroll > maxScroll {
		p.scroll = maxScroll
	}
	start := p.scroll
	end := min(len(lines), start+visibleRows)
	content := strings.Join(lines[start:end], "\n")
	return paneStyle(focused, width, boxHeight).Render(header + "\n\n" + content)
}
