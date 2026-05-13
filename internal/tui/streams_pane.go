package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/stackriot/iwatch/internal/stream"
)

// StreamsPane owns the additional log stream list and selection UI.
type StreamsPane struct {
	open   bool
	cursor int
}

// NewStreamsPane creates the additional stream pane.
func NewStreamsPane(openPanes []string) *StreamsPane {
	pane := &StreamsPane{}
	for _, paneName := range openPanes {
		if paneID(paneName) == paneStreams {
			pane.open = true
			break
		}
	}
	return pane
}

// ID returns the pane identifier.
func (p *StreamsPane) ID() paneID {
	return paneStreams
}

// IsOpen reports whether the pane is visible.
func (p *StreamsPane) IsOpen() bool {
	return p.open
}

// SetOpen toggles the pane visibility.
func (p *StreamsPane) SetOpen(open bool) {
	p.open = open
}

// Move changes the selected stream row.
func (p *StreamsPane) Move(delta int, count int) {
	if count <= 0 {
		p.cursor = 0
		return
	}
	p.cursor += delta
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= count {
		p.cursor = count - 1
	}
}

// Selected returns the selected stream status.
func (p *StreamsPane) Selected(statuses []stream.Status) (stream.Status, bool) {
	if len(statuses) == 0 || p.cursor < 0 || p.cursor >= len(statuses) {
		return stream.Status{}, false
	}
	return statuses[p.cursor], true
}

// View renders the stream pane.
func (p *StreamsPane) View(width, height int, focused bool, statuses []stream.Status) string {
	if p.cursor >= len(statuses) {
		p.cursor = max(0, len(statuses)-1)
	}
	lines := []string{lipgloss.NewStyle().Bold(true).Render("Streams")}
	if len(statuses) == 0 {
		lines = append(lines, "No streams configured.")
	} else {
		for idx, status := range statuses {
			marker := " "
			if idx == p.cursor {
				marker = ">"
			}
			state := "inactive"
			if status.Active {
				state = "active"
			}
			if status.Running {
				state = "running"
			}
			if status.OnDemand && status.Active && !status.Running {
				state = "ready"
			}
			if status.Error != "" {
				state = "error"
			}
			line := fmt.Sprintf("%s %s [%s] %s", marker, status.Title, status.Type, state)
			if status.Error != "" {
				line += " " + status.Error
			}
			lines = append(lines, truncateWithEllipsis(line, max(10, width-4)))
		}
	}
	lines = append(lines, "", "[enter] start/stop [o] modal")
	return paneStyle(focused, width, max(5, height/3)).Render(strings.Join(lines, "\n"))
}
