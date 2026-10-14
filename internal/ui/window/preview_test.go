package window

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/theme"
)

func previewRig(t *testing.T, k float64, lang string, w, h int, mutate func(*fakeHost)) *rig {
	t.Helper()
	return newScaleRig(t, k, lang, w, h, func(host *fakeHost) {
		host.preview = true
		if mutate != nil {
			mutate(host)
		}
	})
}

func differs(a, b *image.RGBA, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return true
			}
		}
	}
	return false
}

func TestPreviewBadgeShowsOnlyWhenTheHostSaysPreview(t *testing.T) {
	tests := []struct {
		name    string
		preview bool
		state   ContentState
		page    Page
		want    bool
	}{
		{"preview of a ready image", true, StateReady, PageViewer, true},
		{"preview while the next image loads", true, StateLoading, PageViewer, true},
		{"not a preview", false, StateReady, PageViewer, false},
		{"gallery page", true, StateReady, PageGallery, false},
		{"settings page", true, StateReady, PageSettings, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newScaleRig(t, 1, "ru", 770, 504, func(h *fakeHost) { h.preview, h.state, h.page = tc.preview, tc.state, tc.page })

			got := !r.root.PreviewBadge().Empty()

			if got != tc.want {
				t.Fatalf("badge shown = %v, want %v (rect %v)", got, tc.want, r.root.PreviewBadge())
			}
		})
	}
}

func TestBadgeIsAbsentForHostsThatKnowNothingOfPreviews(t *testing.T) {
	r := newScaleRig(t, 1, "ru", 770, 504, nil)
	r.root.host = plainHost{r.host}

	if !r.root.PreviewBadge().Empty() || r.root.previewShown() {
		t.Fatal("a host without the Preview method must never get a badge")
	}
	r.frame()
}

type plainHost struct{ *fakeHost }

func (plainHost) Preview() {}

func TestBadgeSitsInTheTopLeftCornerOfTheImageArea(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeHost)
	}{
		{"window", nil},
		{"info panel open", func(h *fakeHost) { h.info = true }},
		{"fullscreen", func(h *fakeHost) { h.fullscreen = true }},
		{"maximised layout area", func(h *fakeHost) { h.autoHide = true }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := previewRig(t, 1, "ru", 900, 600, tc.mutate)
			view := r.root.Layout().View

			b := r.root.PreviewBadge()

			if b.Min.X != view.Min.X+12 || b.Min.Y != view.Min.Y+12 {
				t.Fatalf("badge %v, the image area starts at %v: want a 12 px inset", b, view.Min)
			}
		})
	}
}

func TestBadgeGeometryFollowsTheTextScale(t *testing.T) {
	forEachScale(t, func(t *testing.T, k float64, lang string) {
		r := previewRig(t, k, lang, 900, 600, nil)
		view := r.root.Layout().View

		b := r.root.PreviewBadge()

		if want := int(math.Round(24 * k)); b.Dy() != want {
			t.Errorf("height %d, want %d", b.Dy(), want)
		}
		text := i18n.T("viewer.preview")
		if min := textWidth(previewBadgeFont, text); b.Dx() < min+10 {
			t.Errorf("badge %d wide for a %d wide text", b.Dx(), min)
		}
		if b.Min != view.Min.Add(image.Pt(12, 12)) {
			t.Errorf("badge origin %v for the area %v", b.Min, view.Min)
		}
		if !insideRect(b, view) {
			t.Errorf("badge %v leaves the image area %v", b, view)
		}
	})
}

func TestBadgeGrowsWithTheTextScale(t *testing.T) {
	var prev image.Point
	for _, k := range textScales {
		r := previewRig(t, k, "ru", 900, 600, nil)
		size := r.root.PreviewBadge().Size()
		if size.X <= prev.X || size.Y <= prev.Y {
			t.Errorf("k=%.2f: badge %v is not bigger than %v", k, size, prev)
		}
		prev = size
	}
}

