package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestFitLinesToPaneTopAligns(t *testing.T) {
	got := fitLinesToPane("a\nb", 6)
	if len(got) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(got))
	}
	if strings.TrimSpace(got[0]) != "a" || strings.TrimSpace(got[1]) != "b" {
		t.Fatalf("expected content top-aligned, got %q", got)
	}
	for i := 2; i < 6; i++ {
		if strings.TrimSpace(got[i]) != "" {
			t.Fatalf("expected blank padding below content at line %d, got %q", i, got)
		}
	}
}

func TestFitLinesToPaneTruncatesOverflow(t *testing.T) {
	got := fitLinesToPane("a\nb\nc\nd", 2)
	if len(got) != 2 {
		t.Fatalf("expected output capped at 2 lines, got %d: %q", len(got), got)
	}
}

func TestFitLinesToPaneGradientTopAlignsAndFills(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	got := fitLinesToPaneGradient("a\nb", 6, 8, lipgloss.Color("#32d9ff"))
	if len(got) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(got))
	}
	if strings.TrimSpace(got[0]) != "a" || strings.TrimSpace(got[1]) != "b" {
		t.Fatalf("expected content top-aligned, got %q", got)
	}
	// The gap rows carry background color escapes (the gradient), not blanks.
	for i := 2; i < 6; i++ {
		if !strings.Contains(got[i], "\x1b[") {
			t.Fatalf("expected gradient fill (ANSI background) at line %d, got %q", i, got[i])
		}
	}
}
