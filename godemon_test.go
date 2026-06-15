package godemon_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bduffany/godemon"
)

func TestWatchNextReturnsWatchedChange(t *testing.T) {
	ws := t.TempDir()
	watchedPath := filepath.Join(ws, "watched.go")
	if err := os.WriteFile(watchedPath, nil, 0644); err != nil {
		t.Fatal(err)
	}

	useGitignore := false
	w, err := godemon.Watch(godemon.Config{
		Watch:        []string{ws},
		Only:         []string{"*.go"},
		Ignore:       []string{"ignored.go"},
		UseGitignore: &useGitignore,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan godemon.FSEvent, 1)
	errCh := make(chan error, 1)
	go func() {
		event, err := w.Next(ctx)
		if err != nil {
			errCh <- err
			return
		}
		done <- event
	}()

	// An ignored change should not be returned by Next.
	time.Sleep(500 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(ws, "ignored.go"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-done:
		t.Fatalf("Next returned ignored change: %+v", event)
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(250 * time.Millisecond):
	}

	// A watched change should be returned with the public event type.
	if err := os.WriteFile(watchedPath, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-done:
		if event.Path != watchedPath {
			t.Fatalf("event path = %q, want %q", event.Path, watchedPath)
		}
		if event.Op&godemon.OpWrite == 0 {
			t.Fatalf("event op = %s, want write bit", event.Op)
		}
	case err := <-errCh:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
