package window

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

var textScales = []float64{1, 1.2, 1.33, 1.6}

func forEachScale(t *testing.T, f func(t *testing.T, k float64, lang string)) {
	t.Helper()
	for _, lang := range []string{"ru", "en"} {
		for _, k := range textScales {
			t.Run(fmt.Sprintf("%s k=%.2f", lang, k), func(t *testing.T) { f(t, k, lang) })
		}
	}
}

func newScaleRig(t *testing.T, k float64, lang string, w, h int, mutate func(*fakeHost)) *rig {
	t.Helper()
	oldScale, oldLang := fonts.TextScale(), i18n.Lang()
	fonts.SetTextScale(k)
	t.Cleanup(func() {
		fonts.SetTextScale(oldScale)
		i18n.SetLang(oldLang)
	})
	r := newSizedRig(t, w, h, mutate)
	i18n.SetLang(lang)
	return r
}

func (r *rig) frame() *image.RGBA {
	r.root.Refresh()
	r.eng.Flush()
	return r.eng.RenderOnce()
}

func insideRect(inner, outer image.Rectangle) bool { return !inner.Empty() && inner.In(outer) }

func centerY(r image.Rectangle) float64 { return float64(r.Min.Y+r.Max.Y) / 2 }

func interactiveOverlap(a, b Zone) bool {
	skip := func(k ZoneKind) bool { return k == ZoneNone || k == ZoneDrag || k == ZoneSlideBar || isResize(k) }
	return !skip(a.Kind) && !skip(b.Kind) && a.Rect.Overlaps(b.Rect)
}

func TestTextScaleOneKeepsTheMockupGeometry(t *testing.T) {
	r := newScaleRig(t, 1, "ru", 770, 504, nil)
	l := r.root.Layout()
	want := Compute(770, 504, Options{Toolbar: true, Arrows: true, Resizable: true})
	if len(l.Zones) != len(want.Zones) {
		t.Fatalf("%d zones, want %d", len(l.Zones), len(want.Zones))
	}
	for i := range want.Zones {
		if l.Zones[i] != want.Zones[i] {
			t.Errorf("zone %d = %+v, want %+v", i, l.Zones[i], want.Zones[i])
		}
	}
	fs := newScaleRig(t, 1, "ru", mockupW, mockupH, func(h *fakeHost) { h.fullscreen = true; h.slide = slideshowMockupView() })
	if got, want := fs.root.Layout().Slide, mockupLayout().Slide; got != want {
		t.Errorf("slide geometry at scale 1 = %+v, want %+v", got, want)
	}
	for _, total := range []int{3, 24, 99} {
		fs.host.slide.Total = total
		if got := fs.root.Layout().Slide.Panel; got != image.Rect(18, 287, 310, 333) {
			t.Errorf("total %d: panel %v", total, got)
		}
	}
}

func TestViewerTitleStaysBetweenTheBackAndInfoButtons(t *testing.T) {
	long := strings.Repeat("Длинное имя файла ", 12) + ".jpg — 6000 × 4000"
	forEachScale(t, func(t *testing.T, k float64, lang string) {
		for _, c := range []struct {
			name, title string
			centred     bool
		}{{"short", "ABC 123", true}, {"long", long, false}} {
			for _, size := range []image.Point{{770, 504}, {420, 320}} {
				r := newScaleRig(t, k, lang, size.X, size.Y, func(h *fakeHost) { h.title = c.title })
				l := r.root.Layout()
				img := r.frame()
				bar := r.root.Theme.TitleBar
				limit := l.Rect(ZoneInfo).Min.X - titleGap
				area := image.Rect(TitleTextX-6, 2, l.Rect(ZoneInfo).Min.X, TitleHeight-2)
				box := inkWhere(img, area, func(p color.RGBA) bool { return channelGap(p, bar) > 60 })
				if box.Empty() {
					t.Fatalf("%s %v: no title drawn", c.name, size)
				}
				if box.Min.X < TitleTextX-3 || box.Max.X > limit+1 {
					t.Errorf("%s %v: title ink %v leaves [%d, %d]", c.name, size, box, TitleTextX-3, limit)
				}
				if box.Min.Y < 6 || box.Max.Y > TitleHeight-6 {
					t.Errorf("%s %v: title ink %v sticks out of the bar", c.name, size, box)
				}
				if c.centred && math.Abs(centerY(box)-24.5) > 1.5 {
					t.Errorf("%s %v: title is not centred vertically: ink %v", c.name, size, box)
				}
				if c.centred && size.X >= 770 && box.Dx() < textWidth(titleFont, c.title)-4 {
					t.Errorf("%s %v: title is cut: ink %v for a text %d wide", c.name, size, box, textWidth(titleFont, c.title))
				}
			}
		}
	})
}

