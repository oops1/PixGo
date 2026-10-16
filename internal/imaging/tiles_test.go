package imaging

import (
	"context"
	"errors"
	"image"
	"image/color"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/render"
)

type tileRig struct {
	t       *testing.T
	pic     picture
	rd      *fakeReader
	m       *TileManager
	closer  *countingCloser
	base    int
	changes atomic.Int32
}

func newTileRig(t *testing.T, pic picture, base int, opt TileOptions) *tileRig {
	t.Helper()
	rd := newFakeReader(pic)
	ov, err := rd.Overview(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	r := &tileRig{t: t, pic: pic, rd: rd, closer: &countingCloser{}, base: base}
	r.m = NewTileManager(rd, BuildPyramid(ov, 0), base, pic.w, pic.h, opt, r.closer)
	r.m.SetOnChange(func() { r.changes.Add(1) })
	t.Cleanup(func() { r.m.Close() })
	return r
}

func (r *tileRig) idle() bool {
	st := r.m.Stats()
	return st.Pending == 0 && st.Inflight == 0
}

func (r *tileRig) settle(shift int, need image.Rectangle) render.Window {
	r.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		w := r.m.Window(shift, need)
		if r.idle() {
			return r.m.Window(shift, need)
		}
		if time.Now().After(deadline) {
			r.t.Fatalf("tiles did not settle: %+v", r.m.Stats())
		}
		time.Sleep(time.Millisecond)
		_ = w
	}
}

func (r *tileRig) mismatches(w render.Window, channels int) int {
	bad := 0
	b := w.Img.Rect
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			want := r.pic.pixel(w.Shift, w.X+x, w.Y+y)
			o := w.Img.PixOffset(x, y)
			for c := 0; c < channels; c++ {
				if w.Img.Pix[o+c] != want[c] {
					bad++
					break
				}
			}
		}
	}
	return bad
}

func (r *tileRig) closeTo(w render.Window, tol int) int {
	bad := 0
	b := w.Img.Rect
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			want := r.pic.pixel(w.Shift, w.X+x, w.Y+y)
			o := w.Img.PixOffset(x, y)
			for c := 0; c < 2; c++ {
				d := int(w.Img.Pix[o+c]) - int(want[c])
				if d < -tol || d > tol {
					bad++
					break
				}
			}
		}
	}
	return bad
}

var bigPic = picture{w: 20000, h: 15000}

func TestWindowCoversNeedPlusTileMargin(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{})
	need := image.Rect(5000, 3000, 6000, 3800)
	w := r.m.Window(1, need)
	if w.Img == nil || w.Shift != 1 {
		t.Fatalf("window %+v", w)
	}
	rect := w.Rect()
	if !need.In(rect) {
		t.Fatalf("window %v does not cover %v", rect, need)
	}
	if rect.Min.X%TileSide != 0 || rect.Min.Y%TileSide != 0 || rect.Max.X%TileSide != 0 || rect.Max.Y%TileSide != 0 {
		t.Fatalf("window %v is not aligned to tiles", rect)
	}
	margin := image.Rect(need.Min.X-TileSide, need.Min.Y-TileSide, need.Max.X+TileSide, need.Max.Y+TileSide)
	if !margin.In(rect) {
		t.Fatalf("window %v lacks the one tile margin around %v", rect, need)
	}
	if rect.Dx() > need.Dx()+2*TileSide+2*(TileSide-1) || rect.Dy() > need.Dy()+2*TileSide+2*(TileSide-1) {
		t.Fatalf("window %v is much larger than needed", rect)
	}
	if w.Img.Rect.Dx() != rect.Dx() || w.Img.Rect.Dy() != rect.Dy() || w.Img.Stride != rect.Dx()*4 {
		t.Fatalf("image geometry %v stride %d for %v", w.Img.Rect, w.Img.Stride, rect)
	}
}

