//go:build darwin && cgo

package godemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsevents"
)

// newWatcher returns a watcher backed by the native macOS FSEvents API, which
// watches an entire directory tree with a single event stream per watch root.
// Unlike kqueue (fsnotify's macOS backend), it does not need an open file
// descriptor per watched directory.
func newWatcher() (watcher, error) {
	return &fseventsWatcher{
		events: make(chan FSEvent, 1024),
		errors: make(chan error, 1),
	}, nil
}

type fseventsWatcher struct {
	events chan FSEvent
	errors chan error

	mu      sync.Mutex
	streams []*fsevents.EventStream
}

func (w *fseventsWatcher) Add(path string) error {
	// FSEvents reports symlink-resolved paths (e.g. /private/var/... for
	// /var/...), so watch the resolved path and map event paths back to the
	// path as given.
	addedAt := time.Now()
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
	go w.translate(es, resolved, path, addedAt)
	return nil
}

func (w *fseventsWatcher) translate(es *fsevents.EventStream, resolved, watchPath string, addedAt time.Time) {
	for batch := range es.Events {
		for _, e := range batch {
			if e.Flags&(fsevents.MustScanSubDirs|fsevents.KernelDropped|fsevents.UserDropped|fsevents.RootChanged) != 0 {
				// Events were coalesced or dropped, or the watch root itself
				// changed. Report a change at the watch root so the command
				// still restarts.
				w.events <- FSEvent{Path: watchPath, Op: OpWrite}
				continue
			}
			path := e.Path
			// Event paths are absolute since the stream is not created
			// relative to a device, but the leading "/" is not guaranteed.
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			flags := e.Flags
			if flags&fsevents.ItemCreated != 0 && createdBefore(path, addedAt) {
				// FSEvents can report already-existing paths as created
				// shortly after starting the stream. Treat only that create
				// bit as startup noise and keep any other change flags.
				flags &^= fsevents.ItemCreated
			}
			op := toFsOp(flags)
			if op == 0 {
				continue
			}
			if path == resolved {
				path = watchPath
			} else if strings.HasPrefix(path, resolved+"/") {
				path = watchPath + strings.TrimPrefix(path, resolved)
			}
			w.events <- FSEvent{Path: path, Op: op}
		}
	}
}

func createdBefore(path string, t time.Time) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	if stat.Birthtimespec.Sec == 0 && stat.Birthtimespec.Nsec == 0 {
		return false
	}
	birthTime := time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec)
	return !birthTime.After(t)
}

func toFsOp(flags fsevents.EventFlags) FSOp {
	var op FSOp
	if flags&fsevents.ItemCreated != 0 {
		op |= OpCreate
	}
	if flags&(fsevents.ItemModified|fsevents.ItemInodeMetaMod) != 0 {
		op |= OpWrite
	}
	if flags&fsevents.ItemRemoved != 0 {
		op |= OpRemove
	}
	if flags&fsevents.ItemRenamed != 0 {
		op |= OpRename
	}
	if flags&(fsevents.ItemChangeOwner|fsevents.ItemXattrMod|fsevents.ItemFinderInfoMod) != 0 {
		op |= OpChmod
	}
	return op
}

func (w *fseventsWatcher) Events() <-chan FSEvent { return w.events }

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