func TestBadgeTextIsDrawnInsideTheBadgeAtEveryScale(t *testing.T) {
	forEachScale(t, func(t *testing.T, k float64, lang string) {
		for _, th := range []*theme.Theme{theme.Dark(), theme.Light()} {
			r := previewRig(t, k, lang, 900, 600, nil)
			r.root.Theme = th
			b := r.root.PreviewBadge()
			img := r.frame()

			ink := inkWhere(img, b.Inset(2), func(c color.RGBA) bool { return channelGap(c, th.OverlayText) < 60 })

			if ink.Empty() {
				t.Fatalf("%s: no text pixels inside %v", th.Name, b)
			}
			if !insideRect(ink, b.Inset(1)) {
				t.Errorf("%s: text ink %v sticks out of the badge %v", th.Name, ink, b)
			}
			if math.Abs(centerY(ink)-centerY(b)) > 2.5 {
				t.Errorf("%s: text centre %.1f is off the badge centre %.1f", th.Name, centerY(ink), centerY(b))
			}
			pad := ink.Min.X - b.Min.X
			if pad < 5 || b.Max.X-ink.Max.X < 5 {
				t.Errorf("%s: padding %d / %d is too tight", th.Name, pad, b.Max.X-ink.Max.X)
			}
		}
	})
}

func TestBadgeIsPaintedWithTheOverlayTokens(t *testing.T) {
	for _, th := range []*theme.Theme{theme.Dark(), theme.Light(), theme.HighContrast()} {
		t.Run(th.Name, func(t *testing.T) {
			r := previewRig(t, 1.2, "ru", 900, 600, nil)
			r.root.Theme = th
			b := r.root.PreviewBadge()
			bare := previewRig(t, 1.2, "ru", 900, 600, func(h *fakeHost) { h.preview = false })
			bare.root.Theme = th

			with, without := r.frame(), bare.frame()

			if !differs(with, without, b) {
				t.Fatal("the badge left no pixels")
			}
			edge := false
			for dy := -1; dy <= 1; dy++ {
				x, y := b.Min.X+b.Dx()/2, b.Min.Y+dy
				edge = edge || with.RGBAAt(x, y) != without.RGBAAt(x, y)
			}
			if !edge {
				t.Errorf("the top edge of %v keeps the canvas colour: no border or fill", b)
			}
			outside := image.Rect(b.Min.X-4, b.Min.Y-4, b.Max.X+4, b.Max.Y+4)
			for y := outside.Min.Y; y < outside.Max.Y; y++ {
				for x := outside.Min.X; x < outside.Max.X; x++ {
					if !image.Pt(x, y).In(b.Inset(-1)) && with.RGBAAt(x, y) != without.RGBAAt(x, y) {
						t.Fatalf("pixel (%d,%d) next to the badge changed", x, y)
					}
				}
			}
		})
	}
}

func TestBadgeCornersAreRounded(t *testing.T) {
	r := previewRig(t, 1, "ru", 900, 600, nil)
	bare := previewRig(t, 1, "ru", 900, 600, func(h *fakeHost) { h.preview = false })
	b := r.root.PreviewBadge()

	with, without := r.frame(), bare.frame()

	if with.RGBAAt(b.Min.X, b.Min.Y) != without.RGBAAt(b.Min.X, b.Min.Y) {
		t.Error("the very corner pixel is painted: the corner is square")
	}
	if with.RGBAAt(b.Min.X+previewBadgeRadius+2, b.Min.Y+previewBadgeRadius+2) == without.RGBAAt(b.Min.X+previewBadgeRadius+2, b.Min.Y+previewBadgeRadius+2) {
		t.Error("the inside of the badge is not painted")
	}
}

func TestBadgeStaysOnTopOfTheImageButUnderTheChrome(t *testing.T) {
	photo := &fakeContent{has: true}
	r := previewRig(t, 1, "ru", 900, 600, func(h *fakeHost) {
		h.contentOverride = &paintedContent{fakeContent: photo, fill: color.RGBA{200, 40, 40, 255}}
	})
	b := r.root.PreviewBadge()
	l := r.root.Layout()

	img := r.frame()

	if got := img.RGBAAt(b.Min.X+b.Dx()/2, b.Min.Y+2); got == (color.RGBA{200, 40, 40, 255}) {
		t.Errorf("the image covers the badge: %v", got)
	}
	for name, zone := range map[string]image.Rectangle{"prev": l.Prev, "next": l.Next, "toolbar": l.Toolbar} {
		if zone.Overlaps(b) {
			t.Errorf("the badge %v runs into the %s %v", b, name, zone)
		}
	}
}

