package godemon

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
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

	signalName := strings.ToUpper(name)
	if !strings.HasPrefix(signalName, "SIG") {
		signalName = "SIG" + signalName
	}
	if signal := unix.SignalNum(signalName); signal != 0 {
		return signal, nil
	}
	return 0, fmt.Errorf("unsupported signal %q", name)
}
