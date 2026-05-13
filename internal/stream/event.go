package stream

// Event describes one lifecycle, output, or error event from an additional stream.
type Event struct {
	Type     EventType
	StreamID string
	Source   string
	Text     string
	Err      error
	Code     int
	PID      int
}
