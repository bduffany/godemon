package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
)

const (
	// DefaultNotifySignal is the default signal sent to a command on changes
	// (in order to get it to restart).
	DefaultNotifySignal = syscall.SIGTERM
)

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

func parseSignal(name string) (syscall.Signal, error) {
	n, err := strconv.Atoi(name)
	if err == nil {
		return syscall.Signal(n), nil
	}
	switch strings.TrimPrefix(strings.ToUpper(name), "SIG") {
	case "INT":
		return syscall.SIGINT, nil
	case "TERM":
		return syscall.SIGTERM, nil
	case "QUIT":
		return syscall.SIGQUIT, nil
	case "KILL":
		return syscall.SIGKILL, nil
	}
	return 0, fmt.Errorf("unsupported signal %q", name)
}

func printUsage() {
	fmt.Print(`
godemon: run a command and restart on changes to files/directories.

Usage:
  godemon [options...] <command> [args...]

Basic options:
  -w, --watch          set files or dirs to watch (default ".")
  -o, --only           only restart when changed paths match these patterns
  -i, --ignore         do not restart if changed paths match these patterns

Advanced options:
  --no-default-ignore  disable the default ignore list (version control dirs, etc.)
  --no-gitignore       disable adding patterns from .gitignore to ignore list
  -s, --signal         restart signal name or number (default SIGINT)
  -v, --verbose        log more info; set twice to log lower level debug info
`)
}