func TestZoomLabelFitsItsZoneAndTouchesNoNeighbour(t *testing.T) {
	forEachScale(t, func(t *testing.T, k float64, lang string) {
		for _, zoom := range []string{"5%", "100%", "6400%"} {
			for _, size := range []image.Point{{770, 504}, {1920, 1080}} {
				r := newScaleRig(t, k, lang, size.X, size.Y, func(h *fakeHost) { h.zoom = zoom })
				l := r.root.Layout()
				img := r.frame()
				z, out, in := l.Rect(ZoneToolZoomLabel), l.Rect(ZoneToolZoomOut), l.Rect(ZoneToolZoomIn)
				if z.Dx() < ToolZoomLabelW || k == 1 && z.Dx() != ToolZoomLabelW {
					t.Errorf("%q %v: label zone %v is %d wide", zoom, size, z, z.Dx())
				}
				if out.Max.X > z.Min.X || z.Max.X > in.Min.X {
					t.Errorf("%q %v: label zone %v overlaps %v or %v", zoom, size, z, out, in)
				}
				area := image.Rect(out.Max.X, l.Toolbar.Min.Y+3, in.Min.X, l.Toolbar.Max.Y-3)
				box := inkWhere(img, area, lightAbove(150))
				if !insideRect(box, z) {
					t.Errorf("%q %v: label ink %v is outside its zone %v", zoom, size, box, z)
				}
				if math.Abs(centerY(box)-centerY(l.Toolbar)) > 2 {
					t.Errorf("%q %v: label ink %v is off the toolbar centre %v", zoom, size, box, centerY(l.Toolbar))
				}
				if box.Dx() < textWidth(zoomFont, zoom)-4 {
					t.Errorf("%q %v: label is cut: ink %v for a text %d wide", zoom, size, box, textWidth(zoomFont, zoom))
				}
			}
		}
	})
}

func TestSlidePanelFitsEveryTextScale(t *testing.T) {
	forEachScale(t, func(t *testing.T, k float64, lang string) {
		for _, size := range []image.Point{{mockupW, mockupH}, {1920, 1080}} {
			for _, total := range []int{24, 1200} {
				r := newScaleRig(t, k, lang, size.X, size.Y, func(h *fakeHost) {
					h.fullscreen = true
					h.slide = SlideshowView{Active: true, Playing: true, Index: total / 2, Total: total, Progress: .5, Interval: 5 * time.Second}
				})
				l := r.root.Layout()
				sl := l.Slide
				img := r.frame()
				name := fmt.Sprintf("%v total %d", size, total)
				if !sl.Panel.In(image.Rect(0, 0, size.X, size.Y)) {
					t.Fatalf("%s: panel %v leaves the window", name, sl.Panel)
				}
				if sl.Panel.Min.X < SlideBarMargin || sl.Panel.Max.X > size.X-SlideBarMargin {
					t.Errorf("%s: panel %v is closer than %d px to the window sides", name, sl.Panel, SlideBarMargin)
				}
				for _, z := range l.Zones {
					if z.Kind >= ZoneSlidePlay && z.Kind <= ZoneSlideExit && !z.Rect.In(sl.Panel) {
						t.Errorf("%s: zone %d %v is outside the panel %v", name, z.Kind, z.Rect, sl.Panel)
					}
				}
				if !sl.HasTrack() {
					t.Errorf("%s: no room for the progress bar in %v", name, sl.Panel)
				}
				right := sl.Settings.Min.X
				if sl.HasTrack() {
					right = int(math.Floor(sl.TrackX0)) - 2
				}
				area := image.Rect(sl.CounterX-3, sl.Panel.Min.Y+2, right, sl.Panel.Max.Y-2)
				box := inkWhere(img, area, lightAbove(170))
				text := r.host.slide.CounterText()
				if box.Empty() {
					t.Fatalf("%s: counter %q is not drawn", name, text)
				}
				if box.Min.X < sl.CounterX-1 || box.Max.X > right {
					t.Errorf("%s: counter ink %v leaves [%d, %d]", name, box, sl.CounterX-1, right)
				}
				if math.Abs(centerY(box)-centerY(sl.Panel)) > 2 {
					t.Errorf("%s: counter ink %v is off the panel centre %.1f", name, box, centerY(sl.Panel))
				}
				if want := textWidth(slideCounterFont, text); box.Dx() > want || box.Dx() < want-5 {
					t.Errorf("%s: counter ink is %d wide, the text measures %d", name, box.Dx(), want)
				}
				if sl.TrackX1 > float64(sl.Settings.Min.X) || sl.TrackX0 < float64(sl.CounterX) {
					t.Errorf("%s: track %.1f..%.1f runs into the counter or the buttons", name, sl.TrackX0, sl.TrackX1)
				}
				if !sl.Damage.In(sl.Panel) {
					t.Errorf("%s: progress strip %v sticks out of the panel", name, sl.Damage)
				}
			}
		}
	})
}

