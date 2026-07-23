package godemon

import (
	"testing"
	"time"
)

func TestThrottleRestartsImmediately(t *testing.T) {
	events := make(chan struct{}, 10)
	restarts := throttleRestarts(events, 50*time.Millisecond)

	events <- struct{}{}

	select {
	case <-restarts:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for immediate restart")
	}

	close(events)
}

func TestThrottleRestartsIgnoresEventsDuringThrottlePeriod(t *testing.T) {
	events := make(chan struct{}, 10)
	restarts := throttleRestarts(events, 50*time.Millisecond)

	events <- struct{}{}
	select {
	case <-restarts:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for immediate restart")
	}

	events <- struct{}{}
	events <- struct{}{}

	select {
	case <-restarts:
		t.Fatal("got restart for events that should have been ignored")
	case <-time.After(75 * time.Millisecond):
	}

	close(events)
}

func TestThrottleRestartsDoesNotExtendThrottlePeriod(t *testing.T) {
	events := make(chan struct{}, 10)
	restarts := throttleRestarts(events, 50*time.Millisecond)

	events <- struct{}{}
	select {
	case <-restarts:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for immediate restart")
	}

	time.Sleep(25 * time.Millisecond)
	events <- struct{}{}

	select {
	case <-restarts:
		t.Fatal("ignored event scheduled another restart")
	case <-time.After(75 * time.Millisecond):
	}

	select {
	case events <- struct{}{}:
	default:
		t.Fatal("events channel unexpectedly blocked")
	}
	select {
	case <-restarts:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for restart after throttle period")
	}

	close(events)
}
