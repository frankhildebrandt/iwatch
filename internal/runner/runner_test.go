package runner

import (
	"strings"
	"testing"
	"time"

	"github.com/stackriot/iwatch/internal/detect"
)

func TestRunnerStartStopLifecycle(t *testing.T) {
	run := New()
	command := detect.Command{ID: "test", Title: "test", Cmd: "printf 'hello\\n'; sleep 0.05", CWD: t.TempDir()}

	if err := run.Start(command); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !run.Running() {
		t.Fatal("expected runner to be active")
	}

	events := collectEvents(t, run, 3, 2*time.Second)
	if events[0].Type != EventStarted {
		t.Fatalf("first event = %s", events[0].Type)
	}
	if events[1].Type != EventOutput || events[1].Text != "hello" {
		t.Fatalf("unexpected output event: %#v", events[1])
	}
	if events[2].Type != EventExited || events[2].Code != 0 {
		t.Fatalf("unexpected exit event: %#v", events[2])
	}
	if run.Running() {
		t.Fatal("expected runner to be idle after exit")
	}
}

func TestRunnerRestartReplacesActiveCommand(t *testing.T) {
	run := New()
	cwd := t.TempDir()
	first := detect.Command{ID: "first", Title: "first", Cmd: "sleep 1", CWD: cwd}
	second := detect.Command{ID: "second", Title: "second", Cmd: "printf 'second\\n'", CWD: cwd}

	if err := run.Start(first); err != nil {
		t.Fatalf("start first: %v", err)
	}
	started := collectEvents(t, run, 1, 2*time.Second)
	if started[0].Type != EventStarted {
		t.Fatalf("expected first start event, got %#v", started[0])
	}
	if err := run.Restart(second); err != nil {
		t.Fatalf("restart: %v", err)
	}

	events := collectEvents(t, run, 4, 2*time.Second)
	foundSecond := false
	foundSecondStart := false
	for _, event := range events {
		if event.Type == EventStarted && event.Text == second.Cmd {
			foundSecondStart = true
		}
		if event.Type == EventOutput && event.Text == "second" {
			foundSecond = true
		}
	}
	if !foundSecondStart || !foundSecond {
		t.Fatalf("expected second command output, events=%#v", events)
	}
}

func TestRunnerStreamsLongOutputLine(t *testing.T) {
	run := New()
	command := detect.Command{
		ID:    "long-line",
		Title: "long-line",
		Cmd:   "head -c 70000 /dev/zero | tr '\\000' 'a'; printf '\\n'",
		CWD:   t.TempDir(),
	}

	if err := run.Start(command); err != nil {
		t.Fatalf("start: %v", err)
	}

	events := collectEvents(t, run, 3, 2*time.Second)
	if events[0].Type != EventStarted {
		t.Fatalf("first event = %s", events[0].Type)
	}
	if events[1].Type != EventOutput {
		t.Fatalf("unexpected output event: %#v", events[1])
	}
	if len(events[1].Text) != 70000 {
		t.Fatalf("output length = %d, want 70000", len(events[1].Text))
	}
	if events[1].Text != strings.Repeat("a", 70000) {
		t.Fatal("unexpected output content")
	}
	if events[2].Type != EventExited || events[2].Code != 0 {
		t.Fatalf("unexpected exit event: %#v", events[2])
	}
}

func collectEvents(t *testing.T, run *Runner, count int, timeout time.Duration) []Event {
	t.Helper()
	events := make([]Event, 0, count)
	deadline := time.After(timeout)
	for len(events) < count {
		select {
		case event := <-run.Events():
			events = append(events, event)
		case <-deadline:
			t.Fatalf("timed out waiting for %d events, got %#v", count, events)
		}
	}
	return events
}
