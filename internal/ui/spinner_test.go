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

func TestSpinner_StopIdempotent(_ *testing.T) {
	s := NewSpinner()
	s.out = &bytes.Buffer{}

	s.Stop()
	s.StopWith("msg")
	s.StopWithWarning("msg")
}

func TestSpinner_ImplementsStatusUpdater(_ *testing.T) {
	s := NewSpinner()
	fn := s.UpdateStatus
	_ = fn
}