func TestWindowStartsWithPlaceholderThenFillsWithTiles(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 1})
	gate := make(chan struct{})
	r.rd.setHook(func(ctx context.Context, shift int, rc image.Rectangle) error {
		select {
		case <-gate:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	need := image.Rect(7000, 6000, 7600, 6400)
	w := r.m.Window(0, need)
	if bad := r.mismatches(w, 2); bad == 0 {
		t.Fatal("window must start as an upsampled placeholder, not as exact pixels")
	}
	if bad := r.closeTo(w, 3); bad != 0 {
		t.Fatalf("%d placeholder pixels are far from the picture", bad)
	}
	close(gate)
	w = r.settle(0, need)
	if bad := r.mismatches(w, 4); bad != 0 {
		t.Fatalf("%d pixels still differ after every tile arrived", bad)
	}
	if r.changes.Load() == 0 {
		t.Fatal("onChange never fired")
	}
}

func TestWindowEdgeTilesAreClippedToTheLevel(t *testing.T) {
	pic := picture{w: 1000, h: 700}
	r := newTileRig(t, pic, 1, TileOptions{})
	for _, need := range []image.Rectangle{image.Rect(900, 600, 1000, 700), image.Rect(0, 0, 40, 30), image.Rect(0, 0, 1000, 700)} {
		w := r.settle(0, need)
		if w.Img == nil {
			t.Fatalf("no window for %v", need)
		}
		if !w.Rect().In(image.Rect(0, 0, 1000, 700)) || !need.In(w.Rect()) {
			t.Fatalf("window %v for need %v", w.Rect(), need)
		}
		if bad := r.mismatches(w, 4); bad != 0 {
			t.Fatalf("need %v: %d pixels differ", need, bad)
		}
	}
}

func TestWindowIsReusedWhilePanningInsideTheMargin(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{})
	need := image.Rect(8000, 7000, 8800, 7600)
	first := r.settle(0, need)
	builds := r.m.Stats().WindowBuilds
	for dx := 0; dx <= 200; dx += 20 {
		moved := need.Add(image.Pt(dx, dx/2))
		w := r.m.Window(0, moved)
		if w.Img != first.Img || w.X != first.X || w.Y != first.Y {
			t.Fatalf("window rebuilt while panning by %d inside the margin", dx)
		}
	}
	if got := r.m.Stats().WindowBuilds; got != builds {
		t.Fatalf("builds %d -> %d while panning inside the margin", builds, got)
	}
	far := need.Add(image.Pt(3*TileSide, 0))
	w := r.m.Window(0, far)
	if !far.In(w.Rect()) || r.m.Stats().WindowBuilds != builds+1 {
		t.Fatalf("leaving the window must rebuild it exactly once: %+v", r.m.Stats())
	}
}

func TestWindowReusesTheSameImageHeader(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{})
	a := r.m.Window(0, image.Rect(1000, 1000, 1500, 1500))
	b := r.m.Window(0, image.Rect(9000, 9000, 9500, 9500))
	c := r.m.Window(1, image.Rect(1000, 1000, 1500, 1500))
	if a.Img != b.Img || b.Img != c.Img {
		t.Fatal("the window image header must be stable so the opacity cache is not polluted")
	}
}

func TestWindowTileArrivalPatchesInPlace(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 1})
	need := image.Rect(3000, 3000, 3300, 3300)
	release := make(chan struct{})
	r.rd.setHook(func(ctx context.Context, shift int, rc image.Rectangle) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	w := r.m.Window(0, need)
	builds := r.m.Stats().WindowBuilds
	close(release)
	deadline := time.Now().Add(10 * time.Second)
	for !r.idle() {
		if time.Now().After(deadline) {
			t.Fatal("tiles did not arrive")
		}
		time.Sleep(time.Millisecond)
	}
	w2 := r.m.Window(0, need)
	if w2.Img != w.Img || r.m.Stats().WindowBuilds != builds {
		t.Fatal("tile arrival must patch the existing window, not rebuild it")
	}
	if r.m.Stats().WindowPatch == 0 {
		t.Fatal("no patch recorded")
	}
	if bad := r.mismatches(w2, 4); bad != 0 {
		t.Fatalf("%d pixels differ after patching", bad)
	}
}

