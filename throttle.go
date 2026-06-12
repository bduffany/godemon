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
			for {
				// Restart immediately. The send blocks while the receiver is
				// mid-restart so that restart requests are never dropped.
				restart <- struct{}{}
				// Make sure we go at least 50ms with no events before the next
				// restart.
				// TODO: Make this configurable, and/or adaptive
				pending := false
				for {
					<-time.After(50 * time.Millisecond)
					if nonBlockingDrain(events) == 0 {
						break
					}
					pending = true
				}
				if !pending {
					break
				}
				// Events arrived during the cooldown, i.e. possibly after the
				// command was restarted, so they may not be reflected in the
				// current run: restart again to pick them up.
			}
		}
	}()
	return restart
}
