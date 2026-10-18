package app

import (
	"fmt"
	"image"

	"github.com/oops1/PixGo/internal/codec"
	coreedit "github.com/oops1/PixGo/internal/edit"
)

const maxShift = 16

func levelRect(r image.Rectangle, shift int) image.Rectangle {
	if shift <= 0 {
		return r
	}
	d := 1 << uint(shift)
	return image.Rect(r.Min.X/d, r.Min.Y/d, (r.Max.X+d-1)/d, (r.Max.Y+d-1)/d)
}

func applyTiled(ctx contextT, reg *codec.Registry, lim codec.Limits, path string, p coreedit.Plan, stored image.Point) (*image.NRGBA, error) {
	src, err := codec.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	dec, ok := reg.Detect(src.Head, path)
	if !ok {
		return nil, codec.ErrUnknownFormat
	}
	td, ok := dec.(codec.TiledDecoder)
	if !ok {
		return nil, fmt.Errorf("%w: %s cannot be read in parts", codec.ErrTooLarge, dec.ID())
	}
	tr, err := td.OpenTiled(ctx, src.R, src.Size, lim)
	if err != nil {
		return nil, err
	}
	defer tr.Close()
	crop := p.CropRect(stored.X, stored.Y)
	rect := storedRectOf(p.Orientation, crop, stored).Intersect(image.Rectangle{Max: stored})
	if rect.Empty() {
		return nil, coreedit.ErrEmptyCrop
	}
	limit := lim.WithDefaults().MaxPixels
	shift := tr.MinShift()
	for shift < maxShift {
		lr := levelRect(rect, shift)
		if int64(lr.Dx())*int64(lr.Dy()) <= limit {
			break
		}
		shift++
	}
	if shift > 0 && !p.Resizes(stored.X, stored.Y) {
		return nil, fmt.Errorf("%w: the area is larger than the pixel limit", codec.ErrTooLarge)
	}
	region, err := tr.Region(ctx, shift, levelRect(rect, shift))
	if err != nil {
		return nil, err
	}
	q := coreedit.Plan{Orientation: p.Orientation, Size: p.Size}
	return coreedit.Apply(ctx, region, q)
}

func (c *Controller) Reload() {
	if c.current == "" {
		return
	}
	c.cache.Remove(c.current)
	c.status = StatusLoading
	c.keepView = false
	c.loader.Reload(c.current)
	c.changed()
}