func TestTilesAreRequestedNearestToCentreFirst(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 1})
	var mu sync.Mutex
	var order []image.Rectangle
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	r.rd.setHook(func(ctx context.Context, shift int, rc image.Rectangle) error {
		mu.Lock()
		order = append(order, rc)
		first := len(order) == 1
		mu.Unlock()
		if first {
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	need := image.Rect(4096, 4096, 4096+3*TileSide, 4096+2*TileSide)
	r.m.Window(0, need)
	<-started
	r.m.Window(0, need)
	close(release)
	r.settle(0, need)
	mu.Lock()
	defer mu.Unlock()
	if len(order) < 12 {
		t.Fatalf("only %d tiles requested", len(order))
	}
	cx, cy := (need.Min.X+need.Max.X)/2, (need.Min.Y+need.Max.Y)/2
	dist := func(rc image.Rectangle) int {
		dx, dy := (rc.Min.X+rc.Max.X)/2-cx, (rc.Min.Y+rc.Max.Y)/2-cy
		return dx*dx + dy*dy
	}
	inside := 0
	for _, rc := range order {
		if rc.Overlaps(need) {
			inside++
		}
	}
	prev := -1
	for i, rc := range order[1:] {
		if i < inside-1 {
			if !rc.Overlaps(need) {
				t.Fatalf("request %d (%v) is a margin tile but visible tiles are still missing", i+1, rc)
			}
			if d := dist(rc); d < prev {
				t.Fatalf("visible tiles out of order: %d after %d", d, prev)
			}
			prev = dist(rc)
		}
	}
}

func TestNewViewCancelsTilesNoLongerWanted(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 1})
	started := make(chan image.Rectangle, 8)
	cancelled := make(chan image.Rectangle, 8)
	r.rd.setHook(func(ctx context.Context, shift int, rc image.Rectangle) error {
		if rc.Min.X < 2000 {
			started <- rc
			<-ctx.Done()
			cancelled <- rc
			return ctx.Err()
		}
		return nil
	})
	r.m.Window(0, image.Rect(100, 100, 300, 300))
	var rc image.Rectangle
	select {
	case rc = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first tile not started")
	}
	r.m.Window(0, image.Rect(12000, 9000, 12300, 9300))
	select {
	case got := <-cancelled:
		if got != rc {
			t.Fatalf("cancelled %v, started %v", got, rc)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight tile of the old view was not cancelled")
	}
	w := r.settle(0, image.Rect(12000, 9000, 12300, 9300))
	if bad := r.mismatches(w, 4); bad != 0 {
		t.Fatalf("%d pixels differ", bad)
	}
	if st := r.m.Stats(); st.Failed != 0 {
		t.Fatalf("cancelled tiles must not be remembered as failed: %+v", st)
	}
}

func TestCancelledTileIsRequestedAgainWhenViewReturns(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 1})
	started := make(chan struct{}, 64)
	gate := make(chan struct{})
	r.rd.setHook(func(ctx context.Context, shift int, rc image.Rectangle) error {
		started <- struct{}{}
		select {
		case <-gate:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	home := image.Rect(100, 100, 300, 300)
	r.m.Window(0, home)
	<-started
	r.m.Window(0, image.Rect(12000, 9000, 12300, 9300))
	<-started
	r.m.Window(0, home)
	<-started
	close(gate)
	w := r.settle(0, home)
	if bad := r.mismatches(w, 4); bad != 0 {
		t.Fatalf("%d pixels differ after returning to the first view", bad)
	}
	if st := r.m.Stats(); st.Failed != 0 {
		t.Fatalf("stats %+v", st)
	}
}

func TestTileBudgetBoundsMemory(t *testing.T) {
	budget := int64(12 * TileSide * TileSide * 4)
	r := newTileRig(t, bigPic, 3, TileOptions{Budget: budget})
	for i := 0; i < 8; i++ {
		need := image.Rect(i*2500, i*1500, i*2500+900, i*1500+700)
		r.settle(0, need)
		if st := r.m.Stats(); st.TileBytes > budget {
			t.Fatalf("tile cache holds %d bytes, budget %d", st.TileBytes, budget)
		}
	}
	if st := r.m.Stats(); st.Decoded <= 12 || st.Tiles > 12 {
		t.Fatalf("stats %+v", st)
	}
}

func TestMaxBytesIncludesBudget(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Budget: 40 << 20})
	if got := r.m.MaxBytes(); got < 40<<20 || got > 40<<20+windowReserveBytes {
		t.Fatalf("MaxBytes %d", got)
	}
	d := &Decoded{Levels: []*image.NRGBA{image.NewNRGBA(image.Rect(0, 0, 4, 4))}, Detail: r.m}
	if d.Bytes() != 64+r.m.MaxBytes() {
		t.Fatalf("Decoded.Bytes %d", d.Bytes())
	}
}

