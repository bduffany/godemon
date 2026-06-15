package godemon

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
)

const (
	// DefaultNotifySignal is the default signal sent to a command on changes
	// (in order to get it to restart).
	DefaultNotifySignal = syscall.SIGTERM
)

func parseSignal(name string) (syscall.Signal, error) {
	n, err := strconv.Atoi(name)
	if err == nil {
		return syscall.Signal(n), nil
	}
	switch strings.TrimPrefix(strings.ToUpper(name), "SIG") {
	case "INT":
		return syscall.SIGINT, nil
	case "TERM":
		return syscall.SIGTERM, nil
	case "QUIT":
		return syscall.SIGQUIT, nil
	case "KILL":
		return syscall.SIGKILL, nil
	}
	return 0, fmt.Errorf("unsupported signal %q", name)
}
