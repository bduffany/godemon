// Integration tests for the godemon CLI.
// These tests invoke the built godemon binary.
package godemon

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	binaryPath string
)

const (
	// countRunsScript is a shell script that counts the number of times
	// it was invoked, using the file ../count
	countRunsScript = `
		if ! [ -e ../count ]; then
			echo 0 > ../count
		fi
		count=$(cat ../count)
		echo $(( count + 1 )) > ../count

		sleep infinity
`
)

func init() {
	os.Setenv("GODEMON_LOG_PREFIX", "    [test-godemon] ")
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	binaryPath = filepath.Join(wd, "godemon")
}

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
	// We exit with code 2 when interrupted with Ctrl+C.
	if isExitErrCode(err, 2) {
		return true
	}
	return false
}

func newTestWorkspace(t *testing.T) string {
	root, err := os.MkdirTemp("", "godemon-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("Failed to clean up: %s", err)
		}
	})
	// Layout of `root` looks like this:
	// - godemon-test-abc123/  # `root`
	//   - .                   # temp files go here, to avoid triggering godemon
	//   - workspace/          # godemon working dir
	//     -                   # `fnames` are written here
	ws := filepath.Join(root, "workspace")
	fnames := []string{
		".git/config",
		".gitignore",
		"toplevel.go",
		"lib/foo.go",
		"lib/nested/bar.go",
	}
	for _, fname := range fnames {
		dir := filepath.Dir(fname)
		err := os.MkdirAll(filepath.Join(ws, dir), 0755)
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(ws, fname))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	return ws
}

func runCount(t *testing.T, godemon *exec.Cmd) int {
	path := filepath.Join(godemon.Dir, "../count")
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return 0
	}
	count, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func expectRunCount(t *testing.T, ctx context.Context, godemon *exec.Cmd, count int) {
	var c int
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for run count to equal %d (last count: %d)", count, c)
		default:
		}
		c = runCount(t, godemon)
		// Count will never decrease, so fail early.
		if c > count {
			t.Fatalf("unexpected run count: ran %d times but expected %d", c, count)
		}
		if c == count {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func touch(t *testing.T, ws string, fname string) {
	f, err := os.Create(filepath.Join(ws, fname))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func writeFile(t *testing.T, ws string, fname string, content string) {
	f, err := os.Create(filepath.Join(ws, fname))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_, err = f.Write([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
}

func TestRestartOnCreate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	g := exec.CommandContext(ctx, binaryPath, "-vv", "bash", "-c", countRunsScript)
	g.Dir = newTestWorkspace(t)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, g, 1)
	touch(t, g.Dir, "NEW.go")

	expectRunCount(t, ctx, g, 2)
}

func TestRestartOnEdit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	g := exec.CommandContext(ctx, binaryPath, "-vv", "bash", "-c", countRunsScript)
	g.Dir = newTestWorkspace(t)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, g, 1)
	touch(t, g.Dir, "toplevel.go")

	expectRunCount(t, ctx, g, 2)
}

func TestRestartSignalsAllChildren(t *testing.T) {
	t.Skip() // TODO: make this pass on macOS

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	defer func() {
		t.Logf("Test ran in %s", time.Since(start))
	}()

	// Run a Python program that becomes its own process group leader, so that
	// if we signal the godemon child processs group, it won't be affected.
	// Also
	g := exec.CommandContext(ctx, binaryPath, "-w", "toplevel.go", "-vv", "bash", "-c", `
		sh -c 'sleep 5' &
		echo "bash: waiting for sh..."
		wait
		echo "bash: sh exited."
	`)
	g.Dir = newTestWorkspace(t)
	// g.Stdout = os.Stdout
	// g.Stderr = os.Stderr
	g.Stdin = bytes.NewBuffer(nil)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	defer g.Wait()
	defer func() {
		start := time.Now()
		t.Log("Killing process tree...")
		defer func() {
			t.Logf("Killing process tree took %s", time.Since(start))
		}()
		KillProcessTree(g.Process.Pid)
	}()

	time.Sleep(500 * time.Millisecond)

	touch(t, g.Dir, "toplevel.go")

	time.Sleep(500 * time.Millisecond)

	snap, err := NewTreeSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	shCount := 0
	for _, p := range snap.Processes {
		if p.Executable() == "sh" {
			shCount += 1
		}
	}
	if shCount != 1 {
		t.Errorf("error: expected 1 sh process, got %d", shCount)
	}
}

func TestWatchSingleFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	g := exec.CommandContext(ctx, binaryPath, "-w", "./toplevel.go", "-vv", "bash", "-c", countRunsScript)
	g.Dir = newTestWorkspace(t)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, g, 1)
	touch(t, g.Dir, "toplevel.go")

	expectRunCount(t, ctx, g, 2)
}

func TestDefaultIgnoreList(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	g := exec.CommandContext(ctx, binaryPath, "-vv", "bash", "-c", countRunsScript)
	g.Dir = newTestWorkspace(t)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, g, 1)
	touch(t, g.Dir, ".git/config")

	time.Sleep(50 * time.Millisecond)
	expectRunCount(t, ctx, g, 1)
}

func TestGitignore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	ws := newTestWorkspace(t)
	err := os.Mkdir(filepath.Join(ws, "ignored"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: more complete tests for .gitignore semantics.
	// See: https://git-scm.com/docs/gitignore#_pattern_format

	writeFile(t, ws, ".gitignore", `ignored`)

	g := exec.CommandContext(ctx, binaryPath, "-vv", "bash", "-c", countRunsScript)
	g.Dir = ws
	g.Stdin = &bytes.Buffer{}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, g, 1)
	touch(t, ws, "ignored/somefile.txt")
	touch(t, ws, "lib/ignored")

	time.Sleep(100 * time.Millisecond)
	expectRunCount(t, ctx, g, 1)
}

func TestSendSIGINTToSleepCommandTerminates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	snap, err := NewTreeSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	g := exec.CommandContext(ctx, binaryPath, "-vv", "sleep", "infinity")
	g.Dir = newTestWorkspace(t)
	g.Stdin = &bytes.Buffer{}
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
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	snap, err := NewTreeSnapshot()
	if err != nil {
		t.Fatal(err)
	}

	g := exec.CommandContext(ctx, binaryPath, "-vv", "sh", "-c", `
	  trap "echo 'Ignoring SIGINT'" INT
	  while true; do
			echo 'Listening for SIGINT'
			sleep 0.01
		done
	`)
	g.Dir = newTestWorkspace(t)
	buf := &bytes.Buffer{}
	g.Stderr = buf
	g.Stdout = buf
	g.Stdin = &bytes.Buffer{}
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