func TestReaderPanicDoesNotKillTheManager(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 2})
	r.rd.setHook(func(ctx context.Context, shift int, rc image.Rectangle) error {
		if rc.Min.X == 0 && rc.Min.Y == 0 {
			panic("decoder exploded")
		}
		return nil
	})
	need := image.Rect(0, 0, 600, 600)
	w := r.settle(0, need)
	st := r.m.Stats()
	if st.Failed != 1 {
		t.Fatalf("exactly the panicking tile must be marked failed: %+v", st)
	}
	bad := r.mismatches(w, 4)
	if bad == 0 || bad > TileSide*TileSide {
		t.Fatalf("only the panicking tile may stay a placeholder, %d pixels differ", bad)
	}
	calls := 0
	for _, c := range r.rd.regionCalls() {
		if c.r.Min == image.Pt(0, 0) {
			calls++
		}
	}
	r.m.Window(0, need)
	r.m.Window(0, need.Add(image.Pt(1, 1)))
	time.Sleep(20 * time.Millisecond)
	after := 0
	for _, c := range r.rd.regionCalls() {
		if c.r.Min == image.Pt(0, 0) {
			after++
		}
	}
	if after != calls {
		t.Fatalf("failed tile was requested again: %d -> %d", calls, after)
	}
}

func TestReaderErrorsAreRememberedPerTile(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 2})
	boom := errors.New("boom")
	r.rd.setHook(func(ctx context.Context, shift int, rc image.Rectangle) error {
		if rc.Min.X >= 512 {
			return boom
		}
		return nil
	})
	need := image.Rect(0, 0, 900, 300)
	w := r.settle(0, need)
	if st := r.m.Stats(); st.Failed == 0 || st.Tiles == 0 {
		t.Fatalf("stats %+v", st)
	}
	if bad := r.closeTo(w, 3); bad != 0 {
		t.Fatalf("failed tiles must degrade to the placeholder: %d pixels far from the picture", bad)
	}
}

