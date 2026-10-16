package tiff

import (
	"context"
	"errors"
	"image"
	"io"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/codec/internal/reduce"
	"github.com/oops1/PixGo/internal/codec/internal/shield"
	"github.com/oops1/PixGo/internal/par"
)

const (
	defaultCacheBytes = 48 << 20
	maxResultBytes    = 1 << 31
	maxShift          = 32
	overviewWorkers   = 8
	regionWorkers     = 4
	pipelineBudget    = 64 << 20
)

var errClosed = errors.New("tiff: tile reader is closed")

type reader struct {
	r      io.ReaderAt
	size   int64
	lay    *layout
	cache  *cache
	pools  streamPools
	closed atomic.Bool
}

func newReader(f *file, lay *layout, r io.ReaderAt, size int64, lim codec.Limits, cacheBytes int64) (*reader, error) {
	if pixels := int64(lay.width) * int64(lay.height); pixels > lim.MaxTiledPixels {
		return nil, guard.TooLarge("tiff: %d pixels exceed the tiled limit of %d", pixels, lim.MaxTiledPixels)
	}
	if err := f.loadTables(lay); err != nil {
		return nil, err
	}
	return &reader{r: r, size: size, lay: lay, cache: newCache(cacheBytes)}, nil
}

func (rd *reader) Info() codec.Info { return rd.lay.info }

func (rd *reader) MinShift() int { return 0 }

func (rd *reader) Close() error {
	rd.closed.Store(true)
	rd.cache.drop()
	return nil
}

func (rd *reader) Overview(ctx context.Context, shift int) (*image.NRGBA, error) {
	lw, lh := codec.LevelSize(rd.lay.width, rd.lay.height, shift)
	return rd.shielded(ctx, shift, image.Rect(0, 0, lw, lh), false)
}

func (rd *reader) Region(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA, error) {
	return rd.shielded(ctx, shift, r, true)
}

func (rd *reader) level(ctx context.Context, shift int, want image.Rectangle, cached bool, workers int) (*image.NRGBA, error) {
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
		return image.NewNRGBA(image.Rectangle{}), nil
	}
	if int64(rect.Dx())*int64(rect.Dy())*4 > maxResultBytes {
		return nil, guard.TooLarge("tiff: a %dx%d result is too large", rect.Dx(), rect.Dy())
	}
	if shift > nativeShift {
		return rd.derived(ctx, shift, rect, cached, workers)
	}
	out := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	source := image.Rect(rect.Min.X<<uint(shift), rect.Min.Y<<uint(shift), min(rect.Max.X<<uint(shift), lay.width), min(rect.Max.Y<<uint(shift), lay.height))
	chunks := lay.chunksFor(source)
	bandBytes := int64(lay.chunkW) * int64(lay.bandRows()) * 4
	workers = int(max(1, min(int64(workers), pipelineBudget/(bandBytes*lookahead*(pipelineDepth+1)))))
	produce := rd.producer(source, cached)
	var err error
	if lay.tiled {
		blit := &blitter{source: source, shift: shift, out: out}
		err = walk(ctx, chunks, workers, produce, blit.add)
	} else {
		accumulate := newRowAccumulator(source, shift, out)
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

func (rd *reader) derived(ctx context.Context, shift int, rect image.Rectangle, cached bool, workers int) (*image.NRGBA, error) {
	parentW, parentH := codec.LevelSize(rd.lay.width, rd.lay.height, shift-1)
	parent := image.Rect(rect.Min.X*2, rect.Min.Y*2, rect.Max.X*2, rect.Max.Y*2).Intersect(image.Rect(0, 0, parentW, parentH))
	child, err := rd.level(ctx, shift-1, parent, cached, workers)
	if err != nil {
		return nil, err
	}
	return reduce.Halve(child), nil
}

func (lay *layout) chunksFor(source image.Rectangle) []int {
	firstX, lastX := source.Min.X/lay.chunkW, (source.Max.X-1)/lay.chunkW
	firstY, lastY := source.Min.Y/lay.chunkH, (source.Max.Y-1)/lay.chunkH
	chunks := make([]int, 0, (lastX-firstX+1)*(lastY-firstY+1))
	for y := firstY; y <= lastY; y++ {
		for x := firstX; x <= lastX; x++ {
			chunks = append(chunks, y*lay.across+x)
		}
	}
	return chunks
}

func (rd *reader) producer(source image.Rectangle, cached bool) producer {
	return func(ctx context.Context, chunk int, emit func(piece) error) error {
		x0, y0, _, validH := rd.lay.chunkRect(chunk)
		top := max(source.Min.Y, y0) - y0
		bottom := min(source.Max.Y, y0+validH) - y0
		if top >= bottom {
			return nil
		}
		rows := rd.lay.bandRows()
		first, last := top/rows, (bottom-1)/rows
		if cached {
			return rd.emitCached(ctx, chunk, first, last, rows, x0, y0, emit)
		}
		return rd.emitStreamed(ctx, chunk, first, last, rows, x0, y0, validH, emit)
	}
}

func (rd *reader) emitCached(ctx context.Context, chunk, first, last, rows, x0, y0 int, emit func(piece) error) error {
	for band := first; band <= last; band++ {
		if rd.closed.Load() {
			return errClosed
		}
		img, err := rd.cache.get(ctx, bandKey{chunk: chunk, band: band}, func(ctx context.Context) (*image.NRGBA, error) {
			return rd.decodeBand(ctx, chunk, band, rows)
		})
		if err != nil {
			return err
		}
		if err := emit(piece{x: x0, y: y0 + band*rows, img: img}); err != nil {
			return err
		}
	}
	return nil
}

func (rd *reader) emitStreamed(ctx context.Context, chunk, first, last, rows, x0, y0, validH int, emit func(piece) error) error {
	stream, err := rd.openChunk(ctx, chunk, first*rows)
	if err != nil {
		return err
	}
	defer stream.close()
	for band := first; band <= last; band++ {
		if rd.closed.Load() {
			return errClosed
		}
		img, err := stream.readBand(ctx, min(rows, validH-band*rows), &rd.pools.bands)
		if err != nil {
			return err
		}
		if err := emit(piece{x: x0, y: y0 + band*rows, img: img, done: func() { rd.pools.bands.put(img.Pix) }}); err != nil {
			return err
		}
	}
	return nil
}

func (rd *reader) decodeBand(ctx context.Context, chunk, band, rows int) (*image.NRGBA, error) {
	_, _, _, validH := rd.lay.chunkRect(chunk)
	stream, err := rd.openChunk(ctx, chunk, band*rows)
	if err != nil {
		return nil, err
	}
	defer stream.close()
	return stream.readBand(ctx, min(rows, validH-band*rows), nil)
}

func (rd *reader) shielded(ctx context.Context, shift int, rect image.Rectangle, cached bool) (*image.NRGBA, error) {
	workers := min(par.Count(), overviewWorkers)
	if cached {
		workers = min(par.Count(), regionWorkers)
	}
	var out *image.NRGBA
	err := shield.Run("tiff", func() (err error) {
		out, err = rd.level(ctx, shift, rect, cached, workers)
		return err
	})
	return out, err
}
