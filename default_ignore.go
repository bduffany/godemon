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
	// Python bytecode
	"**/__pycache__/**",
	// Text editor backup / lockfiles
	// Vim
	"**/*.swp",
	"**/*.swx",
	// emacs
	"**/#*",
	"**/.#*#",
}