func TestRegionOfWrongSizeIsRejected(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 1})
	wrong := &wrongSizeReader{fakeReader: r.rd}
	m := NewTileManager(wrong, r.m.levels, 3, bigPic.w, bigPic.h, TileOptions{}, nil)
	defer m.Close()
	m.Window(0, image.Rect(0, 0, 100, 100))
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := m.Stats()
		if st.Pending == 0 && st.Inflight == 0 {
			if st.Tiles != 0 || st.Failed == 0 {
				t.Fatalf("stats %+v", st)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

type wrongSizeReader struct{ *fakeReader }

func (w *wrongSizeReader) Region(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA, error) {
	return image.NewNRGBA(image.Rect(0, 0, r.Dx()+1, r.Dy())), nil
}

func TestCloseStopsWorkersAndClosesOnce(t *testing.T) {
	base := runtime.NumGoroutine()
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 3})
	entered := make(chan struct{}, 8)
	r.rd.setHook(func(ctx context.Context, shift int, rc image.Rectangle) error {
		entered <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	})
	r.m.Window(0, image.Rect(0, 0, 2000, 2000))
	<-entered
	if err := r.m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.m.Close(); err != nil {
		t.Fatal(err)
	}
	if r.rd.closed.Load() != 1 || r.closer.n.Load() != 1 {
		t.Fatalf("reader closed %d times, closer %d times", r.rd.closed.Load(), r.closer.n.Load())
	}
	if w := r.m.Window(0, image.Rect(0, 0, 100, 100)); w.Img != nil {
		t.Fatal("a closed manager must not serve windows")
	}
	if st := r.m.Stats(); !st.Closed || st.Tiles != 0 || st.TileBytes != 0 {
		t.Fatalf("stats after close %+v", st)
	}
	if n := settledGoroutines(base); n > base {
		t.Fatalf("goroutines %d, before %d", n, base)
	}
}

func TestCloseWithoutAnyWork(t *testing.T) {
	base := runtime.NumGoroutine()
	r := newTileRig(t, bigPic, 3, TileOptions{})
	r.m.Close()
	if r.rd.closed.Load() != 1 {
		t.Fatal("reader not closed")
	}
	if n := settledGoroutines(base); n > base {
		t.Fatalf("goroutines %d, before %d", n, base)
	}
}

func TestWindowRejectsLevelsItCannotServe(t *testing.T) {
	pic := picture{w: 20000, h: 15000, minShift: 1}
	r := newTileRig(t, pic, 3, TileOptions{})
	if w := r.m.Window(0, image.Rect(0, 0, 100, 100)); w.Img != nil {
		t.Fatal("level finer than the reader can serve")
	}
	if w := r.m.Window(3, image.Rect(0, 0, 100, 100)); w.Img != nil {
		t.Fatal("the held level itself is not a window")
	}
	if w := r.m.Window(4, image.Rect(0, 0, 100, 100)); w.Img != nil {
		t.Fatal("coarser than the held level")
	}
	if w := r.m.Window(-1, image.Rect(0, 0, 100, 100)); w.Img != nil {
		t.Fatal("negative level")
	}
	if w := r.m.Window(1, image.Rect(-50, -50, -10, -10)); w.Img != nil {
		t.Fatal("empty need")
	}
	if w := r.m.Window(1, image.Rect(0, 0, 100, 100)); w.Img == nil {
		t.Fatal("level 1 is the finest the reader serves")
	}
}

func TestWindowGivesUpOnAbsurdRequests(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{})
	huge := image.Rect(0, 0, 20000, 15000)
	if w := r.m.Window(0, huge); w.Img != nil {
		t.Fatalf("a %v window would need %d MB", huge, huge.Dx()*huge.Dy()*4>>20)
	}
	ok := image.Rect(0, 0, 4000, 3000)
	w := r.m.Window(0, ok)
	if w.Img == nil || !ok.In(w.Rect()) {
		t.Fatal("a reasonable window must still be served")
	}
	if len(w.Img.Pix) > maxWindowPixels*4 {
		t.Fatalf("window of %d bytes exceeds the cap", len(w.Img.Pix))
	}
}

