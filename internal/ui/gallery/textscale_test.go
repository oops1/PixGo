package gallery

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/fonts"
	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

var textScales = []float64{1, 1.2, 1.33, 1.6}

var sampleTime = time.Date(2024, 9, 14, 18, 27, 0, 0, time.UTC)

func withScale(t *testing.T, k float64, lang string) {
	t.Helper()
	old, oldLang := fonts.TextScale(), testLang
	t.Cleanup(func() {
		fonts.SetTextScale(old)
		testLang = oldLang
		i18n.SetLang(oldLang)
	})
	fonts.SetTextScale(k)
	testLang = lang
	i18n.SetLang(lang)
}

func eachScale(t *testing.T, scales []float64, f func(t *testing.T, k float64, lang string)) {
	t.Helper()
	for _, lang := range i18n.Supported() {
		for _, k := range scales {
			t.Run(lang+"/"+ftoa(k), func(t *testing.T) {
				withScale(t, k, lang)
				f(t, k, lang)
			})
		}
	}
}

func ftoa(k float64) string {
	n := int(k*100 + .5)
	return itoa(n/100) + "." + itoa(n%100/10) + itoa(n%10)
}

func localizedStatus() []string {
	return model.StatusParts(len(mockNames), &model.Item{Name: "mountain.jpg", Size: 8_730_000}, 6000, 4000)
}

func scaledRig(t *testing.T, w, h int, mutate func(*fakeSource)) *rig {
	t.Helper()
	src := mockSource()
	src.sidebar = mockSidebar()
	src.status = localizedStatus()
	for i, n := range mockNames {
		src.images[mockPath(n)] = solid(120, 80, color.RGBA{uint8(40 + i*15), uint8(120 + i*5), uint8(200 - i*10), 255})
	}
	if mutate != nil {
		mutate(src)
	}
	return newRig(t, src, w, h)
}

func TestTextMetricsAreExactlyTheMockupAtScaleOne(t *testing.T) {
	m := baseMetrics()

	if m.sideW != 151 || m.openW != 91 || m.btnExtra != 0 || m.statusSx != 0 || m.nav.extra != 0 ||
		m.labelExtra != 0 || m.listRow != 40 || m.statusGap != 16 || m.nav.text != 5 || m.bubbleH != 30 {
		t.Fatalf("metrics at k=1: %+v", m)
	}
	r := newRig(t, mockSource(), mockW, mockH)
	if r.page.m.sideW != 151 || r.page.m.openW != 91 || r.page.geo.statusY != mockStatusY+41-statusFromBot {
		t.Fatalf("laid out metrics: %+v", r.page.m)
	}
}

func TestTextMetricsGrowMonotonicallyAndNeverShrink(t *testing.T) {
	prev := newTextMetrics(1)
	for _, k := range []float64{0.8, 1, 1.2, 1.33, 1.6, 2} {
		m := newTextMetrics(k)
		if k < 1 && (m.nav.extra != 0 || m.btnExtra != 0 || m.labelExtra != 0 || m.statusGap != 16) {
			t.Errorf("k=%v must not change the geometry: %+v", k, m)
		}
		if k >= 1 && (m.nav.extra < prev.nav.extra || m.labelExtra < prev.labelExtra || m.btnExtra < prev.btnExtra || m.listRow < prev.listRow) {
			t.Errorf("k=%v shrank against %+v: %+v", k, prev, m)
		}
		if k >= 1 {
			prev = m
		}
	}
}

