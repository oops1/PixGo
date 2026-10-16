package view

import (
	"context"
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/imaging"
	"github.com/oops1/PixGo/internal/render"
	"github.com/oops1/PixGo/internal/viewer"
)

const (
	bigW       = 1600
	bigH       = 1200
	heldShift  = 2
	tileMargin = 16
)

type windowCall struct {
	shift int
	need  image.Rectangle
}

type recDetail struct {
	mu       sync.Mutex
	levels   []*image.NRGBA
	margin   int
	calls    []windowCall
	onChange func()
	refuse   bool
}

func newRecDetail(levels []*image.NRGBA, margin int) *recDetail {
	return &recDetail{levels: levels, margin: margin}
}

func (d *recDetail) SetOnChange(fn func()) {
	d.mu.Lock()
	d.onChange = fn
	d.mu.Unlock()
}

func (d *recDetail) callback() func() {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.onChange
}

func (d *recDetail) fire() {
	if fn := d.callback(); fn != nil {
		fn()
	}
}

func (d *recDetail) Window(shift int, need image.Rectangle) render.Window {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, windowCall{shift, need})
	if d.refuse || shift < 0 || shift >= len(d.levels) {
		return render.Window{}
	}
	lvl := d.levels[shift]
	r := image.Rect(need.Min.X-d.margin, need.Min.Y-d.margin, need.Max.X+d.margin, need.Max.Y+d.margin).Intersect(lvl.Rect)
	cut := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		so := lvl.PixOffset(r.Min.X, r.Min.Y+y)
		copy(cut.Pix[y*cut.Stride:y*cut.Stride+r.Dx()*4], lvl.Pix[so:so+r.Dx()*4])
	}
	return render.Window{Img: cut, Shift: shift, X: r.Min.X, Y: r.Min.Y}
}

func (d *recDetail) recorded() []windowCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]windowCall(nil), d.calls...)
}

func (d *recDetail) reset() {
	d.mu.Lock()
	d.calls = nil
	d.mu.Unlock()
}

func fullPyramid() []*image.NRGBA {
	return imaging.BuildPyramid(gradient(bigW, bigH), 64)
}

func tiledSource(levels []*image.NRGBA, det render.Detail) render.Source {
	return render.Source{Levels: levels[heldShift:], Width: bigW, Height: bigH, Shift: heldShift, Detail: det}
}

func framesEqual(a, b *image.RGBA) bool {
	if a.Rect != b.Rect {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}

func cloneFrame(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Rect)
	copy(dst.Pix, src.Pix)
	return dst
}

