package gallery

import "image"

type Metrics struct {
	CellW, CellH, GapX, GapY, PadLeft, PadTop, PadRight, PadBottom int
}

type Grid struct {
	Metrics
	Count, Width, Height int
}

type Nav uint8

const (
	NavLeft Nav = iota
	NavRight
	NavUp
	NavDown
	NavHome
	NavEnd
	NavPageUp
	NavPageDown
)

func (g Grid) strideX() int { return max(g.CellW+g.GapX, 1) }

func (g Grid) strideY() int { return max(g.CellH+g.GapY, 1) }

func (g Grid) Columns() int {
	return max((g.Width-g.PadLeft-g.PadRight+g.GapX)/g.strideX(), 1)
}

func (g Grid) Rows() int {
	if g.Count <= 0 {
		return 0
	}
	cols := g.Columns()
	return (g.Count + cols - 1) / cols
}

func (g Grid) ContentHeight() int {
	rows := g.Rows()
	if rows == 0 {
		return 0
	}
	return g.PadTop + rows*g.CellH + (rows-1)*g.GapY + g.PadBottom
}

func (g Grid) MaxScroll() int { return max(g.ContentHeight()-g.Height, 0) }

func (g Grid) ClampScroll(scroll int) int { return min(max(scroll, 0), g.MaxScroll()) }

func (g Grid) Rect(i int) image.Rectangle {
	if i < 0 || i >= g.Count {
		return image.Rectangle{}
	}
	cols := g.Columns()
	x := g.PadLeft + (i%cols)*g.strideX()
	y := g.PadTop + (i/cols)*g.strideY()
	return image.Rect(x, y, x+g.CellW, y+g.CellH)
}

func (g Grid) Visible(scroll int) (first, last int) {
	rows := g.Rows()
	if rows == 0 || g.Height <= 0 {
		return 0, 0
	}
	stride := g.strideY()
	bottom := scroll + g.Height - g.PadTop
	if bottom <= 0 {
		return 0, 0
	}
	firstRow := 0
	if above := scroll - g.PadTop - g.CellH; above >= 0 {
		firstRow = above/stride + 1
	}
	lastRow := min((bottom-1)/stride, rows-1)
	if firstRow > lastRow {
		return 0, 0
	}
	cols := g.Columns()
	return firstRow * cols, min((lastRow+1)*cols, g.Count)
}

func (g Grid) Prefetch(scroll, marginRows int) (first, last int) {
	first, last = g.Visible(scroll)
	if first == last {
		return 0, 0
	}
	margin := max(marginRows, 0) * g.Columns()
	return max(first-margin, 0), min(last+margin, g.Count)
}

func (g Grid) At(x, y, scroll int) int {
	if g.Count <= 0 {
		return -1
	}
	dx, dy := x-g.PadLeft, y+scroll-g.PadTop
	if dx < 0 || dy < 0 {
		return -1
	}
	strideX, strideY := g.strideX(), g.strideY()
	if dx%strideX >= g.CellW || dy%strideY >= g.CellH {
		return -1
	}
	cols := g.Columns()
	col, row := dx/strideX, dy/strideY
	if col >= cols {
		return -1
	}
	if i := row*cols + col; i < g.Count {
		return i
	}
	return -1
}

func (g Grid) Reveal(i, scroll int) int {
	start := g.ClampScroll(scroll)
	if i < 0 || i >= g.Count {
		return start
	}
	cell := g.Rect(i)
	scroll = start
	if cell.Max.Y > scroll+g.Height {
		scroll = cell.Max.Y - g.Height
	}
	if cell.Min.Y < scroll {
		scroll = cell.Min.Y
	}
	if scroll == start {
		return start
	}
	row := i / g.Columns()
	if row == 0 && scroll <= g.PadTop && cell.Max.Y <= g.Height {
		scroll = 0
	}
	if last := g.MaxScroll(); row == g.Rows()-1 && scroll >= last-g.PadBottom && cell.Min.Y >= last {
		scroll = last
	}
	return g.ClampScroll(scroll)
}

func (g Grid) pageRows() int { return max((g.Height+g.GapY)/g.strideY(), 1) }

func (g Grid) Move(cur int, nav Nav) int {
	n := g.Count
	if n <= 0 {
		return -1
	}
	if cur < 0 {
		if nav == NavEnd {
			return n - 1
		}
		return 0
	}
	cur = min(cur, n-1)
	cols := g.Columns()
	row, col := cur/cols, cur%cols
	switch nav {
	case NavLeft:
		return max(cur-1, 0)
	case NavRight:
		return min(cur+1, n-1)
	case NavUp:
		if row == 0 {
			return cur
		}
		return cur - cols
	case NavDown:
		switch {
		case cur+cols < n:
			return cur + cols
		case row < g.Rows()-1:
			return n - 1
		}
		return cur
	case NavHome:
		return 0
	case NavEnd:
		return n - 1
	case NavPageUp:
		return max(row-g.pageRows(), 0)*cols + col
	case NavPageDown:
		return min(min(row+g.pageRows(), g.Rows()-1)*cols+col, n-1)
	}
	return cur
}
