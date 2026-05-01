package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSpinner_StartStop(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner()
	s.out = &buf

	s.Start("loading...")
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	output := buf.String()
	if !strings.Contains(output, "loading...") {
		t.Errorf("expected spinner output to contain message, got: %q", output)
	}
}

func TestSpinner_UpdateStatus(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner()
	s.out = &buf

	s.Start("first")
	time.Sleep(100 * time.Millisecond)
	s.UpdateStatus("second")
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	output := buf.String()
	if !strings.Contains(output, "second") {
		t.Errorf("expected updated message in output, got: %q", output)
	}
}

func TestSpinner_StopWith(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner()
	s.out = &buf

	s.Start("working")
	time.Sleep(100 * time.Millisecond)
	s.StopWith("done!")

	output := buf.String()
	if !strings.Contains(output, "done!") {
		t.Errorf("expected final message in output, got: %q", output)
	}
	if !strings.Contains(output, "✓") {
		t.Errorf("expected checkmark in output, got: %q", output)
	}
}

func TestSpinner_StopWithWarning(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner()
	s.out = &buf

	s.Start("working")
	time.Sleep(100 * time.Millisecond)
	s.StopWithWarning("skipped")

	output := buf.String()
	if !strings.Contains(output, "skipped") {
		t.Errorf("expected warning message in output, got: %q", output)
	}
	if !strings.Contains(output, "⚠") {
		t.Errorf("expected warning symbol in output, got: %q", output)
	}
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
		s.out = &bytes.Buffer{}
		s.Start("working")
		time.Sleep(100 * time.Millisecond)
		s.Stop()
		s.Stop()
	})

	t.Run("stop variants are safe after spinner has already been stopped", func(_ *testing.T) {
		s := NewSpinner()
		s.out = &bytes.Buffer{}
		s.Start("working")
		time.Sleep(100 * time.Millisecond)
		s.StopWith("done")
		s.StopWith("done again")
		s.StopWithWarning("warning")
	})
}

func TestSpinner_ImplementsStatusUpdater(_ *testing.T) {
	var _ interface{ UpdateStatus(string) } = (*Spinner)(nil)
}