func TestTiledSourceNeedsNoWindowWhenFitted(t *testing.T) {
	r := newRig(t)
	det := newRecDetail(fullPyramid(), tileMargin)
	r.c.SetImage(tiledSource(det.levels, det), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	if calls := det.recorded(); len(calls) != 0 {
		t.Fatalf("fit zoom is served by the held levels, got window requests %v", calls)
	}
	if !r.c.HasImage() || r.c.Frame().ImgW != bigW || r.c.Frame().ImgH != bigH {
		t.Fatalf("frame %+v", r.c.Frame())
	}
}

func TestTiledSourceRequestsVisibleWindowAtActualSize(t *testing.T) {
	r := newRig(t)
	det := newRecDetail(fullPyramid(), tileMargin)
	r.c.SetImage(tiledSource(det.levels, det), codec.OrientNormal, viewer.ZoomActual)
	got := cloneFrame(r.frame())
	calls := det.recorded()
	if len(calls) != 1 || calls[0].shift != 0 {
		t.Fatalf("requests %v", calls)
	}
	vis := render.VisibleRect(r.c.view.Mapping(r.c.Frame()), bigW, bigH, areaW, areaH)
	need := calls[0].need
	if vis.Empty() || !vis.In(need) || need.Dx() > vis.Dx()+2 || need.Dy() > vis.Dy()+2 {
		t.Fatalf("request %v for visible %v", need, vis)
	}
	ref := newRig(t)
	ref.c.SetImage(render.Source{Levels: det.levels, Width: bigW, Height: bigH}, codec.OrientNormal, viewer.ZoomActual)
	if !framesEqual(got, ref.frame()) {
		t.Fatal("tiled render differs from the render of the full resolution level")
	}
}

func TestTiledSourceFollowsPanning(t *testing.T) {
	r := newRig(t)
	det := newRecDetail(fullPyramid(), tileMargin)
	r.c.SetImage(tiledSource(det.levels, det), codec.OrientNormal, viewer.ZoomActual)
	r.frame()
	first := det.recorded()[0].need
	det.reset()
	r.c.OnMouseButton(widget.MouseEvent{X: 200, Y: 150, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
	r.c.OnMouseMove(150, 150)
	r.c.OnMouseMove(100, 150)
	r.c.OnMouseButton(widget.MouseEvent{X: 100, Y: 150, Button: widget.MouseLeft, Pressed: false, Clicks: 1})
	got := cloneFrame(r.frame())
	calls := det.recorded()
	if len(calls) != 1 {
		t.Fatalf("requests %v", calls)
	}
	second := calls[0].need
	if d := second.Min.X - first.Min.X; d < 98 || d > 102 {
		t.Fatalf("window moved by %d px for a 100 px drag: %v -> %v", d, first, second)
	}
	if second.Min.Y != first.Min.Y || second.Dx() != first.Dx() {
		t.Fatalf("pan along x changed y or width: %v -> %v", first, second)
	}
	ref := newRig(t)
	ref.c.SetImage(render.Source{Levels: det.levels, Width: bigW, Height: bigH}, codec.OrientNormal, viewer.ZoomActual)
	ref.frame()
	ref.c.OnMouseButton(widget.MouseEvent{X: 200, Y: 150, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
	ref.c.OnMouseMove(150, 150)
	ref.c.OnMouseMove(100, 150)
	ref.c.OnMouseButton(widget.MouseEvent{X: 100, Y: 150, Button: widget.MouseLeft, Pressed: false, Clicks: 1})
	if !framesEqual(got, ref.frame()) {
		t.Fatal("panned tiled render differs from the full resolution render")
	}
}

func TestTiledSourceKeepsWorkingWhenRotatedAndFlipped(t *testing.T) {
	steps := []struct {
		name string
		do   func(c *Canvas)
	}{
		{"rotate right", func(c *Canvas) { c.RotateRight() }},
		{"rotate left", func(c *Canvas) { c.RotateLeft() }},
		{"flip horizontally", func(c *Canvas) { c.FlipH() }},
		{"flip vertically", func(c *Canvas) { c.FlipV() }},
		{"rotate twice", func(c *Canvas) { c.RotateRight(); c.RotateRight() }},
	}
	for _, exif := range []codec.Orientation{codec.OrientNormal, codec.OrientRotate90, codec.OrientTransverse} {
		for _, st := range steps {
			r := newRig(t)
			det := newRecDetail(fullPyramid(), tileMargin)
			r.c.SetImage(tiledSource(det.levels, det), exif, viewer.ZoomActual)
			r.frame()
			det.reset()
			st.do(r.c)
			got := cloneFrame(r.frame())
			calls := det.recorded()
			if len(calls) != 1 || calls[0].shift != 0 {
				t.Fatalf("exif %d %s: requests %v", exif, st.name, calls)
			}
			m := r.c.view.Mapping(r.c.Frame())
			if m.Orient != r.c.Orientation() {
				t.Fatalf("exif %d %s: mapping orientation %d, canvas %d", exif, st.name, m.Orient, r.c.Orientation())
			}
			vis := render.VisibleRect(m, bigW, bigH, areaW, areaH)
			if !vis.In(calls[0].need) {
				t.Fatalf("exif %d %s: request %v misses the visible area %v", exif, st.name, calls[0].need, vis)
			}
			ref := newRig(t)
			ref.c.SetImage(render.Source{Levels: det.levels, Width: bigW, Height: bigH}, exif, viewer.ZoomActual)
			ref.frame()
			st.do(ref.c)
			if !framesEqual(got, ref.frame()) {
				t.Fatalf("exif %d %s: tiled render differs from the full resolution render", exif, st.name)
			}
		}
	}
}

func TestTiledSourceZoomChangesRequestedLevel(t *testing.T) {
	r := newRig(t)
	det := newRecDetail(fullPyramid(), tileMargin)
	r.c.SetImage(tiledSource(det.levels, det), codec.OrientNormal, viewer.ZoomActual)
	r.frame()
	det.reset()
	r.c.View().ZoomTo(r.c.Frame(), 0.5, 200, 150)
	r.c.invalidate()
	r.frame()
	calls := det.recorded()
	if len(calls) != 1 || calls[0].shift != 1 {
		t.Fatalf("at 50%% the level above full resolution is wanted: %v", calls)
	}
	det.reset()
	r.c.View().ZoomTo(r.c.Frame(), 3, 200, 150)
	r.c.invalidate()
	r.frame()
	calls = det.recorded()
	if len(calls) != 1 || calls[0].shift != 0 {
		t.Fatalf("at 300%% full resolution is wanted: %v", calls)
	}
	det.reset()
	r.c.View().ZoomTo(r.c.Frame(), 0.2, 200, 150)
	r.c.invalidate()
	r.frame()
	if calls = det.recorded(); len(calls) != 0 {
		t.Fatalf("at 20%% the held levels are enough: %v", calls)
	}
}

func TestTiledSourceIdleFramesDoNotAskAgain(t *testing.T) {
	r := newRig(t)
	det := newRecDetail(fullPyramid(), tileMargin)
	r.c.SetImage(tiledSource(det.levels, det), codec.OrientNormal, viewer.ZoomActual)
	r.frame()
	r.frame()
	r.frame()
	if n := len(det.recorded()); n != 1 {
		t.Fatalf("a clean canvas must not re-render: %d window requests", n)
	}
}

func TestTiledSourceFallsBackWhenNoWindowIsServed(t *testing.T) {
	r := newRig(t)
	det := newRecDetail(fullPyramid(), tileMargin)
	det.refuse = true
	r.c.SetImage(tiledSource(det.levels, det), codec.OrientNormal, viewer.ZoomActual)
	got := cloneFrame(r.frame())
	if n := len(det.recorded()); n != heldShift {
		t.Fatalf("requests %d, want one per level from full resolution down to the held one (%d)", n, heldShift)
	}
	ref := newRig(t)
	ref.c.SetImage(render.Source{Levels: det.levels[heldShift:], Width: bigW, Height: bigH, Shift: heldShift}, codec.OrientNormal, viewer.ZoomActual)
	if !framesEqual(got, ref.frame()) {
		t.Fatal("without a window the canvas must show the held level upscaled")
	}
	centre := got.RGBAAt(areaW/2, areaH/2)
	if centre.A != 255 || centre == (color.RGBA{10, 20, 30, 255}) {
		t.Fatalf("placeholder missing: %v", centre)
	}
}

func TestDetailChangeRepaintsOnTheUIThread(t *testing.T) {
	r := newRig(t)
	det := newRecDetail(fullPyramid(), tileMargin)
	r.c.SetImage(tiledSource(det.levels, det), codec.OrientNormal, viewer.ZoomActual)
	before := cloneFrame(r.frame())
	det.reset()
	for _, l := range det.levels {
		for i := 0; i < len(l.Pix); i += 4 {
			l.Pix[i] = 255 - l.Pix[i]
		}
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		det.fire()
	}()
	wg.Wait()
	if n := len(det.recorded()); n != 0 {
		t.Fatal("the change callback must not render by itself, it only posts an invalidate")
	}
	after := cloneFrame(r.frame())
	if len(det.recorded()) != 1 {
		t.Fatalf("a change must lead to exactly one new window request, got %d", len(det.recorded()))
	}
	if framesEqual(before, after) {
		t.Fatal("new tile content did not reach the screen")
	}
}

func TestDetailChangeUsesInjectedPost(t *testing.T) {
	r := newRig(t)
	var queued []func()
	r.c.SetPost(func(fn func()) { queued = append(queued, fn) })
	det := newRecDetail(fullPyramid(), tileMargin)
	r.c.SetImage(tiledSource(det.levels, det), codec.OrientNormal, viewer.ZoomActual)
	r.frame()
	r.c.dirty = false
	det.fire()
	det.fire()
	if len(queued) != 2 || r.c.dirty {
		t.Fatalf("queued %d, dirty %v: the callback must only post", len(queued), r.c.dirty)
	}
	queued[0]()
	if !r.c.dirty {
		t.Fatal("posted function did not invalidate")
	}
}

func TestDetailCallbackLifecycle(t *testing.T) {
	r := newRig(t)
	a := newRecDetail(fullPyramid(), tileMargin)
	b := newRecDetail(fullPyramid(), tileMargin)
	r.c.SetImage(tiledSource(a.levels, a), codec.OrientNormal, viewer.ZoomActual)
	if a.callback() == nil {
		t.Fatal("canvas must subscribe to the detail it shows")
	}
	stale := a.callback()
	r.c.SetImageKeepView(tiledSource(a.levels, a), codec.OrientNormal)
	if a.callback() == nil {
		t.Fatal("showing the same detail again must keep the subscription")
	}
	r.c.ReplaceFrame(tiledSource(b.levels, b))
	if a.callback() != nil || b.callback() == nil {
		t.Fatalf("ReplaceFrame must move the subscription: a=%v b=%v", a.callback() != nil, b.callback() != nil)
	}
	r.frame()
	r.c.dirty = false
	stale()
	r.frame()
	r.c.ReplaceFrame(render.Source{Levels: b.levels[heldShift:], Width: bigW, Height: bigH, Shift: heldShift})
	if b.callback() != nil {
		t.Fatal("a source without detail must unsubscribe")
	}
	r.c.SetImage(tiledSource(a.levels, a), codec.OrientNormal, viewer.ZoomActual)
	r.c.Clear()
	if a.callback() != nil {
		t.Fatal("Clear must unsubscribe")
	}
	r.c.dirty = false
	stale()
	r.frame()
	if r.c.HasImage() {
		t.Fatal("a late callback of a cleared detail must not bring anything back")
	}
}

func TestDetailChangesFromManyGoroutinesUnderRace(t *testing.T) {
	r := newRig(t)
	det := newRecDetail(fullPyramid(), tileMargin)
	r.c.SetImage(tiledSource(det.levels, det), codec.OrientNormal, viewer.ZoomActual)
	r.frame()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					det.fire()
				}
			}
		}()
	}
	for i := 0; i < 40; i++ {
		r.c.ZoomIn()
		r.frame()
		r.c.ZoomOut()
		r.frame()
	}
	close(stop)
	wg.Wait()
	r.frame()
}

func TestTiledSourceWithRealTileManager(t *testing.T) {
	pic := gradient(bigW, bigH)
	levels := imaging.BuildPyramid(pic, 64)
	reader := &gradientReader{img: pic, levels: levels}
	held := levels[heldShift]
	mgr := imaging.NewTileManager(reader, imaging.BuildPyramid(held, 64), heldShift, bigW, bigH, imaging.TileOptions{Workers: 2}, nil)
	defer mgr.Close()
	r := newRig(t)
	r.c.SetImage(render.Source{Levels: imaging.BuildPyramid(held, 64), Width: bigW, Height: bigH, Shift: heldShift, Detail: mgr}, codec.OrientNormal, viewer.ZoomActual)
	ref := newRig(t)
	ref.c.SetImage(render.Source{Levels: levels, Width: bigW, Height: bigH}, codec.OrientNormal, viewer.ZoomActual)
	want := cloneFrame(ref.frame())
	deadline := 0
	for {
		got := r.frame()
		if framesEqual(got, want) {
			return
		}
		deadline++
		if deadline > 4000 {
			t.Fatal("the canvas never converged to the full resolution render")
		}
		waitABit()
	}
}

type gradientReader struct {
	img    *image.NRGBA
	levels []*image.NRGBA
}

func (g *gradientReader) Info() codec.Info {
	return codec.Info{Format: "fake", Width: bigW, Height: bigH, Frames: 1, Orientation: codec.OrientNormal}
}

func (g *gradientReader) MinShift() int { return 0 }

func (g *gradientReader) Overview(ctx context.Context, shift int) (*image.NRGBA, error) {
	return g.Region(ctx, shift, g.levels[shift].Rect)
}

func (g *gradientReader) Region(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA, error) {
	lvl := g.levels[shift]
	r = r.Intersect(lvl.Rect)
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		so := lvl.PixOffset(r.Min.X, r.Min.Y+y)
		copy(out.Pix[y*out.Stride:y*out.Stride+r.Dx()*4], lvl.Pix[so:so+r.Dx()*4])
	}
	return out, nil
}

func (g *gradientReader) Close() error { return nil }

func waitABit() { time.Sleep(time.Millisecond) }
