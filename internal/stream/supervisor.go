package stream

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/stackriot/iwatch/internal/config"
)

// Supervisor starts, stops, and reports configured additional log streams.
type Supervisor struct {
	mu       sync.Mutex
	baseDir  string
	configs  map[string]config.StreamConfig
	order    []string
	active   map[string]struct{}
	workers  map[string]worker
	manual   map[string]struct{}
	errors   map[string]string
	events   chan Event
	stopping bool
}

// New creates a supervisor for the provided stream configuration.
func New(streams []config.StreamConfig, baseDir string) *Supervisor {
	supervisor := &Supervisor{
		baseDir: baseDir,
		configs: make(map[string]config.StreamConfig, len(streams)),
		active:  make(map[string]struct{}),
		workers: make(map[string]worker),
		manual:  make(map[string]struct{}),
		errors:  make(map[string]string),
		events:  make(chan Event, 256),
	}
	for _, stream := range streams {
		if stream.ID == "" {
			continue
		}
		supervisor.configs[stream.ID] = stream
		supervisor.order = append(supervisor.order, stream.ID)
	}
	return supervisor
}

// Events returns the merged stream event channel.
func (s *Supervisor) Events() <-chan Event {
	return s.events
}

// Configure replaces the known stream definitions and stops removed streams.
func (s *Supervisor) Configure(streams []config.StreamConfig) {
	nextConfigs := make(map[string]config.StreamConfig, len(streams))
	nextOrder := make([]string, 0, len(streams))
	for _, stream := range streams {
		if stream.ID == "" {
			continue
		}
		nextConfigs[stream.ID] = stream
		nextOrder = append(nextOrder, stream.ID)
	}

	var stop []string
	s.mu.Lock()
	for id := range s.workers {
		nextConfig, ok := nextConfigs[id]
		if !ok || !reflect.DeepEqual(s.configs[id], nextConfig) {
			stop = append(stop, id)
		}
	}
	s.configs = nextConfigs
	s.order = nextOrder
	s.mu.Unlock()

	for _, id := range stop {
		_ = s.Stop(id)
	}
}

// Apply starts and stops streams for the current active preset.
func (s *Supervisor) Apply(streamIDs []string) {
	active := make(map[string]struct{}, len(streamIDs))
	for _, id := range streamIDs {
		if _, ok := s.configs[id]; ok {
			active[id] = struct{}{}
		}
	}

	var start []string
	var stop []string
	s.mu.Lock()
	s.active = active
	for id := range s.workers {
		if _, ok := active[id]; !ok {
			stop = append(stop, id)
			delete(s.manual, id)
		}
	}
	for id := range active {
		cfg := s.configs[id]
		if _, running := s.workers[id]; running || !enabled(cfg) || !autoStart(cfg) {
			continue
		}
		start = append(start, id)
	}
	s.mu.Unlock()

	for _, id := range stop {
		_ = s.Stop(id)
	}
	for _, id := range start {
		_ = s.Start(id)
	}
}

// Start launches one active configured stream.
func (s *Supervisor) Start(id string) error {
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		return errors.New("stream supervisor stopping")
	}
	cfg, ok := s.configs[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("unknown stream %q", id)
	}
	if !enabled(cfg) {
		s.mu.Unlock()
		return fmt.Errorf("stream %q is disabled", id)
	}
	if _, ok := s.active[id]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("stream %q is not active in the current preset", id)
	}
	if _, running := s.workers[id]; running {
		s.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.workers[id] = worker{cancel: cancel}
	s.manual[id] = struct{}{}
	delete(s.errors, id)
	s.mu.Unlock()

	switch cfg.Type {
	case "process":
		go s.runProcess(ctx, cfg)
	default:
		go s.runFile(ctx, cfg)
	}
	return nil
}

// Stop cancels one running stream.
func (s *Supervisor) Stop(id string) error {
	s.mu.Lock()
	running, ok := s.workers[id]
	if ok {
		delete(s.workers, id)
	}
	delete(s.manual, id)
	s.mu.Unlock()
	if ok {
		running.cancel()
	}
	return nil
}

// StopAll cancels every running stream.
func (s *Supervisor) StopAll() {
	s.mu.Lock()
	s.stopping = true
	var workers []worker
	for id, running := range s.workers {
		workers = append(workers, running)
		delete(s.workers, id)
	}
	s.mu.Unlock()
	for _, running := range workers {
		running.cancel()
	}
}

// Statuses returns configured streams in config order with current runtime state.
func (s *Supervisor) Statuses() []Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	statuses := make([]Status, 0, len(s.order))
	for _, id := range s.order {
		cfg := s.configs[id]
		_, active := s.active[id]
		_, running := s.workers[id]
		statuses = append(statuses, Status{
			ID:       id,
			Title:    cfg.Title,
			Type:     cfg.Type,
			Active:   active,
			Running:  running,
			OnDemand: !autoStart(cfg),
			Error:    s.errors[id],
		})
	}
	return statuses
}

