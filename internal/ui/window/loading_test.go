package window

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

type reducedHost struct {
	*fakeHost
	reduced bool
}

func (h *reducedHost) Reduced() bool { return h.reduced }

func newLoadingRig(t *testing.T, lang string, reduced bool) (*rig, *reducedHost) {
	t.Helper()
	var rh *reducedHost
	r := newAccessRig(t, lang, func(h *fakeHost) { h.state = StateLoading })
	rh = &reducedHost{fakeHost: r.host, reduced: reduced}
	r.root.host = rh
	r.root.Refresh()
	return r, rh
}

func TestLoadingIndicatorShowsWhileAPictureIsAlreadyOnScreen(t *testing.T) {
	r, _ := newLoadingRig(t, "en", true)
	if !r.root.loadingShown() {
		t.Fatal("no indicator although the preview is shown and the load goes on")
	}
	rect := loadingRect(r.root.Layout().View)
	if rect.Dx() != loadingSize || rect.Dy() != loadingSize || !rect.In(r.root.Layout().View) {
		t.Errorf("indicator at %v", rect)
	}
	r.eng.Flush()
	with := r.eng.RenderOnce()
	r.host.state = StateReady
	r.root.Refresh()
	r.eng.Flush()
	without := r.eng.RenderOnce()
	changed := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if with.RGBAAt(x, y) != without.RGBAAt(x, y) {
				changed++
			}
		}
	}
	if changed < 100 {
		t.Errorf("only %d pixels of the indicator differ from the ready frame", changed)
	}
	outside := image.Rect(rect.Min.X-6, rect.Min.Y-6, rect.Max.X+6, rect.Max.Y+6)
	for y := 60; y < 400; y++ {
		for x := 100; x < 600; x++ {
			if image.Pt(x, y).In(outside) {
				continue
			}
			if with.RGBAAt(x, y) != without.RGBAAt(x, y) {
				t.Fatalf("the indicator disturbs the picture at (%d,%d)", x, y)
			}
		}
	}
}

func TestLoadingIndicatorIsAbsentWithoutAPictureOrOutsideTheViewer(t *testing.T) {
	r, _ := newLoadingRig(t, "en", true)
	r.host.content.has = false
	if r.root.loadingShown() {
		t.Error("indicator over the full loading text")
	}
	r.host.content.has = true
	r.host.state = StateReady
	if r.root.loadingShown() {
		t.Error("indicator on a finished picture")
	}
	r.host.state = StateLoading
	r.host.page = PageGallery
	if r.root.loadingShown() {
		t.Error("indicator on the gallery")
	}
}

func TestLoadingIndicatorAnimatesAndStopsWhenTheLoadEnds(t *testing.T) {
	r, _ := newLoadingRig(t, "en", true)
	if r.ts.live() != 1 {
		t.Fatalf("%d timers armed by the refresh", r.ts.live())
	}
	r.root.Refresh()
	if r.ts.live() != 1 {
		t.Errorf("a second refresh armed a second timer: %d", r.ts.live())
	}
	frames := map[int]bool{}
	for i := 0; i < loadingDots; i++ {
		r.root.Now = func() time.Time { return time.UnixMilli(int64(i) * loadingTick.Milliseconds()) }
		frames[r.root.loadingPhase()] = true
	}
	if len(frames) != loadingDots {
		t.Errorf("only %d spinner phases", len(frames))
	}
	r.ts.fireAll()
	if r.ts.live() != 1 {
		t.Errorf("the tick did not re-arm itself: %d timers", r.ts.live())
	}
	r.host.state = StateReady
	r.root.Refresh()
	r.ts.fireAll()
	if r.ts.live() != 0 {
		t.Errorf("the animation goes on after the load ended: %d timers", r.ts.live())
	}
}

func TestSpinnerFramesDiffer(t *testing.T) {
	r, _ := newLoadingRig(t, "en", true)
	rect := loadingRect(r.root.Layout().View)
	var first *image.RGBA
	for _, ms := range []int64{0, loadingTick.Milliseconds() * 3} {
		r.root.Now = func() time.Time { return time.UnixMilli(ms) }
		r.root.Refresh()
		r.eng.Flush()
		img := r.eng.RenderOnce()
		if first == nil {
			first = img
			continue
		}
		differs := false
		for y := rect.Min.Y; y < rect.Max.Y && !differs; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				if first.RGBAAt(x, y) != img.RGBAAt(x, y) {
					differs = true
					break
				}
			}
		}
		if !differs {
			t.Error("the spinner does not move")
		}
	}
}

func TestLoadingIndicatorHasALocalisedName(t *testing.T) {
	for _, lang := range i18n.Supported() {
		r, rh := newLoadingRig(t, lang, true)
		tree := accessTree(r)
		testutil.RequireNamed(t, tree)
		n := testutil.FindAccess(tree, widget.RoleProgressBar, i18n.T("access.loadingFull"))
		if n == nil || n.Bounds != loadingRect(r.root.Layout().View) {
			t.Fatalf("%s: indicator node %+v\n%s", lang, n, testutil.DescribeAccess(tree))
		}
		rh.reduced = false
		if testutil.FindAccess(accessTree(r), widget.RoleProgressBar, i18n.T("viewer.loading")) == nil {
			t.Errorf("%s: an ordinary load is not announced as loading", lang)
		}
	}
	if i18n.T("access.loadingFull") == "access.loadingFull" {
		t.Error("the key has no text")
	}
}
