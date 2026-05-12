package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/stackriot/iwatch/internal/detect"
)

// EventType identifies the kind of process event emitted by Runner.
type EventType string

const (
	EventStarted EventType = "started"
	EventOutput  EventType = "output"
	EventExited  EventType = "exited"
	EventError   EventType = "error"
)

// Event describes one lifecycle or output event from the running command.
type Event struct {
	Type   EventType
	Source string
	Text   string
	Err    error
	Code   int
	PID    int
}

// Runner manages starting, stopping, and streaming a single command at a time.
type Runner struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	running bool
	events  chan Event
	done    chan struct{}
}

// New creates an idle runner.
func New() *Runner {
	return &Runner{
		events: make(chan Event, 256),
		done:   make(chan struct{}),
	}
}

// Events returns the runner event stream.
func (r *Runner) Events() <-chan Event {
	return r.events
}

// Start launches the provided command.
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

// Restart stops any active command before starting the next one.
func (r *Runner) Restart(command detect.Command) error {
	if err := r.Stop(2 * time.Second); err != nil {
		return err
	}
	return r.Start(command)
}

// Stop requests process termination and escalates to SIGKILL after the timeout.
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

// Running reports whether a command is currently active.
func (r *Runner) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

func (r *Runner) stream(source string, reader io.Reader) {
	bufferedReader := bufio.NewReader(reader)
	for {
		line, err := bufferedReader.ReadString('\n')
		if len(line) > 0 {
			r.events <- Event{
				Type:   EventOutput,
				Source: source,
				Text:   strings.TrimRight(line, "\r\n"),
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) || errors.Is(err, fs.ErrClosed) {
			return
		}
		r.events <- Event{Type: EventError, Source: source, Err: err}
		return
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