func TestToastStaysInsideTheWindow(t *testing.T) {
	texts := []string{"Скопировано", "Copied", "ABC 123", strings.Repeat("Очень длинный текст уведомления ", 12)}
	forEachScale(t, func(t *testing.T, k float64, lang string) {
		for _, c := range []struct {
			size       image.Point
			fullscreen bool
		}{{image.Pt(770, 504), false}, {image.Pt(420, 320), false}, {image.Pt(mockupW, mockupH), true}} {
			for _, text := range texts {
				r := newScaleRig(t, k, lang, c.size.X, c.size.Y, func(h *fakeHost) {
					h.fullscreen = c.fullscreen
					h.slide = slideshowMockupView()
				})
				r.root.Toast(text)
				l := r.root.fullOverlayLayout()
				rect := r.root.toastRect(l)
				window := image.Rect(0, 0, c.size.X, c.size.Y)
				if !rect.In(window) || rect.Dy() != toastH() {
					t.Fatalf("%v: toast %v is not %d px high inside %v", c.size, rect, toastH(), window)
				}
				if rect.Max.Y > l.Toolbar.Min.Y {
					t.Errorf("%v: toast %v covers the toolbar %v", c.size, rect, l.Toolbar)
				}
				img := r.frame()
				band := image.Rect(0, rect.Min.Y-6, c.size.X, rect.Max.Y+6)
				box := inkWhere(img, band, lightAbove(200))
				if !insideRect(box, plateText(rect, toastPadX)) {
					t.Errorf("%v: toast text ink %v is not inside the plate %v", c.size, box, rect)
				}
				if math.Abs(centerY(box)-centerY(rect)) > 1.5 && text == "ABC 123" {
					t.Errorf("%v: toast text ink %v is off the plate centre %.1f", c.size, box, centerY(rect))
				}
			}
		}
	})
}

func plateText(plate image.Rectangle, padX int) image.Rectangle {
	return image.Rect(plate.Min.X+padX-3, plate.Min.Y+2, plate.Max.X-padX+3, plate.Max.Y-2)
}

