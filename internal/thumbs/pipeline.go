package thumbs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	stdjpeg "image/jpeg"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	exifMinNum       = 4
	exifMinDen       = 5
	exifAspectTol    = 50
	exifMaxPixels    = 1 << 22
	aspectDrift      = 20
	guardStateRun    = 0
	guardStateGone   = 1
	guardStateFinish = 2
)

type outcome[T any] struct {
	v   T
	err error
}

func guarded[T any](s *Service, ctx context.Context, fn func() (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if s.abandoned.Load() >= s.maxAbandoned.Load() {
		return fn()
	}
	done := make(chan outcome[T], 1)
	var state atomic.Int32
	go func() {
		var out outcome[T]
		defer func() {
			if r := recover(); r != nil {
				out = outcome[T]{err: fmt.Errorf("thumbs: panic: %v", r)}
			}
			if !state.CompareAndSwap(guardStateRun, guardStateFinish) {
				s.abandoned.Add(-1)
			}
			done <- out
		}()
		v, err := fn()
		out = outcome[T]{v: v, err: err}
	}()
	select {
	case out := <-done:
		return out.v, out.err
	case <-ctx.Done():
		if state.CompareAndSwap(guardStateRun, guardStateGone) {
			s.abandoned.Add(1)
			return zero, ctx.Err()
		}
		out := <-done
		return out.v, out.err
	}
}

func (s *Service) safeProduce(ctx context.Context, j *job) (t Thumb, fromRender bool) {
	defer func() {
		if v := recover(); v != nil {
			t, fromRender = Thumb{Err: fmt.Errorf("thumbs: panic: %v", v)}, false
		}
	}()
	return s.produce(ctx, j)
}

func (s *Service) produce(ctx context.Context, j *job) (Thumb, bool) {
	if err := ctx.Err(); err != nil {
		return Thumb{Err: err}, false
	}
	if s.disk != nil {
		if t, ok := s.disk.load(j.key); ok {
			return t, false
		}
	}
	return s.render(ctx, j), true
}

type probed struct {
	dec  codec.Decoder
	info codec.Info
}

func (s *Service) render(ctx context.Context, j *job) Thumb {
	side := j.key.Side
	lim := j.lim.WithDefaults()
	src, err := codec.OpenFile(j.key.Path)
	if err != nil {
		return Thumb{Err: err}
	}
	defer src.Close()
	if src.Size > lim.MaxFileBytes {
		return Thumb{Err: fmt.Errorf("%w: file is %d bytes", codec.ErrTooLarge, src.Size)}
	}
	p, err := guarded(s, ctx, func() (probed, error) {
		d, info, err := s.reg.Probe(ctx, src, lim)
		return probed{dec: d, info: info}, err
	})
	info := p.info
	fail := func(err error) Thumb {
		return Thumb{Err: err, SrcW: info.Width, SrcH: info.Height}
	}
	oversize := false
	if err != nil {
		if p.dec == nil || !errors.Is(err, codec.ErrTooLarge) || info.Width <= 0 || info.Height <= 0 {
			return fail(err)
		}
		oversize = true
	} else if err := lim.Check(info.Width, info.Height); err != nil {
		return fail(fmt.Errorf("%w: %dx%d", err, info.Width, info.Height))
	}
	o := validOrientation(info.Orientation)
	ow, oh := orientedSize(o, info.Width, info.Height)
	tw, th := fitSize(ow, oh, side)
	sw, sh := storedSize(o, tw, th)

	if p.dec.ID() == codec.JPEG {
		if img, ok := s.fromEXIF(src, info, side, sw, sh); ok {
			return Thumb{Img: orient(img, o), SrcW: info.Width, SrcH: info.Height, Orientation: o}
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}

	var pix *image.NRGBA
	var imgInfo codec.Info
	if oversize {
		pix, imgInfo, err = s.decodeReduced(ctx, p.dec, src, lim, info, side)
	} else {
		pix, imgInfo, err = s.decodeScaled(ctx, p.dec, src, lim, info, side, sw, sh)
	}
	if err != nil {
		return fail(err)
	}
	if pix == nil || pix.Bounds().Empty() {
		return fail(codec.ErrCorrupt)
	}
	if imgInfo.Format != "" && imgInfo.Orientation.Valid() {
		o = imgInfo.Orientation
	}
	pw, ph := pix.Bounds().Dx(), pix.Bounds().Dy()
	ow, oh = orientedSize(o, info.Width, info.Height)
	if !sameAspect(pw, ph, info.Width, info.Height) {
		ow, oh = orientedSize(o, pw, ph)
	}
	tw, th = fitSize(ow, oh, side)
	sw, sh = storedSize(o, tw, th)
	sw, sh = min(sw, pw), min(sh, ph)
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	out := orient(resizeArea(pix, sw, sh), o)
	return Thumb{Img: out, SrcW: info.Width, SrcH: info.Height, Orientation: o}
}

func sameAspect(aw, ah, bw, bh int) bool {
	l, r := int64(aw)*int64(bh), int64(ah)*int64(bw)
	d := l - r
	if d < 0 {
		d = -d
	}
	return d*aspectDrift <= max(l, r)
}

func exifUsable(tw, th, w, h, side int) bool {
	if tw <= 0 || th <= 0 || w <= 0 || h <= 0 {
		return false
	}
	if int64(max(tw, th))*exifMinDen < int64(side)*exifMinNum {
		return false
	}
	l, r := int64(tw)*int64(h), int64(th)*int64(w)
	d := l - r
	if d < 0 {
		d = -d
	}
	return d*exifAspectTol <= r
}

func (s *Service) fromEXIF(src *codec.Source, info codec.Info, side, sw, sh int) (*image.RGBA, bool) {
	payload, err := exif.FromJPEG(src.R, src.Size)
	if err != nil {
		return nil, false
	}
	data, ok := exif.JPEGThumbnail(payload)
	if !ok {
		return nil, false
	}
	cfg, err := stdjpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || int64(cfg.Width)*int64(cfg.Height) > exifMaxPixels {
		return nil, false
	}
	if !exifUsable(cfg.Width, cfg.Height, info.Width, info.Height, side) {
		return nil, false
	}
	im, err := stdjpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	b := im.Bounds()
	if b.Dx() != cfg.Width || b.Dy() != cfg.Height {
		return nil, false
	}
	if max(b.Dx(), b.Dy()) <= side {
		return toRGBA(im), true
	}
	return resizeArea(toNRGBA(im), min(sw, b.Dx()), min(sh, b.Dy())), true
}
