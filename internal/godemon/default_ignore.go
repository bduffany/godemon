package godemon

import "path/filepath"

// TODO: Remove this workaround once https://github.com/openai/codex/issues/31599 is fixed.
var defaultIgnoredDirectoryNames = map[string]struct{}{
	".agents": {},
	".codex":  {},
}

func isDefaultIgnoredDirectory(path string, isDir bool) bool {
	if !isDir {
		return false
	}
	_, ok := defaultIgnoredDirectoryNames[filepath.Base(path)]
	return ok
}

var defaultIgnorePatterns = []string{
	// Log files
	"*.log",
	// Version control
	"**/.git/**",
	"**/.hg/**",
	"**/.svn/**",
	"**/CVS/**",
	// NPM
	"**/node_modules/**",
	// Bazel output symlinks
	"**/bazel-*/**",
	// Python
	"**/__pycache__/**",
	"**/.pytest_cache/**",
	// Text editor backup / lockfiles
	// Vim
	"**/*.swp",
	"**/*.swx",
	// emacs
	"**/#*",
	"**/.#*#",
}
