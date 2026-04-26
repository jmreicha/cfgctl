package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestStylesNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	rendered := sectionStyle.Render("hello")
	if rendered != "hello" {
		t.Fatalf("expected plain text with NO_COLOR=1, got %q", rendered)
	}

	rendered = labelStyle.Render("label")
	if rendered != "label" {
		t.Fatalf("expected plain text with NO_COLOR=1, got %q", rendered)
	}

	rendered = pathStyle.Render("/some/path")
	if rendered != "/some/path" {
		t.Fatalf("expected plain text with NO_COLOR=1, got %q", rendered)
	}

	rendered = warningStyle.Render("warn")
	if rendered != "warn" {
		t.Fatalf("expected plain text with NO_COLOR=1, got %q", rendered)
	}
}

func TestStylesWithColor(t *testing.T) {
	// os.Unsetenv rather than t.Setenv("NO_COLOR","") — termenv treats the
	// variable as "set" even when the value is an empty string.
	orig, had := os.LookupEnv("NO_COLOR")
	_ = os.Unsetenv("NO_COLOR")
	if had {
		t.Cleanup(func() { _ = os.Setenv("NO_COLOR", orig) })
	}

	var buf bytes.Buffer
	r := lipgloss.NewRenderer(&buf)
	r.SetColorProfile(termenv.TrueColor)
	style := r.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))

	rendered := style.Render("hello")
	if rendered == "hello" {
		t.Fatal("expected styled output with TrueColor profile active, got plain text")
	}
}