func parseConfig(args []string) (*Config, error) {
	cfg := &Config{}

	// Ignore arg0 (executable name)
	args = args[1:]

	if len(args) == 1 && args[0] == "help" {
		printUsage()
		os.Exit(1)
	}

	cmdArgs := []string{}
	paths := []string{}
	ignore := []string{}
	only := []string{}
	isParsingCommand := false
	noDefaultIgnore := false
	noGitignore := false
	signal := ""
	dryRun := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if isParsingCommand {
			cmdArgs = append(cmdArgs, arg)
			continue
		}

		if arg == "-h" || arg == "--help" {
			printUsage()
			os.Exit(1)
		}

		// TODO: more flexible bool parsing
		if arg == "--no-default-ignore" {
			noDefaultIgnore = true
			continue
		}
		if arg == "--no-gitignore" {
			noGitignore = true
			continue
		}
		if arg == "--verbose" {
			logLevel--
			continue
		}
		if arg == "--dry-run" {
			dryRun = true
			continue
		}
		if arg == "-v" || arg == "-vv" || arg == "-vvv" || arg == "-vvvv" {
			logLevel -= len(arg) - 1
			continue
		}

		for _, name := range []string{"-s", "--signal"} {
			if arg == name {
				i++
				arg = args[i]
				signal = arg
				goto nextarg
			}
			if strings.HasPrefix(arg, name+"=") {
				arg = strings.TrimPrefix(arg, name+"=")
				signal = arg
				goto nextarg
			}
		}

		// Parse string slice flags
		// TODO: Simplify this flag parsing
		for _, spec := range []struct {
			Short, Long string
			Dest        *[]string
		}{
			{"-w", "--watch", &paths},
			{"-o", "--only", &only},
			{"-i", "--ignore", &ignore},
		} {
			for _, name := range []string{spec.Short, spec.Long} {
				if arg == name {
					i++
					arg = args[i]
					*spec.Dest = append(*spec.Dest, arg)
					goto nextarg
				}
				if strings.HasPrefix(arg, name+"=") {
					arg = strings.TrimPrefix(arg, name+"=")
					*spec.Dest = append(*spec.Dest, arg)
					goto nextarg
				}
			}
		}

		if strings.HasPrefix(arg, "-") {
			return nil, fmt.Errorf("unrecognized option %q", arg)
		}

		cmdArgs = append(cmdArgs, arg)
		isParsingCommand = true

		// TODO: Parse -c,--config and merge config file
	nextarg:
	}

	// TODO: Validate glob patterns
	// TODO: Convert all paths to use "/" as path separator

	cfg.Command = cmdArgs
	cfg.Ignore = ignore
	cfg.Only = only
	if cfg.Watch != nil && len(cfg.Watch) == 0 {
		return nil, fmt.Errorf("watch list in config is empty")
	}
	if cfg.Watch == nil {
		cfg.Watch = paths
	}
	if len(cfg.Watch) == 0 {
		cfg.Watch = []string{"."}
	}
	for i := range cfg.Watch {
		cfg.Watch[i] = filepath.Clean(cfg.Watch[i])
		abs, err := filepath.Abs(cfg.Watch[i])
		if err != nil {
			return nil, err
		}
		cfg.Watch[i] = abs
	}

	if cfg.UseDefaultIgnoreList == nil {
		val := true
		cfg.UseDefaultIgnoreList = &val
	}
	if noDefaultIgnore {
		cfg.UseDefaultIgnoreList = pointerTo(false)
	}
	if *cfg.UseDefaultIgnoreList {
		cfg.Ignore = append(cfg.Ignore, defaultIgnorePatterns...)
	}

	// Patterns in the config or on the command line can only be relative patterns
	// for now.
	cfg.ignorePatterns = relativePatterns(cfg.Ignore)
	cfg.onlyPatterns = relativePatterns(cfg.Only)

	if cfg.UseGitignore == nil {
		cfg.UseGitignore = pointerTo(true)
	}
	if noGitignore {
		cfg.UseGitignore = pointerTo(false)
	}
	if *cfg.UseGitignore {
		for _, w := range cfg.Watch {
			if err := addGitignorePatterns(cfg, w); err != nil {
				return nil, fmt.Errorf("failed to add .gitignore patterns: %s", err)
			}
		}
	}

	if signal != "" {
		cfg.NotifySignal = &signal
	}
	if cfg.NotifySignal == nil {
		cfg.notifySignal = DefaultNotifySignal
	} else {
		s, err := parseSignal(*cfg.NotifySignal)
		if err != nil {
			return nil, err
		}
		cfg.notifySignal = s
	}
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("must specify a command to run")
	}
	cfg.dryRun = dryRun

	debugf("Configured ignore patterns:")
	for _, p := range cfg.ignorePatterns {
		t := ""
		if p.Absolute {
			t = "(absolute)"
		}
		debugf("- %q %s", p.Expr, t)
	}

	return cfg, nil
}

type pattern struct {
	// Expr is the glob expression matched by this pattern, with .gitignore
	// semantics.
	Expr string
	// Absolute modifier. If set, the semantics of a leading "/" are modified to
	// mean "relative to the FS root", rather than relative to the parent watch
	// dir.
	Absolute bool
}

func relativePatterns(exprs []string) []*pattern {
	out := make([]*pattern, 0, len(exprs))
	for _, e := range exprs {
		out = append(out, &pattern{Expr: e})
	}
	return out
}

