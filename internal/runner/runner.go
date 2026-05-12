package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/stackriot/iwatch/internal/detect"
)

type EventType string

const (
	EventStarted EventType = "started"
	EventOutput  EventType = "output"
	EventExited  EventType = "exited"
	EventError   EventType = "error"
)

type Event struct {
	Type   EventType
	Source string
	Text   string
	Err    error
	Code   int
	PID    int
}

type Runner struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	running bool
	events  chan Event
	done    chan struct{}
}

func New() *Runner {
	return &Runner{
		events: make(chan Event, 256),
		done:   make(chan struct{}),
	}
}

func (r *Runner) Events() <-chan Event {
	return r.events
}

func (r *Runner) Start(command detect.Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return errors.New("runner already active")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "sh", "-lc", command.Cmd)
	cmd.Dir = command.CWD
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start command: %w", err)
	}

	r.cmd = cmd
	r.cancel = cancel
	r.running = true
	r.events <- Event{Type: EventStarted, PID: cmd.Process.Pid, Text: command.Cmd}

	go r.stream("stdout", stdout)
	go r.stream("stderr", stderr)
	go r.wait()
	return nil
}

func (r *Runner) Restart(command detect.Command) error {
	if err := r.Stop(2 * time.Second); err != nil {
		return err
	}
	return r.Start(command)
}

func (r *Runner) Stop(timeout time.Duration) error {
	r.mu.Lock()
	if !r.running || r.cmd == nil {
		r.mu.Unlock()
		return nil
	}
	cmd := r.cmd
	cancel := r.cancel
	done := r.done
	r.mu.Unlock()

	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-done:
		if cancel != nil {
			cancel()
		}
		return nil
	case <-time.After(timeout):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if cancel != nil {
			cancel()
		}
		<-done
		return nil
	}
}

func (r *Runner) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

func (r *Runner) stream(source string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		r.events <- Event{Type: EventOutput, Source: source, Text: scanner.Text()}
	}
	if err := scanner.Err(); err != nil {
		r.events <- Event{Type: EventError, Source: source, Err: err}
	}
}

func (r *Runner) wait() {
	r.mu.Lock()
	cmd := r.cmd
	r.mu.Unlock()

	err := cmd.Wait()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		}
		r.events <- Event{Type: EventExited, Err: err, Code: code}
	} else {
		r.events <- Event{Type: EventExited, Code: 0}
	}

	r.mu.Lock()
	r.running = false
	r.cmd = nil
	r.cancel = nil
	close(r.done)
	r.done = make(chan struct{})
	r.mu.Unlock()
}
