package ui

import "testing"

func TestPaneLayoutKeepsQuadAsFourLargeQuadrants(t *testing.T) {
	layout := paneLayout(220, 70, 10, 0)

	if layout.Columns != 2 || layout.Rows != 2 {
		t.Fatalf("quad layout = %dx%d, want 2x2", layout.Columns, layout.Rows)
	}
	if layout.PageSize() != quadPageSize {
		t.Fatalf("page size = %d, want fixed quad page size %d", layout.PageSize(), quadPageSize)
	}
	if layout.CellWidth < 100 {
		t.Fatalf("cell width = %d, want wide monitor quadrants", layout.CellWidth)
	}
	if layout.BodyLines < 28 {
		t.Fatalf("body lines = %d, want tall monitor quadrants", layout.BodyLines)
	}
	if layout.Pages != 3 {
		t.Fatalf("pages = %d, want 3 pages for 10 hosts at 4 per page", layout.Pages)
	}
}

func TestPaneLayoutUsesTwoLargePanesForTwoHosts(t *testing.T) {
	layout := paneLayout(220, 70, 2, 0)

	if layout.Columns != 2 || layout.Rows != 1 {
		t.Fatalf("two-host layout = %dx%d, want 2x1", layout.Columns, layout.Rows)
	}
	if layout.CellWidth < 100 {
		t.Fatalf("cell width = %d, want two wide panes", layout.CellWidth)
	}
	if layout.BodyLines < 58 {
		t.Fatalf("body lines = %d, want nearly full-height panes", layout.BodyLines)
	}
}

func TestPaneLayoutStacksTwoHostsOnNarrowTerminals(t *testing.T) {
	layout := paneLayout(78, 40, 2, 0)

	if layout.Columns != 1 || layout.Rows != 2 {
		t.Fatalf("narrow two-host layout = %dx%d, want 1x2", layout.Columns, layout.Rows)
	}
	if layout.CellWidth != 78 {
		t.Fatalf("cell width = %d, want full narrow terminal width", layout.CellWidth)
	}
}
