package runner

import (
	"bufio"
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
	running bool
	events  chan Event
	done    chan struct{}
	stop    chan time.Duration
	force   chan struct{}
}

// New creates an idle runner.
func New() *Runner {
	return &Runner{
		events: make(chan Event, 256),
		done:   make(chan struct{}),
		stop:   make(chan time.Duration, 1),
		force:  make(chan struct{}, 1),
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

	cmd := exec.Command("sh", "-lc", command.Cmd)
	cmd.Dir = command.CWD
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	r.cmd = cmd
	r.running = true
	r.events <- Event{Type: EventStarted, PID: cmd.Process.Pid, Text: command.Cmd}

	go r.stream("stdout", stdout)
	go r.stream("stderr", stderr)
	go r.monitorStop(cmd.Process.Pid)
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
	done, err := r.RequestStop(timeout)
	if err != nil {
		return err
	}
	if done == nil {
		return nil
	}
	<-done
	return nil
}

// RequestStop requests process termination and returns a channel that closes
// when the command has exited.
func (r *Runner) RequestStop(timeout time.Duration) (<-chan struct{}, error) {
	r.mu.Lock()
	if !r.running || r.cmd == nil {
		r.mu.Unlock()
		return nil, nil
	}
	done := r.done
	stop := r.stop
	r.mu.Unlock()

	select {
	case stop <- timeout:
	default:
	}
	return done, nil
}

// ForceStop immediately terminates the running command process group.
func (r *Runner) ForceStop() error {
	r.mu.Lock()
	running := r.running && r.cmd != nil
	force := r.force
	r.mu.Unlock()
	if !running {
		return nil
	}
	select {
	case force <- struct{}{}:
	default:
	}
	return nil
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
	close(r.done)
	r.done = make(chan struct{})
	r.stop = make(chan time.Duration, 1)
	r.force = make(chan struct{}, 1)
	r.mu.Unlock()
}

func (r *Runner) monitorStop(pid int) {
	select {
	case timeout := <-r.stop:
		_ = syscall.Kill(-pid, syscall.SIGTERM)
		select {
		case <-r.done:
		case <-r.force:
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		case <-time.After(timeout):
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	case <-r.force:
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	case <-r.done:
	}
}
