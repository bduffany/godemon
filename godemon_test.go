// Integration tests for the godemon CLI.
// These tests invoke the built godemon binary.
package godemon

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

var (
	binaryPath string
)

const (
	// countRunsScript is a shell script that counts the number of times
	// it was invoked, using the file ../count
	countRunsScript = `
		while ! [[ -e ./count ]]; do
			cd ..
			if [[ "$PWD" == "/" ]]; then
				echo >&2 "Failed to find ./count file!"
				exit 1
			fi
		done
		count=$(cat ./count)
		echo $(( count + 1 )) > ./count

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
	//   - count               # keeps track of how many times countRunsScript was executed
	//   - workspace/          # godemon working dir
	//     -                   # `fnames` are written here
	writeFile(t, root, "count", "0")
	ws := filepath.Join(root, "workspace")
	fnames := []string{
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
	git := exec.Command("sh", "-c", `
		git init
		git config --local user.email "test@example.com"
		git config --local user.name "Test"
		git add .gitignore
		git commit -m "Initial commit with .gitignore"
	`)
	git.Dir = ws
	b, err := git.CombinedOutput()
	if err != nil {
		t.Log(string(b))
		t.Fatalf("failed to initialize git repository: %s", err)
	}
	return ws
}

func runCount(t *testing.T, ws string) int {
	path := filepath.Join(ws, "../count")
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

func expectRunCount(t *testing.T, ctx context.Context, ws string, count int) {
	var c int
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for run count to equal %d (last count: %d)", count, c)
		default:
		}
		c = runCount(t, ws)
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

	ws := newTestWorkspace(t)
	g := exec.CommandContext(ctx, binaryPath, "-vv", "bash", "-c", countRunsScript)
	g.Dir = ws
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, ws, 1)
	touch(t, g.Dir, "NEW.go")

	expectRunCount(t, ctx, ws, 2)
}

func TestRestartOnEdit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	ws := newTestWorkspace(t)
	g := exec.CommandContext(ctx, binaryPath, "-vv", "bash", "-c", countRunsScript)
	g.Dir = ws
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, ws, 1)
	touch(t, g.Dir, "toplevel.go")

	expectRunCount(t, ctx, ws, 2)
}

func TestWatchSingleFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	ws := newTestWorkspace(t)
	g := exec.CommandContext(ctx, binaryPath, "-w", "./toplevel.go", "-vv", "bash", "-c", countRunsScript)
	g.Dir = ws
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, ws, 1)
	touch(t, g.Dir, "toplevel.go")

	expectRunCount(t, ctx, ws, 2)
}

func TestDefaultIgnoreList(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	ws := newTestWorkspace(t)
	g := exec.CommandContext(ctx, binaryPath, "-vv", "bash", "-c", countRunsScript)
	g.Dir = ws
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, ws, 1)
	touch(t, g.Dir, ".git/config")

	time.Sleep(50 * time.Millisecond)
	expectRunCount(t, ctx, ws, 1)
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
	expectRunCount(t, ctx, ws, 1)
	touch(t, ws, "ignored/somefile.txt")
	touch(t, ws, "lib/ignored")

	time.Sleep(100 * time.Millisecond)
	expectRunCount(t, ctx, ws, 1)
}

func TestGitignoreInParentDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	ws := newTestWorkspace(t)
	err := os.Mkdir(filepath.Join(ws, "ignored"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, ws, ".gitignore", `ignored`)

	g := exec.CommandContext(ctx, binaryPath, "-vv", "bash", "-c", countRunsScript)
	g.Dir = filepath.Join(ws, "lib")
	g.Stdin = &bytes.Buffer{}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, ws, 1)
	touch(t, ws, "lib/ignored")

	time.Sleep(100 * time.Millisecond)
	expectRunCount(t, ctx, ws, 1)
}

func TestGitignoreInChildDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	ws := newTestWorkspace(t)

	writeFile(t, ws, "lib/.gitignore", `ignored`)

	g := exec.CommandContext(ctx, binaryPath, "-vv", "bash", "-c", countRunsScript)
	g.Dir = filepath.Join(ws, "lib")
	g.Stdin = &bytes.Buffer{}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, ws, 1)

	touch(t, ws, "lib/ignored")

	time.Sleep(75 * time.Millisecond)
	expectRunCount(t, ctx, ws, 1)

	touch(t, ws, "lib/nested/ignored")

	time.Sleep(75 * time.Millisecond)
	expectRunCount(t, ctx, ws, 1)
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

func TestStdioIsTTY(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	g := exec.CommandContext(ctx, binaryPath, "-vv", "python3", "-c", `
import sys

values = [f.isatty() for f in (sys.stdin, sys.stdout, sys.stderr)]
print('isatty: stdin={}, stdout={}, stderr={}'.format(*values))
`)
	g.Dir = newTestWorkspace(t)

	ptmx, err := pty.Start(g)
	if err != nil {
		t.Fatal(err)
	}
	buf := bytes.NewBuffer(nil)
	go io.Copy(buf, ptmx)

	for ctx.Err() == nil && !strings.Contains(buf.String(), "isatty:") {
		time.Sleep(5 * time.Millisecond)
	}
	if err := ctx.Err(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(buf.String(), "\n")
	ttyLine := ""
	for _, line := range lines {
		if strings.Contains(line, "isatty:") {
			ttyLine = strings.TrimRight(line, "\r")
			break
		}
	}

	if ttyLine != "isatty: stdin=True, stdout=True, stderr=True" {
		t.Fatalf("stdin, stdout, stderr should all be a tty; but child process reported %q", ttyLine)
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
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
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
		time.Sleep(5 * time.Millisecond)
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

func TestLockfile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	ws := newTestWorkspace(t)
	lockPath := filepath.Join(ws, "../godemon.lock")

	g := exec.CommandContext(ctx, binaryPath, "--lockfile="+lockPath, "-w", "./toplevel.go", "-vv", "bash", "-c", countRunsScript)
	g.Dir = ws
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, ws, 1)
	touch(t, ws, "../godemon.lock")
	touch(t, g.Dir, "toplevel.go")

	time.Sleep(25 * time.Millisecond)
	expectRunCount(t, ctx, ws, 1)

	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	expectRunCount(t, ctx, ws, 2)
}
