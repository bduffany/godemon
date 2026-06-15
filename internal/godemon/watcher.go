package godemon

import "strings"

// FSOp is a bitmask describing the kinds of change observed for a path.
type FSOp uint32

const (
	// OpCreate indicates that a path was created.
	OpCreate FSOp = 1 << iota
	// OpWrite indicates that a path was written.
	OpWrite
	// OpRemove indicates that a path was removed.
	OpRemove
	// OpRename indicates that a path was renamed.
	OpRename
	// OpChmod indicates that a path's mode changed.
	OpChmod
)

func (op FSOp) String() string {
	var names []string
	for _, f := range []struct {
		op   FSOp
		name string
	}{
		{OpCreate, "CREATE"},
		{OpWrite, "WRITE"},
		{OpRemove, "REMOVE"},
		{OpRename, "RENAME"},
		{OpChmod, "CHMOD"},
	} {
		if op&f.op != 0 {
			names = append(names, f.name)
		}
	}
	if len(names) == 0 {
		return "UNKNOWN"
	}
	return strings.Join(names, "|")
}

// FSEvent is a single file change notification. Path is absolute, expressed
// under the watch path as given by the user (symlinks not resolved).
type FSEvent struct {
	// Path is the path that changed.
	Path string
	// Op describes the kind of change observed for Path.
	Op FSOp
}

// watcher delivers file change notifications for watched paths. The
// implementation is chosen per platform by newWatcher.
type watcher interface {
	// Events returns the channel on which change notifications are delivered.
	Events() <-chan FSEvent
	// Errors returns the channel on which watch errors are delivered.
	Errors() <-chan error
	// Add starts watching the given path. If Recursive reports true, the
	// entire tree rooted at the path is watched; otherwise only the path
	// itself is.
	Add(path string) error
	// Recursive reports whether Add watches entire directory trees.
	Recursive() bool
	// Close stops delivering events and releases watch resources.
	Close() error
}
