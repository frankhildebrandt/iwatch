package stream

import (
	"context"
	"time"
)

// worker owns the cancellation hook for one running stream.
type worker struct {
	cancel context.CancelFunc
	done   chan struct{}
	stop   chan time.Duration
	force  chan struct{}
}
