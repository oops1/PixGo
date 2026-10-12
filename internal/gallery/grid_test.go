package gallery

import (
	"image"
	"math/rand"
	"testing"
)

var testMetrics = Metrics{CellW: 100, CellH: 120, GapX: 10, GapY: 8, PadLeft: 12, PadTop: 10, PadRight: 12, PadBottom: 14}

const threeColumns = 12 + 3*100 + 2*10 + 12

func testGrid(count, width, height int) Grid {
	return Grid{Metrics: testMetrics, Count: count, Width: width, Height: height}
}

func TestGridColumns(t *testing.T) {
	cases := []struct {
		name  string
		width int
		want  int
	}{
		{"exact fit for three", threeColumns, 3},
		{"one pixel short of three", threeColumns - 1, 2},
		{"one pixel over three", threeColumns + 1, 3},
		{"exact fit for four", threeColumns + 110, 4},
		{"exact fit for one", 12 + 100 + 12, 1},
		{"narrower than a cell", 50, 1},
		{"zero width", 0, 1},
		{"negative width", -300, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := testGrid(10, tc.width, 400).Columns(); got != tc.want {
				t.Fatalf("Columns = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestGridRowsAndContentHeight(t *testing.T) {
	cases := []struct {
		name       string
		count      int
		wantRows   int
		wantHeight int
	}{
		{"no items", 0, 0, 0},
		{"negative count", -4, 0, 0},
		{"one item", 1, 1, 10 + 120 + 14},
		{"full row", 3, 1, 10 + 120 + 14},
		{"one more than a row", 4, 2, 10 + 2*120 + 8 + 14},
		{"incomplete last row", 7, 3, 10 + 3*120 + 2*8 + 14},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := testGrid(tc.count, threeColumns, 300)
			if g.Rows() != tc.wantRows || g.ContentHeight() != tc.wantHeight {
				t.Fatalf("rows %d, height %d; want %d, %d", g.Rows(), g.ContentHeight(), tc.wantRows, tc.wantHeight)
			}
		})
	}
}

func TestGridRect(t *testing.T) {
	g := testGrid(7, threeColumns, 300)
	cases := []struct {
		name string
		i    int
		want image.Rectangle
	}{
		{"first cell", 0, image.Rect(12, 10, 112, 130)},
		{"second cell", 1, image.Rect(122, 10, 222, 130)},
		{"last cell of the first row", 2, image.Rect(232, 10, 332, 130)},
		{"first cell of the second row", 3, image.Rect(12, 138, 112, 258)},
		{"last cell", 6, image.Rect(12, 266, 112, 386)},
		{"negative index", -1, image.Rectangle{}},
		{"index past the end", 7, image.Rectangle{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := g.Rect(tc.i); got != tc.want {
				t.Fatalf("Rect(%d) = %v, want %v", tc.i, got, tc.want)
			}
		})
	}
}

func TestGridVisible(t *testing.T) {
	cases := []struct {
		name        string
		count       int
		height      int
		scroll      int
		first, last int
	}{
		{"top of the content", 20, 250, 0, 0, 6},
		{"row boundary hides the first row", 20, 250, 130, 3, 9},
		{"one pixel of the first row remains", 20, 250, 129, 0, 9},
		{"viewport inside the gap shows nothing", 20, 8, 130, 0, 0},
		{"viewport over one row", 20, 100, 130, 3, 6},
		{"scrolled into the middle", 20, 250, 300, 6, 15},
		{"bottom of the content", 20, 250, 662, 15, 20},
		{"incomplete last row only", 20, 100, 780, 18, 20},
		{"scroll past the end", 20, 250, 5000, 0, 0},
		{"negative scroll shows the start", 20, 250, -40, 0, 6},
		{"scroll far above hides everything", 20, 100, -500, 0, 0},
		{"no items", 0, 250, 0, 0, 0},
		{"zero height", 20, 0, 0, 0, 0},
		{"negative height", 20, -5, 0, 0, 0},
		{"single item", 1, 250, 0, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, last := testGrid(tc.count, threeColumns, tc.height).Visible(tc.scroll)
			if first != tc.first || last != tc.last {
				t.Fatalf("Visible(%d) = %d, %d; want %d, %d", tc.scroll, first, last, tc.first, tc.last)
			}
		})
	}
}

func TestGridPrefetchExpandsByWholeRows(t *testing.T) {
	g := testGrid(30, threeColumns, 250)
	cases := []struct {
		name        string
		scroll      int
		margin      int
		first, last int
	}{
		{"no margin equals the visible range", 300, 0, 6, 15},
		{"one row on both sides", 300, 1, 3, 18},
		{"margin clamps at the start", 0, 2, 0, 12},
		{"margin clamps at the end", g.MaxScroll(), 5, 9, 30},
		{"negative margin is ignored", 300, -3, 6, 15},
		{"nothing visible stays empty", 99999, 2, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, last := g.Prefetch(tc.scroll, tc.margin)
			if first != tc.first || last != tc.last {
				t.Fatalf("Prefetch(%d, %d) = %d, %d; want %d, %d", tc.scroll, tc.margin, first, last, tc.first, tc.last)
			}
		})
	}
}

