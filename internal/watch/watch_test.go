package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherEmitsFileChange(t *testing.T) {
	root := t.TempDir()
	watcher, err := New(root)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Run(ctx, 10*time.Millisecond)

	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	select {
	case event := <-watcher.Events():
		if event.Path != path {
			t.Fatalf("event path = %q", event.Path)
		}
	case err := <-watcher.Errors():
		t.Fatalf("watcher error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher event")
	}
}

func TestWatcherClosesChannelsOnCancel(t *testing.T) {
	root := t.TempDir()
	watcher, err := New(root)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watcher.Run(ctx, 10*time.Millisecond)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher shutdown")
	}

	select {
	case _, ok := <-watcher.Events():
		if ok {
			t.Fatal("expected events channel to be closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for events channel to close")
	}
}

func TestWatcherIgnoresGitIgnoreMatches(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored.txt\nignored-dir/\n")

	watcher, err := New(root)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Run(ctx, 10*time.Millisecond)

	writeFile(t, filepath.Join(root, "ignored.txt"), "hello")
	mustNotReceiveEvent(t, watcher, 250*time.Millisecond)

	writeFile(t, filepath.Join(root, "tracked.txt"), "hello")
	mustReceiveEventForPath(t, watcher, filepath.Join(root, "tracked.txt"))

	if err := os.MkdirAll(filepath.Join(root, "ignored-dir"), 0o755); err != nil {
		t.Fatalf("mkdir ignored dir: %v", err)
	}
	writeFile(t, filepath.Join(root, "ignored-dir", "nested.txt"), "hello")
	mustNotReceiveEvent(t, watcher, 250*time.Millisecond)
}

func TestWatcherIgnoresIWatchIgnoreMatches(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".iwatchignore"), "*.tmp\n")

	watcher, err := New(root)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Run(ctx, 10*time.Millisecond)

	writeFile(t, filepath.Join(root, "build.tmp"), "hello")
	mustNotReceiveEvent(t, watcher, 250*time.Millisecond)

	writeFile(t, filepath.Join(root, "build.log"), "hello")
	mustReceiveEventForPath(t, watcher, filepath.Join(root, "build.log"))
}

func TestWatcherReloadsIgnoreFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".iwatchignore"), "")

	watcher, err := New(root)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Run(ctx, 10*time.Millisecond)

	writeFile(t, filepath.Join(root, ".iwatchignore"), "ignored-after-reload.txt\n")
	mustNotReceiveEvent(t, watcher, 250*time.Millisecond)

	writeFile(t, filepath.Join(root, "ignored-after-reload.txt"), "hello")
	mustNotReceiveEvent(t, watcher, 250*time.Millisecond)

	writeFile(t, filepath.Join(root, "tracked-after-reload.txt"), "hello")
	mustReceiveEventForPath(t, watcher, filepath.Join(root, "tracked-after-reload.txt"))
}

func mustReceiveEventForPath(t *testing.T, watcher *Watcher, path string) {
	t.Helper()

	for {
		select {
		case event := <-watcher.Events():
			if event.Path == path {
				return
			}
		case err := <-watcher.Errors():
			t.Fatalf("watcher error: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for watcher event %q", path)
		}
	}
}

func mustNotReceiveEvent(t *testing.T, watcher *Watcher, timeout time.Duration) {
	t.Helper()

	select {
	case event := <-watcher.Events():
		t.Fatalf("unexpected watcher event for %q", event.Path)
	case err := <-watcher.Errors():
		t.Fatalf("watcher error: %v", err)
	case <-time.After(timeout):
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
