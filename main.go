package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
)

var (
	defaultIgnorePatterns = []string{
		// Version control
		"**/.git/**",
		"**/.hg/**",
		"**/.svn/**",
		"**/CVS/**",
		// Build artifacts
		"**/node_modules/**",
		"**/bazel-*/**",
		"**/__pycache__/**",
		// Editor artifacts
		"**/*.swp",
		"**/*.swx",
	}

	logLevels = map[string]int{
		"DEBUG":   0,
		"INFO":    1,
		"NOTIFY":  2,
		"WARNING": 3,
		"ERROR":   4,
		"FATAL":   5,
	}
	logLevel = logLevels["NOTIFY"]
)

func logf(level, format string, args ...interface{}) {
	if logLevels[level] < logLevel {
		return
	}
	const gray = "\x1b[90m"
	const reset = "\x1b[0m"
	prefix := "[godemon] "
	// Don't show NOTIFY prefix since these are very common and are intended
	// to be user friendly.
	if level != "NOTIFY" {
		prefix += level + ": "
	}
	fmt.Printf(gray+prefix+format+reset+"\n", args...)
	if level == "FATAL" {
		os.Exit(1)
	}
}

func debugf(format string, args ...interface{}) {
	logf("DEBUG", format, args...)
}
func infof(format string, args ...interface{}) {
	logf("INFO", format, args...)
}
func notifyf(format string, args ...interface{}) {
	logf("NOTIFY", format, args...)
}
func warnf(format string, args ...interface{}) {
	logf("WARNING", format, args...)
}
func errorf(format string, args ...interface{}) {
	logf("ERROR", format, args...)
}
func fatalf(format string, args ...interface{}) {
	logf("FATAL", format, args...)
}

type Config struct {
	// Command is the command and arguments to be executed upon any change.
	//
	// The command is not interpreted using a shell. If you would like shell
	// features such as environment variable expansion, specify the command
	// using something like the following:
	//
	//     ["sh", "-c", "$YOUR_COMMAND"]
	Command []string `json:"command"`

	// Watch specifies a list of files or directories to be watched. Defaults to
	// the current working directory from which the command is invoked.
	// Directories are watched recursively.
	// TODO(bduffany): Accept glob patterns here.
	Watch []string `json:"watch,omitempty"`
	// Only specifies a list of allowed patterns. If non-empty, at least one
	// pattern must match in order for the command to be executed.
	Only []string `json:"only,omitempty"`
	// Ignore specifies a list of paths to be ignored. Glob patterns are supported.
	Ignore []string `json:"ignore,omitempty"`

	// UseDefaultIgnoreList specifies whether to use the default list of
	// ignore patterns. These will be appended to the list of ignore patterns.
	// Defaults to true.
	UseDefaultIgnoreList *bool `json:"useDefaultIgnoreList,omitempty"`
	// NotifySignal is the signal used to notify the command of file changes.
	// Defaults to "SIGINT" (Ctrl+C), which should gracefully stop most
	// well-behaved commands.
	NotifySignal *string `json:"notifySignal,omitempty"`
	notifySignal syscall.Signal

	// TODO: FollowSymlinks
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
	return 0, fmt.Errorf("Unsupported signal %q", name)
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
  --no-default-ignore  disable the default ignore list (.git, etc.)
  -s, --signal         restart signal name or number (default SIGINT)
  -v, --verbose        log more info; set twice to log lower level debug info
`)
}

func parseConfig() (*Config, error) {
	cfg := &Config{}

	args := os.Args[1:]

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
	signal := ""
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
		if arg == "-v" || arg == "--verbose" {
			logLevel--
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
	if cfg.UseDefaultIgnoreList == nil {
		val := true
		cfg.UseDefaultIgnoreList = &val
	}
	if *cfg.UseDefaultIgnoreList {
		cfg.Ignore = append(cfg.Ignore, defaultIgnorePatterns...)
	}
	if signal != "" {
		cfg.NotifySignal = &signal
	}
	if cfg.NotifySignal == nil {
		cfg.notifySignal = syscall.SIGINT
	} else {
		s, err := parseSignal(*cfg.NotifySignal)
		if err != nil {
			return nil, err
		}
		cfg.notifySignal = s
	}
	if noDefaultIgnore {
		val := false
		cfg.UseDefaultIgnoreList = &val
	}
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("must specify a command to run")
	}
	return cfg, nil
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
			// Make sure we go at least 25ms with no events before the next restart
			// TODO: Make this configurable, and/or adaptive
			for {
				<-time.After(25 * time.Millisecond)
				n := nonBlockingDrain(events)
				if n == 0 {
					break
				}
			}
		}
	}()
	return restart
}

type godemon struct {
	cfg *Config
	w   *fsnotify.Watcher
}

func (g *godemon) loopCommand(restart <-chan struct{}, onShutdown chan<- struct{}) {
	// Small buffer in case we get signals before the command starts.
	sig := make(chan os.Signal, 4)
	signal.Notify(sig)

	newCommand := func() *exec.Cmd {
		cmd := exec.Command(g.cfg.Command[0], g.cfg.Command[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd
	}

	cmd := newCommand()
	if err := cmd.Start(); err != nil {
		fatalf("Could not start command: %s", err)
	}
	nShutdownAttempts := 0
	var mu sync.Mutex

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
				case onShutdown <- struct{}{}:
				default:
				}
			}

			debugf("Forwarding signal %d %q", s, s)

			// Forward the signal and wait for shutdown in the background so that we
			// can quickly respond to subsequent signals.
			go func() {
				cmd.Process.Signal(s)

				if isShutdown && n == 1 {
					cmd.Wait()
					infof("Command exited")
					os.Exit(1)
				}
				if isShutdown && n == 2 {
					notifyf("Command is still shutting down (Ctrl+C again to force)")
					return
				}
				if isShutdown && n == 3 {
					cmd.Process.Signal(syscall.SIGKILL)
					os.Exit(1)
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
		current.Process.Signal(g.cfg.notifySignal)
		if err := current.Wait(); err != nil {
			errorf("%s", err)
		}
		debugf("Command shutdown successfully.")

		next := newCommand()
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

	onShutdown := make(chan struct{}, 1)

	// Command worker
	restart := throttleRestarts(events)

	go g.loopCommand(restart, onShutdown)

	// Wait for events and dispatch to the appropriate handler.
	g.handleEvents(addCh, events, onShutdown)

	return nil
}

func (g *godemon) shouldIgnore(path string) bool {
	for _, pattern := range g.cfg.Ignore {
		match, err := doublestar.PathMatch(pattern, path)
		if err != nil {
			warnf("%s", err)
			continue
		}
		if match {
			return true
		}
	}
	if len(g.cfg.Only) == 0 {
		return false
	}
	for _, pattern := range g.cfg.Only {
		match, err := doublestar.PathMatch(pattern, path)
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
					absPath := filepath.Join(path, entry.Name())
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

func (g *godemon) handleEvents(addCh chan string, eventsCh chan struct{}, onShutdown chan struct{}) {
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
			if event.Op == fsnotify.Create {
				if isDir(event.Name) {
					addCh <- event.Name
				}
			}
			eventsCh <- struct{}{}
		case <-onShutdown:
			return
		}
	}
}

func main() {
	cfg, err := parseConfig()
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