func TestWindowOpacityFollowsTheTiles(t *testing.T) {
	opaque := newTileRig(t, bigPic, 3, TileOptions{})
	w := opaque.settle(0, image.Rect(1000, 1000, 1500, 1500))
	if !render.IsOpaque(w.Img) {
		t.Fatal("window over an opaque picture must be opaque")
	}
	clear := picture{w: 20000, h: 15000, translucent: true}
	tr := newTileRig(t, clear, 3, TileOptions{})
	w = tr.settle(0, image.Rect(1000, 1000, 1500, 1500))
	if render.IsOpaque(w.Img) {
		t.Fatal("window over a translucent picture must not be flagged opaque")
	}
	if bad := tr.mismatches(w, 4); bad != 0 {
		t.Fatalf("%d translucent pixels differ", bad)
	}
}

func TestTranslucentPlaceholderKeepsAlpha(t *testing.T) {
	pic := picture{w: 20000, h: 15000, translucent: true}
	r := newTileRig(t, pic, 3, TileOptions{Workers: 1})
	r.rd.setHook(func(ctx context.Context, shift int, rc image.Rectangle) error {
		<-ctx.Done()
		return ctx.Err()
	})
	w := r.m.Window(0, image.Rect(4000, 4000, 4400, 4400))
	partial := 0
	for i := 3; i < len(w.Img.Pix); i += 4 {
		if w.Img.Pix[i] >= 128 {
			partial++
		}
	}
	if partial != len(w.Img.Pix)/4 {
		t.Fatal("placeholder alpha must follow the source alpha range")
	}
}

func TestOnChangeIsCoalescedUntilTheWindowIsRead(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 1})
	need := image.Rect(2000, 2000, 2600, 2600)
	r.m.Window(0, need)
	deadline := time.Now().Add(10 * time.Second)
	for !r.idle() {
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		time.Sleep(time.Millisecond)
	}
	st := r.m.Stats()
	if got := r.changes.Load(); got != 1 || st.Decoded < 4 {
		t.Fatalf("%d notifications for %d tiles without any read in between", got, st.Decoded)
	}
	r.m.Window(0, need)
	other := image.Rect(9000, 9000, 9600, 9600)
	r.m.Window(0, other)
	for !r.idle() {
		time.Sleep(time.Millisecond)
	}
	if got := r.changes.Load(); got != 2 {
		t.Fatalf("a read must re-arm the notification: %d", got)
	}
}

func TestSetOnChangeNilIsSafe(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{})
	r.m.SetOnChange(nil)
	r.settle(0, image.Rect(0, 0, 300, 300))
}

func TestConcurrentWindowAndCloseUnderRace(t *testing.T) {
	for round := 0; round < 5; round++ {
		r := newTileRig(t, bigPic, 3, TileOptions{Workers: 3})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				x := (i * 97) % 15000
				r.m.Window(i%3, image.Rect(x, x/2, x+1200, x/2+800))
				r.m.Stats()
			}
		}()
		go func() {
			defer wg.Done()
			time.Sleep(time.Duration(round) * time.Millisecond)
			r.m.Close()
		}()
		wg.Wait()
		if r.rd.closed.Load() != 1 {
			t.Fatalf("round %d: reader closed %d times", round, r.rd.closed.Load())
		}
	}
}

func TestMultipleWorkersRunConcurrently(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 3})
	r.rd.setHook(func(ctx context.Context, shift int, rc image.Rectangle) error {
		time.Sleep(2 * time.Millisecond)
		return nil
	})
	r.settle(0, image.Rect(0, 0, 1200, 1200))
	if got := atomic.LoadInt32(&r.rd.maxConcurrent); got < 2 || got > MaxTileWorkers {
		t.Fatalf("max concurrent regions %d", got)
	}
	r2 := newTileRig(t, bigPic, 3, TileOptions{Workers: 99})
	if r2.m.workers != MaxTileWorkers {
		t.Fatalf("workers %d", r2.m.workers)
	}
	r3 := newTileRig(t, bigPic, 3, TileOptions{Workers: -4})
	if r3.m.workers != 1 {
		t.Fatalf("workers %d", r3.m.workers)
	}
}

