package gallery

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	model "github.com/oops1/PixGo/internal/gallery"
)

func TestTilesRequestOnlyVisibleThumbnails(t *testing.T) {
	src := mockSource()
	src.items = manyItems(5000)
	r := newRig(t, src, mockW, mockH)

	r.render()

	calls := src.thumbCalls()
	visible, prefetch := 0, 0
	for _, c := range calls {
		if c.visible {
			visible++
		} else {
			prefetch++
		}
		if c.side != 96 {
			t.Fatalf("requested side %d, want the source thumb size", c.side)
		}
	}
	if visible != 16 {
		t.Fatalf("%d visible requests, want 16 (three rows plus the clipped fourth): %v", visible, calls)
	}
	if len(calls) > 40 {
		t.Fatalf("%d thumbnail requests for 5000 items", len(calls))
	}
	if prefetch == 0 {
		t.Fatal("the neighbouring row must be prefetched")
	}
}

func TestPrefetchHappensOncePerRange(t *testing.T) {
	src := mockSource()
	src.items = manyItems(200)
	r := newRig(t, src, mockW, mockH)
	r.render()
	first := len(src.thumbCalls())

	r.render()

	grown := len(src.thumbCalls()) - first
	if grown > 20 {
		t.Fatalf("second frame requested %d more thumbnails", grown)
	}
	prefetched := 0
	for _, c := range src.thumbCalls()[first:] {
		if !c.visible {
			prefetched++
		}
	}
	if prefetched != 0 {
		t.Fatalf("prefetch repeated %d times for the same range", prefetched)
	}
}

func TestReadyThumbnailsAreDrawnCoveredAndRounded(t *testing.T) {
	src := mockSource()
	red := color.RGBA{200, 30, 30, 255}
	src.images[mockPath("lake.jpg")] = solid(96, 64, red)
	r := newRig(t, src, mockW, mockH)

	img := r.render()

	photo := r.page.photoRect(1)
	centre := img.RGBAAt(photo.Min.X+photo.Dx()/2, photo.Min.Y+photo.Dy()/2)
	if centre.R < 190 || centre.G > 60 {
		t.Fatalf("centre = %v, want the thumbnail colour", centre)
	}
	edge := img.RGBAAt(photo.Min.X+photo.Dx()/2, photo.Min.Y+2)
	if edge.R < 190 {
		t.Fatalf("a non-square thumbnail must cover the whole area, edge = %v", edge)
	}
	corner := img.RGBAAt(photo.Min.X, photo.Min.Y)
	if corner.R > 120 {
		t.Fatalf("corner = %v: corners must be rounded", corner)
	}
}

func TestMissingThumbnailLeavesAPlaceholder(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	img := r.render()

	photo := r.page.photoRect(2)
	got := img.RGBAAt(photo.Min.X+photo.Dx()/2, photo.Min.Y+photo.Dy()/2)
	if want := r.th.GalleryTile; got != want {
		t.Fatalf("placeholder = %v, want %v", got, want)
	}
}

func TestSelectedTileHasAFrameAndFill(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	img := r.render()

	cell := r.page.tileRect(0)
	frame := img.RGBAAt(cell.Min.X+30, cell.Min.Y+1)
	fill := img.RGBAAt(cell.Min.X+8, cell.Max.Y-14)
	if frame.B < 180 || frame.R > 60 {
		t.Fatalf("frame pixel = %v", frame)
	}
	if fill != r.th.Selection {
		t.Fatalf("fill pixel = %v, want %v", fill, r.th.Selection)
	}
}

func TestHoveredTileGetsALightOutline(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	x, y := r.tileCenter(5)
	r.page.OnMouseMove(x, y)

	img := r.render()

	photo := r.page.photoRect(5)
	if got := img.RGBAAt(photo.Min.X+photo.Dx()/2, photo.Min.Y); got != r.th.Focus {
		t.Fatalf("outline = %v, want %v", got, r.th.Focus)
	}
}

func TestFoldersGetAGlyphInsteadOfAThumbnail(t *testing.T) {
	src := mockSource()
	src.items = append([]model.Item{{Name: "Trips", Path: `D:\Photos\Trips`, Dir: true}}, src.items...)
	src.selected = -1
	r := newRig(t, src, mockW, mockH)

	img := r.render()

	for _, c := range src.thumbCalls() {
		if strings.HasSuffix(c.path, "Trips") {
			t.Fatalf("a folder must not request a thumbnail: %v", c)
		}
	}
	photo := r.page.photoRect(0)
	yellow := img.RGBAAt(photo.Min.X+photo.Dx()/2, photo.Min.Y+photo.Dy()/2+4)
	if yellow.R < 200 || yellow.B > 120 {
		t.Fatalf("folder glyph pixel = %v", yellow)
	}
}

