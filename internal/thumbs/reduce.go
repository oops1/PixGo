package thumbs

import (
	"context"
	"image"
	"math/bits"

	"github.com/oops1/PixGo/internal/codec"
)

const maxReducedShift = 3

func (s *Service) decodeScaled(ctx context.Context, dec codec.Decoder, src *codec.Source, lim codec.Limits, info codec.Info, side, sw, sh int) (*image.NRGBA, codec.Info, error) {
	opts := codec.Options{Limits: lim, Frames: codec.FirstFrame}
	if n := scaleHint(max(info.Width, info.Height), side); n > 1 {
		opts.Scale = n
	}
	if info.Vector {
		opts.VectorW, opts.VectorH = sw, sh
	}
	img, err := guarded(s, ctx, func() (*codec.Image, error) {
		return s.reg.Decode(ctx, dec, src, opts)
	})
	if err != nil {
		return nil, codec.Info{}, err
	}
	return img.First(), img.Info, nil
}

func (s *Service) decodeReduced(ctx context.Context, dec codec.Decoder, src *codec.Source, lim codec.Limits, info codec.Info, side int) (*image.NRGBA, codec.Info, error) {
	n := scaleHint(max(info.Width, info.Height), side)
	if n <= 1 {
		return nil, codec.Info{}, codec.ErrTooLarge
	}
	if td, ok := dec.(codec.TiledDecoder); ok {
		return s.overviewOf(ctx, td, src, lim, info, bits.TrailingZeros(uint(n)))
	}
	opts := codec.Options{Limits: lim, Frames: codec.FirstFrame, Scale: n}
	img, err := guarded(s, ctx, func() (*codec.Image, error) {
		return s.reg.Decode(ctx, dec, src, opts)
	})
	if err != nil {
		return nil, codec.Info{}, err
	}
	if img.Scale <= 1 {
		return nil, codec.Info{}, codec.ErrTooLarge
	}
	return img.First(), img.Info, nil
}

func (s *Service) overviewOf(ctx context.Context, td codec.TiledDecoder, src *codec.Source, lim codec.Limits, info codec.Info, wanted int) (*image.NRGBA, codec.Info, error) {
	tr, err := guarded(s, ctx, func() (codec.TileReader, error) {
		return td.OpenTiled(ctx, src.R, src.Size, lim)
	})
	if err != nil {
		return nil, codec.Info{}, err
	}
	shift := max(wanted, tr.MinShift())
	for shift < maxReducedShift {
		lw, lh := codec.LevelSize(info.Width, info.Height, shift)
		if int64(lw)*int64(lh) <= lim.MaxPixels {
			break
		}
		shift++
	}
	if lw, lh := codec.LevelSize(info.Width, info.Height, shift); int64(lw)*int64(lh) > lim.MaxPixels {
		tr.Close()
		return nil, codec.Info{}, codec.ErrTooLarge
	}
	readerInfo := tr.Info()
	pix, err := guarded(s, ctx, func() (*image.NRGBA, error) {
		return tr.Overview(ctx, shift)
	})
	if ctx.Err() == nil {
		tr.Close()
	}
	if err != nil {
		return nil, codec.Info{}, err
	}
	return pix, readerInfo, nil
}
