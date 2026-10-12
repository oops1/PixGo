package icons

import "github.com/oops1/PixGo/internal/ui/gfx"

func gearShape(cx, cy, rOut, rIn, w, ring float64, teeth int, top, base float64) gfx.Shape {
	return gfx.Union(gfx.Path(w, gearPts(cx, cy, rOut, rIn, teeth, top, base)...), gfx.Annulus(cx, cy, ring, w*.9))
}

var interfaceDefs = map[string]def{
	"arrow-left": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Path(1.15, 8, 3.4, 3.4, 8, 8, 12.5), gfx.Capsule(3.4, 8, 13.6, 8, 1.15))
	}),
	"arrow-right": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Path(1.15, 8.4, 3.4, 13, 8, 8.4, 12.5), gfx.Capsule(13, 8, 2.7, 8, 1.15))
	}),
	"arrow-up": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Path(1.15, transpose([]float64{8, 3.4, 3.4, 8, 8, 12.5})...),
			gfx.Capsule(8, 3.4, 8, 13.6, 1.15))
	}),
	"arrow-right-short": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Path(1.1, 7.8, 3.4, 12.4, 8, 7.8, 12.5), gfx.Capsule(12.4, 8, 4.6, 8, 1))
	}),
	"filter": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Capsule(3.9, 4.1, 12.3, 4.1, 1.2), gfx.Path(1.25, 3.8, 4.5, 7.5, 8.9),
			gfx.Path(1.25, 12.4, 4.5, 8.7, 8.9), gfx.Box(7.1, 8.4, 9, 12))
	}),
	"sort": mono(12, 12, func() gfx.Shape {
		return gfx.Union(gfx.Capsule(3.8, 5.5, 8, 5.5, 1.2), gfx.Capsule(8.3, 5.3, 7.2, 2.8, 1.1),
			gfx.Path(1.2, 6.4, 6.3, 6.1, 8.5, 5, 8.4))
	}),
	"view-details": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Box(2, 3, 12.9, 4.4), gfx.Box(2.8, 7.1, 5, 8.9), gfx.Box(6.2, 7.2, 11.6, 8.7),
			gfx.Box(2.8, 11, 5, 12.7), gfx.Box(6.2, 11.2, 11.6, 12.5))
	}),
	"view-tiles": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Frame(2.85, 2.25, 7.75, 6.95, 1.2, 1.1), gfx.Frame(9.05, 2.25, 13.95, 6.95, 1.2, 1.1),
			gfx.Frame(2.85, 8.25, 7.75, 12.95, 1.2, 1.1), gfx.Frame(9.05, 8.25, 13.95, 12.95, 1.2, 1.1))
	}),
	"view-grid": mono(16, 16, func() gfx.Shape {
		var s []gfx.Shape
		for _, y := range []float64{3.4, 7.1, 11} {
			for _, x := range []float64{3, 7, 11} {
				s = append(s, gfx.RRect(x, y, x+2.4, y+1.7, .5))
			}
		}
		return gfx.Union(s...)
	}),
	"images": multi(16, 16,
		ink(func() gfx.Shape {
			return gfx.Union(gfx.Minus(gfx.Frame(2.25, 2.6, 13.9, 13.9, 1.4, 1.25), gfx.Box(0, 12, 16, 16)),
				gfx.Circle(5.8, 5.7, 1.2), gfx.Path(1.4, 4.8, 10.8, 7.3, 8.5, 9, 10.2, 10.4, 9, 12.5, 10.6))
		}),
		ramp(0xa6d6f4, 0x5a9ae0, 1, func() gfx.Shape { return gfx.RRect(2.5, 11.9, 13.8, 13.9, .4) }),
	),
	"recent": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Annulus(8.15, 8, 5.2, 1.3), gfx.Path(1.2, 7.8, 4.2, 7.8, 8.2, 10.1, 9.8))
	}),
	"heart-small":         mono(16, 16, func() gfx.Shape { return gfx.Path(1.25, heartPts(8, 7.2, 2.05, 3, 12.7)...) }),
	"expander":            mono(8, 8, func() gfx.Shape { return gfx.RRect(3, 3, 5.8, 4.9, .5) }),
	"chevron-right-small": mono(8, 8, func() gfx.Shape { return gfx.Path(1.1, 2.4, 1.4, 5.1, 4, 2.4, 6.6) }),
	"chevron-down":        mono(12, 12, func() gfx.Shape { return gfx.Path(1.4, 2.2, 4, 6.4, 8.2, 10.6, 4) }),
	"chevron-down-sm":     mono(12, 12, func() gfx.Shape { return gfx.Path(1.15, 2.4, 4.1, 5.5, 7.2, 8.6, 4.1) }),
	"settings":            mono(16, 16, func() gfx.Shape { return gearShape(8.1, 7.9, 5.2, 4.4, 1.3, 2.6, 6, .2, .3) }),
	"settings-bar":        mono(20, 20, func() gfx.Shape { return gearShape(10, 10, 7.35, 5.6, 1.3, 2.5, 8, .2, .34) }),
	"view": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Frame(2.6, 3.75, 13.95, 12.45, 1.6, 1.1), gfx.Circle(10.6, 6.9, .7),
			gfx.Path(1, 9.2, 10.7, 10.5, 9.5, 11.8, 10.7))
	}),
	"formats": mono(16, 16, func() gfx.Shape {
		return gfx.Union(
			gfx.Path(1.3, 8.6, 3.6, 5, 3.6, 4, 4.6, 4, 13.1, 5, 14.1, 11.9, 14.1, 12.9, 13.1, 12.9, 6.9, 8.6, 3.6),
			gfx.Path(1.1, 6.2, 10.4, 7.2, 8.4, 8.4, 10.4))
	}),
	"slideshow": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Frame(2.5, 3.6, 14, 12.7, 1.6, 1.4), gfx.Box(7.6, 12.7, 9, 14.7))
	}),
	"performance": mono(16, 16, func() gfx.Shape {
		return gfx.Union(ellipseRing(8.1, 9.9, 5.1, 4.2, 1.4), gfx.Capsule(8, 10, 12.4, 4.9, 1.5), gfx.Circle(8, 10, 1.2))
	}),
	"language": mono(16, 16, func() gfx.Shape {
		return gfx.Union(ellipseRing(8.1, 9.1, 4.75, 4.4, 1.5), ellipseRing(8.1, 9.1, 2, 4.4, 1.2),
			gfx.Poly(7, 12.8, 8.1, 15.6, 9.3, 12.8))
	}),
	"about": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Annulus(8.1, 9.1, 4.8, 1.4), gfx.Circle(8.4, 6.5, .75), gfx.Box(7.8, 8.2, 9, 11.8))
	}),
	"checkmark": mono(12, 12, func() gfx.Shape { return gfx.Path(1.5, 2.9, 6, 5.3, 8.4, 9.9, 3.4) }),
	"pause": mono(20, 20, func() gfx.Shape {
		return gfx.Union(gfx.RRect(5.15, 2.7, 7.4, 17, .4), gfx.RRect(12.35, 2.7, 14.6, 17, .4))
	}),
	"play": mono(20, 20, func() gfx.Shape {
		return gfx.Union(gfx.Poly(6, 3, 15.4, 10, 6, 17), gfx.Path(1.4, 6.3, 3.3, 15, 10, 6.3, 16.7, 6.3, 3.3))
	}),
	"exit-fullscreen": multi(20, 20,
		ink(func() gfx.Shape {
			return gfx.Union(gfx.Frame(2.5, 2.8, 18, 17.4, 1.6, 1.4), gfx.Path(1.2, 17.2, 10.5, 12.7, 10.5, 12.7, 13.8))
		}),
		soft(func() gfx.Shape { return gfx.Circle(8.5, 10, .9) }),
	),
}