func TestSidebarLabelsStayWholeAndInsideThePanel(t *testing.T) {
	eachScale(t, textScales, func(t *testing.T, k float64, lang string) {
		r := scaledRig(t, mockW, mockH, nil)
		img := r.render()
		g := r.page.geo

		for _, row := range r.page.rows {
			f := navFont
			if row.entry.Header {
				f = sectionFont
			}
			x := row.textX()
			avail := g.sidebar.Max.X - x - 6
			if got := f.Fit(nil, row.entry.Label, avail); got != row.entry.Label {
				t.Errorf("%q is truncated to %q (room %d, width %d)", row.entry.Label, got, avail, f.Measure(nil, row.entry.Label))
			}
		}
		strip := image.Rect(g.sidebar.Max.X-4, g.sidebar.Min.Y, g.sidebar.Max.X, g.sidebar.Max.Y)
		for y := strip.Min.Y; y < strip.Max.Y; y++ {
			for x := strip.Min.X; x < strip.Max.X; x++ {
				if got := img.RGBAAt(x, y); got != r.th.GalleryNav {
					t.Fatalf("something is drawn at (%d,%d) = %v against the panel edge %d", x, y, got, g.sidebar.Max.X)
				}
			}
		}
		if k == 1 && (lang == "ru" || lang == "en") && g.sidebar.Max.X != 151 {
			t.Errorf("panel width %d at k=1", g.sidebar.Max.X)
		}
		if k == 1 && (g.sidebar.Max.X < 151 || g.sidebar.Max.X > 160) {
			t.Errorf("panel width %d at k=1 is not within 151..160", g.sidebar.Max.X)
		}
	})
}

func TestSidebarWidthIsBoundedByTheWindow(t *testing.T) {
	withScale(t, 2.2, "ru")
	src := mockSource()
	src.sidebar[11].Label = "Очень длинное название тома для проверки обрезания (E:)"
	r := newRig(t, src, 480, mockH)

	if w := r.page.geo.sidebar.Max.X; w > 480*sidebarMaxShare/100 || w < sidebarWidth {
		t.Fatalf("panel width %d in a 480 px window", w)
	}
	row := r.page.rows[11]
	if got := navFont.Fit(nil, row.entry.Label, r.page.geo.sidebar.Max.X-row.textX()-6); !strings.HasSuffix(got, "…") {
		t.Fatalf("an oversized label must be cut with an ellipsis, got %q", got)
	}
	r.render()
}

func TestSidebarRowsKeepTheirSpacingAndHitTesting(t *testing.T) {
	eachScale(t, textScales[1:], func(t *testing.T, k float64, lang string) {
		r := scaledRig(t, mockW, 700, nil)
		extra := r.page.m.nav.extra
		if extra <= 0 {
			t.Fatalf("rows must grow, extra = %d", extra)
		}
		base := buildNavRows(mockSidebar(), navMetricsFor(1))
		for i, row := range r.page.rows {
			if i > 0 {
				want := base[i].center - base[i-1].center + float64(extra)
				if got := row.center - r.page.rows[i-1].center; got != want {
					t.Errorf("row %d pitch %v, want %v", i, got, want)
				}
			}
			pill := row.rect(r.page.geo.sidebar.Min.Y, r.page.geo.sidebar.Max.X)
			if pill.Dy() != 24+extra {
				t.Errorf("row %d pill height %d", i, pill.Dy())
			}
			if row.entry.Header || row.entry.Disabled {
				continue
			}
			y := r.page.geo.sidebar.Min.Y + int(row.center)
			if got := r.page.hitAt(30, y); got.kind != hitNav || got.index != i {
				t.Errorf("row %d at y=%d hit %+v", i, y, got)
			}
		}
		last := r.page.rows[len(r.page.rows)-1]
		if bottom := r.page.geo.sidebar.Min.Y + int(last.center) + 12; bottom > r.page.geo.sidebar.Max.Y {
			t.Errorf("a 700 px window must show every row, last ends at %d of %d", bottom, r.page.geo.sidebar.Max.Y)
		}
	})
}

