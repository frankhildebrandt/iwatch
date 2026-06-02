package watch

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Event describes one filesystem notification emitted by Watcher.
type Event struct {
	Path string
	Op   fsnotify.Op
}

// Watcher recursively watches a directory tree and debounces emitted events.
type Watcher struct {
	root    string
	events  chan Event
	errors  chan error
	fs      *fsnotify.Watcher
	ignores *ignoreMatcher
}

// New creates a recursive watcher rooted at the provided path.
func New(root string) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	ignores, err := newIgnoreMatcher(root)
	if err != nil {
		_ = fsWatcher.Close()
		return nil, fmt.Errorf("load ignore rules: %w", err)
	}

	w := &Watcher{
		root:    root,
		events:  make(chan Event, 128),
		errors:  make(chan error, 32),
		fs:      fsWatcher,
		ignores: ignores,
	}
	if err := w.addRecursive(root); err != nil {
		_ = fsWatcher.Close()
		return nil, err
	}
	return w, nil
}

// Events returns the debounced event stream.
func (w *Watcher) Events() <-chan Event {
	return w.events
}

// Errors returns watcher errors reported by fsnotify.
func (w *Watcher) Errors() <-chan error {
	return w.errors
}

// Run forwards fsnotify events until the context is canceled.
func (w *Watcher) Run(ctx context.Context, debounce time.Duration) {
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}

	var pending *Event
	flush := func() {
		if pending == nil {
			return
		}
		w.events <- *pending
		pending = nil
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			_ = w.fs.Close()
			close(w.events)
			close(w.errors)
			return
		case err, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			w.errors <- err
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			if w.isIgnoreFile(ev.Name) {
				if err := w.reloadIgnores(); err != nil {
					w.errors <- err
				}
				continue
			}
			if ev.Has(fsnotify.Create) {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() && !w.ignores.matches(ev.Name, true) {
					_ = w.addRecursive(ev.Name)
				}
			}
			if w.shouldIgnoreEventPath(ev.Name) {
				continue
			}
			pending = &Event{Path: ev.Name, Op: ev.Op}
			timer.Reset(debounce)
		case <-timer.C:
			flush()
		}
	}
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if w.isDotGitOrUnder(path) {
			return filepath.SkipDir
		}
		if path != w.root && w.ignores.matches(path, true) {
			return filepath.SkipDir
		}
		return w.fs.Add(path)
	})
}

func (w *Watcher) isIgnoreFile(path string) bool {
	return samePath(path, filepath.Join(w.root, ".gitignore")) || samePath(path, filepath.Join(w.root, ".iwatchignore"))
}

func (w *Watcher) reloadIgnores() error {
	ignores, err := newIgnoreMatcher(w.root)
	if err != nil {
		return fmt.Errorf("reload ignore rules: %w", err)
	}
	w.ignores = ignores
	return nil
}

func (w *Watcher) shouldIgnoreEventPath(path string) bool {
	if w.isDotGitOrUnder(path) {
		return true
	}
	info, err := os.Stat(path)
	if err == nil {
		return w.ignores.matches(path, info.IsDir())
	}
	return w.ignores.matches(path, false)
}

func (w *Watcher) isDotGitOrUnder(path string) bool {
	relative, err := filepath.Rel(w.root, path)
	if err != nil {
		return false
	}
	relative = filepath.ToSlash(relative)
	if relative == ".git" || strings.HasPrefix(relative, ".git/") {
		return true
	}
	return strings.Contains(relative, "/.git/")
}

func samePath(left string, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
