package print

import (
	"errors"
	"fmt"
	"image"
	"math"
)

var (
	ErrNoPicture = errors.New("print: there is no picture")
	ErrNoRoom    = errors.New("print: the margins leave no room on the sheet")
	ErrBadSize   = errors.New("print: the custom size is not valid")
	ErrTooLarge  = errors.New("print: the page is too large")
)

const snapPixels = 2

type Picture struct {
	W, H int
	DPI  float64
}

func (p Picture) Known() bool { return p.DPI > 0 }

func (p Picture) density() float64 {
	if p.DPI > 0 {
		return p.DPI
	}
	return UnknownDPI
}

type Page struct {
	Sheet     image.Point
	Landscape bool
	Content   image.Rectangle
	Dest      image.Rectangle
	Crop      image.Rectangle
	Scaled    image.Point
	PrintDPI  float64
	Cut       bool
	DPI       int
}

func (pg Page) DestMM() (w, h float64) {
	k := mmPerInch / float64(pg.DPI)
	return float64(pg.Dest.Dx()) * k, float64(pg.Dest.Dy()) * k
}

func (pg Page) ScaledMM() (w, h float64) {
	k := mmPerInch / float64(pg.DPI)
	return float64(pg.Scaled.X) * k, float64(pg.Scaled.Y) * k
}

func MMToPx(mm float64, dpi int) int { return int(math.Round(mm * float64(dpi) / mmPerInch)) }

func SheetPixels(p Paper, landscape bool, dpi int) (w, h int) {
	wmm, hmm := p.Size(landscape)
	return MMToPx(wmm, dpi), MMToPx(hmm, dpi)
}

func (o Options) Landscape(p Picture) bool {
	switch o.Orientation {
	case OrientLandscape:
		return true
	case OrientPortrait:
		return false
	}
	return p.W > p.H
}

func (o Options) Layout(p Picture, dpi int) (Page, error) {
	o = o.Normalize()
	if p.W < 1 || p.H < 1 {
		return Page{}, ErrNoPicture
	}
	if dpi < 1 || dpi > MaxDPI {
		return Page{}, fmt.Errorf("print: resolution %d dpi is outside 1..%d", dpi, MaxDPI)
	}
	land := o.Landscape(p)
	sw, sh := SheetPixels(o.PaperValue(), land, dpi)
	if sw < 1 || sh < 1 || int64(sw)*int64(sh) > MaxSheetPixels {
		return Page{}, ErrTooLarge
	}
	m := MMToPx(o.Margin.MM(), dpi)
	content := image.Rect(m, m, sw-m, sh-m)
	if content.Empty() {
		return Page{}, ErrNoRoom
	}
	cw, ch := float64(content.Dx()), float64(content.Dy())
	pw, ph := float64(p.W), float64(p.H)
	var tw, th int
	switch o.Placement {
	case PlaceFill:
		s := math.Max(cw/pw, ch/ph)
		tw, th = int(math.Round(pw*s)), int(math.Round(ph*s))
	case PlaceActual:
		s := float64(dpi) / p.density()
		tw, th = int(math.Round(pw*s)), int(math.Round(ph*s))
	case PlaceCustom:
		if !o.CustomValid() {
			return Page{}, ErrBadSize
		}
		tw, th = MMToPx(o.CustomW, dpi), MMToPx(o.CustomH, dpi)
	default:
		s := math.Min(cw/pw, ch/ph)
		tw, th = int(math.Round(pw*s)), int(math.Round(ph*s))
	}
	tw, th = max(tw, 1), max(th, 1)
	if tw > content.Dx() && tw-content.Dx() <= snapPixels {
		tw = content.Dx()
	}
	if th > content.Dy() && th-content.Dy() <= snapPixels {
		th = content.Dy()
	}
	vw, vh := min(tw, content.Dx()), min(th, content.Dy())
	dest := image.Rect(0, 0, vw, vh).Add(image.Pt(content.Min.X+(content.Dx()-vw)/2, content.Min.Y+(content.Dy()-vh)/2))
	cropW, cropH := p.W, p.H
	if vw < tw {
		cropW = min(p.W, max(1, int(math.Round(float64(vw)/float64(tw)*pw))))
	}
	if vh < th {
		cropH = min(p.H, max(1, int(math.Round(float64(vh)/float64(th)*ph))))
	}
	x0, y0 := (p.W-cropW)/2, (p.H-cropH)/2
	crop := image.Rect(x0, y0, x0+cropW, y0+cropH)
	return Page{
		Sheet:     image.Pt(sw, sh),
		Landscape: land,
		Content:   content,
		Dest:      dest,
		Crop:      crop,
		Scaled:    image.Pt(tw, th),
		PrintDPI:  pw * float64(dpi) / float64(tw),
		Cut:       cropW < p.W || cropH < p.H,
		DPI:       dpi,
	}, nil
}

func (o Options) FitSizeMM(p Picture) (w, h float64) {
	o = o.Normalize()
	if p.W < 1 || p.H < 1 {
		return 0, 0
	}
	land := o.Landscape(p)
	sw, sh := o.PaperValue().Size(land)
	m := o.Margin.MM()
	cw, ch := sw-2*m, sh-2*m
	if cw <= 0 || ch <= 0 {
		return 0, 0
	}
	s := math.Min(cw/float64(p.W), ch/float64(p.H))
	return float64(p.W) * s, float64(p.H) * s
}

func (o Options) ActualSizeMM(p Picture) (w, h float64) {
	d := p.density()
	return float64(p.W) / d * mmPerInch, float64(p.H) / d * mmPerInch
}

func ClampCustomMM(w, h float64) (float64, float64) {
	return math.Min(math.Max(w, MinCustomMM), MaxCustomMM), math.Min(math.Max(h, MinCustomMM), MaxCustomMM)
}

func KeepRatio(p Picture, mm float64, fromWidth bool) float64 {
	if p.W < 1 || p.H < 1 {
		return mm
	}
	if fromWidth {
		return mm * float64(p.H) / float64(p.W)
	}
	return mm * float64(p.W) / float64(p.H)
}
