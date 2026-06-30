package ui

import (
	"fmt"
	"strings"

	"github.com/allisonhere/rigwatch/internal"
)

type helpEntry struct {
	keys string
	desc string
}

type helpGroup struct {
	title   string
	entries []helpEntry
}

// helpGroups is the single source of truth for the keybinding overlay. Keep it in
// sync with the handlers in update.go — the footers and this table are the only
// places keys are documented.
func helpGroups() []helpGroup {
	return []helpGroup{
		{"Host list", []helpEntry{
			{"↑/↓", "move cursor"},
			{"space", "toggle host selection"},
			{"enter", "monitor selected hosts"},
			{"/", "filter hosts"},
			{"a / e / d", "add / edit / delete host"},
			{"i", "install SSH key on host"},
			{"o", "open settings"},
		}},
		{"Single host", []helpEntry{
			{"n", "next host"},
			{"t", "overview"},
			{"g", "grid view"},
			{"s", "open shell (ssh)"},
			{"c", "back to host list"},
		}},
		{"Grid", []helpEntry{
			{"n / p", "next / previous page"},
			{"tab / shift+tab", "move focus"},
			{"[ / ]", "cycle focused host theme"},
			{"w", "save layout"},
			{"t / g / esc", "back to dashboard"},
		}},
		{"Settings", []helpEntry{
			{"↑/↓", "move between fields"},
			{"←/→", "change default theme"},
			{"enter", "save"},
			{"esc", "cancel"},
		}},
		{"Anywhere", []helpEntry{
			{"?", "toggle this help"},
			{"q", "quit"},
		}},
	}
}

func (m Model) renderHelp() string {
	var b strings.Builder
	b.WriteString(renderHeroHeader("RIGWATCH // HELP", "v"+internal.ShortVersion()+"  •  ? or esc to close", m.width, m.animationFrame))
	b.WriteString("\n\n")

	for _, group := range helpGroups() {
		body := make([]string, 0, len(group.entries))
		for _, e := range group.entries {
			body = append(body, fmt.Sprintf("%s  %s",
				accentStyle.Render(fmt.Sprintf("%-16s", e.keys)),
				panelTextStyle.Render(e.desc)))
		}
		width := clampInt(m.width-4, 36, 72)
		b.WriteString(renderPanel(group.title, strings.Join(body, "\n"), width))
		b.WriteString("\n")
	}

	out := b.String()
	if m.height > 0 {
		out = clampToHeight(out, m.height)
	}
	return out
}
