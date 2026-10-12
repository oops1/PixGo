package gallery

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/testutil"
)

func TestHoverAndPressedStatesChangeTheHeaderButtons(t *testing.T) {
	for _, c := range []struct {
		name string
		x, y int
	}{
		{"back", 25, 22}, {"up", 77, 22}, {"address", 200, 22}, {"filter", 345, 22}, {"refresh", 368, 22},
		{"list", 406, 22}, {"grid inactive", 434, 22}, {"thumbs", 465, 22}, {"open", 496, 400}, {"more", 560, 400},
		{"tile", 316, 100}, {"sidebar", 60, 165},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := mockSource()
			if c.name == "grid inactive" {
				src.mode = ViewList
			}
			r := newRig(t, src, mockW, mockH)
			plain := r.render()

			r.page.OnMouseMove(c.x, c.y)
			hover := r.render()
			r.page.OnMouseButton(widget.MouseEvent{X: c.x, Y: c.y, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
			pressed := r.render()
			r.page.OnMouseButton(widget.MouseEvent{X: c.x, Y: c.y, Button: widget.MouseLeft, Pressed: false, Clicks: 1})

			if string(plain.Pix) == string(hover.Pix) {
				t.Error("hovering must be visible")
			}
			if c.name == "tile" || c.name == "sidebar" || c.name == "address" {
				return
			}
			if string(hover.Pix) == string(pressed.Pix) {
				t.Error("pressing must be visible")
			}
		})
	}
}

func TestActiveViewModeIsHighlighted(t *testing.T) {
	grid := newRig(t, mockSource(), mockW, mockH)
	src := mockSource()
	src.mode = ViewList
	list := newRig(t, src, mockW, mockH)

	a, b := grid.render(), list.render()

	left := image_pt(406, 12)
	right := image_pt(434, 12)
	if !(brighter(a.RGBAAt(right.X, right.Y), a.RGBAAt(left.X, left.Y)) && brighter(b.RGBAAt(left.X, left.Y), b.RGBAAt(right.X, right.Y))) {
		t.Fatalf("grid: left %v right %v; list: left %v right %v", a.RGBAAt(left.X, left.Y), a.RGBAAt(right.X, right.Y), b.RGBAAt(left.X, left.Y), b.RGBAAt(right.X, right.Y))
	}
}

func brighter(a, b color.RGBA) bool { return int(a.R)+int(a.G)+int(a.B) > int(b.R)+int(b.G)+int(b.B) }

type point struct{ X, Y int }

func image_pt(x, y int) point { return point{x, y} }

func TestScrollbarThumbWidensWhenDragged(t *testing.T) {
	r := longRig(t, 60)
	r.page.OnMouseMove(588, 200)
	_, thin, _ := r.page.gridBarRects()
	r.page.OnMouseMove(thin.Min.X+1, thin.Min.Y+2)
	_, hovered, _ := r.page.gridBarRects()

	r.render()

	if hovered.Dx() <= barThin {
		t.Fatalf("hovered thumb %v must be wider", hovered)
	}
}

func TestNavigationRefreshResetsTheScroll(t *testing.T) {
	r := longRig(t, 60)
	r.page.setScroll(500)

	r.src.loc.Path = `D:\Elsewhere`
	r.page.Refresh()

	if r.page.scroll != 0 {
		t.Fatalf("scroll = %d", r.page.scroll)
	}
}

func TestSameFolderRefreshKeepsTheScroll(t *testing.T) {
	r := longRig(t, 60)
	r.page.setScroll(500)

	r.page.Refresh()

	if r.page.scroll != 500 {
		t.Fatalf("scroll = %d", r.page.scroll)
	}
}

func TestChangingTheThumbnailSizeRebakesTiles(t *testing.T) {
	src := mockSource()
	src.images[mockPath("lake.jpg")] = solid(96, 64, color.RGBA{200, 30, 30, 255})
	r := newRig(t, src, mockW, mockH)
	r.render()
	if r.page.tiles.size() == 0 {
		t.Fatal("a tile must have been baked")
	}

	src.side = 128
	r.page.Refresh()

	if r.page.tiles.size() != 0 {
		t.Fatalf("stale tiles kept after a size change: %d", r.page.tiles.size())
	}
	r.render()
	if r.page.tile.photoW != 127 || r.page.tile.photoH != 95 {
		t.Fatalf("tile geometry = %+v", r.page.tile)
	}
	if got := r.page.tileRect(0).Dx(); got != 136 {
		t.Fatalf("cell width = %d", got)
	}
}

func TestSizeChangeThroughTheMenuAppliesImmediately(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.click(465, 22)

	r.rec.menuItems[3].OnClick()

	if r.page.snap.side != 192 {
		t.Fatalf("side = %d", r.page.snap.side)
	}
}

func TestFilterChoiceResetsTheScroll(t *testing.T) {
	r := longRig(t, 60)
	r.page.setScroll(400)
	r.click(345, 22)

	r.rec.menuItems[1].OnClick()

	if r.page.scroll != 0 || r.page.snap.format != "JPEG" {
		t.Fatalf("scroll = %d, format = %q", r.page.scroll, r.page.snap.format)
	}
}

func TestPageWorksWithoutTimersAndClock(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.deps.After = nil
	r.page.deps.Now = nil

	r.typeRune('f')
	r.doubleClick(r.tileCenter(2))
	r.page.OnMouseWheelPixelsMod(300, 200, 0, 40, 0)
	r.page.FocusAddress()
	r.rec.addrErr = nil
	r.key(widget.KeyEnter, 0)

	if r.page.Selected() != 2 {
		t.Fatalf("selected = %d", r.page.Selected())
	}
}

func TestFramedBoundsRenderLikeTheFullWindow(t *testing.T) {
	full := newRig(t, mockSource(), mockW, mockH)
	framed := newRig(t, mockSource(), mockW, mockH)
	framed.page.SetBounds(image.Rect(1, 1, 592, 429))
	framed.page.Refresh()

	a, b := full.render(), framed.render()

	inner := image.Rect(2, 2, 591, 428)
	if e := testutil.MeanError(a, b, inner); e > 0.01 {
		t.Fatalf("mean error between the full and the framed render: %.3f", e)
	}
	if got := b.RGBAAt(0, 200); got == framed.th.GalleryNav {
		t.Fatalf("the page must not paint over the window frame column, got %v", got)
	}
}

func TestPageDoesNotReadTheSourceBeforeItIsShown(t *testing.T) {
	eng := testutil.NewEngine(mockW, mockH)
	src := &explodingSource{fakeSource: mockSource()}

	page := New(eng, Deps{Source: src})
	page.SetBounds(image.Rect(0, 0, mockW, mockH))
	page.HeaderHoles()

	if src.reads != 0 {
		t.Fatalf("%d reads of the source during construction", src.reads)
	}
}

type explodingSource struct {
	*fakeSource
	reads int
}

func (e *explodingSource) Location() model.Location {
	e.reads++
	return e.fakeSource.Location()
}
