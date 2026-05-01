// Package ui provides terminal UI components for cfgctl.
package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	frames       = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	doneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	messageStyle = lipgloss.NewStyle()
)

// Spinner provides an animated terminal spinner with a dynamic message.
type Spinner struct {
	mu      sync.Mutex
	message string
	done    chan struct{}
	out     io.Writer
}

// NewSpinner creates a new Spinner that writes to stderr.
func NewSpinner() *Spinner {
	return &Spinner{out: os.Stderr}
}

// Start begins the spinner animation with the given initial message.
func (s *Spinner) Start(msg string) {
	s.mu.Lock()
	s.message = msg
	s.done = make(chan struct{})
	s.mu.Unlock()

	go s.animate()
}

// UpdateStatus updates the spinner message. Safe for concurrent use.
func (s *Spinner) UpdateStatus(msg string) {
	s.mu.Lock()
	s.message = msg
	s.mu.Unlock()
}

// Stop halts the spinner and clears the line.
func (s *Spinner) Stop() {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()

	if done == nil {
		return
	}
	close(done)
	_, _ = fmt.Fprint(s.out, "\r\033[K")
}

// StopWith halts the spinner and prints a final message on the line.
func (s *Spinner) StopWith(msg string) {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()

	if done == nil {
		return
	}
	close(done)
	_, _ = fmt.Fprintf(s.out, "\r\033[K%s %s\n", doneStyle.Render("✓"), messageStyle.Render(msg))
}

// StopWithWarning halts the spinner and prints a warning-styled final message.
func (s *Spinner) StopWithWarning(msg string) {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()

	if done == nil {
		return
	}
	close(done)
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	_, _ = fmt.Fprintf(s.out, "\r\033[K%s %s\n", warnStyle.Render("⚠"), messageStyle.Render(msg))
}

func (s *Spinner) animate() {
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			msg := s.message
			s.mu.Unlock()

			frame := spinnerStyle.Render(frames[i%len(frames)])
			_, _ = fmt.Fprintf(s.out, "\r\033[K%s %s", frame, messageStyle.Render(msg))
			i++
		}
	}
}