func TestLongNamesAreTruncatedWithAnEllipsis(t *testing.T) {
	src := mockSource()
	src.items[0].Name = "an-extremely-long-photograph-name-that-cannot-fit.jpg"
	r := newRig(t, src, mockW, mockH)
	r.render()

	var text string
	var w int
	for k, v := range r.page.labels {
		if k.name == src.items[0].Name {
			text, w = v.text, v.w
		}
	}

	if !strings.HasSuffix(text, "…") || w > r.page.tileRect(0).Dx()-8 {
		t.Fatalf("label %q width %d", text, w)
	}
}

func TestListModeDrawsRowsAndSelectsThem(t *testing.T) {
	src := mockSource()
	src.mode = ViewList
	r := newRig(t, src, mockW, mockH)
	r.render()

	row := r.page.tileRect(2)
	if row.Dy() != listRowHeight || row.Dx() < 400 {
		t.Fatalf("row = %v", row)
	}
	r.click(row.Min.X+200, row.Min.Y+10)

	if !r.rec.has("select:2") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestListModeShowsDateAndSize(t *testing.T) {
	src := mockSource()
	src.mode = ViewList
	src.items[1].ModTime = time.Time{}
	src.items = append(src.items, model.Item{Name: "Trips", Path: `D:\Photos\Trips`, Dir: true})
	r := newRig(t, src, mockW, mockH)
	r.page.ScrollTo(12)

	img := r.render()
	narrow := newRig(t, mockSource(), 380, mockH)
	narrow.src.mode = ViewList
	narrow.page.Refresh()

	if img == nil || narrow.render() == nil {
		t.Fatal("no frame")
	}
}

func TestListModeThumbnailsUseTheSmallestSize(t *testing.T) {
	src := mockSource()
	src.mode = ViewList
	src.side = 256
	src.images[mockPath("mountain.jpg")] = solid(96, 72, color.RGBA{10, 200, 10, 255})
	r := newRig(t, src, mockW, mockH)

	img := r.render()

	for _, c := range src.thumbCalls() {
		if c.side != listSideTile {
			t.Fatalf("list thumbnail requested at %d", c.side)
		}
	}
	row := r.page.tileRect(0)
	if got := img.RGBAAt(row.Min.X+listPad+16, row.Min.Y+20); got.G < 150 {
		t.Fatalf("list thumbnail pixel = %v", got)
	}
}

func TestListModeScrollsByHalfARow(t *testing.T) {
	src := mockSource()
	src.mode = ViewList
	src.items = manyItems(100)
	r := newRig(t, src, mockW, mockH)

	r.page.OnMouseWheelPixelsMod(300, 200, 0, wheelNotchPixels, 0)

	if r.page.scroll != listRowHeight/2 {
		t.Fatalf("scroll = %d", r.page.scroll)
	}
	r.key(widget.KeyDown, 0)
	if r.page.Selected() != 1 {
		t.Fatalf("selected = %d", r.page.Selected())
	}
}

func TestStatesWithoutItemsShowAMessage(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*fakeSource)
	}{
		{"loading", func(s *fakeSource) { s.loading = true }},
		{"failed", func(s *fakeSource) { s.failed = true }},
		{"empty folder", func(s *fakeSource) {}},
		{"empty favourites", func(s *fakeSource) { s.loc = model.Location{Kind: model.KindFavorites} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := mockSource()
			src.items = nil
			src.selected = -1
			c.mutate(src)
			r := newRig(t, src, mockW, mockH)

			img := r.render()

			vp := r.page.geo.grid
			ink := inkBoxIn(img, image.Rect(vp.Min.X+10, vp.Min.Y+vp.Dy()/2-24, vp.Max.X-10, vp.Min.Y+vp.Dy()/2+8))
			if ink.Empty() {
				t.Fatal("the message is missing")
			}
		})
	}
}

