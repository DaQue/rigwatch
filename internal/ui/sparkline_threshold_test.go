package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// stripANSI removes escape sequences so we can compare the visible glyphs.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc && r == 'm':
			inEsc = false
		case inEsc:
			// skip
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestSparklineThresholdSameGlyphsAsPlain(t *testing.T) {
	values := []float64{10, 40, 60, 95, 30}
	plain := stripANSI(renderSparkline(values, 5))
	// Disabled threshold (0/0) must render the exact same glyphs.
	themed := stripANSI(renderSparklineThreshold(values, 5, Threshold{}))
	if plain != themed {
		t.Fatalf("glyphs differ: plain=%q themed=%q", plain, themed)
	}
}

func TestSparklineThresholdColorsCrossedPoints(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	// The color sequence the code uses for a critical point.
	dangerSeq := dangerStyle.Render("█")
	dangerPrefix := dangerSeq[:strings.IndexRune(dangerSeq, '█')]
	if dangerPrefix == "" {
		t.Fatal("expected a non-empty danger escape sequence with color forced on")
	}

	// One point clearly critical (96 >= crit 95), the rest below warn (85).
	out := renderSparklineThreshold([]float64{10, 20, 96, 15}, 4, Threshold{Warn: 85, Crit: 95})
	if !strings.Contains(out, dangerPrefix) {
		t.Fatalf("expected a danger-colored run for the 96%% point, got %q", out)
	}

	// All-nominal data must not contain the danger color.
	calm := renderSparklineThreshold([]float64{10, 20, 30, 15}, 4, Threshold{Warn: 85, Crit: 95})
	if strings.Contains(calm, dangerPrefix) {
		t.Fatalf("calm trend should not be danger-colored: %q", calm)
	}
}
