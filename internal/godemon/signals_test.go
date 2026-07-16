package godemon

import (
	"strconv"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestParseSignalNameVariants(t *testing.T) {
	for _, testCase := range []struct {
		name string
		want syscall.Signal
	}{
		{name: "USR1", want: syscall.SIGUSR1},
		{name: "usr1", want: syscall.SIGUSR1},
		{name: "SIGUSR1", want: syscall.SIGUSR1},
		{name: "sigusr1", want: syscall.SIGUSR1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := parseSignal(testCase.name)
			if err != nil {
				t.Fatal(err)
			}
			if got != testCase.want {
				t.Fatalf("parseSignal(%q) = %d, want %d", testCase.name, got, testCase.want)
			}
		})
	}
}

func TestParseSignalAcceptsEveryPlatformSignalName(t *testing.T) {
	for signal := syscall.Signal(1); signal < 256; signal++ {
		name := unix.SignalName(signal)
		if name == "" {
			continue
		}

		for _, input := range []string{
			name,
			strings.ToLower(name),
			strings.TrimPrefix(name, "SIG"),
		} {
			got, err := parseSignal(input)
			if err != nil {
				t.Errorf("parseSignal(%q): %s", input, err)
				continue
			}
			if got != signal {
				t.Errorf("parseSignal(%q) = %d, want %d", input, got, signal)
			}
		}
	}
}

func TestParseSignalPreservesNumericSignals(t *testing.T) {
	for _, signal := range []syscall.Signal{0, syscall.SIGUSR1, 255, -1} {
		name := strconv.Itoa(int(signal))
		got, err := parseSignal(name)
		if err != nil {
			t.Fatal(err)
		}
		if got != signal {
			t.Fatalf("parseSignal(%q) = %d, want %d", name, got, signal)
		}
	}
}

func TestParseSignalRejectsUnknownName(t *testing.T) {
	if _, err := parseSignal("definitely-not-a-signal"); err == nil {
		t.Fatal("parseSignal accepted an unknown name")
	}
}
