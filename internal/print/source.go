package print

import (
	"context"
	"image"
	"math"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/edit"
	"github.com/oops1/PixGo/internal/imaging"
)

const (
	maxShift       = 16
	DefaultMaxTile = 64_000_000
)

type Source interface {
	Size() image.Point
	Render(ctx context.Context, crop image.Rectangle, w, h int) (*image.NRGBA, error)
}

func orientation(o codec.Orientation) codec.Orientation {
	if !o.Valid() {
		return codec.OrientNormal
	}
	return o
}

func storedRect(o codec.Orientation, r image.Rectangle, stored image.Point) image.Rectangle {
	w, h := float64(stored.X), float64(stored.Y)
	x0, y0 := o.ToSource(float64(r.Min.X), float64(r.Min.Y), w, h)
	x1, y1 := o.ToSource(float64(r.Max.X), float64(r.Max.Y), w, h)
	return image.Rect(int(math.Round(min(x0, x1))), int(math.Round(min(y0, y1))), int(math.Round(max(x0, x1))), int(math.Round(max(y0, y1)))).Intersect(image.Rectangle{Max: stored})
}

func levelRect(r image.Rectangle, shift int) image.Rectangle {
	if shift <= 0 {
		return r
	}
	d := 1 << uint(shift)
	return image.Rect(r.Min.X/d, r.Min.Y/d, (r.Max.X+d-1)/d, (r.Max.Y+d-1)/d)
}

func checkRender(crop image.Rectangle, size image.Point, w, h int) error {
	if w < 1 || h < 1 || int64(w)*int64(h) > MaxSheetPixels {
		return ErrTooLarge
	}
	if crop.Empty() || !crop.In(image.Rectangle{Max: size}) {
		return ErrNoPicture
	}
	return nil
}

func fromStored(ctx context.Context, sub *image.NRGBA, o codec.Orientation, w, h int) (*image.NRGBA, error) {
	dw, dh := w, h
	if o.Transposes() {
		dw, dh = h, w
	}
	b := sub.Bounds()
	out := sub
	if b.Dx() != dw || b.Dy() != dh {
		var err error
		out, err = edit.Resample(ctx, sub, dw, dh)
		if err != nil {
			return nil, err
		}
	}
	return imaging.Orient(rebase(out), o), nil
}

type ImageSource struct {
	Img    *image.NRGBA
	Orient codec.Orientation
}

func (s ImageSource) stored() image.Point {
	if s.Img == nil {
		return image.Point{}
	}
	return s.Img.Bounds().Size()
}

func (s ImageSource) Size() image.Point {
	st := s.stored()
	w, h := orientation(s.Orient).Size(st.X, st.Y)
	return image.Pt(w, h)
}

func (s ImageSource) Render(ctx context.Context, crop image.Rectangle, w, h int) (*image.NRGBA, error) {
	if s.Img == nil {
		return nil, ErrNoPicture
	}
	if err := checkRender(crop, s.Size(), w, h); err != nil {
		return nil, err
	}
	o := orientation(s.Orient)
	r := storedRect(o, crop, s.stored())
	if r.Empty() {
		return nil, ErrNoPicture
	}
	sub := s.Img.SubImage(r.Add(s.Img.Bounds().Min)).(*image.NRGBA)
	return fromStored(ctx, sub, o, w, h)
}

type TiledSource struct {
	Open      func(ctx context.Context) (codec.TileReader, error)
	Dim       image.Point
	Orient    codec.Orientation
	MaxPixels int64
}

func (s TiledSource) Size() image.Point {
	w, h := orientation(s.Orient).Size(s.Dim.X, s.Dim.Y)
	return image.Pt(w, h)
}

func (s TiledSource) Render(ctx context.Context, crop image.Rectangle, w, h int) (*image.NRGBA, error) {
	if s.Open == nil {
		return nil, ErrNoPicture
	}
	if err := checkRender(crop, s.Size(), w, h); err != nil {
		return nil, err
	}
	o := orientation(s.Orient)
	r := storedRect(o, crop, s.Dim)
	if r.Empty() {
		return nil, ErrNoPicture
	}
	tr, err := s.Open(ctx)
	if err != nil {
		return nil, err
	}
	defer tr.Close()
	dw, dh := w, h
	if o.Transposes() {
		dw, dh = h, w
	}
	limit := s.MaxPixels
	if limit <= 0 {
		limit = DefaultMaxTile
	}
	shift := max(tr.MinShift(), 0)
	for shift < maxShift {
		next := levelRect(r, shift+1)
		if next.Dx() < dw || next.Dy() < dh {
			break
		}
		shift++
	}
	for shift < maxShift {
		lr := levelRect(r, shift)
		if int64(lr.Dx())*int64(lr.Dy()) <= limit {
			break
		}
		shift++
	}
	region, err := tr.Region(ctx, shift, levelRect(r, shift))
	if err != nil {
		return nil, err
	}
	if region == nil || region.Bounds().Empty() {
		return nil, ErrNoPicture
	}
	return fromStored(ctx, region, o, w, h)
}

type VectorSource struct {
	Dim    image.Point
	Orient codec.Orientation
	Decode func(ctx context.Context, w, h int) (*image.NRGBA, error)
}

