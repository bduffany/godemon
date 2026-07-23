//go:build linux

package godemon

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

const inotifyWatchMask = unix.IN_ATTRIB |
	unix.IN_CREATE |
	unix.IN_DELETE |
	unix.IN_DELETE_SELF |
	unix.IN_MODIFY |
	unix.IN_MOVED_FROM |
	unix.IN_MOVED_TO |
	unix.IN_MOVE_SELF

// newWatcher returns a watcher backed by inotify. Godemon uses inotify
// directly instead of fsnotify on Linux because fsnotify does not expose the
// IN_ISDIR flag attached to individual events.
func newWatcher() (watcher, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	w := &inotifyWatcher{
		fd:      fd,
		file:    os.NewFile(uintptr(fd), "inotify"),
		events:  make(chan FSEvent, 1024),
		errors:  make(chan error, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		watches: make(map[int32]inotifyWatch),
		paths:   make(map[string]int32),
	}
	go w.readEvents()
	return w, nil
}

type inotifyWatch struct {
	path  string
	isDir bool
}

type inotifyWatcher struct {
	fd     int
	file   *os.File
	events chan FSEvent
	errors chan error
	stop   chan struct{}
	done   chan struct{}

	closeOnce sync.Once
	mu        sync.Mutex
	watches   map[int32]inotifyWatch
	paths     map[string]int32
}

func (w *inotifyWatcher) readEvents() {
	defer close(w.done)
	defer close(w.errors)
	defer close(w.events)

	var buf [unix.SizeofInotifyEvent * 4096]byte
	for {
		n, err := w.file.Read(buf[:])
		if err != nil {
			if errors.Is(err, os.ErrClosed) || w.stopped() {
				return
			}
			if !w.sendError(err) {
				return
			}
			continue
		}
		if n < unix.SizeofInotifyEvent {
			if n == 0 {
				err = io.EOF
			} else {
				err = fmt.Errorf("short inotify read: got %d bytes", n)
			}
			if !w.sendError(err) {
				return
			}
			continue
		}

		for offset := 0; offset <= n-unix.SizeofInotifyEvent; {
			raw := (*unix.InotifyEvent)(unsafe.Pointer(&buf[offset]))
			recordLen := unix.SizeofInotifyEvent + int(raw.Len)
			if recordLen > n-offset {
				if !w.sendError(fmt.Errorf("short inotify event: got %d bytes, want %d", n-offset, recordLen)) {
					return
				}
				break
			}

			name := strings.TrimRight(string(buf[offset+unix.SizeofInotifyEvent:offset+recordLen]), "\x00")
			if !w.handleEvent(raw.Wd, raw.Mask, name) {
				return
			}
			offset += recordLen
		}
	}
}

func (w *inotifyWatcher) handleEvent(wd int32, mask uint32, name string) bool {
	if mask&unix.IN_Q_OVERFLOW != 0 {
		return w.sendError(errors.New("inotify queue overflow"))
	}

	w.mu.Lock()
	watch, ok := w.watches[wd]
	if mask&(unix.IN_IGNORED|unix.IN_UNMOUNT) != 0 {
		if ok {
			w.removeWatchLocked(wd, watch.path)
		}
		w.mu.Unlock()
		return true
	}
	if !ok {
		w.mu.Unlock()
		return true
	}

	parentWatched := false
	if mask&unix.IN_DELETE_SELF != 0 {
		_, parentWatched = w.paths[filepath.Dir(watch.path)]
		w.removeWatchLocked(wd, watch.path)
	}
	if mask&unix.IN_MOVE_SELF != 0 {
		w.removeWatchLocked(wd, watch.path)
		_, _ = unix.InotifyRmWatch(w.fd, uint32(wd))
	}
	w.mu.Unlock()

	// The parent watch already reports this directory removal. Avoid sending a
	// duplicate event for the watch on the directory itself.
	if mask&unix.IN_DELETE_SELF != 0 && parentWatched {
		return true
	}

	path := watch.path
	isDir := watch.isDir
	if name != "" {
		path = filepath.Join(path, name)
		isDir = mask&unix.IN_ISDIR != 0
	}

	var op FSOp
	if mask&(unix.IN_CREATE|unix.IN_MOVED_TO) != 0 {
		op |= OpCreate
	}
	if mask&unix.IN_MODIFY != 0 {
		op |= OpWrite
	}
	if mask&(unix.IN_DELETE|unix.IN_DELETE_SELF) != 0 {
		op |= OpRemove
	}
	if mask&(unix.IN_MOVED_FROM|unix.IN_MOVE_SELF) != 0 {
		op |= OpRename
	}
	if mask&unix.IN_ATTRIB != 0 {
		op |= OpChmod
	}
	if op == 0 {
		return true
	}
	return w.sendEvent(FSEvent{Path: path, Op: op, isDir: isDir})
}

func (w *inotifyWatcher) removeWatchLocked(wd int32, path string) {
	delete(w.watches, wd)
	if w.paths[path] == wd {
		delete(w.paths, path)
	}
}

func (w *inotifyWatcher) sendEvent(event FSEvent) bool {
	select {
	case w.events <- event:
		return true
	case <-w.stop:
		return false
	}
}

func (w *inotifyWatcher) sendError(err error) bool {
	select {
	case w.errors <- err:
		return true
	case <-w.stop:
		return false
	}
}

func (w *inotifyWatcher) stopped() bool {
	select {
	case <-w.stop:
		return true
	default:
		return false
	}
}

func (w *inotifyWatcher) Events() <-chan FSEvent { return w.events }

func (w *inotifyWatcher) Errors() <-chan error { return w.errors }

func (w *inotifyWatcher) Add(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped() {
		return os.ErrClosed
	}
	wd, err := unix.InotifyAddWatch(w.fd, path, inotifyWatchMask)
	if err != nil {
		return err
	}
	watch := inotifyWatch{path: path, isDir: info.IsDir()}
	if previous, ok := w.watches[int32(wd)]; ok && previous.path != path {
		delete(w.paths, previous.path)
	}
	w.watches[int32(wd)] = watch
	w.paths[path] = int32(wd)
	return nil
}

func (w *inotifyWatcher) Recursive() bool { return false }

func (w *inotifyWatcher) Close() error {
	var err error
	w.closeOnce.Do(func() {
		close(w.stop)
		w.mu.Lock()
		err = w.file.Close()
		w.mu.Unlock()
		<-w.done
	})
	return err
}
