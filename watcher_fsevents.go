//go:build darwin && cgo

package godemon

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsevents"
)

// newWatcher returns a watcher backed by the native macOS FSEvents API, which
// watches an entire directory tree with a single event stream per watch root.
// Unlike kqueue (fsnotify's macOS backend), it does not need an open file
// descriptor per watched directory.
func newWatcher() (watcher, error) {
	return &fseventsWatcher{
		events: make(chan fsEvent, 1024),
		errors: make(chan error, 1),
	}, nil
}

type fseventsWatcher struct {
	events chan fsEvent
	errors chan error

	mu      sync.Mutex
	streams []*fsevents.EventStream
}

func (w *fseventsWatcher) Add(path string) error {
	// FSEvents reports symlink-resolved paths (e.g. /private/var/... for
	// /var/...), so watch the resolved path and map event paths back to the
	// path as given.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve watch path %q: %w", path, err)
	}
	es := &fsevents.EventStream{
		Paths: []string{resolved},
		// Latency is the window FSEvents may use to coalesce events. NoDefer
		// delivers the first event in a window immediately, so this only
		// delays delivery during bursts.
		Latency: 50 * time.Millisecond,
		Flags:   fsevents.FileEvents | fsevents.WatchRoot | fsevents.NoDefer,
	}
	if err := es.Start(); err != nil {
		return fmt.Errorf("start FSEvents stream for %q: %w", path, err)
	}
	w.mu.Lock()
	w.streams = append(w.streams, es)
	w.mu.Unlock()
	go w.translate(es, resolved, path)
	return nil
}

func (w *fseventsWatcher) translate(es *fsevents.EventStream, resolved, watchPath string) {
	for batch := range es.Events {
		for _, e := range batch {
			if e.Flags&(fsevents.MustScanSubDirs|fsevents.KernelDropped|fsevents.UserDropped|fsevents.RootChanged) != 0 {
				// Events were coalesced or dropped, or the watch root itself
				// changed. Report a change at the watch root so the command
				// still restarts.
				w.events <- fsEvent{Path: watchPath, Op: opWrite}
				continue
			}
			op := toFsOp(e.Flags)
			if op == 0 {
				continue
			}
			path := e.Path
			// Event paths are absolute since the stream is not created
			// relative to a device, but the leading "/" is not guaranteed.
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			if path == resolved {
				path = watchPath
			} else if strings.HasPrefix(path, resolved+"/") {
				path = watchPath + strings.TrimPrefix(path, resolved)
			}
			w.events <- fsEvent{Path: path, Op: op}
		}
	}
}

func toFsOp(flags fsevents.EventFlags) fsOp {
	var op fsOp
	if flags&fsevents.ItemCreated != 0 {
		op |= opCreate
	}
	if flags&(fsevents.ItemModified|fsevents.ItemInodeMetaMod) != 0 {
		op |= opWrite
	}
	if flags&fsevents.ItemRemoved != 0 {
		op |= opRemove
	}
	if flags&fsevents.ItemRenamed != 0 {
		op |= opRename
	}
	if flags&(fsevents.ItemChangeOwner|fsevents.ItemXattrMod|fsevents.ItemFinderInfoMod) != 0 {
		op |= opChmod
	}
	return op
}

func (w *fseventsWatcher) Events() <-chan fsEvent { return w.events }

func (w *fseventsWatcher) Errors() <-chan error { return w.errors }

func (w *fseventsWatcher) Recursive() bool { return true }

func (w *fseventsWatcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, es := range w.streams {
		es.Stop()
	}
	w.streams = nil
	return nil
}