func (s VectorSource) Size() image.Point {
	w, h := orientation(s.Orient).Size(s.Dim.X, s.Dim.Y)
	return image.Pt(w, h)
}

func (s VectorSource) Render(ctx context.Context, crop image.Rectangle, w, h int) (*image.NRGBA, error) {
	if s.Decode == nil {
		return nil, ErrNoPicture
	}
	if err := checkRender(crop, s.Size(), w, h); err != nil {
		return nil, err
	}
	k := float64(w) / float64(crop.Dx())
	k = math.Max(k, float64(h)/float64(crop.Dy()))
	fw, fh := max(1, int(math.Round(float64(s.Dim.X)*k))), max(1, int(math.Round(float64(s.Dim.Y)*k)))
	if int64(fw)*int64(fh) > MaxSheetPixels {
		return nil, ErrTooLarge
	}
	img, err := s.Decode(ctx, fw, fh)
	if err != nil {
		return nil, err
	}
	if img == nil || img.Bounds().Empty() {
		return nil, ErrNoPicture
	}
	o := orientation(s.Orient)
	got := img.Bounds().Size()
	ow, oh := o.Size(got.X, got.Y)
	full := image.Rectangle{Max: s.Size()}
	scaled := image.Rect(
		int(math.Round(float64(crop.Min.X)*float64(ow)/float64(full.Dx()))),
		int(math.Round(float64(crop.Min.Y)*float64(oh)/float64(full.Dy()))),
		int(math.Round(float64(crop.Max.X)*float64(ow)/float64(full.Dx()))),
		int(math.Round(float64(crop.Max.Y)*float64(oh)/float64(full.Dy()))),
	).Intersect(image.Rect(0, 0, ow, oh))
	if scaled.Empty() {
		return nil, ErrNoPicture
	}
	return ImageSource{Img: img, Orient: o}.Render(ctx, scaled, w, h)
}

func Render(ctx context.Context, src Source, pg Page) (*image.RGBA, error) {
	if src == nil {
		return nil, ErrNoPicture
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if pg.Sheet.X < 1 || pg.Sheet.Y < 1 || int64(pg.Sheet.X)*int64(pg.Sheet.Y) > MaxSheetPixels {
		return nil, ErrTooLarge
	}
	if pg.Dest.Empty() || pg.Crop.Empty() {
		return nil, ErrNoPicture
	}
	img, err := src.Render(ctx, pg.Crop, pg.Dest.Dx(), pg.Dest.Dy())
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sheet := image.NewRGBA(image.Rectangle{Max: pg.Sheet})
	for i := range sheet.Pix {
		sheet.Pix[i] = 0xff
	}
	compose(sheet, pg.Dest, img)
	return sheet, ctx.Err()
}

func compose(dst *image.RGBA, at image.Rectangle, src *image.NRGBA) {
	sb := src.Bounds()
	w := min(at.Dx(), sb.Dx())
	h := min(at.Dy(), sb.Dy())
	for y := 0; y < h; y++ {
		s := src.Pix[src.PixOffset(sb.Min.X, sb.Min.Y+y):]
		d := dst.Pix[dst.PixOffset(at.Min.X, at.Min.Y+y):]
		for x := 0; x < w; x++ {
			a := uint32(s[x*4+3])
			if a == 255 {
				d[x*4], d[x*4+1], d[x*4+2], d[x*4+3] = s[x*4], s[x*4+1], s[x*4+2], 255
				continue
			}
			inv := 255 - a
			d[x*4] = uint8((uint32(s[x*4])*a + 255*inv + 127) / 255)
			d[x*4+1] = uint8((uint32(s[x*4+1])*a + 255*inv + 127) / 255)
			d[x*4+2] = uint8((uint32(s[x*4+2])*a + 255*inv + 127) / 255)
			d[x*4+3] = 255
		}
	}
}

func rebase(src *image.NRGBA) *image.NRGBA {
	b := src.Bounds()
	if b.Min == (image.Point{}) {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		o := src.PixOffset(b.Min.X, b.Min.Y+y)
		copy(dst.Pix[y*dst.Stride:y*dst.Stride+b.Dx()*4], src.Pix[o:o+b.Dx()*4])
	}
	return dst
}

type Proxy struct {
	Source Source
	Full   image.Point
}

func (p Proxy) Size() image.Point { return p.Full }

func (p Proxy) Render(ctx context.Context, crop image.Rectangle, w, h int) (*image.NRGBA, error) {
	if p.Source == nil || p.Full.X < 1 || p.Full.Y < 1 {
		return nil, ErrNoPicture
	}
	if err := checkRender(crop, p.Full, w, h); err != nil {
		return nil, err
	}
	own := p.Source.Size()
	fx, fy := float64(own.X)/float64(p.Full.X), float64(own.Y)/float64(p.Full.Y)
	r := image.Rect(
		int(math.Floor(float64(crop.Min.X)*fx)), int(math.Floor(float64(crop.Min.Y)*fy)),
		int(math.Ceil(float64(crop.Max.X)*fx)), int(math.Ceil(float64(crop.Max.Y)*fy)),
	).Intersect(image.Rectangle{Max: own})
	if r.Empty() {
		return nil, ErrNoPicture
	}
	return p.Source.Render(ctx, r, w, h)
}
