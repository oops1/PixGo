package render

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

var allOrients = []codec.Orientation{
	codec.OrientNormal, codec.OrientFlipH, codec.OrientRotate180, codec.OrientFlipV,
	codec.OrientTranspose, codec.OrientRotate90, codec.OrientTransverse, codec.OrientRotate270,
}

func randomImage(rng *rand.Rand, w, h int, translucent bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = uint8(rng.Intn(256))
		img.Pix[i+1] = uint8(rng.Intn(256))
		img.Pix[i+2] = uint8(rng.Intn(256))
		a := uint8(255)
		if translucent {
			switch rng.Intn(4) {
			case 0:
				a = 0
			case 1:
				a = uint8(rng.Intn(256))
			case 2:
				a = 255
			default:
				a = uint8(1 + rng.Intn(254))
			}
		}
		img.Pix[i+3] = a
	}
	return img
}

func halve(prev *image.NRGBA, rng *rand.Rand, translucent bool) *image.NRGBA {
	b := prev.Bounds()
	nw, nh := (b.Dx()+1)/2, (b.Dy()+1)/2
	img := randomImage(rng, nw, nh, translucent)
	return img
}

func randomSource(rng *rand.Rand, w, h, levels int, translucent bool) Source {
	l0 := randomImage(rng, w, h, translucent)
	lv := []*image.NRGBA{l0}
	for len(lv) < levels {
		last := lv[len(lv)-1]
		if last.Bounds().Dx() <= 1 && last.Bounds().Dy() <= 1 {
			break
		}
		lv = append(lv, halve(last, rng, translucent))
	}
	return Source{Levels: lv, Width: w, Height: h}
}

func refRender(dst *image.RGBA, src Source, m Mapping, bg Background) {
	b := dst.Bounds()
	if math.Abs(m.Scale-1) < 1e-9 {
		m.Scale = 1
		m.OriginX = math.Floor(m.OriginX + 0.5)
		m.OriginY = math.Floor(m.OriginY + 0.5)
	}
	W, H := src.Width, src.Height
	ow, oh := m.Orient.Size(W, H)
	level := 0
	if len(src.Levels) > 0 {
		level = clampInt(int(math.Floor(math.Log2(1/m.Scale))), 0, len(src.Levels)-1)
	}
	nearest := m.Filter == FilterNearest || (m.Filter == FilterAuto && m.Scale >= 4)
	cell := float64(bg.Cell)
	if bg.Cell <= 0 {
		cell = 8
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			lx, ly := x-b.Min.X, y-b.Min.Y
			u := m.OriginX + (float64(lx)+0.5)/m.Scale
			v := m.OriginY + (float64(ly)+0.5)/m.Scale
			var out [3]float64
			if len(src.Levels) == 0 || !(u >= 0 && u < float64(ow) && v >= 0 && v < float64(oh)) {
				out = [3]float64{float64(bg.Canvas.R), float64(bg.Canvas.G), float64(bg.Canvas.B)}
			} else {
				sx, sy := m.Orient.ToSource(u, v, float64(W), float64(H))
				lvl := src.Levels[level]
				scale := float64(int(1) << uint(level))
				fx, fy := sx/scale, sy/scale
				lb := lvl.Bounds()
				fetch := func(ix, iy int) (pr, pg, pb, pa float64) {
					ix = clampInt(ix, 0, lb.Dx()-1)
					iy = clampInt(iy, 0, lb.Dy()-1)
					o := lvl.PixOffset(lb.Min.X+ix, lb.Min.Y+iy)
					a := float64(lvl.Pix[o+3]) / 255
					return float64(lvl.Pix[o]) * a, float64(lvl.Pix[o+1]) * a, float64(lvl.Pix[o+2]) * a, a
				}
				var pr, pg, pb, pa float64
				if nearest {
					pr, pg, pb, pa = fetch(int(math.Floor(fx)), int(math.Floor(fy)))
				} else {
					px, py := fx-0.5, fy-0.5
					x0, y0 := math.Floor(px), math.Floor(py)
					tx, ty := px-x0, py-y0
					r00, g00, b00, a00 := fetch(int(x0), int(y0))
					r01, g01, b01, a01 := fetch(int(x0)+1, int(y0))
					r10, g10, b10, a10 := fetch(int(x0), int(y0)+1)
					r11, g11, b11, a11 := fetch(int(x0)+1, int(y0)+1)
					mix := func(c00, c01, c10, c11 float64) float64 {
						return (c00*(1-tx)+c01*tx)*(1-ty) + (c10*(1-tx)+c11*tx)*ty
					}
					pr, pg, pb, pa = mix(r00, r01, r10, r11), mix(g00, g01, g10, g11), mix(b00, b01, b10, b11), mix(a00, a01, a10, a11)
				}
				under := bg.Fill
				if bg.Kind == BgChecker {
					cx := int64(math.Floor((float64(lx) + 0.5 + m.OriginX*m.Scale) / cell))
					cy := int64(math.Floor((float64(ly) + 0.5 + m.OriginY*m.Scale) / cell))
					under = bg.Checker[(cx+cy)&1]
				}
				out = [3]float64{
					pr + (1-pa)*float64(under.R),
					pg + (1-pa)*float64(under.G),
					pb + (1-pa)*float64(under.B),
				}
			}
			o := dst.PixOffset(x, y)
			for c := 0; c < 3; c++ {
				dst.Pix[o+c] = uint8(math.Max(0, math.Min(255, math.Floor(out[c]+0.5))))
			}
			dst.Pix[o+3] = 255
		}
	}
}