func TestRenderThroughManagerEqualsFullResolutionRender(t *testing.T) {
	pic := picture{w: 2300, h: 1700}
	full := pic.level(0, image.Rect(0, 0, pic.w, pic.h))
	fullLevels := BuildPyramid(full, 0)
	r := newTileRig(t, pic, 2, TileOptions{})
	for _, o := range []codec.Orientation{codec.OrientNormal, codec.OrientRotate90, codec.OrientTransverse, codec.OrientFlipH} {
		ow, oh := o.Size(pic.w, pic.h)
		for _, sc := range []float64{1, 0.8, 0.55, 2.2} {
			m := render.Mapping{Scale: sc, Orient: o, Filter: render.FilterBilinear, OriginX: float64(ow)/2 - 400/sc, OriginY: float64(oh)/2 - 250/sc}
			src := render.Source{Levels: r.m.levels, Width: pic.w, Height: pic.h, Shift: 2, Detail: r.m}
			got := image.NewRGBA(image.Rect(0, 0, 800, 500))
			want := image.NewRGBA(image.Rect(0, 0, 800, 500))
			bg := render.Background{Canvas: color.RGBA{9, 9, 9, 255}}
			deadline := time.Now().Add(20 * time.Second)
			for {
				render.Render(got, src, m, bg)
				if r.idle() {
					render.Render(got, src, m, bg)
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("timeout")
				}
				time.Sleep(time.Millisecond)
			}
			render.Render(want, render.Source{Levels: fullLevels, Width: pic.w, Height: pic.h}, m, bg)
			for i := range got.Pix {
				if got.Pix[i] != want.Pix[i] {
					t.Fatalf("orient=%d scale=%v: render through the tile manager differs at byte %d", o, sc, i)
				}
			}
		}
	}
}

func TestMemoryStaysBoundedWhilePanningAHugePicture(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates the overview of a 200 megapixel picture")
	}
	pic := picture{w: 16000, h: 12500}
	budget := int64(64 << 20)
	r := newTileRig(t, pic, 2, TileOptions{Budget: budget, Workers: 3})
	var before runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for step := 0; step < 90; step++ {
		x := 300 + (step*173)%13500
		y := 200 + (step*131)%10400
		r.settle(0, image.Rect(x, y, x+1920, y+1080))
		if step%10 == 0 {
			r.settle(1, image.Rect(x/2, y/2, x/2+1920, y/2+1080))
		}
		if st := r.m.Stats(); st.TileBytes > budget {
			t.Fatalf("step %d: tile cache %d over budget %d", step, st.TileBytes, budget)
		}
	}
	var after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&after)
	growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	window := int64(2000*1300*4) * 2
	limit := budget + window + 24<<20
	if growth > limit {
		t.Fatalf("heap grew by %d MB while panning, limit %d MB", growth>>20, limit>>20)
	}
	t.Logf("heap growth while panning: %d MB, tile cache %d MB, builds %d", growth>>20, r.m.Stats().TileBytes>>20, r.m.Stats().WindowBuilds)
}

func TestReusedWindowKeepsTheVisibleTilesRecent(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 3})
	home := image.Rect(2000, 2000, 2000+3*TileSide, 2000+2*TileSide)
	r.settle(0, home)
	inner := image.Rect(2300, 2100, 2500, 2300)
	r.m.mu.Lock()
	r.m.lru.MoveToFront(r.m.lru.Back())
	before := r.m.lru.Front().Value.(*tileEntry).key
	r.m.mu.Unlock()
	r.m.Window(0, inner)
	r.m.mu.Lock()
	front := r.m.lru.Front().Value.(*tileEntry).key
	r.m.mu.Unlock()
	tx0, ty0, tx1, ty1 := tileRange(inner)
	if front.shift != 0 || front.tx < tx0 || front.tx >= tx1 || front.ty < ty0 || front.ty >= ty1 {
		t.Fatalf("most recent tile is %+v (was %+v), want one covering %v", front, before, inner)
	}
	if r.m.Stats().WindowBuilds != 1 {
		t.Fatalf("the window should have been reused: %+v", r.m.Stats())
	}
}

