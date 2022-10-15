package main

import (
	"io"
	"os"
	"sync"
)

// StdinRouter reads from stdin and sends the input to the currently active
// command.
//
// It warns when input is received and there is no currently active command
// to handle the input.
type StdinRouter struct {
	pw *io.PipeWriter
	mu sync.Mutex
	// closed indicates whether stdin is NOT from a terminal and we have received
	// EOF (when stdin is from a terminal, we don't ever consider stdin closed,
	// since the user can send EOF via Ctrl+D on subsequent command restarts).
	closed bool
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
				debugf("stdin read error: %s", err)
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

			if len(b) > 0 {
				_, err = pw.Write(b)
				if err == io.ErrClosedPipe {
					debugf("failed to write to command stdin: %s", err)
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
			}

			if eof {
				pw.Close()
				fi, err := os.Stdin.Stat()
				if err != nil {
					debugf("stat stdin failed: %s", err)
				} else if (fi.Mode() & os.ModeCharDevice) == 0 {
					debugf("stdin is not from a terminal; permanently closing stdin.")
					p.mu.Lock()
					p.closed = true
					p.mu.Unlock()
					return
				} else {
					debugf("stdin is from a terminal; will continue streaming.")
				}
			}

			warned = false
		}
	}()
}

// Reader returns a reader from os.Stdin.
//
// The returned PipeReader must be closed after calling cmd.Process.Wait(), NOT
// cmd.Wait(), since cmd.Wait() will wait until the command exhausts os.Stdin.
func (p *StdinRouter) Reader() *io.PipeReader {
	p.mu.Lock()
	defer p.mu.Unlock()

	pr, pw := io.Pipe()
	p.pw = pw

	if p.closed {
		pw.Close()
	}

	return pr
}
