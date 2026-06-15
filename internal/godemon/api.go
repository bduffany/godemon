package godemon

import (
	"context"
	"io"
	"sync"
)

// Watcher yields filesystem change notifications.
type Watcher interface {
	// Next waits until the next filesystem change is observed. If ctx is
	// canceled before a change is observed, Next stops the watcher and returns
	// ctx.Err().
	Next(ctx context.Context) (FSEvent, error)
}

type apiWatcher struct {
	g         *godemon
	addCh     chan string
	done      chan struct{}
	closeOnce sync.Once
}

// Watch starts watching files according to cfg.
func Watch(cfg Config) (Watcher, error) {
	if err := cfg.prepare(); err != nil {
		return nil, err
	}

	w, err := newWatcher()
	if err != nil {
		return nil, err
	}

	g := &godemon{
		cfg: &cfg,
		w:   w,
	}
	addCh := make(chan string, 100_000)
	done := make(chan struct{})
	go g.handleAdds(addCh, done)

	for _, path := range cfg.Watch {
		addCh <- path
	}

	return &apiWatcher{
		g:     g,
		addCh: addCh,
		done:  done,
	}, nil
}

func (w *apiWatcher) close() {
	w.closeOnce.Do(func() {
		close(w.done)
		_ = w.g.w.Close()
	})
}

func (w *apiWatcher) Next(ctx context.Context) (FSEvent, error) {
	for {
		select {
		case <-ctx.Done():
			w.close()
			return FSEvent{}, ctx.Err()
		case <-w.done:
			return FSEvent{}, io.EOF
		case err, ok := <-w.g.w.Errors():
			if !ok {
				w.close()
				return FSEvent{}, io.EOF
			}
			return FSEvent{}, err
		case event, ok := <-w.g.w.Events():
			if !ok {
				w.close()
				return FSEvent{}, io.EOF
			}
			if w.g.handleChange(w.addCh, event) {
				return event, nil
			}
		}
	}
}
