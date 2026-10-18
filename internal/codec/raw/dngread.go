package raw

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/par"
	"github.com/oops1/PixGo/internal/rawdev"
)

type chunkReader struct {
	plan   *dngPlan
	ctx    context.Context
	r      io.ReaderAt
	size   int64
	pix    []uint16
	stride int
	temps  sync.Pool
}

func (p *dngPlan) rowBytes(w int) int {
	return (w*p.spp*p.bps + 7) / 8
}

func (c *chunkReader) read(ch dngChunk) ([]byte, error) {
	if ch.length <= 0 || ch.offset <= 0 || ch.offset > c.size || ch.length > c.size-ch.offset {
		return nil, guard.Corrupt("raw: image data at %d+%d lies outside the file", ch.offset, ch.length)
	}
	return guard.ReadAt(c.r, ch.offset, ch.length, c.size)
}

func (c *chunkReader) target(ch dngChunk) ([]uint16, int, bool) {
	p := c.plan
	if ch.codedW == ch.w && ch.codedH == ch.h {
		return c.pix[ch.y*c.stride+ch.x*p.spp:], c.stride, true
	}
	need := ch.codedW * ch.codedH * p.spp
	if buf, ok := c.temps.Get().(*[]uint16); ok && len(*buf) >= need {
		return (*buf)[:need], ch.codedW * p.spp, false
	}
	return make([]uint16, need), ch.codedW * p.spp, false
}

func (c *chunkReader) release(buf []uint16, direct bool) {
	if !direct {
		c.temps.Put(&buf)
	}
}

func (c *chunkReader) decode(ch dngChunk) error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	p := c.plan
	data, err := c.read(ch)
	if err != nil {
		return err
	}
	dst, stride, direct := c.target(ch)
	switch p.compression {
	case compressionJPEG:
		err = c.lossless(ch, data, dst, stride)
	case compressionDeflate:
		err = c.inflate(ch, data, dst, stride)
	default:
		err = c.plain(ch, data, dst, stride)
	}
	if err != nil {
		return err
	}
	if !direct {
		n := ch.w * p.spp
		for y := 0; y < ch.h; y++ {
			copy(c.pix[(ch.y+y)*c.stride+ch.x*p.spp:][:n], dst[y*stride:][:n])
		}
		c.release(dst, direct)
	}
	return nil
}

func (c *chunkReader) lossless(ch dngChunk, data []byte, dst []uint16, stride int) error {
	d, err := rawdev.ParseLJ92(data)
	if err != nil {
		return fromDev(err)
	}
	if d.Info.Width*d.Info.Components != ch.codedW*c.plan.spp || d.Info.Height != ch.codedH {
		return guard.Corrupt("raw: lossless block of %dx%dx%d does not fit %dx%d", d.Info.Width, d.Info.Height, d.Info.Components, ch.codedW, ch.codedH)
	}
	return fromDev(d.Decode(c.ctx, dst, stride))
}

func (c *chunkReader) plain(ch dngChunk, data []byte, dst []uint16, stride int) error {
	p := c.plan
	rowBytes := p.rowBytes(ch.codedW)
	if int64(len(data)) < int64(rowBytes)*int64(ch.codedH) {
		return guard.Corrupt("raw: image block of %d bytes is shorter than %d", len(data), rowBytes*ch.codedH)
	}
	for y := 0; y < ch.codedH; y++ {
		unpack(dst[y*stride:][:ch.codedW*p.spp], data[y*rowBytes:][:rowBytes], p.bps, p.order.Uint16)
	}
	return nil
}

func (c *chunkReader) inflate(ch dngChunk, data []byte, dst []uint16, stride int) error {
	p := c.plan
	rowBytes := p.rowBytes(ch.codedW)
	total := rowBytes * ch.codedH
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return guard.Corrupt("raw: deflate: %v", err)
	}
	defer zr.Close()
	raw := make([]byte, total)
	if _, err := io.ReadFull(zr, raw); err != nil {
		return guard.Corrupt("raw: deflate: %v", err)
	}
	step := 0
	switch p.predictor {
	case predictorHorizontal:
		step = p.spp
	case predictorHorizontal2:
		step = 2 * p.spp
	case predictorHorizontal4:
		step = 4 * p.spp
	}
	mask := uint16(1<<uint(p.bps) - 1)
	n := ch.codedW * p.spp
	for y := 0; y < ch.codedH; y++ {
		row := dst[y*stride:][:n]
		unpack(row, raw[y*rowBytes:][:rowBytes], p.bps, p.order.Uint16)
		for i := step; step > 0 && i < n; i++ {
			row[i] = (row[i] + row[i-step]) & mask
		}
	}
	return nil
}

func unpack(dst []uint16, src []byte, bps int, order func([]byte) uint16) {
	switch bps {
	case 8:
		for i := range dst {
			dst[i] = uint16(src[i])
		}
	case 16:
		for i := range dst {
			dst[i] = order(src[2*i:])
		}
	default:
		var acc uint32
		var have int
		pos := 0
		mask := uint32(1)<<uint(bps) - 1
		for i := range dst {
			for have < bps {
				acc = acc<<8 | uint32(src[pos])
				pos++
				have += 8
			}
			have -= bps
			dst[i] = uint16(acc >> uint(have) & mask)
		}
	}
}

func fromDev(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), guard.IsCodecError(err):
		return err
	case errors.Is(err, rawdev.ErrUnsupported):
		return guard.Unsupported("raw: %v", err)
	}
	return guard.Corrupt("raw: %v", err)
}

func (p *dngPlan) mosaic(ctx context.Context, r io.ReaderAt, size int64) ([]uint16, error) {
	samples := int64(p.width) * int64(p.height) * int64(p.spp)
	c := &chunkReader{plan: p, ctx: ctx, r: r, size: size, pix: make([]uint16, samples), stride: p.width * p.spp}
	var failed atomic.Pointer[error]
	par.Parallel(len(p.chunks), par.Count(), func(i int) {
		if failed.Load() != nil {
			return
		}
		if err := c.decode(p.chunks[i]); err != nil {
			failed.CompareAndSwap(nil, &err)
		}
	})
	if err := failed.Load(); err != nil {
		return nil, *err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.pix, nil
}

func supportedScale(requested int) int {
	switch {
	case requested >= 8:
		return 8
	case requested >= 4:
		return 4
	case requested >= 2:
		return 2
	}
	return 1
}

func (p *dngPlan) develop(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	pix, err := p.mosaic(ctx, r, size)
	if err != nil {
		return nil, err
	}
	scale := supportedScale(o.Scale)
	m := rawdev.Mosaic{Pix: pix, Stride: p.width * p.spp, Samples: p.spp, X: p.area.Min.X, Y: p.area.Min.Y, W: p.area.Dx(), H: p.area.Dy()}
	img8, img16, err := rawdev.Develop(ctx, m, p.params, scale, o.Deep)
	if err != nil {
		return nil, fromDev(err)
	}
	return &codec.Image{
		Info:   p.info(),
		Frames: []codec.Frame{{Pix: img8, Deep: img16}},
		Scale:  scale,
	}, nil
}
