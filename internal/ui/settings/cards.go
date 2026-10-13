package settings

import (
	"image"
	"image/color"
	"math"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/theme"
)

type picKey struct {
	pic    picture
	w, h   int
	cell   int
	radius float64
	c1, c2 color.RGBA
}

func (p *Page) picture(pic picture, t *theme.Theme) *image.RGBA {
	k := picKey{pic: pic, w: dim.bgPicW, h: dim.bgPicH, radius: dim.bgPicRadius}
	if isTheme(pic) {
		k.w, k.h, k.radius = dim.themePicW, dim.themePicH, dim.themePicRadius
	}
	if pic == picBgChecker {
		k.c1, k.c2, k.cell = t.Checker1, t.Checker2, dim.checkerCell
	}
	if img, ok := p.pics[k]; ok {
		return img
	}
	img := renderPicture(k)
	p.pics[k] = img
	return img
}

func isTheme(pic picture) bool { return pic >= picThemeSystem && pic <= picThemeDark }

func (p *Page) drawCard(c widget.DrawContext, t *theme.Theme, it *item, r image.Rectangle, st state) {
	if isTheme(it.pic) {
		p.drawThemeCard(c, t, it, r, st)
		return
	}
	p.drawBackgroundCard(c, t, it, r, st)
}

func (p *Page) drawThemeCard(c widget.DrawContext, t *theme.Theme, it *item, r image.Rectangle, st state) {
	left := r.Min.X + (r.Dx()-dim.themeCardW)/2
	x, y := float64(left), float64(r.Min.Y)
	ring := frect{x + .5, y + .5, x + .5 + dim.themeRingW, y + .5 + dim.themeRingH}
	col, width := gfx.Blend(t.ChoiceRing, t.Text, .08), dim.themeRingIdle
	if it.on {
		col, width = t.SelectionBorder, dim.themeRingOn
	} else if st.hover {
		col = gfx.Blend(col, t.Text, .2)
		if st.pressed {
			col = gfx.Blend(col, t.Page, .35)
		}
	}
	strokeRound(c, ring.grow(1), dim.themeRingRadius+1, 1, haloRing)
	strokeRound(c, ring, dim.themeRingRadius, width, col)
	picR := image.Rect(left+dim.themePicDX, r.Min.Y+dim.themePicDY, left+dim.themePicDX+dim.themePicW, r.Min.Y+dim.themePicDY+dim.themePicH)
	c.DrawImage(p.picture(it.pic, t), picR.Min.X, picR.Min.Y)
	strokeRound(c, fr(picR), dim.themePicRadius, 1, gfx.Alpha(t.Text, .22))
	w := captionFont.Measure(c, it.text)
	drawText(c, captionFont, it.text, r.Min.X+(r.Dx()-w)/2, r.Min.Y+dim.themeLabelBase, t.Text)
	if st.focus {
		p.ring(c, t, ring, dim.themeRingRadius)
	}
}

func (p *Page) drawBackgroundCard(c widget.DrawContext, t *theme.Theme, it *item, r image.Rectangle, st state) {
	left := r.Min.X + (r.Dx()-dim.bgItemW)/2
	x, y := float64(left), float64(r.Min.Y)
	pic := image.Rect(left+dim.bgPicDX, r.Min.Y+dim.bgPicDY, left+dim.bgPicDX+dim.bgPicW, r.Min.Y+dim.bgPicDY+dim.bgPicH)
	ring := frect{x + bgRingOffsetX, y + bgRingOffsetY, x + bgRingOffsetX + dim.bgItemRingW, y + bgRingOffsetY + dim.bgItemRingH}
	switch {
	case it.on:
		strokeRound(c, ring, dim.bgRingRadius, dim.bgRingWidth, t.SelectionBorder)
	case st.hover:
		alpha := .3
		if st.pressed {
			alpha = .15
		}
		strokeRound(c, fr(pic).grow(3), dim.bgPicRadius+3, 1.5, gfx.Alpha(t.Text, alpha))
	}
	vec := frect{x + dim.bgSolidDX, y + dim.bgSolidDY, x + dim.bgSolidDX + dim.bgSolidW, y + dim.bgSolidDY + dim.bgSolidH}
	switch it.pic {
	case picBgWhite:
		strokeRound(c, vec.grow(1), dim.bgPicRadius+1, 1, shadowRing)
		fillRound(c, vec, dim.bgPicRadius, rgb8(248, 249, 249))
	case picBgBlack:
		strokeRound(c, vec.grow(1), dim.bgPicRadius+1, 1, shadowRing)
		fillRound(c, vec, dim.bgPicRadius, rgb8(0, 0, 0))
		strokeRound(c, vec, dim.bgPicRadius, 1, gfx.Alpha(t.Text, .2))
	default:
		strokeRound(c, fr(pic).grow(1), dim.bgPicRadius+1, 1, shadowRing)
		c.DrawImage(p.picture(it.pic, t), pic.Min.X, pic.Min.Y)
	}
	w := smallCapFont.Measure(c, it.text)
	drawText(c, smallCapFont, it.text, r.Min.X+(r.Dx()-w)/2, r.Min.Y+dim.bgLabelBase, t.Text)
	if st.focus {
		p.ring(c, t, ring, dim.bgRingRadius)
	}
}

type pt struct{ x, y float64 }

var farRidge = []pt{{0, .78}, {.1, .70}, {.2, .60}, {.32, .50}, {.45, .40}, {.5, .38}, {.6, .28}, {.7, .34}, {.8, .30}, {.9, .42}, {1, .50}}

