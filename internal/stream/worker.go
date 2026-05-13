package stream

import "context"

// worker owns the cancellation hook for one running stream.
type worker struct {
	cancel context.CancelFunc
}
