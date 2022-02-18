package main

import (
	// "encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
	// "github.com/sabhiram/go-gitignore"
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

	verbose = false
)

func logf(level, format string, args ...interface{}) {
	if !verbose && (level == "DEBUG" || level == "INFO") {
		return
	}
	const gray = "\x1b[90m"
	const reset = "\x1b[0m"
	prefix := "[godemon] "
	if level != "" {
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
	logf("", format, args...)
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

	// TODO: follow_symlinks
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
  --no-default-ignore  disable the default ignore list (.git, node_modules, etc.)
  -s, --signal         restart signal name or number (default SIGINT)
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
			verbose = true
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

func drain(ch chan struct{}) int {
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

func throttleRestarts(events chan struct{}) chan struct{} {
	// Latch: buf size of one, and sender only does non-blocking sends.
	restart := make(chan struct{}, 1)
	go func() {
		defer close(restart)

		for {
			// Wait for first event since last restart
			_, ok := <-events
			if !ok {
				return
			}
			// Restart immediately
			select {
			case restart <- struct{}{}:
			default:
			}
			// Make sure we go at least 25ms with no events before the next restart
			// TODO: Make this configurable, and/or adaptive
			for {
				<-time.After(25 * time.Millisecond)
				n := drain(events)
				if n == 0 {
					break
				}
			}
		}
	}()
	return restart
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		fmt.Printf("%s: %s\n", os.Args[0], err)
		fmt.Println("Try \"godemon --help\" for more information.")
		os.Exit(1)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer w.Close()

	ignorePatterns := cfg.Ignore
	if *cfg.UseDefaultIgnoreList {
		ignorePatterns = append(ignorePatterns, defaultIgnorePatterns...)
	}

	shouldIgnore := func(path string) bool {
		for _, pattern := range ignorePatterns {
			match, err := doublestar.PathMatch(pattern, path)
			if err != nil {
				warnf("%s", err)
				continue
			}
			if match {
				return true
			}
		}
		if len(cfg.Only) == 0 {
			return false
		}
		for _, pattern := range cfg.Only {
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

	addCh := make(chan string, 100_000)
	for i := 0; i < 16; i++ {
		go func() {
			for {
				path := <-addCh
				if shouldIgnore(path) {
					infof("Not watching ignored path %q", path)
					continue
				}
				w.Add(path)
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
					// TODO: follow symlinks in the directory
				}
			}
		}()
	}

	for _, path := range cfg.Watch {
		abs, err := filepath.Abs(path)
		if err != nil {
			fatalf("%s", err)
		}
		addCh <- abs
	}

	events := make(chan struct{}, 1024)
	defer close(events)

	// Command worker
	restart := throttleRestarts(events)
	sig := make(chan os.Signal, 1)
	// TODO: Consider forwarding all signals, not just these ones.
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	newCommand := func() *exec.Cmd {
		cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd
	}

	go func() {
		cmd := newCommand()
		if err := cmd.Start(); err != nil {
			fatalf("Could not start command: %s", err)
		}
		for {
			select {
			case <-restart:
				notifyf("Restarting due to changes")
				cmd.Process.Signal(cfg.notifySignal)
				if err := cmd.Wait(); err != nil {
					warnf("%s", err)
				}
				cmd = newCommand()
				if err := cmd.Start(); err != nil {
					fatalf("Could not start command: %s", err)
				}
			case s := <-sig:
				// TODO: Escalate multiple SIGINT to SIGTERM then SIGKILL,
				// with this behavior optionally disabled by config?
				infof("Got SIGINT, forwarding to command")
				cmd.Process.Signal(s)
				cmd.Wait()
				os.Exit(1)
			}
		}
	}()

	// Wait for events and dispatch to the appropriate handler.
	for {
		select {
		case err := <-w.Errors:
			warnf("%s", err)
		case event := <-w.Events:
			if shouldIgnore(event.Name) {
				infof("Ignoring event: %s %q", event.Op, event.Name)
				continue
			}
			infof("Got event: %s %q", event.Op, event.Name)
			if event.Op == fsnotify.Create {
				if isDir(event.Name) {
					addCh <- event.Name
				}
			}
			events <- struct{}{}
		}
	}
}