func TestGridAt(t *testing.T) {
	g := testGrid(7, threeColumns, 300)
	cases := []struct {
		name         string
		x, y, scroll int
		want         int
	}{
		{"inside the first cell", 50, 50, 0, 0},
		{"top left pixel of the first cell", 12, 10, 0, 0},
		{"bottom right pixel of the first cell", 111, 129, 0, 0},
		{"just right of the first cell", 112, 50, 0, -1},
		{"gap between columns", 115, 50, 0, -1},
		{"second column", 125, 50, 0, 1},
		{"third column", 300, 50, 0, 2},
		{"right padding", 340, 50, 0, -1},
		{"left padding", 5, 50, 0, -1},
		{"top padding", 50, 5, 0, -1},
		{"gap between rows", 50, 134, 0, -1},
		{"second row", 50, 140, 0, 3},
		{"scrolled second row", 50, 20, 130, 3},
		{"last item", 50, 270, 0, 6},
		{"empty slot of the last row", 125, 270, 0, -1},
		{"below the last row", 50, 395, 0, -1},
		{"negative coordinates", -3, -3, 0, -1},
		{"top padding after scrolling", 50, -15, 20, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := g.At(tc.x, tc.y, tc.scroll); got != tc.want {
				t.Fatalf("At(%d, %d, %d) = %d, want %d", tc.x, tc.y, tc.scroll, got, tc.want)
			}
		})
	}
	if got := testGrid(0, threeColumns, 300).At(50, 50, 0); got != -1 {
		t.Fatalf("At on an empty grid = %d", got)
	}
}

func TestGridClampScroll(t *testing.T) {
	g := testGrid(20, threeColumns, 250)
	cases := []struct {
		name   string
		scroll int
		want   int
	}{
		{"inside the range", 300, 300},
		{"negative", -20, 0},
		{"past the end", 99999, 662},
		{"exactly the end", 662, 662},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := g.ClampScroll(tc.scroll); got != tc.want {
				t.Fatalf("ClampScroll(%d) = %d, want %d", tc.scroll, got, tc.want)
			}
		})
	}
	if got := testGrid(3, threeColumns, 500).ClampScroll(40); got != 0 {
		t.Fatalf("content shorter than the viewport must not scroll, got %d", got)
	}
	if got := testGrid(0, threeColumns, 500).MaxScroll(); got != 0 {
		t.Fatalf("MaxScroll of an empty grid = %d", got)
	}
}

