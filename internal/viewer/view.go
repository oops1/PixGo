package viewer

import (
	"image"
	"math"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/render"
)

type ZoomMode uint8

const (
	ZoomFit ZoomMode = iota
	ZoomFitWidth
	ZoomActual
	ZoomCustom
)

const (
	MinZoom        = 0.05
	MaxZoom        = 64.0
	ZoomStepFactor = 1.25

	zoomEpsilon = 1e-9
	edgeEpsilon = 1e-6
	maxSteps    = 64
)

type Frame struct {
	ImgW, ImgH   int
	Exif         codec.Orientation
	AreaW, AreaH int
}

type View struct {
	Mode             ZoomMode
	Zoom             float64
	CenterU, CenterV float64
	User             codec.Orientation
	Filter           render.Filter
}

func (v *View) Orient(f Frame) codec.Orientation { return f.Exif.Then(v.User) }

func (v *View) OrientedSize(f Frame) (int, int) {
	return v.Orient(f).Size(f.ImgW, f.ImgH)
}

func (v *View) FitZoom(f Frame) float64 {
	if !v.ready(f) {
		return 1
	}
	w, h := v.dims(f)
	return min(float64(f.AreaW)/w, float64(f.AreaH)/h, 1)
}

func (v *View) EffectiveZoom(f Frame) float64 {
	switch v.Mode {
	case ZoomFit:
		return v.FitZoom(f)
	case ZoomFitWidth:
		if !v.ready(f) {
			return 1
		}
		w, _ := v.dims(f)
		return min(float64(f.AreaW)/w, MaxZoom)
	case ZoomActual:
		return 1
	}
	if v.Zoom > 0 && !math.IsInf(v.Zoom, 0) {
		return v.Zoom
	}
	return 1
}

func (v *View) Mapping(f Frame) render.Mapping {
	scale := v.EffectiveZoom(f)
	cu, cv := v.centre(f, scale)
	return render.Mapping{
		Scale:   scale,
		OriginX: cu - float64(f.AreaW)/(2*scale),
		OriginY: cv - float64(f.AreaH)/(2*scale),
		Orient:  v.Orient(f),
		Filter:  v.Filter,
	}
}

func (v *View) Reset(mode ZoomMode) {
	v.Mode = mode
	v.Zoom = 1
	v.CenterU, v.CenterV = 0, 0
	v.User = codec.OrientNormal
}

func (v *View) Clamp(f Frame) {
	if v.Mode == ZoomFit {
		v.CenterU, v.CenterV = 0, 0
		return
	}
	if !v.ready(f) {
		return
	}
	v.CenterU, v.CenterV = v.centre(f, v.EffectiveZoom(f))
}

func (v *View) SetMode(f Frame, mode ZoomMode) {
	scale := v.EffectiveZoom(f)
	cu, cv := v.centre(f, scale)
	switch mode {
	case ZoomFit:
		v.Mode = ZoomFit
		v.CenterU, v.CenterV = 0, 0
		return
	case ZoomFitWidth:
	case ZoomActual:
		v.Zoom = 1
	case ZoomCustom:
		v.Zoom = scale
	default:
		return
	}
	v.Mode = mode
	v.CenterU, v.CenterV = cu, cv
	v.Clamp(f)
}

func (v *View) ZoomAt(f Frame, px, py, factor float64) {
	if !v.ready(f) || !(factor > 0) || math.IsInf(factor, 0) {
		return
	}
	cur := v.EffectiveZoom(f)
	fit := v.FitZoom(f)
	target := snapZoom(cur, cur*factor, [2]float64{1, fit})
	v.zoomTo(f, cur, target, px, py)
}

func (v *View) ZoomTo(f Frame, z, px, py float64) {
	if !v.ready(f) || !(z > 0) || math.IsInf(z, 0) {
		return
	}
	v.zoomTo(f, v.EffectiveZoom(f), z, px, py)
}

func (v *View) StepZoom(f Frame, steps int, px, py float64) {
	factor := ZoomStepFactor
	if steps < 0 {
		factor = 1 / ZoomStepFactor
		steps = -steps
	}
	for i := 0; i < min(steps, maxSteps); i++ {
		v.ZoomAt(f, px, py, factor)
	}
}

func (v *View) ToggleFitActual(f Frame, px, py float64) {
	if !v.ready(f) {
		return
	}
	atFit := v.Mode == ZoomFit ||
		(v.Mode != ZoomActual && sameZoom(v.EffectiveZoom(f), v.FitZoom(f)))
	if !atFit {
		v.SetMode(f, ZoomFit)
		return
	}
	v.keepPointZoom(f, 1, px, py)
	v.Mode = ZoomActual
	v.Zoom = 1
	v.Clamp(f)
}

func (v *View) Pan(f Frame, dx, dy float64) {
	if !v.ready(f) {
		return
	}
	scale := v.EffectiveZoom(f)
	cu, cv := v.centre(f, scale)
	v.CenterU, v.CenterV = cu-dx/scale, cv-dy/scale
	v.Clamp(f)
}

func (v *View) CanPan(f Frame) bool {
	if !v.ready(f) {
		return false
	}
	scale := v.EffectiveZoom(f)
	w, h := v.dims(f)
	return w*scale-float64(f.AreaW) > edgeEpsilon || h*scale-float64(f.AreaH) > edgeEpsilon
}