func TestSidebarScrollsToTheLastRowAtLargeText(t *testing.T) {
	eachScale(t, []float64{1.6}, func(t *testing.T, k float64, lang string) {
		r := scaledRig(t, mockW, 460, nil)
		last := r.page.rows[len(r.page.rows)-1]
		sel := selectedNavRow(r)
		pill := sel.rect(r.page.geo.sidebar.Min.Y-r.page.sideY, r.page.geo.sidebar.Max.X)
		if !pill.In(r.page.geo.sidebar) {
			t.Fatalf("the selected row %v must start in view %v", pill, r.page.geo.sidebar)
		}

		r.page.OnMouseWheelPixelsMod(60, 150, 0, 1e6, 0)

		bottom := r.page.geo.sidebar.Min.Y - r.page.sideY + int(last.center) + 12 + last.extra/2
		if bottom > r.page.geo.sidebar.Max.Y {
			t.Fatalf("the last row ends at %d, panel ends at %d", bottom, r.page.geo.sidebar.Max.Y)
		}
	})
}

func TestStatusBarShowsEverySegmentWithoutOverlappingTheButtons(t *testing.T) {
	eachScale(t, textScales, func(t *testing.T, k float64, lang string) {
		r := scaledRig(t, mockW, mockH, nil)
		g := r.page.geo
		x := statusTextX
		for _, part := range r.src.status {
			x += statusFont.Measure(nil, part) + r.page.m.statusGap
		}
		x -= r.page.m.statusGap
		if limit := g.open.Min.X - statusLimitGap; x > limit {
			t.Errorf("segments end at %d, the buttons start at %d (limit %d)", x, g.open.Min.X, limit)
		}
		img := r.render()
		zone := image.Rect(g.open.Min.X-statusLimitGap, g.statusY+1, g.open.Min.X-2, g.win.Max.Y-1)
		for y := zone.Min.Y; y < zone.Max.Y; y++ {
			for xx := zone.Min.X; xx < zone.Max.X; xx++ {
				if got := img.RGBAAt(xx, y); got != r.th.Status {
					t.Fatalf("status text reaches the button zone at (%d,%d) = %v", xx, y, got)
				}
			}
		}
	})
}

func TestStatusSegmentsAreDroppedWholeWhenTheyDoNotFit(t *testing.T) {
	withScale(t, 1.6, "ru")
	r := scaledRig(t, 420, mockH, nil)

	img := r.render()

	g := r.page.geo
	zone := image.Rect(g.open.Min.X-statusLimitGap, g.statusY+1, g.open.Min.X-2, g.win.Max.Y-1)
	for y := zone.Min.Y; y < zone.Max.Y; y++ {
		for x := zone.Min.X; x < zone.Max.X; x++ {
			if img.RGBAAt(x, y) != r.th.Status {
				t.Fatalf("a partly clipped segment at (%d,%d)", x, y)
			}
		}
	}
}

func TestButtonsGrowWithTheirLabelsAndKeepTheStatusBarProportions(t *testing.T) {
	eachScale(t, textScales, func(t *testing.T, k float64, lang string) {
		r := scaledRig(t, mockW, mockH, nil)
		g := r.page.geo
		label := buttonFont.Measure(nil, i18n.T("cmd.open"))
		if g.open.Dx() < label+openLabelPadding {
			t.Errorf("open button %d px wide for a %d px label", g.open.Dx(), label)
		}
		m := fonts.MetricsOf(fonts.UI, buttonFont.Size(), 96)
		if g.open.Dy() < m.Ascent+m.Descent+8 {
			t.Errorf("open button %d px tall for a %d px line", g.open.Dy(), m.Ascent+m.Descent)
		}
		if g.open.Dy() != g.more.Dy() || g.open.Min.Y != g.more.Min.Y || g.more.Min.X-g.open.Max.X != 2 {
			t.Errorf("buttons %v and %v must stay a pair", g.open, g.more)
		}
		above, below := g.open.Min.Y-g.statusY, g.win.Max.Y-g.open.Max.Y
		if above < 6 || below < 6 || above-below > 1 || below-above > 1 {
			t.Errorf("buttons %v must sit in the middle of the status bar from %d to %d (above %d, below %d)", g.open, g.statusY, g.win.Max.Y, above, below)
		}
		img := r.render()
		ink := inkBoxIn(img, g.open.Inset(2))
		if ink.Min.X <= g.open.Min.X+2 || ink.Max.X >= g.open.Max.X-2 || ink.Min.Y <= g.open.Min.Y || ink.Max.Y >= g.open.Max.Y {
			t.Errorf("the label ink %v touches the button %v", ink, g.open)
		}
	})
}

