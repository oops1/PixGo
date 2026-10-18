package tiff

import (
	"context"
	"errors"
	"image"
	"io"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/codec/internal/reduce"
	"github.com/oops1/PixGo/internal/codec/internal/shield"
	"github.com/oops1/PixGo/internal/meta"
	"github.com/oops1/PixGo/internal/par"
)

const (
	tagICCProfile  = 0x8773
	deepPixelBytes = 8
)

func (lay *layout) deepSource() bool {
	switch lay.mode {
	case modeGray16, modeRGB16, modeRGBA16:
		return true
	}
	return false
}

func (lay *layout) bandRowsDeep() int {
	rows := maxBandBytes / (lay.chunkW * deepPixelBytes)
	if lay.tiled {
		rows &^= 7
		rows = max(rows, 8)
	}
	rows = max(rows, 1)
	return min(rows, lay.chunkH)
}

func (f *file) colorInfo() codec.ColorInfo {
	e, ok := f.entries[tagICCProfile]
	if !ok || e.count == 0 || e.count > meta.MaxICCBytes {
		return codec.ColorInfo{}
	}
	switch e.typ {
	case typeByte, typeUndef, typeSByte, typeASCII:
	default:
		return codec.ColorInfo{}
	}
	data, err := f.valueBytes(e, e.count)
	if err != nil {
		return codec.ColorInfo{}
	}
	return meta.NewColor(data, nil)
}

func (lay *layout) convertRowDeep(dst, src []byte, pixels int) {
	switch lay.mode {
	case modeGray16:
		for i := 0; i < pixels; i++ {
			hi, lo := src[2*i], src[2*i+1]
			if lay.littleEndian {
				hi, lo = lo, hi
			}
			if lay.invert {
				hi, lo = ^hi, ^lo
			}
			o := 8 * i
			dst[o], dst[o+1], dst[o+2], dst[o+3], dst[o+4], dst[o+5], dst[o+6], dst[o+7] = hi, lo, hi, lo, hi, lo, 0xff, 0xff
		}
	case modeRGB16:
		for i := 0; i < pixels; i++ {
			o := 8 * i
			s := src[6*i : 6*i+6]
			if lay.littleEndian {
				dst[o], dst[o+1], dst[o+2], dst[o+3], dst[o+4], dst[o+5] = s[1], s[0], s[3], s[2], s[5], s[4]
			} else {
				copy(dst[o:o+6], s)
			}
			dst[o+6], dst[o+7] = 0xff, 0xff
		}
	case modeRGBA16:
		for i := 0; i < pixels; i++ {
			o := 8 * i
			s := src[8*i : 8*i+8]
			if lay.littleEndian {
				dst[o], dst[o+1], dst[o+2], dst[o+3], dst[o+4], dst[o+5], dst[o+6], dst[o+7] = s[1], s[0], s[3], s[2], s[5], s[4], s[7], s[6]
			} else {
				copy(dst[o:o+8], s)
			}
			if lay.associated {
				unpremultiplyWord(dst[o : o+8])
			}
		}
	}
}

func unpremultiplyWord(px []byte) {
	a := uint32(px[6])<<8 | uint32(px[7])
	switch a {
	case 0xffff:
	case 0:
		clear(px[:6])
	default:
		for c := 0; c < 6; c += 2 {
			v := min((uint32(px[c])<<8|uint32(px[c+1]))*0xffff/a, 0xffff)
			px[c], px[c+1] = byte(v>>8), byte(v)
		}
	}
}

func (cs *chunkStream) readBandDeep(ctx context.Context, rows int, bands *bytePool) (*image.NRGBA64, error) {
	lay := cs.lay
	if rows <= 0 || cs.consumed+rows > cs.validRows {
		return nil, guard.Corrupt("tiff: band of %d rows beyond the chunk", rows)
	}
	img := &image.NRGBA64{Pix: bands.get(cs.validW * rows * deepPixelBytes), Stride: cs.validW * deepPixelBytes, Rect: image.Rect(0, 0, cs.validW, rows)}
	lastRowBytes := rowBytesFor(lay, cs.validW)
	for r := 0; r < rows; r++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		want := lay.rowBytes
		if cs.consumed+1 == cs.validRows {
			want = lastRowBytes
		}
		if _, err := io.ReadFull(cs.source, cs.row[:want]); err != nil {
			return nil, classifyStream(ctx, err)
		}
		cs.consumed++
		if lay.predictor {
			lay.undoPredictor(cs.row, cs.validW)
		}
		lay.convertRowDeep(img.Pix[r*img.Stride:r*img.Stride+cs.validW*deepPixelBytes], cs.row, cs.validW)
	}
	return img, nil
}

func (rd *reader) Color() codec.ColorInfo { return rd.color }

func (rd *reader) Deep() bool { return rd.lay.deepSource() }

