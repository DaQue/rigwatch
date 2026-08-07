package ui

import (
	"math"
	"strconv"
	"strings"

	"github.com/allisonhere/rigwatch/internal"
	"github.com/charmbracelet/lipgloss"
)

// bigDigits is a three-row block font for the headline metric. Terminals have no
// font sizes, so scale has to come from glyph geometry: half-block glyphs give
// each digit six rows of effective vertical resolution across three text rows,
// which is enough to read a tile from across a room.
var bigDigits = map[rune][3]string{
	'0': {"█▀▀█", "█  █", "█▄▄█"},
	'1': {" ▀█ ", "  █ ", " ▄█▄"},
	'2': {"▀▀▀█", " ▄▄▀", "█▄▄▄"},
	'3': {"▀▀▀█", " ▀▀▄", "▄▄▄▀"},
	'4': {"█  █", "▀▀▀█", "   █"},
	'5': {"█▀▀▀", "▀▀▀▄", "▄▄▄▀"},
	'6': {"█▀▀▀", "█▀▀▄", "▀▄▄▀"},
	'7': {"▀▀▀█", "  ▄▀", " █  "},
	'8': {"▄▀▀▄", "▄▀▀▄", "▀▄▄▀"},
	'9': {"▄▀▀▄", "▀▄▄█", "▄▄▄▀"},
}

// bigDigitWidth is the cell width of one block digit, and headlineMinWidth is the
// narrowest tile that can show the digits plus their label without crowding.
const (
	bigDigitWidth    = 4
	headlineMinWidth = 30
)

// headlineReading is the single metric a tile renders large.
type headlineReading struct {
	label string
	value float64
	sev   Severity
}

// dominantMetric picks the reading a tile should shout: the worst-severity metric
// among CPU, GPU utilization and RAM, breaking ties by raw value. Choosing by
// severity rather than always showing a fixed metric means the big number is the
// one worth reacting to.
func dominantMetric(info *internal.SystemInfo) (headlineReading, bool) {
	if info == nil {
		return headlineReading{}, false
	}

	candidates := []headlineReading{{
		label: "CPU LOAD",
		value: info.CPU.UsagePercent,
		sev:   severityFor(info.CPU.UsagePercent, activeThresholds.CPUPct),
	}}
	if info.RAM.Total > 0 {
		candidates = append(candidates, headlineReading{
			label: "RAM USED",
			value: info.RAM.UsagePercent,
			sev:   severityFor(info.RAM.UsagePercent, activeThresholds.RAMPct),
		})
	}
	if len(info.GPUs) > 0 {
		total := 0
		for _, gpu := range info.GPUs {
			total += gpu.Utilization
		}
		avg := float64(total) / float64(len(info.GPUs))
		candidates = append(candidates, headlineReading{
			label: "GPU UTIL",
			value: avg,
			sev:   severityFor(avg, activeThresholds.GPUPct),
		})
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.sev > best.sev || (c.sev == best.sev && c.value > best.value) {
			best = c
		}
	}
	return best, true
}

// renderBigInt renders n as three rows of block digits in the given style.
func renderBigInt(n int, style lipgloss.Style) [3]string {
	text := strconv.Itoa(clampInt(n, 0, 999))
	var rows [3]string
	for i, r := range []rune(text) {
		glyph, ok := bigDigits[r]
		if !ok {
			continue
		}
		for row := 0; row < 3; row++ {
			if i > 0 {
				rows[row] += " "
			}
			rows[row] += glyph[row]
		}
	}
	for row := 0; row < 3; row++ {
		rows[row] = style.Render(rows[row])
	}
	return rows
}

// renderHeadline returns the three-row headline block for a tile, or an empty
// string when there is no telemetry or the tile is too narrow to carry it.
func renderHeadline(info *internal.SystemInfo, width int) string {
	reading, ok := dominantMetric(info)
	if !ok || width < headlineMinWidth {
		return ""
	}

	valueStyle := severityStyle(reading.sev, accentStyle)
	digits := renderBigInt(int(math.Round(reading.value)), valueStyle)
	caption := valueStyle.Render("%") + "  " + mutedStyle.Render(reading.label)

	return strings.Join([]string{
		digits[0],
		digits[1] + "  " + caption,
		digits[2],
	}, "\n")
}
