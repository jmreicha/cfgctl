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
	wg      sync.WaitGroup
	out     io.Writer
}

// NewSpinner creates a new Spinner that writes to stderr.
func NewSpinner() *Spinner {
	return &Spinner{out: os.Stderr}
}

// Start begins the spinner animation with the given initial message.
// If the spinner is already running, it is stopped first.
func (s *Spinner) Start(msg string) {
	existing := s.stopChan()
	if existing != nil {
		close(existing)
		s.wg.Wait()
	}

	s.mu.Lock()
	s.message = msg
	done := make(chan struct{})
	s.done = done
	s.mu.Unlock()

	s.wg.Add(1)
	go s.animate(done)
}

// stopChan nils out s.done under the lock and returns the channel to close.
// Returns nil if the spinner is not running.
func (s *Spinner) stopChan() chan struct{} {
	s.mu.Lock()
	done := s.done
	s.done = nil
	s.mu.Unlock()
	return done
}

// UpdateStatus updates the spinner message. Safe for concurrent use.
func (s *Spinner) UpdateStatus(msg string) {
	s.mu.Lock()
	s.message = msg
	s.mu.Unlock()
}

// Stop halts the spinner and clears the line.
func (s *Spinner) Stop() {
	done := s.stopChan()
	if done == nil {
		return
	}
	close(done)
	s.wg.Wait()
	_, _ = fmt.Fprint(s.out, "\r\033[K")
}

// StopWith halts the spinner and prints a final message on the line.
func (s *Spinner) StopWith(msg string) {
	done := s.stopChan()
	if done == nil {
		return
	}
	close(done)
	s.wg.Wait()
	_, _ = fmt.Fprintf(s.out, "\r\033[K%s %s\n", doneStyle.Render("✓"), messageStyle.Render(msg))
}

// StopWithWarning halts the spinner and prints a warning-styled final message.
func (s *Spinner) StopWithWarning(msg string) {
	done := s.stopChan()
	if done == nil {
		return
	}
	close(done)
	s.wg.Wait()
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	_, _ = fmt.Fprintf(s.out, "\r\033[K%s %s\n", warnStyle.Render("⚠"), messageStyle.Render(msg))
}

func (s *Spinner) animate(done <-chan struct{}) {
	defer s.wg.Done()
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-done:
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
