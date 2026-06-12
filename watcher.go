package godemon

import "strings"

// fsOp is a bitmask describing the kinds of change observed for a path.
type fsOp uint32

const (
	opCreate fsOp = 1 << iota
	opWrite
	opRemove
	opRename
	opChmod
)

func (op fsOp) String() string {
	var names []string
	for _, f := range []struct {
		op   fsOp
		name string
	}{
		{opCreate, "CREATE"},
		{opWrite, "WRITE"},
		{opRemove, "REMOVE"},
		{opRename, "RENAME"},
		{opChmod, "CHMOD"},
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

// fsEvent is a single file change notification. Path is absolute, expressed
// under the watch path as given by the user (symlinks not resolved).
type fsEvent struct {
	Path string
	Op   fsOp
}

// watcher delivers file change notifications for watched paths. The
// implementation is chosen per platform by newWatcher.
type watcher interface {
	// Events returns the channel on which change notifications are delivered.
	Events() <-chan fsEvent
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