func TestGridReveal(t *testing.T) {
	g := testGrid(20, threeColumns, 250)
	cases := []struct {
		name   string
		i      int
		scroll int
		want   int
	}{
		{"visible cell keeps the scroll", 3, 100, 100},
		{"first row from below snaps to the very top", 0, 80, 0},
		{"cell above the viewport aligns to its top edge", 9, 700, 394},
		{"cell below the viewport aligns to its bottom edge", 9, 0, 394 + 120 - 250},
		{"last row snaps to the very bottom", 19, 0, 662},
		{"last row from above snaps to the very bottom", 18, 100, 662},
		{"first row visible near the top stays", 1, 3, 3},
		{"index out of range keeps the clamped scroll", 99, 5000, 662},
		{"negative index keeps the scroll", -1, 120, 120},
		{"middle row partially hidden at the bottom", 6, 0, 266 + 120 - 250},
		{"middle row partially hidden at the top", 6, 400, 266},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := g.Reveal(tc.i, tc.scroll); got != tc.want {
				t.Fatalf("Reveal(%d, %d) = %d, want %d", tc.i, tc.scroll, got, tc.want)
			}
		})
	}
}

func TestGridRevealWithCellTallerThanViewportShowsItsTop(t *testing.T) {
	g := testGrid(20, threeColumns, 80)
	if got, want := g.Reveal(9, 0), g.Rect(9).Min.Y; got != want {
		t.Fatalf("Reveal = %d, want the top edge %d", got, want)
	}
}

func TestGridRevealWithoutPaddingNeedsNoSnap(t *testing.T) {
	g := Grid{Metrics: Metrics{CellW: 50, CellH: 50, GapX: 5, GapY: 5}, Count: 40, Width: 170, Height: 120}
	if got := g.Reveal(0, 30); got != 0 {
		t.Fatalf("first row: %d", got)
	}
	if got, want := g.Reveal(39, 0), g.MaxScroll(); got != want {
		t.Fatalf("last row: %d, want %d", got, want)
	}
}

func TestGridMove(t *testing.T) {
	g := testGrid(10, 12+4*100+3*10+12, 2*120+8)
	if g.Columns() != 4 {
		t.Fatalf("test layout expects 4 columns, got %d", g.Columns())
	}
	cases := []struct {
		name string
		cur  int
		nav  Nav
		want int
	}{
		{"left from the first stays", 0, NavLeft, 0},
		{"left moves back by one", 6, NavLeft, 5},
		{"left wraps to the previous row", 4, NavLeft, 3},
		{"right moves on by one", 5, NavRight, 6},
		{"right wraps to the next row", 3, NavRight, 4},
		{"right from the last stays", 9, NavRight, 9},
		{"up on the first row stays", 2, NavUp, 2},
		{"up keeps the column", 6, NavUp, 2},
		{"up from the short last row", 9, NavUp, 5},
		{"down keeps the column", 1, NavDown, 5},
		{"down into the short last row", 5, NavDown, 9},
		{"down from a column missing in the last row goes to the last item", 6, NavDown, 9},
		{"down from the last column goes to the last item", 7, NavDown, 9},
		{"down on the last row stays", 8, NavDown, 8},
		{"down on the last item stays", 9, NavDown, 9},
		{"home", 7, NavHome, 0},
		{"end", 2, NavEnd, 9},
		{"page up moves by the visible rows", 9, NavPageUp, 1},
		{"page up clamps at the first row", 6, NavPageUp, 2},
		{"page up on the first row stays", 3, NavPageUp, 3},
		{"page down moves by the visible rows", 0, NavPageDown, 8},
		{"page down into the short row goes to the last item", 3, NavPageDown, 9},
		{"page down on the last row stays", 8, NavPageDown, 8},
		{"nothing selected: down picks the first", -1, NavDown, 0},
		{"nothing selected: right picks the first", -1, NavRight, 0},
		{"nothing selected: page up picks the first", -1, NavPageUp, 0},
		{"nothing selected: home picks the first", -1, NavHome, 0},
		{"nothing selected: end picks the last", -1, NavEnd, 9},
		{"stale index past the end is clamped", 15, NavLeft, 8},
		{"unknown key keeps the index", 5, Nav(200), 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := g.Move(tc.cur, tc.nav); got != tc.want {
				t.Fatalf("Move(%d, %d) = %d, want %d", tc.cur, tc.nav, got, tc.want)
			}
		})
	}
}