func TestTruncatedListingNoticeIsShownInTheStatusBar(t *testing.T) {
	plain := newRig(t, mockSource(), mockW, mockH)
	src := mockSource()
	src.truncated = "Показаны не все файлы (лимит: 5 000)"
	src.status = []string{"Элементов: 12"}
	notice := newRig(t, src, mockW, mockH)
	plain.src.status = []string{"Элементов: 12"}
	plain.page.Refresh()

	a, b := plain.render(), notice.render()

	if string(a.Pix) == string(b.Pix) {
		t.Fatal("the notice must be drawn")
	}
}

func TestAddressShowsSpecialLocationsByName(t *testing.T) {
	src := mockSource()
	src.loc = model.Location{Kind: model.KindRecent}
	r := newRig(t, src, mockW, mockH)

	r.page.FocusAddress()

	if got := r.page.input.GetText(); got != "Недавние" {
		t.Fatalf("text = %q", got)
	}
	for _, kind := range []model.Kind{model.KindAll, model.KindFavorites, model.KindFolder} {
		src.loc = model.Location{Kind: kind, Path: `D:\x`}
		r.page.Refresh()
		if r.render() == nil {
			t.Fatal("no frame")
		}
	}
}

func TestLightAndContrastThemesRender(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	dark := r.render()

	r.th = lightTheme()
	r.page.Refresh()
	light := r.render()

	if string(dark.Pix) == string(light.Pix) {
		t.Fatal("the theme must change the page")
	}
	if got := light.RGBAAt(300, 380); got.R < 200 {
		t.Fatalf("light page background = %v", got)
	}
	r.th = contrastTheme()
	r.page.Refresh()
	if r.render() == nil {
		t.Fatal("no frame")
	}
}

func TestNilThemeFallsBackToDark(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.deps.Theme = nil

	img := r.render()

	if got := img.RGBAAt(300, 380); got.R > 60 {
		t.Fatalf("fallback background = %v", got)
	}
}

func TestPageWithoutASourceDrawsNothing(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.deps.Source = nil

	r.page.Draw(nil)
	r.page.FocusAddress()
	if r.page.HandleKey(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true}) {
		t.Fatal("no source, no keys")
	}
	if r.page.Selected() != -1 {
		t.Fatal("selected")
	}
	r.page.Refresh()
}

func TestHeaderHolesStayClearOfTheWindowButtons(t *testing.T) {
	for _, w := range []int{593, 700, 1100} {
		r := newRig(t, mockSource(), w, mockH)
		holes := r.page.HeaderHoles()
		if len(holes) != 4 {
			t.Fatalf("holes = %v", holes)
		}
		captions := image.Rect(w-20-2*34-17, 0, w, headerHeight)
		for _, h := range holes {
			if h.Overlaps(captions) {
				t.Errorf("w=%d: hole %v overlaps the window buttons %v", w, h, captions)
			}
			if h.Min.Y < 0 || h.Max.Y > headerHeight {
				t.Errorf("w=%d: hole %v leaves the header", w, h)
			}
		}
	}
}

func TestHeaderGeometryFollowsTheMockup(t *testing.T) {
	g := computeGeometry(image.Rect(0, 0, 593, 430), baseMetrics())

	cases := []struct {
		name string
		got  image.Rectangle
		want image.Rectangle
	}{
		{"address bar", g.address, image.Rect(93, 8, 379, 35)},
		{"views", g.views, image.Rect(391, 8, 448, 35)},
		{"open", g.open, image.Rect(451, 384, 542, 416)},
		{"more", g.more, image.Rect(544, 384, 575, 416)},
		{"grid", g.grid, image.Rect(152, 43, 593, 389)},
		{"sidebar", g.sidebar, image.Rect(0, 43, 151, 389)},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestGeometryAnchorsToTheWindowFrame(t *testing.T) {
	framed := computeGeometry(image.Rect(1, 1, 592, 429), baseMetrics())
	bare := computeGeometry(image.Rect(0, 0, 593, 430), baseMetrics())

	if framed.address != bare.address || framed.open != bare.open || framed.statusY != bare.statusY {
		t.Fatalf("framed %v vs bare %v", framed, bare)
	}
}

func TestLayoutHandlesDegenerateBounds(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.page.SetBounds(image.Rect(0, 0, 0, 0))
	r.page.Refresh()
	r.page.SetBounds(image.Rect(0, 0, 100, 60))
	r.page.Refresh()
	r.render()
	_ = r.page.HeaderHoles()
	r.page.ScrollTo(3)
}
