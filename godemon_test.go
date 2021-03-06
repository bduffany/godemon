// Integration tests for the godemon CLI.
// These tests invoke the built godemon binary.
package main

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	anyWhitespace = regexp.MustCompile(`\s+`)
)

func isSignaledErr(err error) bool {
	if err, ok := err.(*exec.ExitError); ok {
		if s, ok := err.ProcessState.Sys().(syscall.WaitStatus); ok {
			return s.Signaled()
		}
	}
	return false
}

func isExitErrCode(err error, code int) bool {
	if err, ok := err.(*exec.ExitError); ok {
		return err.ExitCode() == code
	}
	return false
}

func isCleanExit(err error) bool {
	if isSignaledErr(err) {
		return true
	}
	// We exit with code 130 when interrupted with Ctrl+C.
	if isExitErrCode(err, 130) {
		return true
	}
	return false
}

func TestSendSIGINTToSleepCommandTerminates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	snap, err := NewProcSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	g := exec.CommandContext(ctx, "./godemon", "-v", "-v", "sleep", "infinity")
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}

	g.Process.Signal(syscall.SIGINT)

	if err := WaitContext(ctx, g); err != nil && !isCleanExit(err) {
		t.Fatal(err)
	}

	psDiff, err := snap.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(psDiff.Processes) != 0 {
		t.Log("Unexpected extra processes")
		for _, info := range psDiff.Processes {
			t.Log(info)
		}
		t.Fail()
	}
}

// WaitContext waits for the command to exit on its own. If it doesn't exit
// before the context is done, the command is terminated using SIGKILL.
func WaitContext(ctx context.Context, cmd *exec.Cmd) error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		cmd.Process.Signal(syscall.SIGKILL)
		return ctx.Err()
	}
}

func TestSendMultipleCtrlCToBadlyBehavedCommandTerminatesAfter3CtrlC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	snap, err := NewProcSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	g := exec.CommandContext(ctx, "./godemon", "-v", "-v", "sh", "-c", `
	  trap "echo 'Ignoring SIGINT'" INT
	  while true; do
			sleep 0.01
			echo 'Listening for SIGINT'
		done
	`)
	buf := &bytes.Buffer{}
	g.Stderr = buf
	g.Stdout = buf
	if err := g.Start(); err != nil && !isCleanExit(err) {
		t.Fatal(err)
	}

	// Wait until the SIGINT trap is registered
	t.Log("Waiting for SIGINT trap to be registered")
	for !strings.Contains(buf.String(), "Listening for SIGINT") {
		time.Sleep(1 * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Fatalf("Timed out waiting for SIGINT listener to be registered")
		default:
		}
	}
	t.Log("SIGINT trap registered")

	// After 3 SIGINT signals, should be killed.
	go func() {
		g.Process.Signal(syscall.SIGINT)
		time.Sleep(1 * time.Millisecond)
		g.Process.Signal(syscall.SIGINT)
		time.Sleep(1 * time.Millisecond)
		g.Process.Signal(syscall.SIGINT)
		time.Sleep(1 * time.Millisecond)
	}()

	if err := WaitContext(ctx, g); err != nil && !isCleanExit(err) {
		t.Logf("Command logs:\n%s\n", buf.String())
		t.Fatal(err)
	}

	psDiff, err := snap.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(psDiff.Processes) != 0 {
		t.Log("Unexpected extra processes")
		for pid, cmd := range psDiff.Processes {
			t.Logf("PID: %d, CMD: %s", pid, cmd)
		}
		t.Fail()
	}
}