func TestTooltipsStayInsideTheWindowAndOffTheirButton(t *testing.T) {
	type probe struct {
		name       string
		size       image.Point
		fullscreen bool
		zone       ZoneKind
	}
	probes := []probe{
		{"close", image.Pt(770, 504), false, ZoneClose},
		{"maximize", image.Pt(770, 504), false, ZoneMax},
		{"more", image.Pt(770, 504), false, ZoneMore},
		{"back", image.Pt(770, 504), false, ZoneBack},
		{"tool fullscreen", image.Pt(770, 504), false, ZoneToolFullscreen},
		{"tool gallery", image.Pt(770, 504), false, ZoneToolGallery},
		{"tool rotate", image.Pt(420, 320), false, ZoneToolRotateLeft},
		{"fs close", image.Pt(mockupW, mockupH), true, ZoneExit},
		{"fs exit", image.Pt(mockupW, mockupH), true, ZoneSlideExit},
		{"fs settings", image.Pt(mockupW, mockupH), true, ZoneSlideSettings},
		{"fs play", image.Pt(mockupW, mockupH), true, ZoneSlidePlay},
		{"fs next", image.Pt(mockupW, mockupH), true, ZoneNext},
	}
	forEachScale(t, func(t *testing.T, k float64, lang string) {
		for _, p := range probes {
			r := newScaleRig(t, k, lang, p.size.X, p.size.Y, func(h *fakeHost) {
				h.fullscreen = p.fullscreen
				h.slide = slideshowMockupView()
				addSlideCommands(h)
			})
			l := r.root.fullOverlayLayout()
			anchor := l.Rect(p.zone)
			if anchor.Empty() {
				t.Fatalf("%s: no zone %d", p.name, p.zone)
			}
			x, y := l.Center(p.zone)
			r.root.OnMouseMove(x, y)
			r.ts.fireAll()
			if r.root.tip.text == "" {
				t.Fatalf("%s: no tooltip", p.name)
			}
			rect := r.root.tipRect(nil)
			if !rect.In(image.Rect(0, 0, p.size.X, p.size.Y)) || rect.Dy() != tipH() {
				t.Errorf("%s: tooltip %v is not %d px high inside the window", p.name, rect, tipH())
			}
			if rect.Overlaps(anchor) {
				t.Errorf("%s: tooltip %v covers its button %v", p.name, rect, anchor)
			}
			img := r.frame()
			box := inkWhere(img, rect.Inset(1), lightAbove(200))
			if !insideRect(box, plateText(rect, tipPadX)) {
				t.Errorf("%s: tooltip text ink %v is not inside the plate %v (%q)", p.name, box, rect, r.root.tip.text)
			}
			if width := textWidth(tipFont, r.root.tip.text); width+2*tipPadX <= p.size.X-8 && box.Dx() < width-6 {
				t.Errorf("%s: tooltip text is cut: ink %v for a text %d wide (%q)", p.name, box, textWidth(tipFont, r.root.tip.text), r.root.tip.text)
			}
		}
	})
}

func addSlideCommands(h *fakeHost) {
	h.reg.Add(commands.Command{ID: "slideshow.start", Title: "cmd.slideshow", Keys: []commands.Chord{commands.C(0, commands.KeyF5)}, Run: func() {}})
	h.reg.Add(commands.Command{ID: "slideshow.toggle", Title: "cmd.slideshowPause", Keys: []commands.Chord{commands.C(0, commands.KeySpace)}, Run: func() {}})
	h.reg.Add(commands.Command{ID: "view.fullscreen", Title: "cmd.fullscreen", Keys: []commands.Chord{commands.C(0, commands.KeyF11)}, Run: func() {}})
	for id, title := range map[commands.ID]string{"app.more": "cmd.more", "app.back": "cmd.back", "view.gallery": "cmd.gallery", "view.rotateLeft": "cmd.rotateLeft", "nav.next": "cmd.next"} {
		h.reg.Add(commands.Command{ID: id, Title: title, Run: func() {}})
	}
}

func TestStateMessagesStayCentredInsideTheWindow(t *testing.T) {
	forEachScale(t, func(t *testing.T, k float64, lang string) {
		for _, c := range []struct {
			name    string
			state   ContentState
			failure string
		}{
			{"empty", StateEmpty, ""},
			{"loading", StateLoading, ""},
			{"failed", StateFailed, "Формат не поддерживается этой версией программы, обновите её или откройте файл другой"},
		} {
			for _, size := range []image.Point{{770, 504}, {420, 320}} {
				r := newScaleRig(t, k, lang, size.X, size.Y, func(h *fakeHost) {
					h.state = c.state
					h.failure = c.failure
					h.content.has = false
					h.canPrev, h.canNext = false, false
				})
				l := r.root.Layout()
				img := r.frame()
				box := inkWhere(img, image.Rect(l.View.Min.X+2, l.View.Min.Y+2, l.View.Max.X-2, l.Toolbar.Min.Y-6), lightAbove(150))
				if box.Empty() {
					t.Fatalf("%s %v: nothing drawn", c.name, size)
				}
				if !box.In(l.View.Inset(4)) {
					t.Errorf("%s %v: message ink %v leaves the view %v", c.name, size, box, l.View)
				}
				if math.Abs(float64(box.Min.X+box.Max.X)/2-float64(l.View.Min.X+l.View.Max.X)/2) > 4 {
					t.Errorf("%s %v: message ink %v is not centred in %v", c.name, size, box, l.View)
				}
				if math.Abs(centerY(box)-centerY(l.View)) > 8 {
					t.Errorf("%s %v: message ink %v is off the vertical centre of %v", c.name, size, box, l.View)
				}
			}
		}
	})
}

