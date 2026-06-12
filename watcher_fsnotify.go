//go:build !darwin || !cgo

package godemon

import (
	"github.com/fsnotify/fsnotify"
)

// newWatcher returns a watcher backed by fsnotify (inotify on Linux, kqueue
// on BSDs). These APIs watch single directories only, so directory trees are
// watched by adding each directory individually.
func newWatcher() (watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	fw := &fsnotifyWatcher{
		w:      w,
		events: make(chan fsEvent, 1024),
	}
	go fw.translate()
	return fw, nil
}

type fsnotifyWatcher struct {
	w      *fsnotify.Watcher
	events chan fsEvent
}

func (w *fsnotifyWatcher) translate() {
	defer close(w.events)
	for e := range w.w.Events {
		var op fsOp
		if e.Op&fsnotify.Create != 0 {
			op |= opCreate
		}
		if e.Op&fsnotify.Write != 0 {
			op |= opWrite
		}
		if e.Op&fsnotify.Remove != 0 {
			op |= opRemove
		}
		if e.Op&fsnotify.Rename != 0 {
			op |= opRename
		}
		if e.Op&fsnotify.Chmod != 0 {
			op |= opChmod
		}
		w.events <- fsEvent{Path: e.Name, Op: op}
	}
}

func (w *fsnotifyWatcher) Events() <-chan fsEvent { return w.events }

func (w *fsnotifyWatcher) Errors() <-chan error { return w.w.Errors }

func (w *fsnotifyWatcher) Add(path string) error { return w.w.Add(path) }

func (w *fsnotifyWatcher) Recursive() bool { return false }

func (w *fsnotifyWatcher) Close() error { return w.w.Close() }
