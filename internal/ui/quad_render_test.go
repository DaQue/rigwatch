package ui

import (
	"strings"
	"testing"
)

func TestFitLinesToPaneCentersContent(t *testing.T) {
	got := fitLinesToPane("a\nb", 6)
	if len(got) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(got))
	}
	// 4 blank lines of slack should split 2 on top, 2 on bottom.
	if strings.TrimSpace(got[0]) != "" || strings.TrimSpace(got[1]) != "" {
		t.Fatalf("expected top padding before content, got %q", got)
	}
	if strings.TrimSpace(got[2]) != "a" || strings.TrimSpace(got[3]) != "b" {
		t.Fatalf("expected content vertically centered, got %q", got)
	}
	if strings.TrimSpace(got[4]) != "" || strings.TrimSpace(got[5]) != "" {
		t.Fatalf("expected bottom padding after content, got %q", got)
	}
}

func TestFitLinesToPaneTruncatesOverflow(t *testing.T) {
	got := fitLinesToPane("a\nb\nc\nd", 2)
	if len(got) != 2 {
		t.Fatalf("expected output capped at 2 lines, got %d: %q", len(got), got)
	}
}
