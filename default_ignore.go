package main

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
