package godemon

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
)

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isDir(path string) bool {
	s, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		warnf("%s", err)
		return false
	}
	return s.IsDir()
}

func setMaxRLimit() error {
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return fmt.Errorf("get existing open files limit: %s", err)
	}
	if runtime.GOOS == "darwin" && limit.Max > 10240 {
		// The max file limit is 10240, even though
		// the max returned by Getrlimit is 1<<63-1.
		// This is OPEN_MAX in sys/syslimits.h.
		limit.Max = 10240
	}
	if limit.Cur != limit.Max {
		limit.Cur = limit.Max
		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
			return fmt.Errorf("increase open files limit: %s", err)
		}
	}
	return nil
}
