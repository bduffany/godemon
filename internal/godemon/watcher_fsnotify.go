//go:build !linux && (!darwin || !cgo)

package godemon

import (
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// newWatcher returns a watcher backed by fsnotify on platforms without a
// specialized watcher. Its backends watch single directories only, so
// directory trees are watched by adding each directory individually.
func newWatcher() (watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	fw := &fsnotifyWatcher{
		w:      w,
		events: make(chan FSEvent, 1024),
		dirs:   make(map[string]struct{}),
	}
	go fw.translate()
	return fw, nil
}

type fsnotifyWatcher struct {
	w      *fsnotify.Watcher
	events chan FSEvent

	mu   sync.Mutex
	dirs map[string]struct{}
}

func (w *fsnotifyWatcher) translate() {
	defer close(w.events)
	for e := range w.w.Events {
		isDir := w.pathIsDir(e.Name, e.Op)
		var op FSOp
		if e.Op&fsnotify.Create != 0 {
			op |= OpCreate
		}
		if e.Op&fsnotify.Write != 0 {
			op |= OpWrite
		}
		if e.Op&fsnotify.Remove != 0 {
			op |= OpRemove
		}
		if e.Op&fsnotify.Rename != 0 {
			op |= OpRename
		}
		if e.Op&fsnotify.Chmod != 0 {
			op |= OpChmod
		}
		w.events <- FSEvent{Path: e.Name, Op: op, isDir: isDir}
	}
}

func (w *fsnotifyWatcher) pathIsDir(path string, op fsnotify.Op) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			w.dirs[path] = struct{}{}
		} else {
			delete(w.dirs, path)
		}
	}
	_, isDir := w.dirs[path]
	if op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		delete(w.dirs, path)
	}
	return isDir
}

func (w *fsnotifyWatcher) Events() <-chan FSEvent { return w.events }

func (w *fsnotifyWatcher) Errors() <-chan error { return w.w.Errors }

func (w *fsnotifyWatcher) Add(path string) error {
	if err := w.w.Add(path); err != nil {
		return err
	}
	w.pathIsDir(path, 0)
	return nil
}

func (w *fsnotifyWatcher) Recursive() bool { return false }

func (w *fsnotifyWatcher) Close() error { return w.w.Close() }
