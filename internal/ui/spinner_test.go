package ui

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuf is a thread-safe buffer for tests that read while animate is running.
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitForContent polls buf until substr appears or a 2s deadline is exceeded.
func waitForContent(t *testing.T, buf *syncBuf, substr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), substr) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in spinner output; got: %q", substr, buf.String())
}

func TestSpinner_StartStop(t *testing.T) {
	buf := &syncBuf{}
	s := NewSpinner()
	s.out = buf

	s.Start("loading...")
	waitForContent(t, buf, "loading...")
	s.Stop()
}

func TestSpinner_UpdateStatus(t *testing.T) {
	buf := &syncBuf{}
	s := NewSpinner()
	s.out = buf

	s.Start("first")
	waitForContent(t, buf, "first")
	s.UpdateStatus("second")
	waitForContent(t, buf, "second")
	s.Stop()
}

func TestSpinner_StopWith(t *testing.T) {
	buf := &syncBuf{}
	s := NewSpinner()
	s.out = buf

	s.Start("working")
	s.StopWith("done!")

	// StopWith writes the final line itself after wg.Wait; no sleep needed.
	output := buf.String()
	if !strings.Contains(output, "done!") {
		t.Errorf("expected final message in output, got: %q", output)
	}
	if !strings.Contains(output, "✓") {
		t.Errorf("expected checkmark in output, got: %q", output)
	}
}

func TestSpinner_StopWithWarning(t *testing.T) {
	buf := &syncBuf{}
	s := NewSpinner()
	s.out = buf

	s.Start("working")
	s.StopWithWarning("skipped")

	// StopWithWarning writes the final line itself after wg.Wait; no sleep needed.
	output := buf.String()
	if !strings.Contains(output, "skipped") {
		t.Errorf("expected warning message in output, got: %q", output)
	}
	if !strings.Contains(output, "⚠") {
		t.Errorf("expected warning symbol in output, got: %q", output)
	}
}

func TestSpinner_StartTwice(t *testing.T) {
	buf := &syncBuf{}
	s := NewSpinner()
	s.out = buf

	s.Start("first")
	waitForContent(t, buf, "first")
	// Second Start must stop the first goroutine before starting a new one.
	s.Start("second")
	waitForContent(t, buf, "second")
	s.Stop()
}

func TestSpinner_StopIdempotent(t *testing.T) {
	t.Run("stop variants are safe before start", func(_ *testing.T) {
		s := NewSpinner()
		s.out = &bytes.Buffer{}
		s.Stop()
		s.StopWith("msg")
		s.StopWithWarning("msg")
	})

	t.Run("stop is safe to call multiple times after start", func(_ *testing.T) {
		s := NewSpinner()
		buf := &syncBuf{}
		s.out = buf
		s.Start("working")
		waitForContent(t, buf, "working")
		s.Stop()
		s.Stop()
	})

	t.Run("stop variants are safe after spinner has already been stopped", func(_ *testing.T) {
		s := NewSpinner()
		buf := &syncBuf{}
		s.out = buf
		s.Start("working")
		waitForContent(t, buf, "working")
		s.StopWith("done")
		s.StopWith("done again")
		s.StopWithWarning("warning")
	})
}

func TestSpinner_ImplementsStatusUpdater(_ *testing.T) {
	var _ interface{ UpdateStatus(string) } = (*Spinner)(nil)
}