func TestZonesDoNotOverlapAtEveryTextScale(t *testing.T) {
	forEachScale(t, func(t *testing.T, k float64, lang string) {
		for _, c := range []struct {
			size       image.Point
			fullscreen bool
			zoom       string
			total      int
		}{
			{image.Pt(770, 504), false, "6400%", 24},
			{image.Pt(1920, 1080), false, "100%", 24},
			{image.Pt(mockupW, mockupH), true, "100%", 24},
			{image.Pt(mockupW, mockupH), true, "100%", 1200},
			{image.Pt(1920, 1080), true, "100%", 9999},
		} {
			r := newScaleRig(t, k, lang, c.size.X, c.size.Y, func(h *fakeHost) {
				h.fullscreen = c.fullscreen
				h.zoom = c.zoom
				h.slide = SlideshowView{Active: true, Playing: true, Index: 1, Total: c.total}
			})
			l := r.root.Layout()
			for i, a := range l.Zones {
				for _, b := range l.Zones[i+1:] {
					if interactiveOverlap(a, b) {
						t.Errorf("%v fullscreen=%v: zones %d %v and %d %v overlap", c.size, c.fullscreen, a.Kind, a.Rect, b.Kind, b.Rect)
					}
				}
				if a.Kind != ZoneDrag && !isResize(a.Kind) && !a.Rect.In(image.Rect(0, 0, c.size.X, c.size.Y)) {
					t.Errorf("%v fullscreen=%v: zone %d %v leaves the window", c.size, c.fullscreen, a.Kind, a.Rect)
				}
			}
		}
	})
}

func TestTextScaleSnapshots(t *testing.T) {
	dir := testutil.OutDir()
	if dir == "" {
		t.Skip("set PIXGO_SNAPSHOTS to write the snapshots")
	}
	save := func(name string, img *image.RGBA) {
		t.Helper()
		if err := testutil.SavePNG(filepath.Join(dir, name), img); err != nil {
			t.Fatal(err)
		}
	}
	r := newScaleRig(t, 1.2, "ru", 770, 504, func(h *fakeHost) { h.title = "mountain.jpg — 6000 × 4000"; h.zoom = "6400%"; addSlideCommands(h) })
	l := r.root.Layout()
	x, y := l.Center(ZoneToolRotateLeft)
	r.root.OnMouseMove(x, y)
	r.ts.fireAll()
	r.root.Toast("Путь скопирован")
	save("window_k12_viewer.png", r.frame())

	fs := newScaleRig(t, 1.2, "ru", mockupW, mockupH, func(h *fakeHost) { h.fullscreen = true; h.slide = slideshowMockupView() })
	save("window_k12_slideshow.png", fs.frame())

	big := newScaleRig(t, 1.2, "ru", 1920, 1080, func(h *fakeHost) {
		h.fullscreen = true
		h.slide = SlideshowView{Active: true, Playing: true, Index: 120, Total: 1200, Progress: .3, Interval: 5 * time.Second}
	})
	save("window_k12_slideshow_hd.png", big.frame())

	for name, st := range map[string]ContentState{"empty": StateEmpty, "loading": StateLoading, "failed": StateFailed} {
		st := st
		s := newScaleRig(t, 1.2, "ru", 420, 320, func(h *fakeHost) {
			h.state = st
			h.failure = "Формат не поддерживается"
			h.content.has = false
		})
		save("window_k12_"+name+".png", s.frame())
	}
}
