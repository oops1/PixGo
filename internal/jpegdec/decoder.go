package jpegdec

import (
	"context"
	"image"
	"io"
	"sync"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/par"
)

const (
	MaxShift             = 3
	DefaultCheckpoints   = 16
	DefaultMaxPixels     = 1_000_000_000
	maxWorkers           = 8
	parallelPixels       = 1 << 18
	minBitsPerBlock      = 2
	progressiveBitsFloor = 1
)

type Header struct {
	Width           int
	Height          int
	Components      int
	Progressive     bool
	RestartInterval int
	RGB             bool
	Adobe           bool
	AdobeTransform  uint8
}

type Options struct {
	Workers         int
	CheckpointEvery int
	MaxPixels       int64
}

func headerOf(f *frame) Header {
	h := Header{
		Width:           f.width,
		Height:          f.height,
		Components:      f.nComp,
		Progressive:     f.progressive,
		RestartInterval: f.ri,
		Adobe:           f.adobeValid,
		AdobeTransform:  f.adobeTransform,
	}
	if f.nComp == 3 {
		h.RGB = f.isRGB()
	}
	return h
}

func ReadHeader(ctx context.Context, r io.ReaderAt, size int64) (Header, error) {
	if err := ctx.Err(); err != nil {
		return Header{}, err
	}
	s := newStream(ctx, r, size)
	defer s.release()
	f, err := readConfig(s)
	if err != nil {
		return Header{}, err
	}
	switch f.nComp {
	case 1, 3, 4:
	default:
		return Header{}, FormatError("missing SOF marker")
	}
	return headerOf(f), nil
}

type Decoder struct {
	reader     io.ReaderAt
	size       int64
	opt        Options
	hdr        Header
	f          *frame
	mode       colorMode
	first      scanHeader
	scanOffset int64
	streaming  bool

	indexMu sync.Mutex
	index   atomic.Pointer[checkpointIndex]

	storeMu sync.RWMutex
	store   *coefStore

	closed atomic.Bool
}

func Open(ctx context.Context, r io.ReaderAt, size int64, o Options) (*Decoder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := newStream(ctx, r, size)
	defer s.release()
	p := newParser(s, false)
	if err := p.readSOI(); err != nil {
		return nil, err
	}
	var first scanHeader
	for {
		marker, n, err := p.next()
		if err != nil {
			return nil, err
		}
		if marker == eoiMarker {
			return nil, FormatError("missing SOS marker")
		}
		if marker == sosMarker {
			first, err = p.processSOS(n)
			if err != nil {
				return nil, err
			}
			break
		}
		if err = p.segment(marker, n); err != nil {
			return nil, err
		}
	}
	f := &p.f
	if f.width <= 0 || f.height <= 0 {
		return nil, FormatError("zero image dimension")
	}
	limit := o.MaxPixels
	if limit <= 0 {
		limit = DefaultMaxPixels
	}
	if int64(f.width)*int64(f.height) > limit {
		return nil, ErrTooLarge
	}
	mode, err := f.colorMode()
	if err != nil {
		return nil, err
	}
	d := &Decoder{
		reader:     r,
		size:       size,
		opt:        o,
		hdr:        headerOf(f),
		f:          f,
		mode:       mode,
		first:      first,
		scanOffset: s.offset(),
		streaming:  !f.progressive && first.n == f.nComp,
	}
	if err := d.checkDensity(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Decoder) checkDensity() error {
	mxx, myy := d.f.mcuCounts()
	blocks := int64(mxx) * int64(myy)
	perMCU := 0
	for c := 0; c < d.f.nComp; c++ {
		perMCU += d.f.comp[c].h * d.f.comp[c].v
	}
	blocks *= int64(perMCU)
	floor := int64(minBitsPerBlock)
	if d.f.progressive || !d.streaming {
		floor = progressiveBitsFloor
	}
	if d.size*8 < blocks*floor {
		return FormatError("file is too short for the image size")
	}
	return nil
}

func (d *Decoder) Header() Header { return d.hdr }

func (d *Decoder) Close() {
	d.closed.Store(true)
	d.storeMu.Lock()
	d.store = nil
	d.storeMu.Unlock()
	d.index.Store(nil)
}

func (d *Decoder) workerCount() int {
	n := d.opt.Workers
	if n <= 0 {
		n = min(par.Count(), maxWorkers)
	}
	return max(n, 1)
}

func (d *Decoder) checkShift(shift int) error {
	if d.closed.Load() {
		return ErrClosed
	}
	if shift < 0 || shift > MaxShift {
		return UnsupportedError("scale")
	}
	return nil
}

func (d *Decoder) CanRegion() bool {
	return d.streaming && d.size < maxIndexOffset
}

func (d *Decoder) MinShift() int {
	if d.CanRegion() {
		return 0
	}
	d.storeMu.RLock()
	defer d.storeMu.RUnlock()
	if d.store != nil {
		return d.store.shift
	}
	return MaxShift
}

func (d *Decoder) Ink() bool {
	_, ok := inkModeOf(d.mode)
	return ok
}

func asCMYK(img *image.NRGBA) *image.CMYK {
	return &image.CMYK{Pix: img.Pix, Stride: img.Stride, Rect: img.Rect}
}

func (d *Decoder) Decode(ctx context.Context, shift int) (*image.NRGBA, error) {
	return d.decode(ctx, shift, d.mode)
}

func (d *Decoder) DecodeInk(ctx context.Context, shift int) (*image.CMYK, error) {
	mode, ok := inkModeOf(d.mode)
	if !ok {
		return nil, UnsupportedError("ink samples of a file that is not CMYK")
	}
	img, err := d.decode(ctx, shift, mode)
	if err != nil {
		return nil, err
	}
	return asCMYK(img), nil
}

func (d *Decoder) decode(ctx context.Context, shift int, mode colorMode) (*image.NRGBA, error) {
	if err := d.checkShift(shift); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.streaming {
		return d.decodeStreaming(ctx, shift, mode)
	}
	return d.decodeCoefficients(ctx, shift, mode)
}

func (d *Decoder) Region(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA, error) {
	return d.region(ctx, shift, r, d.mode)
}

func (d *Decoder) RegionInk(ctx context.Context, shift int, r image.Rectangle) (*image.CMYK, error) {
	mode, ok := inkModeOf(d.mode)
	if !ok {
		return nil, UnsupportedError("ink samples of a file that is not CMYK")
	}
	img, err := d.region(ctx, shift, r, mode)
	if err != nil {
		return nil, err
	}
	return asCMYK(img), nil
}

func (d *Decoder) region(ctx context.Context, shift int, r image.Rectangle, mode colorMode) (*image.NRGBA, error) {
	if err := d.checkShift(shift); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g := d.f.geometry(shift, shift)
	r = r.Intersect(image.Rect(0, 0, g.outW, g.outH))
	if r.Empty() {
		return image.NewNRGBA(image.Rectangle{}), nil
	}
	if d.streaming {
		if !d.CanRegion() {
			return nil, UnsupportedError("region access to a file of this size")
		}
		return d.regionStreaming(ctx, g, r, mode)
	}
	return d.regionCoefficients(ctx, shift, r, mode)
}
