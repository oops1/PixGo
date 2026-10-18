package colorm

import (
	"context"
	"image"
	"image/color"

	"github.com/oops1/PixGo/internal/codec"
)

func hasInk(img *codec.Image) bool {
	if img == nil {
		return false
	}
	for i := range img.Frames {
		if img.Frames[i].CMYK != nil {
			return true
		}
	}
	return false
}

func naiveInk(src *image.CMYK) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	bands(w, h, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			s := src.Pix[src.PixOffset(b.Min.X, b.Min.Y+y):]
			d := dst.Pix[y*dst.Stride : y*dst.Stride+w*4]
			for x := 0; x < w; x++ {
				d[x*4], d[x*4+1], d[x*4+2] = color.CMYKToRGB(s[x*4], s[x*4+1], s[x*4+2], s[x*4+3])
				d[x*4+3] = 0xff
			}
		}
	})
	return dst
}

func (m *Manager) inkTransform(ci codec.ColorInfo) (*Transform, Report, bool) {
	t, ok := m.ResolveCMYK(ci)
	if !ok {
		return nil, Report{Managed: m.Enabled(), CMYK: true}, false
	}
	src, _ := m.resolveSource(ci)
	rep := Report{Managed: true, Origin: OriginEmbedded, Profile: src.Description, CMYK: true, Converted: true}
	dst, name := m.Display()
	if name != "" {
		rep.Display = name
	} else if dst != SRGB() {
		rep.Display = dst.Description
	}
	return t, rep, true
}

func convertInkFrame(t *Transform, src *image.CMYK) (*image.NRGBA, error) {
	if t == nil {
		return naiveInk(src), nil
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	if err := t.ConvertCMYK(dst, src); err != nil {
		return nil, err
	}
	return dst, nil
}

func (m *Manager) convertInk(img *codec.Image) (Report, error) {
	t, rep, ok := m.inkTransform(img.Color)
	if !ok {
		t = nil
	}
	for i := range img.Frames {
		f := &img.Frames[i]
		if f.CMYK == nil {
			continue
		}
		pix, err := convertInkFrame(t, f.CMYK)
		if err != nil {
			return rep, err
		}
		f.Pix, f.CMYK = pix, nil
	}
	return rep, nil
}

type inkTileReader struct {
	codec.TileReader
	ink codec.InkTileReader
	t   *Transform
}

func (t *inkTileReader) Overview(ctx context.Context, shift int) (*image.NRGBA, error) {
	src, err := t.ink.OverviewInk(ctx, shift)
	if err != nil {
		return nil, err
	}
	return t.fromInk(src)
}

func (t *inkTileReader) Region(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA, error) {
	src, err := t.ink.RegionInk(ctx, shift, r)
	if err != nil {
		return nil, err
	}
	return t.fromInk(src)
}

func (t *inkTileReader) fromInk(src *image.CMYK) (*image.NRGBA, error) {
	if src == nil {
		return nil, codec.ErrCorrupt
	}
	if src.Bounds().Empty() {
		return image.NewNRGBA(image.Rectangle{}), nil
	}
	return convertInkFrame(t.t, src)
}

func (m *Manager) wrapInkTiles(tr codec.TileReader, ci codec.ColorInfo) (codec.TileReader, Report, bool) {
	ik, ok := tr.(codec.InkTileReader)
	if !ok || !ik.Ink() {
		return tr, Report{}, false
	}
	t, rep, ok := m.inkTransform(ci)
	if !ok {
		return tr, Report{}, false
	}
	return &inkTileReader{TileReader: tr, ink: ik, t: t}, rep, true
}
