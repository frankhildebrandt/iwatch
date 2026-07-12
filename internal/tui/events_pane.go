package tui

import (
	"strings"
)

// EventsPane keeps recent system event lines.
type EventsPane struct {
	open   bool
	events []string
}

// NewEventsPane creates the system events pane.
func NewEventsPane(openPanes []string) *EventsPane {
	pane := &EventsPane{}
	for _, paneName := range openPanes {
		if paneID(paneName) == paneEvents {
			pane.open = true
			break
		}
	}
	return pane
}

// ID returns the pane identifier.
func (p *EventsPane) ID() paneID {
	return paneEvents
}

// IsOpen reports whether the pane is visible.
func (p *EventsPane) IsOpen() bool {
	return p.open
}

// SetOpen toggles the pane visibility.
func (p *EventsPane) SetOpen(open bool) {
	p.open = open
}

// Append adds a new event line while trimming the buffer.
func (p *EventsPane) Append(value string) {
	p.events = appendTrimmed(p.events, value, 200)
}

// View renders the system events pane.
func (p *EventsPane) View(width, height int, focused bool) string {
	events := strings.Join(lastN(p.events, max(3, height/3-2)), "\n")
	if events == "" {
		events = "No system events yet."
	}
	return paneStyle(focused, width, max(5, height/3)).Render("Events\n" + events)
}
