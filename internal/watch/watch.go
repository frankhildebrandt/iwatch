package watch

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Event struct {
	Path string
	Op   fsnotify.Op
}

type Watcher struct {
	root   string
	events chan Event
	errors chan error
	fs     *fsnotify.Watcher
}

func New(root string) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		root:   root,
		events: make(chan Event, 128),
		errors: make(chan error, 32),
		fs:     fsWatcher,
	}
	if err := w.addRecursive(root); err != nil {
		_ = fsWatcher.Close()
		return nil, err
	}
	return w, nil
}

func (w *Watcher) Events() <-chan Event {
	return w.events
}

func (w *Watcher) Errors() <-chan error {
	return w.errors
}

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
			if ev.Has(fsnotify.Create) {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = w.addRecursive(ev.Name)
				}
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
		return w.fs.Add(path)
	})
}