// ActiveCount returns the number of currently running streams.
func (s *Supervisor) ActiveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.workers)
}

func (s *Supervisor) runFile(ctx context.Context, cfg config.StreamConfig) {
	defer s.markStopped(cfg.ID)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	states := map[string]fileState{}
	source := s.resolvePath(cfg.Source)
	for {
		if err := s.pollFiles(ctx, cfg, source, states); err != nil {
			s.recordError(cfg.ID, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Supervisor) pollFiles(ctx context.Context, cfg config.StreamConfig, source string, states map[string]fileState) error {
	paths, err := filepath.Glob(source)
	if err != nil {
		return fmt.Errorf("glob %s: %w", source, err)
	}
	if len(paths) == 0 {
		if _, err := os.Stat(source); err == nil {
			paths = []string{source}
		}
	}
	sort.Strings(paths)

	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		seen[path] = struct{}{}
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				delete(states, path)
				continue
			}
			return fmt.Errorf("stat %s: %w", path, err)
		}
		if info.IsDir() {
			continue
		}
		state, ok := states[path]
		if !ok || info.Size() < state.offset {
			states[path] = fileState{offset: info.Size()}
			continue
		}
		if info.Size() == state.offset {
			continue
		}
		next, err := s.readNewLines(ctx, cfg, path, state.offset)
		if err != nil {
			return err
		}
		states[path] = fileState{offset: next}
	}
	for path := range states {
		if _, ok := seen[path]; !ok {
			delete(states, path)
		}
	}
	return nil
}

func (s *Supervisor) readNewLines(ctx context.Context, cfg config.StreamConfig, path string, offset int64) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return offset, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return offset, fmt.Errorf("seek %s: %w", path, err)
	}
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			select {
			case <-ctx.Done():
				return offset, nil
			case s.events <- Event{Type: EventOutput, StreamID: cfg.ID, Source: cfg.ID, Text: strings.TrimRight(line, "\r\n")}:
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			next, seekErr := file.Seek(0, io.SeekCurrent)
			if seekErr != nil {
				return offset, fmt.Errorf("tell %s: %w", path, seekErr)
			}
			return next, nil
		}
		return offset, fmt.Errorf("read %s: %w", path, err)
	}
}

func (s *Supervisor) runProcess(ctx context.Context, cfg config.StreamConfig) {
	defer s.markStopped(cfg.ID)
	cmd := exec.CommandContext(ctx, "sh", "-lc", cfg.Cmd)
	cmd.Dir = s.resolvePath(cfg.CWD)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.recordError(cfg.ID, fmt.Errorf("stdout pipe: %w", err))
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.recordError(cfg.ID, fmt.Errorf("stderr pipe: %w", err))
		return
	}
	if err := cmd.Start(); err != nil {
		s.recordError(cfg.ID, fmt.Errorf("start stream: %w", err))
		return
	}
	s.events <- Event{Type: EventStarted, StreamID: cfg.ID, Source: cfg.ID, Text: cfg.Cmd, PID: cmd.Process.Pid}
	go terminateProcessGroup(ctx, cmd.Process.Pid)

	go s.streamReader(ctx, cfg.ID, cfg.ID+":stdout", stdout)
	go s.streamReader(ctx, cfg.ID, cfg.ID+":stderr", stderr)

	err = cmd.Wait()
	if ctx.Err() != nil {
		return
	}
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		}
		s.events <- Event{Type: EventExited, StreamID: cfg.ID, Source: cfg.ID, Err: err, Code: code}
		return
	}
	s.events <- Event{Type: EventExited, StreamID: cfg.ID, Source: cfg.ID, Code: code}
}

func (s *Supervisor) streamReader(ctx context.Context, streamID string, source string, reader io.Reader) {
	bufferedReader := bufio.NewReader(reader)
	for {
		line, err := bufferedReader.ReadString('\n')
		if len(line) > 0 {
			select {
			case <-ctx.Done():
				return
			case s.events <- Event{Type: EventOutput, StreamID: streamID, Source: source, Text: strings.TrimRight(line, "\r\n")}:
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) || errors.Is(err, fs.ErrClosed) {
			return
		}
		s.recordError(streamID, err)
		return
	}
}

func (s *Supervisor) markStopped(id string) {
	s.mu.Lock()
	delete(s.workers, id)
	s.mu.Unlock()
}

func (s *Supervisor) recordError(id string, err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.errors[id] = err.Error()
	s.mu.Unlock()
	select {
	case s.events <- Event{Type: EventError, StreamID: id, Source: id, Err: err}:
	default:
	}
}

func (s *Supervisor) resolvePath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.baseDir, path)
}

func enabled(cfg config.StreamConfig) bool {
	return cfg.Enabled == nil || *cfg.Enabled
}

func autoStart(cfg config.StreamConfig) bool {
	return cfg.AutoStart == nil || *cfg.AutoStart
}

func terminateProcessGroup(ctx context.Context, pid int) {
	<-ctx.Done()
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	time.Sleep(time.Second)
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
