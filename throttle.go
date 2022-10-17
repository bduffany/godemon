package godemon

import (
	"time"
)

func nonBlockingDrain(ch <-chan struct{}) int {
	n := 0
	for {
		select {
		case <-ch:
			n++
		default:
			return n
		}
	}
}

func throttleRestarts(events <-chan struct{}) chan struct{} {
	restart := make(chan struct{})
	go func() {
		defer close(restart)

		for {
			// Wait for first event since last restart
			_, ok := <-events
			if !ok {
				return
			}
			// Restart immediately. The send here is non-blocking, so the receiver
			// will ignore restart attempts while it is already mid-restart.
			select {
			case restart <- struct{}{}:
			default:
			}
			// Make sure we go at least 50ms with no events before the next restart
			// TODO: Make this configurable, and/or adaptive
			for {
				<-time.After(50 * time.Millisecond)
				n := nonBlockingDrain(events)
				if n == 0 {
					break
				}
			}
		}
	}()
	return restart
}
