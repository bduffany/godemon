package godemon

import (
	"time"
)

// DefaultThrottle is the default period after a restart during which
// additional filesystem changes are ignored.
const DefaultThrottle = 50 * time.Millisecond

func throttleRestarts(events <-chan struct{}, quiet time.Duration) chan struct{} {
	restart := make(chan struct{})
	go func() {
		defer close(restart)

		for {
			_, ok := <-events
			if !ok {
				return
			}
			// The send blocks while the receiver is mid-restart so that restart
			// requests are never dropped.
			restart <- struct{}{}
			if !ignoreEventsFor(events, quiet) {
				return
			}
		}
	}()
	return restart
}

func ignoreEventsFor(events <-chan struct{}, quiet time.Duration) bool {
	if quiet <= 0 {
		return true
	}

	timer := time.NewTimer(quiet)
	defer timer.Stop()

	for {
		select {
		case _, ok := <-events:
			if !ok {
				return false
			}
		case <-timer.C:
			return true
		}
	}
}