func TestGridMoveInSingleColumn(t *testing.T) {
	g := testGrid(5, 100, 300)
	if g.Columns() != 1 {
		t.Fatalf("expected one column, got %d", g.Columns())
	}
	cases := []struct {
		name string
		cur  int
		nav  Nav
		want int
	}{
		{"down", 1, NavDown, 2},
		{"up", 2, NavUp, 1},
		{"up on the first", 0, NavUp, 0},
		{"down on the last", 4, NavDown, 4},
		{"left", 3, NavLeft, 2},
		{"right", 3, NavRight, 4},
		{"page down by visible rows", 0, NavPageDown, 2},
		{"page up by visible rows", 4, NavPageUp, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := g.Move(tc.cur, tc.nav); got != tc.want {
				t.Fatalf("Move(%d, %d) = %d, want %d", tc.cur, tc.nav, got, tc.want)
			}
		})
	}
}

func TestGridMovePageStepIsAtLeastOneRow(t *testing.T) {
	g := testGrid(10, threeColumns, 5)
	if got := g.Move(0, NavPageDown); got != 3 {
		t.Fatalf("page down in a tiny viewport = %d, want one row down", got)
	}
	if got := g.Move(9, NavPageUp); got != 6 {
		t.Fatalf("page up in a tiny viewport = %d, want one row up", got)
	}
}

func TestGridMoveOnEmptyGrid(t *testing.T) {
	g := testGrid(0, threeColumns, 300)
	for nav := NavLeft; nav <= NavPageDown; nav++ {
		if got := g.Move(-1, nav); got != -1 {
			t.Errorf("Move(-1, %d) = %d on an empty grid", nav, got)
		}
		if got := g.Move(3, nav); got != -1 {
			t.Errorf("Move(3, %d) = %d on an empty grid", nav, got)
		}
	}
}

func TestGridMoveSingleItem(t *testing.T) {
	g := testGrid(1, threeColumns, 300)
	for nav := NavLeft; nav <= NavPageDown; nav++ {
		if got := g.Move(0, nav); got != 0 {
			t.Errorf("Move(0, %d) = %d with one item", nav, got)
		}
	}
}

func TestGridDegenerateMetricsDoNotPanic(t *testing.T) {
	cases := []Grid{
		{},
		{Count: 5},
		{Count: 5, Width: 100, Height: 100},
		{Metrics: Metrics{CellW: 0, CellH: 0}, Count: 5, Width: 100, Height: 100},
		{Metrics: Metrics{CellW: -4, CellH: -4, GapX: -2, GapY: -2}, Count: 5, Width: 100, Height: 100},
		{Metrics: testMetrics, Count: 5, Width: 5, Height: 5},
	}
	for _, g := range cases {
		g.Columns()
		g.Rows()
		g.ContentHeight()
		g.MaxScroll()
		g.ClampScroll(10)
		g.Rect(2)
		g.Visible(3)
		g.Prefetch(3, 1)
		g.At(4, 4, 1)
		g.Reveal(2, 7)
		for nav := NavLeft; nav <= NavPageDown; nav++ {
			g.Move(1, nav)
		}
	}
}

func randomGrid(rng *rand.Rand) Grid {
	m := Metrics{
		CellW:     1 + rng.Intn(60),
		CellH:     1 + rng.Intn(60),
		GapX:      rng.Intn(12),
		GapY:      rng.Intn(12),
		PadLeft:   rng.Intn(20),
		PadTop:    rng.Intn(20),
		PadRight:  rng.Intn(20),
		PadBottom: rng.Intn(20),
	}
	return Grid{Metrics: m, Count: rng.Intn(120), Width: rng.Intn(400), Height: 1 + rng.Intn(300)}
}

