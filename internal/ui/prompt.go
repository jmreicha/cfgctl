package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Prompter implements core.Prompter with stdin y/n prompts.
// It pauses/resumes the spinner to avoid clobbering the prompt.
type Prompter struct {
	Spinner *Spinner
}

// Confirm asks the user a y/n question and returns true for yes.
func (p Prompter) Confirm(msg string) bool {
	if p.Spinner != nil {
		p.Spinner.Stop()
	}
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	fmt.Fprintf(os.Stderr, "%s %s [y/N]: ", warnStyle.Render("⚠"), msg)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	if p.Spinner != nil {
		p.Spinner.Start("")
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "y")
}