func addGitignorePatterns(cfg *Config, path string) error {
	// TODO: Handle negation (!)
	// TODO: Traverse upwards to find .gitignore, rather than assuming we're in
	// the repo root.

	dotGitExists, err := exists(filepath.Join(path, ".git"))
	if err != nil {
		return err
	}
	if !dotGitExists {
		return nil
	}
	// TODO: Use `git ls-files` to find all .gitignore paths in the repo, and
	// incorporate those too.
	b, err := os.ReadFile(filepath.Join(path, ".gitignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	patterns := parseGitignore(b, path)
	cfg.ignorePatterns = append(cfg.ignorePatterns, patterns...)
	return nil
}

func parseGitignore(b []byte, path string) []*pattern {
	var patterns []*pattern
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		expr := line
		// Patterns like "foo", "*.txt" become "**/foo", "**/*.txt"
		if !strings.HasPrefix(expr, "/") && !strings.HasPrefix(expr, "**/") {
			expr = "**/" + expr
		}
		if !strings.HasSuffix(expr, "*") {
			expr = expr + "/**"
		}
		p := &pattern{
			Expr:     filepath.Join(path, expr),
			Absolute: true,
		}
		patterns = append(patterns, p)
	}
	return patterns
}

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

var (
	multipleSlashes = regexp.MustCompile(`/+`)
)

func pathJoin(paths ...string) string {
	joined := strings.Join(paths, "/")
	joined = multipleSlashes.ReplaceAllLiteralString(joined, "/")
	return joined
}

func toAbsolutePattern(parent string, p *pattern) string {
	if p.Absolute {
		return p.Expr
	}
	pattern := p.Expr
	// Paths starting with / or ./ are interpreted directly relative
	// to the parent, matching .gitignore behavior
	if strings.HasPrefix(pattern, "./") {
		pattern = pattern[1:]
	}
	if strings.HasPrefix(pattern, "/") {
		return pathJoin(parent, pattern)
	}
	// Paths not starting with "/" are reinterpreted to be
	// under any child dir, if not explicitly starting with **/
	if !strings.HasPrefix(pattern, "**/") {
		return pathJoin(parent, "**", pattern)
	}
	return pathJoin(parent, pattern)
}

type Cmd struct {
	*exec.Cmd
	stdin     *StdinRouter
	stdinPipe *io.PipeReader

	willShutdown bool

	waitOnce sync.Once
	waitErr  error
}

func newCommand(stdin *StdinRouter, cfg *Config) *Cmd {
	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return &Cmd{Cmd: cmd, stdin: stdin}
}

func (c *Cmd) Shutdown(s syscall.Signal) error {
	c.willShutdown = true
	if err := c.Process.Signal(s); err != nil {
		if err == os.ErrProcessDone {
			debugf("Signal() failed: %s", err)
		} else {
			errorf("Signal() failed: %s", err)
			// TODO: crash here?
		}
	}
	err := c.Wait()
	debugf("Wait() error: %s", err)
	return nil
}

func (c *Cmd) Start() error {
	c.stdinPipe = c.stdin.Reader()
	c.Cmd.Stdin = c.stdinPipe
	if err := c.Cmd.Start(); err != nil {
		return err
	}
	go c.Wait()
	return nil
}

func (c *Cmd) Wait() error {
	c.waitOnce.Do(func() {
		debugf("c.Wait()")
		s, err := c.Cmd.Process.Wait()
		c.waitErr = err
		c.stdinPipe.Close()
		if !c.willShutdown {
			info := ""
			if s, ok := s.Sys().(syscall.WaitStatus); ok {
				if s.Exited() {
					info = fmt.Sprintf(" (exit code %d)", s.ExitStatus())
				} else if s.Signaled() {
					info = fmt.Sprintf(" (%s)", s.Signal())
				}
			}
			notifyf("Command finished%s", info)
		}
	})
	return c.waitErr
}

type godemon struct {
	cfg *Config
	w   *fsnotify.Watcher
}

func (g *godemon) loopCommand(restart <-chan struct{}, shutdownCh chan<- struct{}) {
	// Small buffer in case we get signals before the command starts.
	sig := make(chan os.Signal, 4)
	signal.Notify(sig)

	nShutdownAttempts := 0
	var mu sync.Mutex

	stdin := NewStdinRouter()

	cmd := newCommand(stdin, g.cfg)
	if err := cmd.Start(); err != nil {
		fatalf("Could not start command: %s", err)
	}

	// Signal handler
	go func() {
		for s := range sig {
			if s == syscall.SIGIO {
				debugf("Skipping signal %d %q", s, s)
				continue
			}
			s := s
			isShutdown := false
			mu.Lock()
			cmd := cmd
			if s == syscall.SIGINT || s == syscall.SIGTERM || s == syscall.SIGQUIT {
				isShutdown = true
				nShutdownAttempts++
			}
			n := nShutdownAttempts
			mu.Unlock()

			if isShutdown {
				select {
				case shutdownCh <- struct{}{}:
				default:
				}
			}

			// TODO: Test in non-Unix environments
			if isShutdown && n == 3 {
				debugf("Escalating to SIGKILL")
				if err := KillProcessTree(cmd.Process.Pid); err != nil {
					debugf("Failed to clean up process tree: %s", err)
				}
				debugf("Killed process tree for pid %d", cmd.Process.Pid)
				os.Exit(2)
			}

			// Forward the signal and wait for shutdown in the background so that we
			// can quickly respond to subsequent signals.
			go func() {
				debugf("Forwarding signal %d %q", s, s)
				if err := cmd.Process.Signal(s); err != nil {
					debugf("Failed to forward signal %s: %s", s, err)
				}

				if isShutdown && n == 1 {
					debugf("Waiting for command to exit.")
					if err := cmd.Wait(); err != nil {
						debugf("Command exited with err (expected): %s", err)
					} else {
						debugf("Command exited")
					}
					fmt.Println()
					os.Exit(2)
				}
				if isShutdown && n == 2 {
					notifyf("Command is still shutting down (Ctrl+C again to force)")
					return
				}
			}()
		}
	}()

	for range restart {
		mu.Lock()
		current := cmd
		// If shutting down, ignore restart attempts and just wait
		// for the program to be terminated.
		if nShutdownAttempts > 0 {
			mu.Unlock()
			continue
		}
		mu.Unlock()

		notifyf("Restarting due to changes")
		debugf("Sending notify signal: %s", g.cfg.notifySignal)

		_ = current.Shutdown(g.cfg.notifySignal)

		next := newCommand(stdin, g.cfg)
		if err := next.Start(); err != nil {
			fatalf("Could not start command: %s", err)
		}

		mu.Lock()
		cmd = next
		mu.Unlock()
	}
}

func (g *godemon) Start() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer func() {
		debugf("Removing file watchers")
		w.Close()
		select {} // Wait for shutdown to finish.
	}()
	g.w = w

	addCh := make(chan string, 100_000)

	go g.handleAdds(addCh)

	for _, path := range g.cfg.Watch {
		abs, err := filepath.Abs(path)
		if err != nil {
			fatalf("%s", err)
		}
		addCh <- abs
	}

	events := make(chan struct{}, 1024)
	defer close(events)

	shutdownCh := make(chan struct{}, 1)

	// Command worker
	restart := throttleRestarts(events)

	if g.cfg.dryRun {
		// Allow 1s for file watchers to be set up.
		// TODO: Wait till addCh settles instead.
		time.Sleep(1 * time.Second)
		notifyf("Exiting due to --dry-run flag.")
	}

	go g.loopCommand(restart, shutdownCh)

	// Wait for events and dispatch to the appropriate handler.
	g.handleEvents(addCh, events, shutdownCh)

	return nil
}