func TestPanningPastTheWindowFindsPrefetchedTiles(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{Workers: 3})
	need := image.Rect(5000, 5000, 6000, 5700)
	first := r.settle(0, need)
	builds := r.m.Stats().WindowBuilds
	for _, d := range []image.Point{{first.Rect().Max.X - need.Max.X + 40, 0}, {-(need.Min.X - first.Rect().Min.X) - 40, 0}, {0, first.Rect().Max.Y - need.Max.Y + 40}, {0, -(need.Min.Y - first.Rect().Min.Y) - 40}} {
		moved := need.Add(d)
		if moved.In(first.Rect()) {
			t.Fatalf("shift %v does not leave the window %v", d, first.Rect())
		}
		w := r.m.Window(0, moved)
		builds++
		if got := r.m.Stats().WindowBuilds; got != builds {
			t.Fatalf("builds %d, want %d", got, builds)
		}
		bad := 0
		for y := moved.Min.Y; y < moved.Max.Y; y++ {
			for x := moved.Min.X; x < moved.Max.X; x++ {
				want := r.pic.pixel(0, x, y)
				px := w.Img.NRGBAAt(x-w.X, y-w.Y)
				if px.R != want[0] || px.G != want[1] || px.B != want[2] {
					bad++
				}
			}
		}
		if bad != 0 {
			t.Fatalf("shift %v: %d visible pixels are still placeholders right after the window moved", d, bad)
		}
		r.settle(0, need)
		builds = r.m.Stats().WindowBuilds
	}
}

func TestOversizedViewFallsBackToACoarserWindowInsteadOfTheBaseLevel(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{})
	src := render.Source{Levels: r.m.levels, Width: bigPic.w, Height: bigPic.h, Shift: 3, Detail: r.m}
	dst := image.NewRGBA(image.Rect(0, 0, 6400, 3000))
	m := render.Mapping{Scale: 1, Filter: render.FilterBilinear, OriginX: 5000, OriginY: 4000}
	deadline := time.Now().Add(10 * time.Second)
	for !r.idle() || r.m.win.shift != 1 {
		render.Render(dst, src, m, render.Background{})
		if time.Now().After(deadline) {
			t.Fatalf("no level-1 window: shift %d stats %+v", r.m.win.shift, r.m.Stats())
		}
		time.Sleep(time.Millisecond)
	}
	if got := r.m.win.shift; got != 1 {
		t.Fatalf("a 19 Mpx view must use the level-1 window, got shift %d", got)
	}
	if pixelsOf(r.m.win.rect) > maxWindowPixels {
		t.Fatalf("window of %d pixels exceeds the cap", pixelsOf(r.m.win.rect))
	}
}

func TestStatsCountPlaceholderFramesUntilTheTilesArrive(t *testing.T) {
	r := newTileRig(t, bigPic, 3, TileOptions{})
	need := image.Rect(1000, 1000, 1500, 1500)
	r.m.Window(0, need)
	st := r.m.Stats()
	if st.Requests != 1 || st.Placeholders != 1 {
		t.Fatalf("first frame: %d requests, %d placeholders", st.Requests, st.Placeholders)
	}
	r.settle(0, need)
	before := r.m.Stats()
	r.m.Window(0, need)
	after := r.m.Stats()
	if after.Requests != before.Requests+1 || after.Placeholders != before.Placeholders {
		t.Fatalf("a frame served entirely by tiles must not count as a placeholder: before %+v after %+v", before, after)
	}
	r.m.Window(0, image.Rect(0, 0, 0, 0))
	if r.m.Stats().Requests != after.Requests {
		t.Fatal("an empty request is not a frame")
	}
}