func TestAddressTextAndBubbleStayInsideTheirBoxes(t *testing.T) {
	eachScale(t, textScales, func(t *testing.T, k float64, lang string) {
		r := scaledRig(t, mockW, mockH, nil)
		a := r.page.geo.address
		m := fonts.MetricsOf(fonts.UI, addressFont.Size(), 96)
		base := a.Min.Y + 18 + r.page.m.addrMid
		if base-m.Ascent < a.Min.Y-1 || base+m.Descent > a.Max.Y {
			t.Errorf("address text rows %d..%d leave the bar %d..%d", base-m.Ascent, base+m.Descent, a.Min.Y, a.Max.Y)
		}
		r.rec.addrErr = &model.AddressError{Path: `D:\Nope`, Err: model.ErrNotFound}
		r.page.FocusAddress()
		r.key(widget.KeyEnter, 0)
		if r.page.bubble == "" || r.page.m.bubbleH < bubbleBaseHeight {
			t.Fatalf("bubble %q height %d", r.page.bubble, r.page.m.bubbleH)
		}
		if r.page.m.bubbleH < m.Ascent+m.Descent+8 {
			t.Errorf("bubble %d px for a %d px line", r.page.m.bubbleH, m.Ascent+m.Descent)
		}
		r.render()
		if area := r.page.bubbleArea(); area.Dy() < r.page.m.bubbleH+8 {
			t.Errorf("invalidation area %v is smaller than the bubble", area)
		}
	})
}

func TestTileLabelsAreCutOnlyByAnEllipsisInsideTheCell(t *testing.T) {
	eachScale(t, textScales, func(t *testing.T, k float64, lang string) {
		r := scaledRig(t, mockW, mockH, func(s *fakeSource) {
			s.items[1].Name = "an-extremely-long-photograph-name-that-cannot-fit.jpg"
			s.items[2].Name = "IMG_20240914_182700.jpg"
		})
		img := r.render()

		for i := 1; i < 8; i++ {
			cell := r.page.tileRect(i)
			name := r.src.items[i].Name
			text, w := r.page.fitted(nil, labelFont, name, cell.Dx()-8)
			if text != name && !strings.HasSuffix(text, "…") {
				t.Errorf("%q was cut to %q without an ellipsis", name, text)
			}
			if w > cell.Dx()-8 {
				t.Errorf("%q is %d px wide in a %d px cell", text, w, cell.Dx())
			}
			base := r.page.photoRect(i).Max.Y + labelBelow - photoInsetY + r.page.m.labelAsc
			if desc := fonts.MetricsOf(fonts.UI, labelFont.Size(), 96).Descent; base+desc >= cell.Max.Y {
				t.Errorf("tile %d: the label descends to %d, the cell ends at %d", i, base+desc, cell.Max.Y)
			}
			band := image.Rect(cell.Min.X, base-labelFont.Ascent(), cell.Max.X, base+4).Intersect(r.page.geo.grid)
			ink := inkBoxIn(img, band)
			if !ink.Empty() && (ink.Min.X <= cell.Min.X+1 || ink.Max.X >= cell.Max.X-1) {
				t.Errorf("tile %d: label ink %v leaves the cell %v", i, ink, cell)
			}
		}
		if text, _ := r.page.fitted(nil, labelFont, mockNames[0], r.page.tileRect(0).Dx()-8); text != mockNames[0] {
			t.Errorf("a short name %q must stay whole, got %q", mockNames[0], text)
		}
	})
}

