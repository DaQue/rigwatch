package ui

import (
	"testing"
	"time"

	"github.com/allisonhere/rigwatch/internal"
	tea "github.com/charmbracelet/bubbletea"
)

// Regression: focusing the Auth toggle row (one past the text inputs) must not
// panic when a non-key message (window resize, cursor blink, spinner tick)
// reaches the form's fall-through update path.
func TestFormAuthRowHandlesNonKeyMessages(t *testing.T) {
	m := InitialModel(nil, time.Second)
	(&m).startAddForm()

	// Tab down to the Auth row (the focus stop just past the inputs).
	(&m).setFormFocus(m.formAuthRow())
	if m.formFocus != len(m.formInputs) {
		t.Fatalf("expected focus on auth row %d, got %d", len(m.formInputs), m.formFocus)
	}

	// A WindowSizeMsg falls through to the form's input-update path; with focus
	// on the toggle there is no input to update, and it must not index OOB.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	got := updated.(Model)
	if got.manageMode != manageForm {
		t.Fatalf("form should still be open after resize, got mode %v", got.manageMode)
	}
}

func TestFormAuthRowTogglesAndWraps(t *testing.T) {
	m := InitialModel(nil, time.Second)
	(&m).startEditForm(internal.SSHHost{Name: "box", Hostname: "h", User: "u", Port: "22"})

	// shift+tab from the first field wraps to the auth row (last focus stop).
	(&m).setFormFocus(-1)
	if m.formFocus != m.formAuthRow() {
		t.Fatalf("wrap-up should land on auth row, got %d", m.formFocus)
	}

	// Space toggles the value.
	before := m.formAuthPassword
	upd, _ := m.updateManage(key(" "))
	m = upd.(Model)
	if m.formAuthPassword == before {
		t.Fatalf("space should toggle the auth value")
	}
}
