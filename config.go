package godemon

import (
	"syscall"
)

type Config struct {
	// Command is the command and arguments to be executed upon any change.
	//
	// The command is not interpreted using a shell. If you would like shell
	// features such as environment variable expansion, specify the command
	// using something like the following:
	//
	//     ["sh", "-c", "$YOUR_COMMAND"]
	Command []string `json:"command"`

	// Watch specifies a list of files or directories to be watched. Defaults to
	// the current working directory from which the command is invoked.
	// Directories are watched recursively.
	// TODO(bduffany): Accept glob patterns here.
	Watch []string `json:"watch,omitempty"`
	// Only specifies a list of allowed patterns. If non-empty, at least one
	// pattern must match in order for the command to be executed.
	Only []string `json:"only,omitempty"`
	// Ignore specifies a list of paths to be ignored. Glob patterns are supported.
	Ignore []string `json:"ignore,omitempty"`

	// UseDefaultIgnoreList specifies whether to use the default list of
	// ignore patterns. These will be appended to the list of ignore patterns.
	// Defaults to true.
	UseDefaultIgnoreList *bool `json:"useDefaultIgnoreList,omitempty"`
	// UseGitignore specifies whether to respect .gitignore files when watching
	// directories.
	// Defaults to true.
	UseGitignore *bool `json:"useGitignore,omitempty"`

	// NotifySignal is the signal used to notify the command of file changes.
	// Defaults to "SIGINT" (Ctrl+C), which should gracefully stop most
	// well-behaved commands.
	NotifySignal *string `json:"notifySignal,omitempty"`

	// Lockfile specifies a path to a file which, if it exists, will cause
	// file-based restarts to be paused until the file is removed. This can be
	// used to temporarily pause godemon's file-watching behavior while running a
	// batch of updates, then finally trigger a restart only once all the updates
	// are complete.
	Lockfile *string `json:"lockfile,omitempty"`

	// Clear specifies whether to clear the terminal on each re-run.
	//
	// Note, this option is just for convenience. The same functionality can
	// be achieved with the following command:
	// godemon [ flags ... ] sh -c 'clear && exec {command}'
	Clear bool `json:"clear,omitempty"`

	// TODO: FollowSymlinks

	// Private fields below

	notifySignal   syscall.Signal
	dryRun         bool
	ignorePatterns []*pattern
	onlyPatterns   []*pattern
}
