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
	unit  string
	sev   Severity
}

// headlineMetric picks the reading a tile should shout: the sensor that most
// recently crossed into alert, across every metric hostAlerts evaluates. Recency
// wins outright — a warning that just tripped displaces a critical that has been
// sitting there for an hour, because the new event is the one that needs looking
// at. With nothing in alert the tile falls back to CPU load.
func (m Model) headlineMetric(hostName string, info *internal.SystemInfo) (headlineReading, bool) {
	if info == nil {
		return headlineReading{}, false
	}

	alerts := hostAlerts(info, activeThresholds)
	if len(alerts) == 0 {
		return headlineReading{
			label: "CPU LOAD",
			value: info.CPU.UsagePercent,
			unit:  "%",
			sev:   severityFor(info.CPU.UsagePercent, activeThresholds.CPUPct),
		}, true
	}

	onsets := m.alertOnsets[hostName]
	best, bestSeq := alerts[0], onsets[alerts[0].Metric].seq
	for _, a := range alerts[1:] {
		seq := onsets[a.Metric].seq
		// Newest first; within one sample the worse (then larger, then
		// alphabetically first) reading leads so the choice is deterministic.
		if seq > bestSeq ||
			(seq == bestSeq && (a.Sev > best.Sev ||
				(a.Sev == best.Sev && (a.Value > best.Value ||
					(a.Value == best.Value && a.Metric < best.Metric))))) {
			best, bestSeq = a, seq
		}
	}

	return headlineReading{
		label: strings.ToUpper(best.Metric),
		value: best.Value,
		unit:  best.Unit,
		sev:   best.Sev,
	}, true
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
func (m Model) renderHeadline(hostName string, info *internal.SystemInfo, width int) string {
	reading, ok := m.headlineMetric(hostName, info)
	if !ok || width < headlineMinWidth {
		return ""
	}

	valueStyle := severityStyle(reading.sev, accentStyle)
	value := clampInt(int(math.Round(reading.value)), 0, 999)
	digits := renderBigInt(value, valueStyle)

	// Alert labels are metric names ("GPU 0 TEMP", "DISK /HOME"), so unlike the
	// old fixed-width captions they have to be trimmed to whatever the digits and
	// unit leave behind.
	digitsWidth := len(strconv.Itoa(value))*(bigDigitWidth+1) - 1
	labelWidth := width - digitsWidth - 2 - lipgloss.Width(reading.unit) - 2
	caption := valueStyle.Render(reading.unit) + "  " +
		mutedStyle.Render(truncateVisible(reading.label, labelWidth))

	return strings.Join([]string{
		digits[0],
		digits[1] + "  " + caption,
		digits[2],
	}, "\n")
}
