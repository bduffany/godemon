package main

import (
	"fmt"
	"os"
)

var (
	logLevels = map[string]int{
		"DEBUG":   0,
		"INFO":    1,
		"NOTIFY":  2,
		"WARNING": 3,
		"ERROR":   4,
		"FATAL":   5,
	}
	logLevel = logLevels["NOTIFY"]
)

func logf(level, format string, args ...interface{}) {
	if logLevels[level] < logLevel {
		return
	}
	const gray = "\x1b[90m"
	const reset = "\x1b[0m"
	prefix := "[godemon] "
	// Don't show NOTIFY prefix since these are very common and are intended
	// to be user friendly.
	if level != "NOTIFY" {
		prefix += level + ": "
	}
	fmt.Printf(gray+prefix+format+reset+"\n", args...)
}

func debugf(format string, args ...interface{}) {
	logf("DEBUG", format, args...)
}
func infof(format string, args ...interface{}) {
	logf("INFO", format, args...)
}
func notifyf(format string, args ...interface{}) {
	logf("NOTIFY", format, args...)
}
func warnf(format string, args ...interface{}) {
	logf("WARNING", format, args...)
}
func errorf(format string, args ...interface{}) {
	logf("ERROR", format, args...)
}
func fatalf(format string, args ...interface{}) {
	logf("FATAL", format, args...)
	os.Exit(1)
}
