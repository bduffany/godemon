package godemon

import (
	"context"

	internal "github.com/bduffany/godemon/internal/godemon"
)

// Config configures which paths are watched and how filesystem changes are
// filtered.
type Config = internal.Config

// FSOp is a bitmask describing the kinds of change observed for a path.
type FSOp = internal.FSOp

const (
	// OpCreate indicates that a path was created.
	OpCreate FSOp = internal.OpCreate
	// OpWrite indicates that a path was written.
	OpWrite FSOp = internal.OpWrite
	// OpRemove indicates that a path was removed.
	OpRemove FSOp = internal.OpRemove
	// OpRename indicates that a path was renamed.
	OpRename FSOp = internal.OpRename
	// OpChmod indicates that a path's mode changed.
	OpChmod FSOp = internal.OpChmod
)

// FSEvent is a single filesystem change notification.
type FSEvent = internal.FSEvent

// Watcher yields filesystem change notifications.
type Watcher interface {
	// Next waits until the next filesystem change is observed. If ctx is
	// canceled before a change is observed, Next stops the watcher and returns
	// ctx.Err().
	Next(ctx context.Context) (FSEvent, error)
}

// Watch starts watching files according to cfg.
func Watch(cfg Config) (Watcher, error) {
	return internal.Watch(cfg)
}

// Main runs the godemon command line interface.
//
// Deprecated: Use [Watch] instead.
func Main(args []string) {
	internal.Main(args)
}
