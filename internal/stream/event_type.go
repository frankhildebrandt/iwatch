package stream

// EventType identifies the kind of event emitted by a stream.
type EventType string

const (
	EventStarted EventType = "started"
	EventOutput  EventType = "output"
	EventExited  EventType = "exited"
	EventError   EventType = "error"
)