func TestRowPitchGrowsWithTheLabelAndHitTestingFollows(t *testing.T) {
	eachScale(t, textScales, func(t *testing.T, k float64, lang string) {
		r := scaledRig(t, mockW, mockH, func(s *fakeSource) { s.items = manyItems(200) })
		cols := r.page.grid.Columns()
		pitch := r.page.tile.cellH + gridGapY
		if want := 97 + 5 + r.page.m.labelExtra; pitch != want {
			t.Fatalf("pitch %d, want %d", pitch, want)
		}
		if r.page.tile.cellW != 104 || r.page.tile.photoW != 95 || r.page.tile.photoH != 71 {
			t.Fatalf("the photo must not change: %+v", r.page.tile)
		}

		for _, i := range []int{0, cols, 3*cols + 1, 7*cols + cols - 1} {
			r.page.ScrollTo(i)
			cell := r.page.tileRect(i)
			if !cell.In(r.page.geo.grid) {
				t.Errorf("tile %d at %v is outside %v after ScrollTo", i, cell, r.page.geo.grid)
			}
			x, y := (cell.Min.X+cell.Max.X)/2, cell.Min.Y+3
			if got := r.page.hitAt(x, y); got.kind != hitTile || got.index != i {
				t.Errorf("tile %d: hit %+v at (%d,%d)", i, got, x, y)
			}
			y = cell.Max.Y + 1
			if got := r.page.hitAt(x, y); got.kind == hitTile && got.index == i {
				t.Errorf("tile %d: the gap below the cell must not hit it", i)
			}
		}
		r.page.setScroll(0)
		r.page.OnMouseWheelPixelsMod(400, 200, 0, wheelNotchPixels, 0)
		if want := pitch / 3; r.page.scroll != want {
			t.Errorf("one notch scrolls %d, want a third of the pitch %d", r.page.scroll, want)
		}
	})
}

func TestVirtualisationFollowsTheGrowingRows(t *testing.T) {
	eachScale(t, textScales, func(t *testing.T, k float64, lang string) {
		r := scaledRig(t, mockW, mockH, func(s *fakeSource) { s.items = manyItems(5000) })
		r.page.setScroll(r.page.grid.MaxScroll() / 3)
		r.src.mu.Lock()
		r.src.calls = nil
		r.src.mu.Unlock()

		r.render()

		first, last := r.page.grid.Visible(r.page.scroll)
		visible := 0
		for _, c := range r.src.thumbCalls() {
			if c.visible {
				visible++
			}
		}
		if visible != last-first || last-first > 6*r.page.grid.Columns() {
			t.Errorf("%d visible requests for the range %d..%d", visible, first, last)
		}
		rowsInView := (r.page.geo.grid.Dy() + gridGapY) / (r.page.tile.cellH + gridGapY)
		if got := (last - first) / r.page.grid.Columns(); got < rowsInView || got > rowsInView+2 {
			t.Errorf("%d rows drawn for %d fully visible rows", got, rowsInView)
		}
		if n := len(r.rec.visible); n == 0 || r.rec.visible[n-1] != [2]int{first, last} {
			t.Errorf("reported %v, grid says %d..%d", r.rec.visible, first, last)
		}
	})
}

func TestListRowsAndColumnsFitTheLargerText(t *testing.T) {
	eachScale(t, textScales, func(t *testing.T, k float64, lang string) {
		r := scaledRig(t, 900, mockH, func(s *fakeSource) {
			s.mode = ViewList
			s.items = manyItems(40)
			for i := range s.items {
				s.items[i].ModTime = sampleTime
			}
		})
		m := r.page.m
		row := r.page.tileRect(2)
		if row.Dy() != m.listRow || row.Dy() < listRowHeight {
			t.Fatalf("row height %d", row.Dy())
		}
		line := fonts.MetricsOf(fonts.UI, listFont.Size(), 96)
		if row.Dy() < line.Ascent+line.Descent+8 {
			t.Errorf("a %d px row for a %d px line", row.Dy(), line.Ascent+line.Descent)
		}
		date := listSubFont.Measure(nil, i18n.FormatDateTime(sampleTime))
		size := listSubFont.Measure(nil, i18n.FormatBytes(1023<<20))
		if date+listColumnPadding > m.listDateW || size+listColumnPadding > m.listSizeW {
			t.Errorf("columns %d/%d for texts %d/%d", m.listDateW, m.listSizeW, date, size)
		}
		if row.Dx() < m.listWide() {
			t.Fatalf("a 900 px window must show the columns: row %d, need %d", row.Dx(), m.listWide())
		}
		nameX := row.Min.X + listPad + listThumb + 12
		nameRight := row.Max.X - 14 - m.listColumns()
		if nameRight-nameX < 120 {
			t.Errorf("only %d px left for names", nameRight-nameX)
		}
		if r.render() == nil {
			t.Fatal("no frame")
		}
		r.page.ScrollTo(30)
		if !r.page.tileRect(30).In(r.page.geo.grid) {
			t.Errorf("row 30 at %v is outside %v", r.page.tileRect(30), r.page.geo.grid)
		}
		row = r.page.tileRect(30)
		if got := r.page.hitAt(row.Min.X+200, row.Min.Y+5); got.kind != hitTile || got.index != 30 {
			t.Errorf("hit %+v", got)
		}
	})
}