func (v *View) Rotate(f Frame, cw bool) {
	if cw {
		v.reorient(f, v.User.RotateCW())
		return
	}
	v.reorient(f, v.User.RotateCCW())
}

func (v *View) Rotate180(f Frame) { v.reorient(f, v.User.Rotate180()) }

func (v *View) FlipH(f Frame) { v.reorient(f, v.User.FlipH()) }

func (v *View) FlipV(f Frame) { v.reorient(f, v.User.FlipV()) }

func (v *View) ImageRect(f Frame) image.Rectangle {
	w, h := v.dims(f)
	m := v.Mapping(f)
	x0 := -m.OriginX * m.Scale
	y0 := -m.OriginY * m.Scale
	x1 := (w - m.OriginX) * m.Scale
	y1 := (h - m.OriginY) * m.Scale
	return image.Rect(
		int(math.Floor(x0+edgeEpsilon)),
		int(math.Floor(y0+edgeEpsilon)),
		int(math.Ceil(x1-edgeEpsilon)),
		int(math.Ceil(y1-edgeEpsilon)),
	)
}

func (v *View) AreaToImage(f Frame, px, py float64) (iu, iv float64) {
	m := v.Mapping(f)
	return m.OriginX + px/m.Scale, m.OriginY + py/m.Scale
}

func (v *View) ImageToArea(f Frame, iu, iv float64) (px, py float64) {
	m := v.Mapping(f)
	return (iu - m.OriginX) * m.Scale, (iv - m.OriginY) * m.Scale
}

func (v *View) ZoomPercent(f Frame) int {
	return int(math.Round(v.EffectiveZoom(f) * 100))
}

func (v *View) zoomTo(f Frame, cur, target, px, py float64) {
	lo := min(MinZoom, v.FitZoom(f))
	wanted := target
	target = min(max(target, lo), MaxZoom)
	if sameZoom(target, cur) || (wanted < cur) != (target < cur) {
		return
	}
	v.keepPointZoom(f, target, px, py)
}

func (v *View) keepPointZoom(f Frame, z, px, py float64) {
	iu, iv := v.AreaToImage(f, px, py)
	v.Mode = ZoomCustom
	v.Zoom = z
	v.CenterU = iu - (px-float64(f.AreaW)/2)/z
	v.CenterV = iv - (py-float64(f.AreaH)/2)/z
	if sameZoom(z, v.FitZoom(f)) {
		v.Mode = ZoomFit
	}
	v.Clamp(f)
}

func (v *View) reorient(f Frame, user codec.Orientation) {
	scale := v.EffectiveZoom(f)
	cu, cv := v.centre(f, scale)
	before := v.Orient(f)
	v.User = user
	after := v.Orient(f)
	switch v.Mode {
	case ZoomFit, ZoomFitWidth:
		v.CenterU, v.CenterV = 0, 0
	default:
		sx, sy := before.ToSource(cu, cv, float64(f.ImgW), float64(f.ImgH))
		v.CenterU, v.CenterV = fromSource(after, sx, sy, float64(f.ImgW), float64(f.ImgH))
	}
	v.Clamp(f)
}

func (v *View) ready(f Frame) bool {
	w, h := v.OrientedSize(f)
	return w > 0 && h > 0 && f.AreaW > 0 && f.AreaH > 0
}

func (v *View) dims(f Frame) (w, h float64) {
	iw, ih := v.OrientedSize(f)
	return float64(iw), float64(ih)
}

func (v *View) centre(f Frame, scale float64) (cu, cv float64) {
	w, h := v.dims(f)
	if v.Mode == ZoomFit {
		return w / 2, h / 2
	}
	cu, cv = v.CenterU, v.CenterV
	if (cu == 0 && cv == 0) || !finite(cu) || !finite(cv) {
		return w / 2, h / 2
	}
	if !v.ready(f) {
		return cu, cv
	}
	return clampAxis(cu, w, float64(f.AreaW), scale), clampAxis(cv, h, float64(f.AreaH), scale)
}

func clampAxis(c, size, area, scale float64) float64 {
	if size*scale-area <= edgeEpsilon {
		return size / 2
	}
	half := area / (2 * scale)
	return min(max(c, half), size-half)
}

func snapZoom(cur, target float64, stops [2]float64) float64 {
	best, found := 0.0, false
	switch {
	case target > cur:
		for _, s := range stops {
			if s > cur*(1+zoomEpsilon) && s <= target*(1+zoomEpsilon) && (!found || s < best) {
				best, found = s, true
			}
		}
	case target < cur:
		for _, s := range stops {
			if s < cur*(1-zoomEpsilon) && s >= target*(1-zoomEpsilon) && (!found || s > best) {
				best, found = s, true
			}
		}
	}
	if found {
		return best
	}
	return target
}

func sameZoom(a, b float64) bool {
	return math.Abs(a-b) <= zoomEpsilon*math.Max(math.Abs(a), math.Abs(b))
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func fromSource(o codec.Orientation, x, y, w, h float64) (u, v float64) {
	a, b, c, d, e, f := o.Matrix(w, h)
	dx, dy := x-c, y-f
	return a*dx + d*dy, b*dx + e*dy
}