func solidBg() Background {
	return Background{
		Canvas: color.RGBA{30, 40, 50, 255},
		Kind:   BgSolid,
		Fill:   color.RGBA{200, 120, 60, 255},
	}
}

func checkerBg() Background {
	return Background{
		Canvas:  color.RGBA{30, 40, 50, 255},
		Kind:    BgChecker,
		Fill:    color.RGBA{200, 120, 60, 255},
		Checker: [2]color.RGBA{{250, 250, 250, 255}, {40, 90, 180, 255}},
		Cell:    7,
	}
}

type diffStats struct {
	max     int
	count   int
	x, y, c int
}

func compare(a, b *image.RGBA) diffStats {
	var st diffStats
	bb := a.Bounds()
	for y := bb.Min.Y; y < bb.Max.Y; y++ {
		for x := bb.Min.X; x < bb.Max.X; x++ {
			oa, ob := a.PixOffset(x, y), b.PixOffset(x, y)
			for c := 0; c < 4; c++ {
				d := int(a.Pix[oa+c]) - int(b.Pix[ob+c])
				if d < 0 {
					d = -d
				}
				if d > 0 {
					st.count++
				}
				if d > st.max {
					st.max, st.x, st.y, st.c = d, x, y, c
				}
			}
		}
	}
	return st
}

func newDst(w, h, minX, minY int) *image.RGBA {
	return image.NewRGBA(image.Rect(minX, minY, minX+w, minY+h))
}

func fillGarbage(img *image.RGBA, v uint8) {
	for i := range img.Pix {
		img.Pix[i] = v
	}
}

func TestRenderMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	scales := []float64{0.13, 0.27, 0.5, 1, 1.7, 3, 6}
	filters := []Filter{FilterAuto, FilterBilinear, FilterNearest}
	worst := 0
	defer func() { t.Logf("worst channel difference: %d", worst) }()
	for _, translucent := range []bool{false, true} {
		src := randomSource(rng, 83, 57, 5, translucent)
		for _, o := range allOrients {
			for _, sc := range scales {
				for _, f := range filters {
					for _, bgKind := range []string{"solid", "checker"} {
						bg := solidBg()
						if bgKind == "checker" {
							bg = checkerBg()
						}
						ow, oh := o.Size(src.Width, src.Height)
						for trial := 0; trial < 3; trial++ {
							dw, dh := 61, 43
							m := Mapping{Scale: sc, Orient: o, Filter: f}
							switch trial {
							case 0:
								m.OriginX = -rng.Float64()*8 - 0.3
								m.OriginY = -rng.Float64()*8 - 0.3
							case 1:
								m.OriginX = rng.Float64() * math.Max(1, float64(ow)-float64(dw)/sc)
								m.OriginY = rng.Float64() * math.Max(1, float64(oh)-float64(dh)/sc)
							default:
								m.OriginX = float64(ow)/2 - float64(dw)/sc/2 + rng.Float64()
								m.OriginY = float64(oh)/2 - float64(dh)/sc/2 + rng.Float64()
							}
							got := newDst(dw, dh, 5, 3)
							fillGarbage(got, 77)
							want := newDst(dw, dh, 5, 3)
							Render(got, src, m, bg)
							refRender(want, src, m, bg)
							st := compare(got, want)
							worst = max(worst, st.max)
							if st.max > 2 {
								t.Fatalf("translucent=%v orient=%d scale=%v filter=%d bg=%s trial=%d: max diff %d at (%d,%d) channel %d (%d differing bytes)",
									translucent, o, sc, f, bgKind, trial, st.max, st.x, st.y, st.c, st.count)
							}
						}
					}
				}
			}
		}
	}
}

func TestRenderParallelMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for _, translucent := range []bool{false, true} {
		src := randomSource(rng, 301, 197, 4, translucent)
		for _, o := range allOrients {
			for _, sc := range []float64{0.3, 1, 2.3} {
				ow, oh := o.Size(src.Width, src.Height)
				m := Mapping{
					Scale:   sc,
					Orient:  o,
					OriginX: float64(ow)/2 - 150/sc + rng.Float64(),
					OriginY: float64(oh)/2 - 100/sc + rng.Float64(),
				}
				bg := checkerBg()
				got := newDst(300, 200, 0, 0)
				want := newDst(300, 200, 0, 0)
				Render(got, src, m, bg)
				refRender(want, src, m, bg)
				if st := compare(got, want); st.max > 2 {
					t.Fatalf("translucent=%v orient=%d scale=%v: max diff %d at (%d,%d) c=%d", translucent, o, sc, st.max, st.x, st.y, st.c)
				}
			}
		}
	}
}

func TestRenderIntegerOffsetIsExactCopy(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	src := randomSource(rng, 120, 90, 3, false)
	bg := solidBg()
	for _, off := range [][2]float64{{0, 0}, {-10, -7}, {15, 20}, {-200, 3}, {50, -3}} {
		m := Mapping{Scale: 1, OriginX: off[0], OriginY: off[1], Orient: codec.OrientNormal}
		got := newDst(100, 60, 2, 2)
		Render(got, src, m, bg)
		for y := 0; y < 60; y++ {
			for x := 0; x < 100; x++ {
				sx, sy := int(off[0])+x, int(off[1])+y
				o := got.PixOffset(2+x, 2+y)
				var want [4]uint8
				if sx >= 0 && sx < 120 && sy >= 0 && sy < 90 {
					so := src.Levels[0].PixOffset(sx, sy)
					copy(want[:], src.Levels[0].Pix[so:so+4])
				} else {
					want = [4]uint8{bg.Canvas.R, bg.Canvas.G, bg.Canvas.B, 255}
				}
				if [4]uint8(got.Pix[o:o+4]) != want {
					t.Fatalf("offset %v pixel (%d,%d): got %v want %v", off, x, y, got.Pix[o:o+4], want)
				}
			}
		}
	}
}

func TestRenderRectAndShiftEqualFullRender(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	bgs := []Background{solidBg(), checkerBg()}
	for _, translucent := range []bool{false, true} {
		src := randomSource(rng, 211, 143, 4, translucent)
		for _, o := range allOrients {
			for _, sc := range []float64{0.4, 1, 2.5, 5} {
				for _, f := range []Filter{FilterAuto, FilterBilinear} {
					for _, bg := range bgs {
						for _, d := range [][2]int{{13, 0}, {-9, 6}, {0, -11}, {25, 17}, {-3, -2}} {
							w, h := 160, 110
							m := Mapping{Scale: sc, Orient: o, Filter: f, OriginX: 20.3 + rng.Float64()*5, OriginY: 15.7 + rng.Float64()*5}
							dst := newDst(w, h, 3, 4)
							Render(dst, src, m, bg)
							Shift(dst, d[0], d[1])
							m2 := m
							m2.OriginX -= float64(d[0]) / sc
							m2.OriginY -= float64(d[1]) / sc
							b := dst.Bounds()
							for _, strip := range exposedStrips(b, d[0], d[1]) {
								RenderRect(dst, strip, src, m2, bg)
							}
							want := newDst(w, h, 3, 4)
							Render(want, src, m2, bg)
							if st := compare(dst, want); st.max > 2 {
								t.Fatalf("translucent=%v orient=%d scale=%v shift=%v: max diff %d at (%d,%d) c=%d", translucent, o, sc, d, st.max, st.x, st.y, st.c)
							}
						}
					}
				}
			}
		}
	}
}