func (g *godemon) shouldIgnore(path string) bool {
	// Determine which watched parent dir the path falls under.
	// TODO: Handle overlapping watch paths.
	// TODO: Handle file watch paths.

	// TODO: Can this be simplified by instead appending all ignore patterns to
	// the watch paths ahead of time, then just doing a simple prefix check?
	parent := ""
	for _, w := range g.cfg.Watch {
		// Never ignore explicitly watched paths.
		// TODO: Consider allowing ignore patterns to act as override flags to
		// prevent earlier explicitly watched paths from actually being watched.
		if w == path {
			debugf("not ignoring explicitly watched path %s", path)
			return false
		}
		// TODO: Cache this syscall result?
		if !isDir(w) {
			continue
		}
		if w == path || strings.HasPrefix(path, w+"/") {
			parent = w
			break
		}
		debugf("Ruled out %q as parent watch path", w)
	}
	if parent == "" {
		errorf("Could not determine parent watch path of %q", path)
		return true
	}
	debugf("parent watch path of %s: %s", path, parent)
	// If explicitly ignored, do not watch.
	for _, p := range g.cfg.ignorePatterns {
		pattern := toAbsolutePattern(parent, p)
		match, err := doublestar.Match(pattern, path)
		if err != nil {
			warnf("%s", err)
			continue
		}
		if match {
			return true
		}
	}
	// If there are no "--only" patterns, then the path shouldn't be ignored,
	// since the input path to this func must have fallen under the watched dirs.
	if len(g.cfg.Only) == 0 {
		return false
	}
	// If this is a dir, return whether any of the "--only" patterns might fall
	// under the dir.

	// TODO: Handle symlinks.
	if isDir(path) {
		// If path is /foo/bar and we are watching only /foo, and absolute watch
		// patterns are /foo/**/*.go, Then /foo/bar should be watched because
		// /foo/bar/ matches /foo/**/
		for _, p := range g.cfg.onlyPatterns {
			pattern := toAbsolutePattern(parent, p)
			if strings.Contains(pattern, "/**/") {
				// Delete everything after the first occurrence of "/**/"
				i := strings.Index(pattern, "/**/")
				pattern = pattern[:i+len("/**/")]
				// Check whether this modified pattern matches the dir path
				match, err := doublestar.Match(pattern, path+"/")
				if err != nil {
					warnf("%s", err)
					continue
				}
				if match {
					return false
				}
			}
		}
		return true
	}
	// If this is a file, return whether it matches any of the --only patterns.
	for _, p := range g.cfg.onlyPatterns {
		pattern := toAbsolutePattern(parent, p)
		match, err := doublestar.Match(pattern, path)
		if err != nil {
			warnf("%s", err)
			continue
		}
		if match {
			return false
		}
	}
	return true
}

