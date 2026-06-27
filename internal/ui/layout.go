package ui

type paneGrid struct {
	Columns   int
	Rows      int
	CellWidth int
	BodyLines int
	Start     int
	End       int
	Pages     int
}

func (l paneGrid) PageSize() int {
	return l.Columns * l.Rows
}

func paneLayout(width, height, total, page int) paneGrid {
	width = max(1, width)
	height = max(1, height)

	visible := total
	if visible <= 0 {
		visible = 1
	}
	if visible > quadPageSize {
		visible = quadPageSize
	}

	columns, rows := paneShape(width, visible)
	const gutter = 2
	cellWidth := (width - (columns-1)*gutter) / columns
	if cellWidth < 1 {
		cellWidth = 1
	}

	usableHeight := max(1, height-6)
	bodyLines := (usableHeight - (rows - 1)) / rows
	bodyLines = max(4, bodyLines-2)

	pageSize := columns * rows
	pages := 1
	if total > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	page = clampInt(page, 0, pages-1)
	start := page * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	return paneGrid{
		Columns:   columns,
		Rows:      rows,
		CellWidth: cellWidth,
		BodyLines: bodyLines,
		Start:     start,
		End:       end,
		Pages:     pages,
	}
}

func paneShape(width, visible int) (columns, rows int) {
	switch {
	case visible <= 1:
		return 1, 1
	case visible == 2:
		if width >= 110 {
			return 2, 1
		}
		return 1, 2
	default:
		if width >= 100 {
			return 2, 2
		}
		return 1, visible
	}
}