func TestHeaderControlsDoNotMoveWithTheText(t *testing.T) {
	base := newRig(t, mockSource(), mockW, mockH)
	want := base.page.HeaderHoles()

	for _, k := range textScales[1:] {
		withScale(t, k, "ru")
		r := newRig(t, mockSource(), mockW, mockH)
		got := r.page.HeaderHoles()
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("k=%v: hole %d %v, want %v", k, i, got[i], want[i])
			}
		}
	}
}

func TestChangingTheScaleRelaysOutWithoutARefresh(t *testing.T) {
	withScale(t, 1, "ru")
	r := scaledRig(t, mockW, mockH, nil)
	r.render()
	before := r.page.geo.sidebar.Max.X

	fonts.SetTextScale(1.6)
	r.render()

	if r.page.geo.sidebar.Max.X <= before || r.page.m.k != 1.6 {
		t.Fatalf("panel %d -> %d, k = %v", before, r.page.geo.sidebar.Max.X, r.page.m.k)
	}
	fonts.SetTextScale(1)
	r.render()
	if r.page.geo.sidebar.Max.X != before {
		t.Fatalf("panel did not return to %d: %d", before, r.page.geo.sidebar.Max.X)
	}
}

func TestEditorStaysInsideTheAddressBarAtLargeText(t *testing.T) {
	eachScale(t, textScales[1:], func(t *testing.T, k float64, lang string) {
		r := scaledRig(t, mockW, mockH, nil)
		r.page.FocusAddress()

		in := r.page.input.Bounds()
		a := r.page.geo.address
		if !in.In(a) || in.Max.X > r.page.geo.filter.Min.X {
			t.Fatalf("editor %v outside the bar %v", in, a)
		}
		r.render()
	})
}

func TestScaledSnapshots(t *testing.T) {
	dir := testutil.OutDir()
	if dir == "" {
		t.Skip("PIXGO_SNAPSHOTS is not set")
	}
	cases := []struct {
		name string
		lang string
		w, h int
		mode ViewMode
		edit bool
	}{
		{"gallery_k12_ru_grid", "ru", mockW, mockH, ViewGrid, false},
		{"gallery_k12_ru_list", "ru", mockW, mockH, ViewList, false},
		{"gallery_k12_en_grid", "en", mockW, mockH, ViewGrid, false},
		{"gallery_k12_ru_tall", "ru", 760, 560, ViewGrid, false},
		{"gallery_k12_ru_edit", "ru", mockW, mockH, ViewGrid, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			withScale(t, 1.2, c.lang)
			r := scaledRig(t, c.w, c.h, func(s *fakeSource) {
				s.mode = c.mode
				s.items[2].Name = "an-extremely-long-photograph-name.jpg"
			})
			if c.edit {
				r.page.FocusAddress()
				r.rec.addrErr = &model.AddressError{Path: `D:\Nope`, Err: model.ErrNotFound}
				r.key(widget.KeyEnter, 0)
			}
			if err := testutil.SavePNG(filepath.Join(dir, c.name+".png"), r.render()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