func (rd *reader) OverviewDeep(ctx context.Context, shift int) (*image.NRGBA64, error) {
	lw, lh := codec.LevelSize(rd.lay.width, rd.lay.height, shift)
	return rd.shieldedDeep(ctx, shift, image.Rect(0, 0, lw, lh), false)
}

func (rd *reader) RegionDeep(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA64, error) {
	return rd.shieldedDeep(ctx, shift, r, true)
}

func (rd *reader) shieldedDeep(ctx context.Context, shift int, rect image.Rectangle, cached bool) (*image.NRGBA64, error) {
	if !rd.lay.deepSource() {
		return nil, guard.Unsupported("tiff: the picture has no more than 8 bits per sample")
	}
	workers := min(par.Count(), overviewWorkers)
	if cached {
		workers = min(par.Count(), regionWorkers)
	}
	var out *image.NRGBA64
	err := shield.Run("tiff", func() (err error) {
		out, err = rd.levelDeep(ctx, shift, rect, cached, workers)
		return err
	})
	return out, err
}

func (rd *reader) levelDeep(ctx context.Context, shift int, want image.Rectangle, cached bool, workers int) (*image.NRGBA64, error) {
	if rd.closed.Load() {
		return nil, errClosed
	}
	if shift < 0 || shift > maxShift {
		return nil, guard.Unsupported("tiff: level %d is not available", shift)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lay := rd.lay
	levelW, levelH := codec.LevelSize(lay.width, lay.height, shift)
	rect := want.Canon().Intersect(image.Rect(0, 0, levelW, levelH))
	if rect.Empty() {
		return image.NewNRGBA64(image.Rectangle{}), nil
	}
	if int64(rect.Dx())*int64(rect.Dy())*deepPixelBytes > maxResultBytes {
		return nil, guard.TooLarge("tiff: a %dx%d result is too large", rect.Dx(), rect.Dy())
	}
	if shift > nativeShift {
		parentW, parentH := codec.LevelSize(lay.width, lay.height, shift-1)
		parent := image.Rect(rect.Min.X*2, rect.Min.Y*2, rect.Max.X*2, rect.Max.Y*2).Intersect(image.Rect(0, 0, parentW, parentH))
		child, err := rd.levelDeep(ctx, shift-1, parent, cached, workers)
		if err != nil {
			return nil, err
		}
		return reduce.HalveDeep(child), nil
	}
	out := image.NewNRGBA64(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	source := image.Rect(rect.Min.X<<uint(shift), rect.Min.Y<<uint(shift), min(rect.Max.X<<uint(shift), lay.width), min(rect.Max.Y<<uint(shift), lay.height))
	chunks := lay.chunksFor(source)
	bandBytes := int64(lay.chunkW) * int64(lay.bandRowsDeep()) * deepPixelBytes
	workers = int(max(1, min(int64(workers), pipelineBudget/(bandBytes*lookahead*(pipelineDepth+1)))))
	produce := rd.producerDeep(source, cached)
	var err error
	if lay.tiled {
		blit := &deepBlitter{source: source, shift: shift, out: out}
		err = walk(ctx, chunks, workers, produce, blit.add)
	} else {
		accumulate := newDeepAccumulator(source, shift, out)
		if err = walk(ctx, chunks, workers, produce, accumulate.add); err == nil {
			err = accumulate.finish()
		}
	}
	if err != nil {
		if errors.Is(err, errClosed) {
			return nil, err
		}
		return nil, guard.Classify(ctx, err, nil)
	}
	return out, nil
}

func (rd *reader) producerDeep(source image.Rectangle, cached bool) producer {
	return func(ctx context.Context, chunk int, emit func(piece) error) error {
		x0, y0, _, validH := rd.lay.chunkRect(chunk)
		top := max(source.Min.Y, y0) - y0
		bottom := min(source.Max.Y, y0+validH) - y0
		if top >= bottom {
			return nil
		}
		rows := rd.lay.bandRowsDeep()
		first, last := top/rows, (bottom-1)/rows
		if cached {
			return rd.emitCachedDeep(ctx, chunk, first, last, rows, x0, y0, emit)
		}
		return rd.emitStreamedDeep(ctx, chunk, first, last, rows, x0, y0, validH, emit)
	}
}

func (rd *reader) emitCachedDeep(ctx context.Context, chunk, first, last, rows, x0, y0 int, emit func(piece) error) error {
	for band := first; band <= last; band++ {
		if rd.closed.Load() {
			return errClosed
		}
		got, err := rd.cache.get(ctx, bandKey{chunk: chunk, band: band, deep: true}, func(ctx context.Context) (bandValue, error) {
			img, err := rd.decodeBandDeep(ctx, chunk, band, rows)
			return bandValue{deep: img}, err
		})
		if err != nil {
			return err
		}
		if err := emit(piece{x: x0, y: y0 + band*rows, deep: got.deep}); err != nil {
			return err
		}
	}
	return nil
}

func (rd *reader) emitStreamedDeep(ctx context.Context, chunk, first, last, rows, x0, y0, validH int, emit func(piece) error) error {
	stream, err := rd.openChunk(ctx, chunk, first*rows)
	if err != nil {
		return err
	}
	defer stream.close()
	for band := first; band <= last; band++ {
		if rd.closed.Load() {
			return errClosed
		}
		img, err := stream.readBandDeep(ctx, min(rows, validH-band*rows), &rd.pools.bands)
		if err != nil {
			return err
		}
		if err := emit(piece{x: x0, y: y0 + band*rows, deep: img, done: func() { rd.pools.bands.put(img.Pix) }}); err != nil {
			return err
		}
	}
	return nil
}

func (rd *reader) decodeBandDeep(ctx context.Context, chunk, band, rows int) (*image.NRGBA64, error) {
	_, _, _, validH := rd.lay.chunkRect(chunk)
	stream, err := rd.openChunk(ctx, chunk, band*rows)
	if err != nil {
		return nil, err
	}
	defer stream.close()
	return stream.readBandDeep(ctx, min(rows, validH-band*rows), nil)
}

type deepBlitter struct {
	source  image.Rectangle
	shift   int
	out     *image.NRGBA64
	scratch reduce.ScratchDeep
}

func (b *deepBlitter) add(p piece) error {
	at := image.Rect(p.x, p.y, p.x+p.deep.Rect.Dx(), p.y+p.deep.Rect.Dy()).Intersect(b.source)
	if at.Empty() {
		return nil
	}
	view := p.deep.SubImage(at.Sub(image.Pt(p.x, p.y))).(*image.NRGBA64)
	reduced := b.scratch.Levels(view, b.shift)
	destX := (at.Min.X - b.source.Min.X) >> uint(b.shift)
	destY := (at.Min.Y - b.source.Min.Y) >> uint(b.shift)
	width, height := reduced.Rect.Dx(), reduced.Rect.Dy()
	if destX+width > b.out.Rect.Dx() || destY+height > b.out.Rect.Dy() {
		return guard.Corrupt("tiff: reduced piece overflows the output")
	}
	for row := 0; row < height; row++ {
		copy(b.out.Pix[(destY+row)*b.out.Stride+destX*deepPixelBytes:], reduced.Pix[row*reduced.Stride:row*reduced.Stride+width*deepPixelBytes])
	}
	return nil
}

type deepAccumulator struct {
	source  image.Rectangle
	shift   int
	out     *image.NRGBA64
	scratch reduce.ScratchDeep
	pending *image.NRGBA64
	filled  int
	nextRow int
	outRow  int
}

func newDeepAccumulator(source image.Rectangle, shift int, out *image.NRGBA64) *deepAccumulator {
	return &deepAccumulator{
		source:  source,
		shift:   shift,
		out:     out,
		pending: image.NewNRGBA64(image.Rect(0, 0, source.Dx(), 1<<uint(shift))),
		nextRow: source.Min.Y,
	}
}

func (a *deepAccumulator) add(p piece) error {
	top := max(p.y, a.source.Min.Y)
	bottom := min(p.y+p.deep.Rect.Dy(), a.source.Max.Y)
	if top >= bottom {
		return nil
	}
	if top != a.nextRow || p.x > a.source.Min.X || p.x+p.deep.Rect.Dx() < a.source.Max.X {
		return guard.Corrupt("tiff: rows arrive out of order")
	}
	from := (a.source.Min.X - p.x) * deepPixelBytes
	width := a.source.Dx() * deepPixelBytes
	for y := top; y < bottom; y++ {
		src := p.deep.Pix[(y-p.y)*p.deep.Stride+from:]
		copy(a.pending.Pix[a.filled*a.pending.Stride:], src[:width])
		a.filled++
		a.nextRow++
		if a.filled == 1<<uint(a.shift) {
			a.flush()
		}
	}
	return nil
}

func (a *deepAccumulator) flush() {
	if a.filled == 0 {
		return
	}
	view := a.pending.SubImage(image.Rect(0, 0, a.pending.Rect.Dx(), a.filled)).(*image.NRGBA64)
	reduced := a.scratch.Levels(view, a.shift)
	copy(a.out.Pix[a.outRow*a.out.Stride:a.outRow*a.out.Stride+a.out.Rect.Dx()*deepPixelBytes], reduced.Pix[:reduced.Rect.Dx()*deepPixelBytes])
	a.outRow++
	a.filled = 0
}

func (a *deepAccumulator) finish() error {
	a.flush()
	if a.outRow != a.out.Rect.Dy() {
		return guard.Corrupt("tiff: %d of %d output rows produced", a.outRow, a.out.Rect.Dy())
	}
	return nil
}
