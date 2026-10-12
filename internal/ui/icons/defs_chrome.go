package icons

import "github.com/oops1/PixGo/internal/ui/gfx"

var chromeDefs = map[string]def{
	"back": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Path(1.4, 1.3, 7.85, 13.7, 7.85), gfx.Path(1.4, 6.8, 2.4, 1.3, 7.85, 6.8, 13.3))
	}),
	"info": mono(20, 20, func() gfx.Shape {
		return gfx.Union(gfx.Annulus(9.85, 9.9, 7.2, 1.4), gfx.Box(9.2, 9.5, 10.7, 13.9), gfx.Circle(9.95, 6.85, .85))
	}),
	"heart": mono(20, 20, func() gfx.Shape { return gfx.Path(1.3, heartOutline()...) }),
	"heart-filled": mono(20, 20, func() gfx.Shape {
		p := heartOutline()
		return gfx.Union(gfx.Path(1.4, p...), gfx.Poly(p...))
	}),
	"more-title": mono(16, 16, func() gfx.Shape {
		return gfx.Union(gfx.Circle(7.2, 2.5, 1.2), gfx.Circle(7.2, 8, 1.2), gfx.Circle(7.2, 13.5, 1.2),
			gfx.Circle(12.5, 8, 1.2), gfx.Circle(2.5, 8, .5))
	}),
	"caption-min": mono(12, 12, func() gfx.Shape { return gfx.Box(0, 5.2, 11, 6.3) }),
	"caption-max": mono(14, 14, func() gfx.Shape { return gfx.Frame(1.55, .95, 13.35, 12.5, 1.8, 1.25) }),
	"caption-restore": mono(14, 14, func() gfx.Shape {
		front := gfx.Frame(1.55, 3.7, 10.8, 12.5, 1.6, 1.25)
		back := gfx.Minus(gfx.Frame(3.6, .95, 13.35, 9.9, 1.6, 1.25), gfx.Box(0, 2.45, 12.1, 14))
		return gfx.Union(front, back)
	}),
	"caption-close": mono(14, 14, func() gfx.Shape {
		return gfx.Union(gfx.Path(1.2, 1.7, 1.7, 11.9, 11.9), gfx.Path(1.2, 11.9, 1.7, 1.7, 11.9))
	}),
	"caption-min-sm": mono(12, 12, func() gfx.Shape { return gfx.Box(1.5, 5.2, 10.4, 6.1) }),
	"caption-max-sm": mono(14, 14, func() gfx.Shape { return gfx.Frame(1.9, 1.7, 11.65, 11.3, 1.6, 1.1) }),
	"caption-close-sm": mono(14, 14, func() gfx.Shape {
		return gfx.Union(gfx.Path(1.05, 2.6, 2.6, 9.8, 9.8), gfx.Path(1.05, 9.8, 2.6, 2.6, 9.8))
	}),
	"gallery": mono(20, 20, func() gfx.Shape {
		return gfx.Union(gfx.Frame(.3, 1.5, 18.9, 18.3, 2, 1.55), gfx.Circle(6, 6.3, 1.5),
			gfx.Path(1.5, 2.7, 14.6, 7, 10.5, 10, 13.3, 12.7, 9.5, 17, 13.9))
	}),
	"delete": mono(20, 20, func() gfx.Shape {
		var body []float64
		body = append(body, 4.7, 4)
		body = append(body, arcPts(6.9, 16.4, 2.2, 180, 90, 6)...)
		body = append(body, arcPts(14.5, 16.4, 2.2, 90, 0, 6)...)
		body = append(body, 16.7, 4)
		return gfx.Union(gfx.Capsule(2.9, 3.9, 18.4, 3.9, 1.7), gfx.Frame(7.65, .4, 13.7, 4.6, 1.3, 1.5),
			gfx.Path(1.6, body...), gfx.Capsule(8.65, 7.65, 8.65, 15, 1.6), gfx.Capsule(12.65, 7.65, 12.65, 15, 1.6))
	}),
	"rotate-left": mono(20, 20, func() gfx.Shape {
		p := arcPts(10.4, 10.7, 6.9, 150, -92, 40)
		p = append(p, 5.8, 3.8)
		return gfx.Union(gfx.Path(1.6, p...), gfx.RRect(3, 3.4, 6.5, 8.4, .5), gfx.Box(6.4, 6.9, 7.6, 8.4),
			gfx.Capsule(7.3, 2.6, 10, .2, 1.5))
	}),
	"rotate-right": mono(20, 20, func() gfx.Shape {
		p := arcPts(9.9, 10.7, 6.9, 37, 272, 40)
		p = append(p, 14.4, 3.8)
		return gfx.Union(gfx.Path(1.6, p...), gfx.RRect(14.3, 3.2, 17.6, 8.6, .5), gfx.Box(12.4, 7.1, 14.3, 8.6),
			gfx.Capsule(12.5, 2.8, 9.7, .3, 1.5))
	}),
	"zoom-out": mono(20, 20, func() gfx.Shape { return gfx.Box(2.5, 9.1, 16.6, 10.8) }),
	"zoom-in": mono(20, 20, func() gfx.Shape {
		return gfx.Union(gfx.Box(2.6, 9.1, 17.1, 10.8), gfx.Box(9.1, 2.65, 10.8, 17))
	}),
	"more": mono(16, 8, func() gfx.Shape {
		return gfx.Union(gfx.Circle(2.3, 4.4, .75), gfx.Circle(7.9, 4.4, 1.2), gfx.Circle(13.1, 4.4, .4))
	}),
	"fullscreen": mono(20, 20, func() gfx.Shape {
		return gfx.Union(
			gfx.Path(1.55, 12.2, 2.2, 18.2, 2.2, 18.2, 7.9), gfx.Capsule(18, 2, 11.8, 8.2, 1.5),
			gfx.Path(1.55, 2.5, 11.65, 2.5, 17.7, 8.1, 17.7), gfx.Capsule(3, 17, 8.2, 11.8, 1.5),
		)
	}),
	"fullscreen-exit": mono(20, 20, func() gfx.Shape {
		return gfx.Union(
			gfx.Path(1.55, 11.4, 3, 11.4, 8.6, 17, 8.6), gfx.Capsule(11.7, 8.3, 18, 2, 1.5),
			gfx.Path(1.55, 8.6, 17, 8.6, 11.4, 3, 11.4), gfx.Capsule(8.3, 11.7, 2, 18, 1.5),
		)
	}),
	"chevron-left":     mono(20, 20, func() gfx.Shape { return gfx.Path(1.7, 13.5, 2.1, 5.75, 9.75, 13.7, 17.6) }),
	"chevron-right":    mono(20, 20, func() gfx.Shape { return gfx.Path(1.7, 5.7, 2.1, 13.4, 9.7, 5.4, 17.6) }),
	"chevron-left-sm":  mono(16, 16, func() gfx.Shape { return gfx.Path(1.55, 11.2, .9, 4.5, 7.9, 11.2, 15) }),
	"chevron-right-sm": mono(16, 16, func() gfx.Shape { return gfx.Path(1.55, 4.4, .8, 11, 7.6, 4.4, 14.6) }),
	"close": mono(12, 12, func() gfx.Shape {
		return gfx.Union(gfx.Path(1.2, .9, 1.2, 10.4, 10.7), gfx.Path(1.2, 10.4, 1.2, .9, 10.7))
	}),
	"copy": mono(16, 16, func() gfx.Shape {
		return gfx.Union(
			gfx.Path(1.05, 3.2, 3.3, 2, 3.5, 1.5, 4.2, 1.5, 11.8, 2, 12.6, 3.2, 12.8),
			gfx.Capsule(4.15, 1.4, 4.15, 14.4, 1.05),
			gfx.Path(1.15, 4.4, 1.15, 11.3, 1.15, 11.5, 3.6, 14.2, 3.8, 14.45, 4.4, 14.45, 12.2, 13.9, 12.8,
				12.4, 12.8, 12.4, 14.1, 11.9, 14.6, 4.6, 14.6),
		)
	}),
}

func heartOutline() []float64 { return scaleY(heartPts(9.7, 8.1/1.1, 3.1, 4, 16.9/1.1), 1.1) }