func exposedStrips(b image.Rectangle, dx, dy int) []image.Rectangle {
	var out []image.Rectangle
	if dx > 0 {
		out = append(out, image.Rect(b.Min.X, b.Min.Y, b.Min.X+dx, b.Max.Y))
	} else if dx < 0 {
		out = append(out, image.Rect(b.Max.X+dx, b.Min.Y, b.Max.X, b.Max.Y))
	}
	if dy > 0 {
		out = append(out, image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+dy))
	} else if dy < 0 {
		out = append(out, image.Rect(b.Min.X, b.Max.Y+dy, b.Max.X, b.Max.Y))
	}
	return out
}

func TestShiftMovesPixels(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	for _, d := range [][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}, {7, 5}, {-7, 5}, {7, -5}, {-7, -5}, {30, 0}, {0, -40}, {50, 50}} {
		img := newDst(30, 20, 4, 9)
		rng.Read(img.Pix)
		orig := image.NewRGBA(img.Rect)
		copy(orig.Pix, img.Pix)
		Shift(img, d[0], d[1])
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				sx, sy := x-d[0], y-d[1]
				if !(image.Pt(sx, sy).In(b)) {
					continue
				}
				if img.RGBAAt(x, y) != orig.RGBAAt(sx, sy) {
					t.Fatalf("shift %v: pixel (%d,%d) wrong", d, x, y)
				}
			}
		}
	}
}