func TestGridVisibleMatchesCellIntersection(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for n := 0; n < 3000; n++ {
		g := randomGrid(rng)
		scroll := rng.Intn(g.ContentHeight()+g.Height+40) - 20
		view := image.Rect(-1<<20, scroll, 1<<20, scroll+g.Height)
		var want []int
		for i := 0; i < g.Count; i++ {
			if g.Rect(i).Overlaps(view) {
				want = append(want, i)
			}
		}
		first, last := g.Visible(scroll)
		if len(want) == 0 {
			if first != last {
				t.Fatalf("%+v scroll %d: nothing intersects but got %d-%d", g, scroll, first, last)
			}
			continue
		}
		if first > want[0] || last <= want[len(want)-1] {
			t.Fatalf("%+v scroll %d: range %d-%d misses cells %d-%d", g, scroll, first, last, want[0], want[len(want)-1])
		}
		cols := g.Columns()
		if first != want[0]/cols*cols {
			t.Fatalf("%+v scroll %d: first %d is not the start of the first visible row %d", g, scroll, first, want[0])
		}
		if wantLast := min((want[len(want)-1]/cols+1)*cols, g.Count); last != wantLast {
			t.Fatalf("%+v scroll %d: last %d, want %d", g, scroll, last, wantLast)
		}
	}
}

func TestGridAtMatchesCellRectangles(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	for n := 0; n < 3000; n++ {
		g := randomGrid(rng)
		scroll := rng.Intn(g.ContentHeight() + 1)
		x, y := rng.Intn(g.Width+30)-10, rng.Intn(g.Height+30)-10
		want := -1
		for i := 0; i < g.Count; i++ {
			if image.Pt(x, y+scroll).In(g.Rect(i)) {
				want = i
				break
			}
		}
		if got := g.At(x, y, scroll); got != want {
			t.Fatalf("%+v: At(%d, %d, %d) = %d, want %d", g, x, y, scroll, got, want)
		}
	}
}

func TestGridRevealMakesCellFullyVisible(t *testing.T) {
	rng := rand.New(rand.NewSource(23))
	for n := 0; n < 3000; n++ {
		g := randomGrid(rng)
		if g.Count == 0 || g.CellH > g.Height {
			continue
		}
		i := rng.Intn(g.Count)
		start := rng.Intn(g.ContentHeight()+g.Height) - 10
		got := g.Reveal(i, start)
		if got < 0 || got > g.MaxScroll() {
			t.Fatalf("%+v: Reveal(%d, %d) = %d out of [0, %d]", g, i, start, got, g.MaxScroll())
		}
		cell := g.Rect(i)
		if cell.Min.Y < got || cell.Max.Y > got+g.Height {
			t.Fatalf("%+v: Reveal(%d, %d) = %d leaves %v outside the viewport of height %d", g, i, start, got, cell, g.Height)
		}
		clamped := g.ClampScroll(start)
		if cell.Min.Y >= clamped && cell.Max.Y <= clamped+g.Height && got != clamped {
			t.Fatalf("%+v: cell %d already visible at %d but Reveal moved to %d", g, i, clamped, got)
		}
	}
}

func TestGridMoveStaysInRange(t *testing.T) {
	rng := rand.New(rand.NewSource(77))
	for n := 0; n < 3000; n++ {
		g := randomGrid(rng)
		cur := rng.Intn(g.Count+3) - 1
		nav := Nav(rng.Intn(int(NavPageDown) + 1))
		got := g.Move(cur, nav)
		if g.Count == 0 {
			if got != -1 {
				t.Fatalf("%+v: Move on an empty grid = %d", g, got)
			}
			continue
		}
		if got < 0 || got >= g.Count {
			t.Fatalf("%+v: Move(%d, %d) = %d out of range", g, cur, nav, got)
		}
	}
}