var nearRidge = []pt{{0, .90}, {.15, .74}, {.3, .64}, {.5, .62}, {.7, .57}, {.85, .70}, {1, .66}}

func ridgeY(pts []pt, x float64) float64 {
	for i := 1; i < len(pts); i++ {
		if x <= pts[i].x {
			a, b := pts[i-1], pts[i]
			return a.y + (b.y-a.y)*(x-a.x)/(b.x-a.x)
		}
	}
	return pts[len(pts)-1].y
}

func mixRGB(a, b color.RGBA, f float64) color.RGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f + .5) }
	return color.RGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 255}
}

func rgb8(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }

type palette struct {
	sky0, sky1   color.RGBA
	far0, far1   color.RGBA
	near0, near1 color.RGBA
}

var (
	lightLeft  = palette{rgb8(222, 226, 230), rgb8(242, 242, 244), rgb8(200, 206, 214), rgb8(190, 198, 208), rgb8(196, 203, 211), rgb8(205, 212, 220)}
	lightRight = palette{rgb8(218, 223, 228), rgb8(226, 230, 235), rgb8(205, 211, 219), rgb8(200, 206, 214), rgb8(198, 204, 212), rgb8(186, 192, 202)}
	darkLeft   = palette{rgb8(5, 24, 40), rgb8(14, 34, 56), rgb8(9, 29, 50), rgb8(3, 20, 42), rgb8(1, 13, 28), rgb8(1, 5, 12)}
	darkRight  = palette{rgb8(0, 17, 36), rgb8(1, 10, 24), rgb8(1, 15, 30), rgb8(0, 8, 20), rgb8(0, 8, 19), rgb8(1, 2, 6)}
	dayLeft    = palette{rgb8(101, 144, 186), rgb8(180, 186, 202), rgb8(74, 105, 142), rgb8(27, 61, 94), rgb8(24, 58, 92), rgb8(6, 28, 50)}
	nightRight = palette{rgb8(15, 40, 67), rgb8(19, 46, 74), rgb8(36, 64, 97), rgb8(7, 34, 62), rgb8(4, 24, 46), rgb8(1, 8, 20)}
)

func wallpaper(w, h, split int, left, right palette) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		fy := (float64(y) + .5) / float64(h)
		for x := 0; x < w; x++ {
			pal := left
			if x >= split {
				pal = right
			}
			fx := (float64(x) + .5) / float64(w)
			far, near := ridgeY(farRidge, fx), ridgeY(nearRidge, fx)
			var col color.RGBA
			switch {
			case fy >= near:
				col = mixRGB(pal.near0, pal.near1, (fy-near)/(1-near))
			case fy >= far:
				col = mixRGB(pal.far0, pal.far1, (fy-far)/(near-far))
			default:
				col = mixRGB(pal.sky0, pal.sky1, fy/far)
			}
			img.SetRGBA(x, y, col)
		}
	}
	return img
}

func roundCorners(img *image.RGBA, radius float64) {
	b := img.Bounds()
	dist := gfx.RoundRectDistance(b, radius)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if float64(x-b.Min.X) >= radius && float64(b.Max.X-1-x) >= radius || float64(y-b.Min.Y) >= radius && float64(b.Max.Y-1-y) >= radius {
				continue
			}
			cov := math.Max(0, math.Min(1, .5-dist(float64(x)+.5, float64(y)+.5)))
			o := img.PixOffset(x, y)
			for i := 0; i < 4; i++ {
				img.Pix[o+i] = uint8(float64(img.Pix[o+i])*cov + .5)
			}
		}
	}
}

func solid(w, h int, col color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = col.R, col.G, col.B, 255
	}
	return img
}

func checker(w, h, cell int, c1, c2 color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			col := c1
			if (x/cell+y/cell)%2 == 1 {
				col = c2
			}
			img.SetRGBA(x, y, col)
		}
	}
	return img
}

func systemBackground(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	mx, my := w/2, h/2
	cx, cy := float64(mx), float64(my)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)+.5, float64(y)+.5
			d := math.Hypot(fx-cx, fy-cy)
			var col color.RGBA
			switch {
			case x < mx && y < my:
				col = rgb8(45, 52, 59)
				if fx/cx > fy/cy {
					col = rgb8(66, 74, 84)
				}
			case x < mx:
				col = mixRGB(rgb8(49, 60, 70), rgb8(90, 99, 109), fx/cx)
			case y < my:
				col = rgb8(182, 187, 195)
				if d < cy-2 {
					col = rgb8(126, 139, 155)
				}
			default:
				col = mixRGB(rgb8(205, 209, 215), rgb8(224, 226, 231), (fy-cy)/cy)
				if d < cy-2 {
					col = rgb8(152, 163, 176)
				}
			}
			if d < 2.5 {
				col = rgb8(30, 36, 43)
			}
			img.SetRGBA(x, y, col)
		}
	}
	return img
}

func renderPicture(k picKey) *image.RGBA {
	var img *image.RGBA
	w, h := k.w, k.h
	switch k.pic {
	case picThemeSystem:
		img = wallpaper(w, h, w/2, dayLeft, nightRight)
	case picThemeLight:
		img = wallpaper(w, h, w/2, lightLeft, lightRight)
	case picThemeDark:
		img = wallpaper(w, h, w/2, darkLeft, darkRight)
	case picBgSystem:
		img = systemBackground(w, h)
	case picBgWhite:
		img = solid(w, h, rgb8(248, 249, 249))
	case picBgBlack:
		img = solid(w, h, rgb8(0, 0, 0))
	default:
		img = checker(w, h, k.cell, k.c1, k.c2)
	}
	roundCorners(img, k.radius)
	return img
}
