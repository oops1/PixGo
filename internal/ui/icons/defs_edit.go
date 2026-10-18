package icons

import (
	"github.com/oops1/PixGo/internal/ui/gfx"
)

func init() {
	for k, v := range editDefs {
		defs[k] = v
	}
}

func closed(w float64, pts ...float64) gfx.Shape {
	pts = append(pts, pts[0], pts[1])
	return gfx.Path(w, pts...)
}

var editDefs = map[string]def{
	"edit": mono(20, 20, func() gfx.Shape {
		return gfx.Union(
			closed(1.4, 3.2, 16.8, 4.1, 12.7, 13.2, 3.6, 16.4, 6.8),
			gfx.Capsule(4.3, 12.4, 7.5, 15.6, 1.3),
			gfx.Capsule(11.2, 5.6, 14.4, 8.8, 1.3),
		)
	}),
	"crop": mono(20, 20, func() gfx.Shape {
		return gfx.Union(
			gfx.Path(1.5, 5.2, 1.6, 5.2, 14.8, 18.4, 14.8),
			gfx.Path(1.5, 1.6, 5.2, 14.8, 5.2, 14.8, 18.4),
		)
	}),
	"flip-h": mono(20, 20, func() gfx.Shape {
		return gfx.Union(
			gfx.Capsule(10, 1.8, 10, 18.2, 1.3),
			closed(1.3, 2.6, 16.4, 7.6, 16.4, 7.6, 5),
			closed(1.3, 12.4, 16.4, 17.4, 16.4, 12.4, 5),
		)
	}),
	"flip-v": mono(20, 20, func() gfx.Shape {
		return gfx.Union(
			gfx.Capsule(1.8, 10, 18.2, 10, 1.3),
			closed(1.3, 16.4, 2.6, 16.4, 7.6, 5, 7.6),
			closed(1.3, 16.4, 12.4, 16.4, 17.4, 5, 12.4),
		)
	}),
	"resize": mono(20, 20, func() gfx.Shape {
		return gfx.Union(
			gfx.Frame(2.2, 2.2, 17.8, 17.8, 2.2, 1.4),
			gfx.Capsule(7.2, 12.8, 12.8, 7.2, 1.4),
			gfx.Path(1.4, 9.4, 7.2, 12.8, 7.2, 12.8, 10.6),
			gfx.Path(1.4, 10.6, 12.8, 7.2, 12.8, 7.2, 9.4),
		)
	}),
	"reset": mono(20, 20, func() gfx.Shape {
		p := arcPts(10.6, 11.2, 6.2, 35, -158, 36)
		end := p[len(p)-2:]
		return gfx.Union(
			gfx.Path(1.5, p...),
			gfx.Path(1.5, end[0]-.2, end[1]-3.6, end[0], end[1], end[0]+3.7, end[1]+.4),
		)
	}),
	"aspect-swap": mono(20, 20, func() gfx.Shape {
		return gfx.Union(
			gfx.Frame(1.4, 6.2, 12, 13.8, 1.6, 1.3),
			gfx.Frame(12.8, 3, 18.6, 17, 1.6, 1.3),
		)
	}),
}
