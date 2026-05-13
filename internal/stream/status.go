package stream

// Status describes the visible runtime state of a configured stream.
type Status struct {
	ID       string
	Title    string
	Type     string
	Active   bool
	Running  bool
	OnDemand bool
	Error    string
}
