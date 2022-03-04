package main

import (
	"io"
	"os"
	"os/exec"
	"sync"
)

// StdinRouter reads from stdin and sends the input to the currently active
// command.
//
// It warns when input is received and there is no currently active command
// to handle the input.
type StdinRouter struct {
	pr *io.PipeReader
	pw *io.PipeWriter
	mu sync.Mutex
}

func NewStdinRouter() *StdinRouter {
	p := &StdinRouter{}
	p.start()
	return p
}

func (p *StdinRouter) start() {
	go func() {
		buf := make([]byte, 512)
		warned := false
		for {
			debugf("Reading stdin")
			n, err := os.Stdin.Read(buf)
			b := buf[:n]
			eof := err == io.EOF
			if err != nil {
				debugf("read stdin: %s", err)
			}

			p.mu.Lock()
			pw := p.pw
			p.mu.Unlock()

			if pw == nil {
				if !warned {
					warnf("no command is running; input will be dropped.")
					warned = true
				}
				continue
			}
			_, err = pw.Write(b)
			if err == io.ErrClosedPipe {
				if !warned {
					warnf("no command is running; input will be dropped.")
					warned = true
				}
				continue
			}
			if err != nil {
				errorf("Failed to forward stdin to command: %s", err)
				continue
			}

			if eof {
				pw.Close()
			}

			warned = false
		}
	}()
}

func (p *StdinRouter) SetDestination(cmd *exec.Cmd) (notifyStarted func()) {
	pr, pw := io.Pipe()
	p.pr = pr
	p.mu.Lock()
	p.pw = pw
	p.mu.Unlock()

	cmd.Stdin = pr
	return func() {
		go func() {
			// Note: Not calling cmd.Wait() here, because it will also wait for the
			// current read on the PipeReader to complete, which we don't want.
			// We want this to terminate as soon as the process exits.
			if _, err := cmd.Process.Wait(); err != nil {
				debugf("%s", err)
			}
			pr.Close()
		}()
	}
}