type paintedContent struct {
	*fakeContent
	fill color.RGBA
}

func (p *paintedContent) Draw(c widget.DrawContext) {
	b := p.Bounds()
	c.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), p.fill)
}

func TestBadgeNeverClashesWithTheChromeAtSmallWindows(t *testing.T) {
	sizes := []image.Point{{420, 320}, {500, 360}, {770, 504}, {1280, 800}}
	forEachScale(t, func(t *testing.T, k float64, lang string) {
		for _, size := range sizes {
			for _, fullscreen := range []bool{false, true} {
				r := previewRig(t, k, lang, size.X, size.Y, func(h *fakeHost) { h.fullscreen = fullscreen })
				l := r.root.fullOverlayLayout()
				b := r.root.PreviewBadge()

				if b.Empty() {
					t.Fatalf("%v: no badge", size)
				}
				if !insideRect(b, l.View) {
					t.Errorf("%v fullscreen=%v: badge %v leaves the area %v", size, fullscreen, b, l.View)
				}
				for _, z := range l.Zones {
					if overlayZone(z.Kind) && z.Kind != ZoneSlideBar && z.Rect.Overlaps(b) {
						t.Errorf("%v fullscreen=%v: badge %v overlaps zone %d %v", size, fullscreen, b, z.Kind, z.Rect)
					}
				}
			}
		}
	})
}

func TestBadgeIsClippedNotOverflowedInATinyArea(t *testing.T) {
	for _, w := range []int{150, 100, 60, 30} {
		view := image.Rect(1, 49, 1+w, 49+80)

		b := previewBadgeRect(view, "Предпросмотр")

		if b.Dx() > max(w-24, 0) || !b.In(view) && !b.Empty() {
			t.Errorf("view %v gives %v", view, b)
		}
		if b.Dx() < 0 || b.Dy() < 0 {
			t.Errorf("negative badge %v", b)
		}
	}
}

func TestBadgeVisibleAfterTheOverlayAutoHides(t *testing.T) {
	r := previewRig(t, 1, "ru", 900, 600, func(h *fakeHost) { h.fullscreen = true; h.autoHide = true })
	r.ts.fireAll()
	r.root.Refresh()
	b := r.root.PreviewBadge()
	bare := previewRig(t, 1, "ru", 900, 600, func(h *fakeHost) { h.fullscreen, h.autoHide, h.preview = true, true, false })
	bare.ts.fireAll()

	if !differs(r.frame(), bare.frame(), b) {
		t.Fatal("the badge must stay visible in fullscreen even when the chrome is hidden")
	}
}

func TestToggleOfThePreviewFlagRedrawsTheBadge(t *testing.T) {
	r := previewRig(t, 1, "ru", 900, 600, nil)
	b := r.root.PreviewBadge()
	on := r.frame()

	r.host.preview = false
	off := r.frame()
	r.host.preview = true
	back := r.frame()

	if !differs(on, off, b) || differs(on, back, b) {
		t.Fatal("the badge must follow the flag in both directions")
	}
}

func TestPreviewSnapshots(t *testing.T) {
	out := testutil.OutDir()
	if out == "" {
		t.Skip("PIXGO_SNAPSHOTS is not set")
	}
	for _, k := range []float64{1, 1.2, 1.6} {
		for _, lang := range []string{"ru", "en"} {
			for _, th := range []*theme.Theme{theme.Dark(), theme.Light()} {
				r := previewRig(t, k, lang, 770, 504, nil)
				r.root.Theme = th
				name := fmt.Sprintf("preview_%s_%s_k%02d.png", th.Name, lang, int(k*10+.5))
				testutil.SavePNG(filepath.Join(out, name), r.frame())
			}
		}
	}
}
