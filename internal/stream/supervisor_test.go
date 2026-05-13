package stream

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stackriot/iwatch/internal/config"
)

func TestFileStreamReadsOnlyNewLinesAfterStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	supervisor := New([]config.StreamConfig{{ID: "app", Title: "App", Type: "file", Source: path}}, t.TempDir())
	defer supervisor.StopAll()
	supervisor.Apply([]string{"app"})
	time.Sleep(300 * time.Millisecond)

	if err := appendFile(path, "new\n"); err != nil {
		t.Fatal(err)
	}

	event := waitEvent(t, supervisor, EventOutput)
	if event.Text != "new" {
		t.Fatalf("event text = %q, want new", event.Text)
	}
}

func TestFileStreamSurvivesCleanupAndReadsFutureLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	supervisor := New([]config.StreamConfig{{ID: "app", Title: "App", Type: "file", Source: path}}, t.TempDir())
	defer supervisor.StopAll()
	supervisor.Apply([]string{"app"})
	time.Sleep(300 * time.Millisecond)

	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if err := appendFile(path, "after-cleanup\n"); err != nil {
		t.Fatal(err)
	}

	event := waitEvent(t, supervisor, EventOutput)
	if event.Text != "after-cleanup" {
		t.Fatalf("event text = %q, want after-cleanup", event.Text)
	}
}

func TestProcessStreamEmitsOutputAndExit(t *testing.T) {
	supervisor := New([]config.StreamConfig{{ID: "proc", Title: "Proc", Type: "process", Cmd: "printf 'hello\\n'"}}, t.TempDir())
	defer supervisor.StopAll()
	supervisor.Apply([]string{"proc"})

	output := waitEvent(t, supervisor, EventOutput)
	if output.StreamID != "proc" || output.Text != "hello" {
		t.Fatalf("unexpected output: %#v", output)
	}
	exited := waitEvent(t, supervisor, EventExited)
	if exited.Code != 0 {
		t.Fatalf("exit code = %d, want 0", exited.Code)
	}
}

func waitEvent(t *testing.T, supervisor *Supervisor, eventType EventType) Event {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case event := <-supervisor.Events():
			if event.Type == eventType {
				return event
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", eventType)
		}
	}
}

func appendFile(path string, value string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(value)
	return err
}
