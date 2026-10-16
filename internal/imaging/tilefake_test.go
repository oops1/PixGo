package imaging

import (
	"context"
	"errors"
	"image"
	"sync"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/codec"
)

type picture struct {
	w, h        int
	minShift    int
	translucent bool
}

func (p picture) pixel(shift, x, y int) [4]uint8 {
	f := 1 << uint(shift)
	cx := (2*x + 1) * f / 2
	cy := (2*y + 1) * f / 2
	r := uint8(min(255, cx*255/max(1, p.w)))
	g := uint8(min(255, cy*255/max(1, p.h)))
	b := uint8(((cx/64)+(cy/64))%2*120 + 60)
	a := uint8(255)
	if p.translucent {
		a = uint8(128 + (cx/32)%128)
	}
	return [4]uint8{r, g, b, a}
}

func (p picture) level(shift int, r image.Rectangle) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			px := p.pixel(shift, r.Min.X+x, r.Min.Y+y)
			o := img.PixOffset(x, y)
			copy(img.Pix[o:o+4], px[:])
		}
	}
	return img
}

func (p picture) info() codec.Info {
	return codec.Info{Format: "fake", FormatName: "Fake", Width: p.w, Height: p.h, Frames: 1, Orientation: codec.OrientNormal, HasAlpha: p.translucent}
}

type regionCall struct {
	shift int
	r     image.Rectangle
}

type fakeReader struct {
	pic picture

	mu            sync.Mutex
	regions       []regionCall
	overviews     []int
	hook          func(ctx context.Context, shift int, r image.Rectangle) error
	overviewErr   error
	concurrent    int32
	maxConcurrent int32
	closed        atomic.Int32
	opened        atomic.Int32
}

func newFakeReader(p picture) *fakeReader { return &fakeReader{pic: p} }

func (f *fakeReader) Info() codec.Info { return f.pic.info() }

func (f *fakeReader) MinShift() int { return f.pic.minShift }

func (f *fakeReader) Overview(ctx context.Context, shift int) (*image.NRGBA, error) {
	f.mu.Lock()
	f.overviews = append(f.overviews, shift)
	f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.overviewErr != nil {
		return nil, f.overviewErr
	}
	lw, lh := codec.LevelSize(f.pic.w, f.pic.h, shift)
	if int64(lw)*int64(lh) > 64<<20 {
		return nil, codec.ErrUnsupported
	}
	return f.pic.level(shift, image.Rect(0, 0, lw, lh)), nil
}

func (f *fakeReader) Region(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA, error) {
	n := atomic.AddInt32(&f.concurrent, 1)
	for {
		old := atomic.LoadInt32(&f.maxConcurrent)
		if n <= old || atomic.CompareAndSwapInt32(&f.maxConcurrent, old, n) {
			break
		}
	}
	defer atomic.AddInt32(&f.concurrent, -1)
	f.mu.Lock()
	f.regions = append(f.regions, regionCall{shift, r})
	hook := f.hook
	f.mu.Unlock()
	if hook != nil {
		if err := hook(ctx, shift, r); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lw, lh := codec.LevelSize(f.pic.w, f.pic.h, shift)
	r = r.Intersect(image.Rect(0, 0, lw, lh))
	if r.Empty() {
		return nil, errors.New("fake: empty region")
	}
	return f.pic.level(shift, r), nil
}

func (f *fakeReader) Close() error {
	f.closed.Add(1)
	return nil
}

func (f *fakeReader) regionCalls() []regionCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]regionCall(nil), f.regions...)
}

func (f *fakeReader) setHook(h func(ctx context.Context, shift int, r image.Rectangle) error) {
	f.mu.Lock()
	f.hook = h
	f.mu.Unlock()
}

type countingCloser struct{ n atomic.Int32 }

func (c *countingCloser) Close() error {
	c.n.Add(1)
	return nil
}
