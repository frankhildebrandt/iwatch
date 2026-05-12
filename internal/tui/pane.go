package tui

// Pane defines the shared behavior each visible pane exposes to the app shell.
type Pane interface {
	ID() paneID
	IsOpen() bool
	SetOpen(bool)
}