func TestRenderEdgeCases(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	bg := checkerBg()
	canvas := color.RGBA{bg.Canvas.R, bg.Canvas.G, bg.Canvas.B, 255}

	t.Run("empty source", func(t *testing.T) {
		dst := newDst(40, 30, 1, 1)
		fillGarbage(dst, 9)
		Render(dst, Source{}, Mapping{Scale: 1}, bg)
		for y := 1; y < 31; y++ {
			for x := 1; x < 41; x++ {
				if dst.RGBAAt(x, y) != canvas {
					t.Fatalf("pixel (%d,%d) = %v", x, y, dst.RGBAAt(x, y))
				}
			}
		}
	})

	t.Run("invalid scale", func(t *testing.T) {
		src := randomSource(rng, 10, 10, 1, false)
		for _, sc := range []float64{0, -1, math.NaN(), math.Inf(1)} {
			dst := newDst(20, 20, 0, 0)
			fillGarbage(dst, 9)
			Render(dst, src, Mapping{Scale: sc}, bg)
			if dst.RGBAAt(7, 7) != canvas {
				t.Fatalf("scale %v: not canvas", sc)
			}
		}
	})

	t.Run("source smaller than dst", func(t *testing.T) {
		src := randomSource(rng, 8, 6, 1, false)
		m := Mapping{Scale: 1, OriginX: -5, OriginY: -4}
		got, want := newDst(40, 30, 0, 0), newDst(40, 30, 0, 0)
		Render(got, src, m, bg)
		refRender(want, src, m, bg)
		if st := compare(got, want); st.max > 2 {
			t.Fatalf("max diff %d", st.max)
		}
		if got.RGBAAt(0, 0) != canvas || got.RGBAAt(39, 29) != canvas {
			t.Fatal("corners must be canvas")
		}
	})

	t.Run("fully outside", func(t *testing.T) {
		src := randomSource(rng, 50, 50, 3, true)
		for _, o := range []Mapping{
			{Scale: 1, OriginX: 500, OriginY: 0},
			{Scale: 1, OriginX: 0, OriginY: -500},
			{Scale: 2, OriginX: -300, OriginY: -300},
			{Scale: 1, OriginX: 0, OriginY: 500, Orient: codec.OrientRotate90},
		} {
			dst := newDst(30, 30, 0, 0)
			fillGarbage(dst, 9)
			Render(dst, src, o, bg)
			for y := 0; y < 30; y++ {
				for x := 0; x < 30; x++ {
					if dst.RGBAAt(x, y) != canvas {
						t.Fatalf("mapping %+v: pixel (%d,%d) = %v", o, x, y, dst.RGBAAt(x, y))
					}
				}
			}
		}
	})

	t.Run("tiny scale", func(t *testing.T) {
		for _, translucent := range []bool{false, true} {
			src := randomSource(rng, 400, 300, 3, translucent)
			for _, o := range allOrients {
				const sc = 0.0004
				m := Mapping{Scale: sc, Orient: o, OriginX: -10.4 / sc, OriginY: -7.45 / sc}
				got, want := newDst(20, 20, 0, 0), newDst(20, 20, 0, 0)
				Render(got, src, m, bg)
				refRender(want, src, m, bg)
				if st := compare(got, want); st.max > 2 {
					t.Fatalf("orient %d: max diff %d", o, st.max)
				}
				inside := 0
				for y := 0; y < 20; y++ {
					for x := 0; x < 20; x++ {
						if got.RGBAAt(x, y) != canvas {
							inside++
						}
					}
				}
				if inside != 1 {
					t.Fatalf("orient %d: expected exactly one image pixel, got %d", o, inside)
				}
			}
		}
	})

	t.Run("single level deep downscale", func(t *testing.T) {
		src := randomSource(rng, 500, 400, 1, false)
		m := Mapping{Scale: 0.05, OriginX: -10, OriginY: -10}
		got, want := newDst(40, 30, 0, 0), newDst(40, 30, 0, 0)
		Render(got, src, m, bg)
		refRender(want, src, m, bg)
		if st := compare(got, want); st.max > 2 {
			t.Fatalf("max diff %d", st.max)
		}
	})

	t.Run("rect outside dst", func(t *testing.T) {
		src := randomSource(rng, 50, 50, 1, false)
		dst := newDst(20, 20, 0, 0)
		fillGarbage(dst, 9)
		RenderRect(dst, image.Rect(100, 100, 120, 120), src, Mapping{Scale: 1}, bg)
		RenderRect(dst, image.Rect(5, 5, 5, 20), src, Mapping{Scale: 1}, bg)
		for _, v := range dst.Pix {
			if v != 9 {
				t.Fatal("dst must stay untouched")
			}
		}
	})

	t.Run("rect only touches itself", func(t *testing.T) {
		src := randomSource(rng, 50, 50, 1, true)
		dst := newDst(40, 40, 3, 3)
		fillGarbage(dst, 9)
		r := image.Rect(10, 12, 25, 30)
		RenderRect(dst, r.Add(image.Pt(-5, -5)), src, Mapping{Scale: 1.5}, bg)
		r = r.Add(image.Pt(-5, -5)).Intersect(dst.Bounds())
		for y := 3; y < 43; y++ {
			for x := 3; x < 43; x++ {
				c := dst.RGBAAt(x, y)
				inRect := image.Pt(x, y).In(r)
				if !inRect && c != (color.RGBA{9, 9, 9, 9}) {
					t.Fatalf("pixel (%d,%d) outside rect changed", x, y)
				}
				if inRect && c.A != 255 {
					t.Fatalf("pixel (%d,%d) inside rect not written", x, y)
				}
			}
		}
	})
}

func TestOutputAlwaysOpaque(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	src := randomSource(rng, 64, 64, 3, true)
	for _, o := range allOrients {
		dst := newDst(50, 50, 0, 0)
		fillGarbage(dst, 1)
		Render(dst, src, Mapping{Scale: 0.9, Orient: o, OriginX: -7.5, OriginY: 3.25}, checkerBg())
		for i := 3; i < len(dst.Pix); i += 4 {
			if dst.Pix[i] != 255 {
				t.Fatalf("orient %d: alpha %d at byte %d", o, dst.Pix[i], i)
			}
		}
	}
}