func (g *godemon) handleAdds(addCh chan string) {
	for i := 0; i < 16; i++ {
		go func() {
			for {
				path := <-addCh
				if g.shouldIgnore(path) {
					infof("Not watching ignored path %q", path)
					continue
				}
				g.w.Add(path)
				infof("Watching %q", path)

				if !isDir(path) {
					continue
				}
				entries, err := os.ReadDir(path)
				if err != nil {
					warnf("%s", err)
					continue
				}
				for _, entry := range entries {
					absPath := pathJoin(path, entry.Name())
					if entry.IsDir() {
						select {
						case addCh <- absPath:
						default:
							warnf("Too many files being added at once; some paths may not be watched.")
						}
					}
					// TODO: follow symlinks in the directory. Make sure to detect
					// cycles
				}
			}
		}()
	}
}

func (g *godemon) handleEvents(addCh chan<- string, restartCh chan<- struct{}, shutdownCh <-chan struct{}) {
	for {
		select {
		case err := <-g.w.Errors:
			warnf("%s", err)
		case event := <-g.w.Events:
			if g.shouldIgnore(event.Name) {
				infof("Ignoring event: %s %q", event.Op, event.Name)
				continue
			}
			infof("Got event: %s %q", event.Op, event.Name)
			// When creating new dirs, add them to the watch list.
			if event.Op == fsnotify.Create && isDir(event.Name) {
				addCh <- event.Name
			} else {
				restartCh <- struct{}{}
			}
		case <-shutdownCh:
			return
		}
	}
}

func Main(args []string) {
	cfg, err := parseConfig(args)
	if err != nil {
		fmt.Printf("%s: %s\n", os.Args[0], err)
		fmt.Println("Try \"godemon --help\" for more information.")
		os.Exit(1)
	}

	g := &godemon{cfg: cfg}
	if err := g.Start(); err != nil {
		fatalf("%s", err)
	}
}

func main() {
	Main(os.Args)
}