func TestTransparentPixelsDoNotBleed(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	copy(img.Pix, []uint8{255, 0, 0, 0, 0, 0, 255, 255})
	src := Source{Levels: []*image.NRGBA{img}, Width: 2, Height: 1}
	bg := solidBg()
	bg.Fill = color.RGBA{0, 0, 0, 255}
	dst := newDst(4, 1, 0, 0)
	Render(dst, src, Mapping{Scale: 2, Filter: FilterBilinear}, bg)
	for x := 0; x < 4; x++ {
		c := dst.RGBAAt(x, 0)
		if c.R != 0 || c.G != 0 {
			t.Fatalf("pixel %d bled colour from transparent neighbour: %v", x, c)
		}
	}
	if c := dst.RGBAAt(0, 0); c.B != 0 {
		t.Fatalf("left edge should be background: %v", c)
	}
	if c := dst.RGBAAt(3, 0); c.B != 255 {
		t.Fatalf("right edge should be blue: %v", c)
	}
}

func TestCheckerMovesWithImage(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	src := Source{Levels: []*image.NRGBA{img}, Width: 40, Height: 40}
	bg := checkerBg()
	bg.Cell = 4
	a, b := newDst(60, 60, 0, 0), newDst(60, 60, 0, 0)
	Render(a, src, Mapping{Scale: 1, OriginX: -6, OriginY: -6}, bg)
	Render(b, src, Mapping{Scale: 1, OriginX: -10, OriginY: -9}, bg)
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			if a.RGBAAt(6+x, 6+y) != b.RGBAAt(10+x, 9+y) {
				t.Fatalf("checker does not follow the image at (%d,%d)", x, y)
			}
		}
	}
	if a.RGBAAt(6, 6) != bg.Checker[0] || a.RGBAAt(10, 6) != bg.Checker[1] || a.RGBAAt(6, 10) != bg.Checker[1] || a.RGBAAt(10, 10) != bg.Checker[0] {
		t.Fatal("checker phase must start at the image corner")
	}
}

func TestIsOpaqueDetectsSingleTranslucentPixel(t *testing.T) {
	for _, w := range []int{1, 2, 3, 7, 8, 9, 63, 64, 1001} {
		for _, h := range []int{1, 5, 300} {
			for _, at := range []int{0, 1, w*h/2 + 1, w*h - 1} {
				if at >= w*h {
					continue
				}
				img := image.NewNRGBA(image.Rect(0, 0, w, h))
				for i := 3; i < len(img.Pix); i += 4 {
					img.Pix[i] = 255
				}
				if !IsOpaque(img) {
					t.Fatalf("%dx%d: opaque image reported translucent", w, h)
				}
				img = image.NewNRGBA(image.Rect(0, 0, w, h))
				for i := 3; i < len(img.Pix); i += 4 {
					img.Pix[i] = 255
				}
				img.Pix[at*4+3] = 254
				if IsOpaque(img) {
					t.Fatalf("%dx%d: translucent pixel %d missed", w, h, at)
				}
			}
		}
	}
}

func TestOpacityMarkAndForget(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	MarkOpaque(img)
	if !IsOpaque(img) {
		t.Fatal("marked image must report opaque")
	}
	ForgetOpacity(img)
	if IsOpaque(img) {
		t.Fatal("forgotten image must be rescanned and found translucent")
	}
	if IsOpaque(img) {
		t.Fatal("verdict must be stable")
	}
	if !IsOpaque(nil) || !IsOpaque(image.NewNRGBA(image.Rectangle{})) {
		t.Fatal("empty images count as opaque")
	}
}

func TestRenderIntoSubImageLeavesNeighboursAlone(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	src := randomSource(rng, 90, 70, 3, true)
	whole := newDst(100, 80, 0, 0)
	fillGarbage(whole, 42)
	sub := whole.SubImage(image.Rect(11, 9, 71, 59)).(*image.RGBA)
	m := Mapping{Scale: 0.8, Orient: codec.OrientRotate270, OriginX: 3.2, OriginY: -4.1}
	Render(sub, src, m, checkerBg())
	want := newDst(60, 50, 11, 9)
	refRender(want, src, m, checkerBg())
	if st := compare(sub, want); st.max > 2 {
		t.Fatalf("sub-image render differs: %d", st.max)
	}
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			if image.Pt(x, y).In(sub.Bounds()) {
				continue
			}
			if whole.RGBAAt(x, y) != (color.RGBA{42, 42, 42, 42}) {
				t.Fatalf("pixel (%d,%d) outside the sub-image was modified", x, y)
			}
		}
	}
}

func TestRenderFromSubImageLevel(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	big := randomImage(rng, 70, 60, true)
	sub := big.SubImage(image.Rect(7, 5, 57, 45)).(*image.NRGBA)
	flat := image.NewNRGBA(image.Rect(0, 0, 50, 40))
	for y := 0; y < 40; y++ {
		copy(flat.Pix[y*flat.Stride:y*flat.Stride+200], sub.Pix[y*sub.Stride:y*sub.Stride+200])
	}
	for _, o := range allOrients {
		m := Mapping{Scale: 1.3, Orient: o, OriginX: -2.5, OriginY: -1.5}
		got, want := newDst(60, 50, 0, 0), newDst(60, 50, 0, 0)
		Render(got, Source{Levels: []*image.NRGBA{sub}, Width: 50, Height: 40}, m, checkerBg())
		Render(want, Source{Levels: []*image.NRGBA{flat}, Width: 50, Height: 40}, m, checkerBg())
		if st := compare(got, want); st.max != 0 {
			t.Fatalf("orient %d: sub-image level renders differently (%d)", o, st.max)
		}
	}
}

func TestUnitScaleSnapsOriginToWholePixel(t *testing.T) {
	rng := rand.New(rand.NewSource(10))
	src := randomSource(rng, 120, 90, 3, false)
	bg := solidBg()
	for _, sc := range []float64{1, 1 + 1e-12, 1 - 1e-10} {
		for _, off := range [][2]float64{{10.5, 7.5}, {-3.5, 2.5}, {10.49, 7.51}, {0.3, -0.3}} {
			m := Mapping{Scale: sc, OriginX: off[0], OriginY: off[1]}
			got := newDst(64, 48, 1, 1)
			Render(got, src, m, bg)
			ox, oy := int(math.Floor(off[0]+0.5)), int(math.Floor(off[1]+0.5))
			for y := 0; y < 48; y++ {
				for x := 0; x < 64; x++ {
					sx, sy := ox+x, oy+y
					o := got.PixOffset(1+x, 1+y)
					var want [4]uint8
					if sx >= 0 && sx < 120 && sy >= 0 && sy < 90 {
						so := src.Levels[0].PixOffset(sx, sy)
						copy(want[:], src.Levels[0].Pix[so:so+4])
					} else {
						want = [4]uint8{bg.Canvas.R, bg.Canvas.G, bg.Canvas.B, 255}
					}
					if [4]uint8(got.Pix[o:o+4]) != want {
						t.Fatalf("scale %v origin %v pixel (%d,%d): got %v want %v", sc, off, x, y, got.Pix[o:o+4], want)
					}
				}
			}
		}
	}
}

func TestUnitScaleSnapAppliesToEveryOrientation(t *testing.T) {
	rng := rand.New(rand.NewSource(12))
	src := randomSource(rng, 70, 50, 1, true)
	for _, o := range allOrients {
		half := Mapping{Scale: 1, Orient: o, OriginX: 4.5, OriginY: 2.5}
		whole := Mapping{Scale: 1, Orient: o, OriginX: 5, OriginY: 3}
		a, b := newDst(40, 30, 0, 0), newDst(40, 30, 0, 0)
		Render(a, src, half, checkerBg())
		Render(b, src, whole, checkerBg())
		if st := compare(a, b); st.max != 0 {
			t.Fatalf("orient %d: half-pixel origin at 100%% must match the snapped origin, diff %d", o, st.max)
		}
	}
}

func TestUnitScaleSnapIsStableUnderIntegerShifts(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	src := randomSource(rng, 200, 150, 1, false)
	bg := checkerBg()
	m := Mapping{Scale: 1, OriginX: 30.5, OriginY: 20.5}
	dst := newDst(80, 60, 0, 0)
	Render(dst, src, m, bg)
	Shift(dst, -7, 5)
	m2 := m
	m2.OriginX += 7
	m2.OriginY -= 5
	for _, strip := range exposedStrips(dst.Bounds(), -7, 5) {
		RenderRect(dst, strip, src, m2, bg)
	}
	want := newDst(80, 60, 0, 0)
	Render(want, src, m2, bg)
	if st := compare(dst, want); st.max != 0 {
		t.Fatalf("shift at 100%% with a half-pixel origin diverged: %d", st.max)
	}
}
