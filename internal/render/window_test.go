package render

import (
	"image"
	"math"
	"math/rand"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

type cutDetail struct {
	levels   []*image.NRGBA
	margin   int
	wrongLvl bool
	refuse   bool
	refuseTo int
	calls    []image.Rectangle
	shifts   []int
	notified func()
}

func (d *cutDetail) SetOnChange(fn func()) { d.notified = fn }

func (d *cutDetail) Window(shift int, need image.Rectangle) Window {
	d.calls = append(d.calls, need)
	d.shifts = append(d.shifts, shift)
	if d.refuse || shift >= len(d.levels) || shift < d.refuseTo {
		return Window{}
	}
	lvl := d.levels[shift]
	r := image.Rect(need.Min.X-d.margin, need.Min.Y-d.margin, need.Max.X+d.margin, need.Max.Y+d.margin).Intersect(lvl.Rect)
	if r.Empty() {
		return Window{}
	}
	cut := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		so := lvl.PixOffset(r.Min.X, r.Min.Y+y)
		copy(cut.Pix[y*cut.Stride:y*cut.Stride+r.Dx()*4], lvl.Pix[so:so+r.Dx()*4])
	}
	sh := shift
	if d.wrongLvl {
		sh++
	}
	return Window{Img: cut, Shift: sh, X: r.Min.X, Y: r.Min.Y}
}

type fixedDetail struct{ win Window }

func (d fixedDetail) SetOnChange(func())                 {}
func (d fixedDetail) Window(int, image.Rectangle) Window { return d.win }

func windowedSource(full Source, shift int, d Detail) Source {
	return Source{Levels: full.Levels[shift:], Width: full.Width, Height: full.Height, Shift: shift, Detail: d}
}

func TestWindowRenderEqualsFullRender(t *testing.T) {
	rng := rand.New(rand.NewSource(21))
	worst := 0
	for _, translucent := range []bool{false, true} {
		full := randomSource(rng, 317, 211, 5, translucent)
		for _, o := range allOrients {
			ow, oh := o.Size(full.Width, full.Height)
			for _, sc := range []float64{0.11, 0.26, 0.5, 0.74, 1, 1.9, 3.3, 6} {
				for _, f := range []Filter{FilterAuto, FilterBilinear, FilterNearest} {
					for _, bg := range []Background{solidBg(), checkerBg()} {
						for trial := 0; trial < 2; trial++ {
							dw, dh := 90, 64
							m := Mapping{Scale: sc, Orient: o, Filter: f}
							m.OriginX = rng.Float64()*(float64(ow)-float64(dw)/sc) - 3
							m.OriginY = rng.Float64()*(float64(oh)-float64(dh)/sc) - 3
							det := &cutDetail{levels: full.Levels, margin: rng.Intn(3) * 7}
							win := windowedSource(full, 3, det)
							got := newDst(dw, dh, 4, 2)
							want := newDst(dw, dh, 4, 2)
							fillGarbage(got, 9)
							Render(got, win, m, bg)
							Render(want, full, m, bg)
							if wanted := LevelShift(sc); wanted < 3 && len(det.calls) != 1 {
								t.Fatalf("scale %v wants shift %d: %d window requests", sc, wanted, len(det.calls))
							}
							st := compare(got, want)
							worst = max(worst, st.max)
							if wanted := LevelShift(sc); wanted < 3 {
								if st.max != 0 {
									t.Fatalf("translucent=%v orient=%d scale=%v filter=%d trial=%d: window render differs by %d at (%d,%d) c=%d (%d bytes)",
										translucent, o, sc, f, trial, st.max, st.x, st.y, st.c, st.count)
								}
							} else if st.max != 0 {
								t.Fatalf("coarse levels must render identically, orient=%d scale=%v differs by %d", o, sc, st.max)
							}
						}
					}
				}
			}
		}
	}
	if worst != 0 {
		t.Fatalf("worst difference %d", worst)
	}
}

func TestWindowRenderWithRenderRectStrips(t *testing.T) {
	rng := rand.New(rand.NewSource(22))
	full := randomSource(rng, 400, 300, 4, false)
	for _, o := range allOrients {
		ow, oh := o.Size(full.Width, full.Height)
		for _, sc := range []float64{0.3, 0.9, 2.5} {
			m := Mapping{Scale: sc, Orient: o, Filter: FilterBilinear, OriginX: float64(ow) / 3, OriginY: float64(oh) / 4}
			det := &cutDetail{levels: full.Levels}
			win := windowedSource(full, 3, det)
			got := newDst(120, 80, 0, 0)
			want := newDst(120, 80, 0, 0)
			bg := solidBg()
			Render(want, full, m, bg)
			for _, strip := range []image.Rectangle{image.Rect(0, 0, 120, 10), image.Rect(0, 10, 25, 80), image.Rect(25, 10, 120, 80)} {
				RenderRect(got, strip, win, m, bg)
			}
			if st := compare(got, want); st.max != 0 {
				t.Fatalf("orient=%d scale=%v: strip render differs by %d at (%d,%d)", o, sc, st.max, st.x, st.y)
			}
			for _, need := range det.calls {
				if need.Empty() {
					t.Fatal("empty window request")
				}
			}
		}
	}
}

func TestWindowRequestMatchesVisibleArea(t *testing.T) {
	full := randomSource(rand.New(rand.NewSource(23)), 640, 480, 5, false)
	for _, o := range allOrients {
		ow, oh := o.Size(full.Width, full.Height)
		m := Mapping{Scale: 0.9, Orient: o, Filter: FilterBilinear, OriginX: float64(ow)/2 - 50, OriginY: float64(oh)/2 - 40}
		det := &cutDetail{levels: full.Levels}
		src := windowedSource(full, 3, det)
		dst := newDst(100, 80, 0, 0)
		Render(dst, src, m, solidBg())
		if len(det.calls) != 1 || det.shifts[0] != 0 {
			t.Fatalf("orient=%d: requests %v shifts %v", o, det.calls, det.shifts)
		}
		vis := VisibleRect(m, full.Width, full.Height, 100, 80)
		if vis.Empty() {
			t.Fatalf("orient=%d: empty visible rect", o)
		}
		need := det.calls[0]
		if !vis.In(need) {
			t.Fatalf("orient=%d: request %v does not contain visible %v", o, need, vis)
		}
		if need.Dx() > vis.Dx()+2 || need.Dy() > vis.Dy()+2 {
			t.Fatalf("orient=%d: request %v is much larger than visible %v", o, need, vis)
		}
	}
}

func TestWindowFallsBackToCoarseLevel(t *testing.T) {
	rng := rand.New(rand.NewSource(24))
	full := randomSource(rng, 300, 200, 5, false)
	m := Mapping{Scale: 1.5, Orient: codec.OrientRotate90, Filter: FilterBilinear, OriginX: 20, OriginY: 30}
	plain := windowedSource(full, 3, nil)
	want := newDst(80, 60, 0, 0)
	Render(want, plain, m, solidBg())
	cases := map[string]*cutDetail{
		"refused":       {levels: full.Levels, refuse: true},
		"wrong shift":   {levels: full.Levels, wrongLvl: true},
		"not covering":  {levels: full.Levels, margin: -9},
		"outside level": {levels: full.Levels[:0]},
	}
	for name, det := range cases {
		got := newDst(80, 60, 0, 0)
		Render(got, windowedSource(full, 3, det), m, solidBg())
		if st := compare(got, want); st.max != 0 {
			t.Fatalf("%s: fallback differs from the coarse render by %d", name, st.max)
		}
		if len(det.calls) != 3 {
			t.Fatalf("%s: %d requests, want one per level from the wanted one down to the held one", name, len(det.calls))
		}
	}
}

func TestRefusedFinestWindowFallsBackToTheNextCoarserWindow(t *testing.T) {
	rng := rand.New(rand.NewSource(27))
	full := randomSource(rng, 300, 200, 5, false)
	for _, o := range allOrients {
		m := Mapping{Scale: 1.5, Orient: o, Filter: FilterBilinear, OriginX: 12, OriginY: 9}
		det := &cutDetail{levels: full.Levels, refuseTo: 1}
		got := newDst(80, 60, 0, 0)
		Render(got, windowedSource(full, 3, det), m, solidBg())
		want := newDst(80, 60, 0, 0)
		Render(want, windowedSource(full, 1, nil), m, solidBg())
		if st := compare(got, want); st.max != 0 {
			t.Fatalf("orient=%d: the level-1 window differs from rendering the level-1 picture by %d", o, st.max)
		}
		if len(det.calls) != 2 || det.shifts[0] != 0 || det.shifts[1] != 1 {
			t.Fatalf("orient=%d: requests %v shifts %v, want shift 0 then shift 1", o, det.calls, det.shifts)
		}
		coarse := newDst(80, 60, 0, 0)
		Render(coarse, windowedSource(full, 3, nil), m, solidBg())
		if st := compare(got, coarse); st.max == 0 {
			t.Fatalf("orient=%d: a level-1 window must look different from the level-3 fallback", o)
		}
	}
}

func TestWindowNotRequestedForCoarseScale(t *testing.T) {
	full := randomSource(rand.New(rand.NewSource(25)), 300, 200, 5, false)
	det := &cutDetail{levels: full.Levels}
	src := windowedSource(full, 2, det)
	for _, sc := range []float64{0.25, 0.2, 0.1, 0.02} {
		dst := newDst(40, 30, 0, 0)
		Render(dst, src, Mapping{Scale: sc, OriginX: 0, OriginY: 0}, solidBg())
	}
	if len(det.calls) != 0 {
		t.Fatalf("scales that fit the held level must not ask for windows: %v", det.calls)
	}
	Render(newDst(40, 30, 0, 0), src, Mapping{Scale: 0.26}, solidBg())
	if len(det.calls) != 1 || det.shifts[0] != 1 {
		t.Fatalf("scale 0.26 wants shift 1: %v %v", det.calls, det.shifts)
	}
}

func TestSparseSourceWithoutDetailMatchesFullAtCoarseScales(t *testing.T) {
	rng := rand.New(rand.NewSource(26))
	full := randomSource(rng, 333, 222, 5, true)
	for _, shift := range []int{1, 2, 3} {
		src := Source{Levels: full.Levels[shift:], Width: full.Width, Height: full.Height, Shift: shift}
		for _, o := range allOrients {
			ow, oh := o.Size(full.Width, full.Height)
			for _, sc := range []float64{0.5 / float64(int(1)<<uint(shift)), 0.06, 0.03} {
				m := Mapping{Scale: sc, Orient: o, OriginX: -3, OriginY: float64(oh) / 5}
				_ = ow
				got := newDst(60, 50, 1, 1)
				want := newDst(60, 50, 1, 1)
				Render(got, src, m, checkerBg())
				Render(want, full, m, checkerBg())
				if st := compare(got, want); st.max != 0 {
					t.Fatalf("shift=%d orient=%d scale=%v differs by %d", shift, o, sc, st.max)
				}
			}
		}
	}
}

func TestLevelShiftMatchesLegacyLoop(t *testing.T) {
	legacy := func(scale float64, levels int) int {
		level := 0
		for level+1 < levels && scale*float64(uint(2)<<uint(level)) <= 1 {
			level++
		}
		return level
	}
	rng := rand.New(rand.NewSource(27))
	scales := []float64{1, 0.5, 0.25, 0.125, 0.2500000001, 0.2499999999, 2, 17, 1e-9, 0.999999}
	for i := 0; i < 2000; i++ {
		scales = append(scales, math.Exp(rng.Float64()*14-12))
	}
	for _, sc := range scales {
		for _, levels := range []int{1, 3, 8, 64} {
			want := legacy(sc, levels)
			if got := min(LevelShift(sc), levels-1); got != want {
				t.Fatalf("scale %v levels %d: LevelShift %d, legacy %d", sc, levels, got, want)
			}
		}
	}
	if LevelShift(0) != 0 || LevelShift(math.NaN()) != 0 || LevelShift(-1) != 0 || LevelShift(math.Inf(1)) != 0 {
		t.Fatalf("degenerate scales: %d %d %d %d", LevelShift(0), LevelShift(math.NaN()), LevelShift(-1), LevelShift(math.Inf(1)))
	}
}

func TestVisibleRectBoundsEveryDrawnPixel(t *testing.T) {
	rng := rand.New(rand.NewSource(28))
	const w, h = 701, 433
	for _, o := range allOrients {
		ow, oh := o.Size(w, h)
		for trial := 0; trial < 60; trial++ {
			sc := math.Exp(rng.Float64()*6 - 4)
			dw, dh := 1+rng.Intn(150), 1+rng.Intn(120)
			m := Mapping{
				Scale:   sc,
				Orient:  o,
				OriginX: rng.Float64()*float64(ow+200) - 100,
				OriginY: rng.Float64()*float64(oh+200) - 100,
			}
			vis := VisibleRect(m, w, h, dw, dh)
			mm := normalizeMapping(m)
			minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
			for y := 0; y < dh; y++ {
				for x := 0; x < dw; x++ {
					u := mm.OriginX + (float64(x)+0.5)/mm.Scale
					v := mm.OriginY + (float64(y)+0.5)/mm.Scale
					if !(u >= 0 && u < float64(ow) && v >= 0 && v < float64(oh)) {
						continue
					}
					sx, sy := o.ToSource(u, v, w, h)
					minX, maxX = math.Min(minX, sx), math.Max(maxX, sx)
					minY, maxY = math.Min(minY, sy), math.Max(maxY, sy)
				}
			}
			if math.IsInf(minX, 1) {
				if !vis.Empty() {
					t.Fatalf("orient=%d trial=%d: nothing drawn but visible rect %v", o, trial, vis)
				}
				continue
			}
			if vis.Empty() {
				t.Fatalf("orient=%d trial=%d: pixels drawn but visible rect empty", o, trial)
			}
			if float64(vis.Min.X) > minX || float64(vis.Min.Y) > minY || float64(vis.Max.X) < maxX || float64(vis.Max.Y) < maxY {
				t.Fatalf("orient=%d trial=%d: visible rect %v misses source span [%v,%v]x[%v,%v]", o, trial, vis, minX, maxX, minY, maxY)
			}
			if float64(vis.Min.X) < minX-1-1/mm.Scale || float64(vis.Max.X) > maxX+1+1/mm.Scale {
				t.Fatalf("orient=%d trial=%d: visible rect %v is loose for [%v,%v]", o, trial, vis, minX, maxX)
			}
		}
	}
}

func TestVisibleRectDegenerateInput(t *testing.T) {
	m := Mapping{Scale: 1, Orient: codec.OrientNormal}
	for name, r := range map[string]image.Rectangle{
		"zero source":  VisibleRect(m, 0, 10, 5, 5),
		"zero dst":     VisibleRect(m, 10, 10, 0, 5),
		"bad scale":    VisibleRect(Mapping{Scale: 0}, 10, 10, 5, 5),
		"nan scale":    VisibleRect(Mapping{Scale: math.NaN()}, 10, 10, 5, 5),
		"inf scale":    VisibleRect(Mapping{Scale: math.Inf(1)}, 10, 10, 5, 5),
		"left of it":   VisibleRect(Mapping{Scale: 1, OriginX: -100}, 10, 10, 5, 5),
		"right of it":  VisibleRect(Mapping{Scale: 1, OriginX: 100}, 10, 10, 5, 5),
		"above it":     VisibleRect(Mapping{Scale: 1, OriginY: -100}, 10, 10, 5, 5),
		"below it":     VisibleRect(Mapping{Scale: 1, OriginY: 100}, 10, 10, 5, 5),
		"far negative": VisibleRect(Mapping{Scale: 0.001, OriginX: -1e9}, 10, 10, 5, 5),
	} {
		if !r.Empty() {
			t.Fatalf("%s: %v", name, r)
		}
	}
	if r := VisibleRect(m, 10, 10, 100, 100); r != image.Rect(0, 0, 10, 10) {
		t.Fatalf("whole image: %v", r)
	}
	if r := VisibleRect(Mapping{Scale: 2, OriginX: 3, OriginY: 4}, 100, 100, 20, 10); r != image.Rect(3, 4, 13, 9) {
		t.Fatalf("zoomed: %v", r)
	}
}

func TestWindowRectOfEmptyWindow(t *testing.T) {
	if !(Window{}).Rect().Empty() {
		t.Fatal("nil window must have an empty rectangle")
	}
	w := Window{Img: image.NewNRGBA(image.Rect(0, 0, 5, 7)), X: 3, Y: 4}
	if w.Rect() != image.Rect(3, 4, 8, 11) {
		t.Fatalf("rect %v", w.Rect())
	}
}

func BenchmarkWindowRender(b *testing.B) {
	rng := rand.New(rand.NewSource(31))
	l0 := randomImage(rng, 4096, 3072, false)
	levels := []*image.NRGBA{l0}
	for len(levels) < 4 {
		levels = append(levels, boxHalve(levels[len(levels)-1]))
	}
	for _, o := range []codec.Orientation{codec.OrientNormal, codec.OrientRotate90} {
		b.Run(map[codec.Orientation]string{codec.OrientNormal: "Normal", codec.OrientRotate90: "Rotate90"}[o], func(b *testing.B) {
			ow, oh := o.Size(4096, 3072)
			m := Mapping{Scale: 0.9, Orient: o, Filter: FilterBilinear, OriginX: float64(ow)/2 - 1000, OriginY: float64(oh)/2 - 600}
			det := &cutDetail{levels: levels, margin: 256}
			need := levelNeed(VisibleRect(m, 4096, 3072, 1920, 1080), 0, 4096, 3072)
			src := Source{Levels: levels[3:], Width: 4096, Height: 3072, Shift: 3, Detail: fixedDetail{det.Window(0, need)}}
			full := Source{Levels: levels, Width: 4096, Height: 3072}
			dst := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
			bg := benchBg()
			b.Run("Window", func(b *testing.B) {
				Render(dst, src, m, bg)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					Render(dst, src, m, bg)
				}
			})
			b.Run("FullLevel", func(b *testing.B) {
				Render(dst, full, m, bg)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					Render(dst, full, m, bg)
				}
			})
		})
	}
}

type scriptedDetail struct {
	win Window
}

func (d scriptedDetail) SetOnChange(func())                 {}
func (d scriptedDetail) Window(int, image.Rectangle) Window { return d.win }

func FuzzWindowRender(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24})
	f.Add(make([]byte, 24))
	f.Add([]byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 24 {
			return
		}
		rng := rand.New(rand.NewSource(int64(data[0])<<8 | int64(data[1])))
		full := randomSource(rng, 40+int(data[2])%60, 30+int(data[3])%50, 4, data[4]%2 == 0)
		wImg := image.NewNRGBA(image.Rect(int(int8(data[5]))%4, int(int8(data[6]))%4, int(data[7])%70, int(data[8])%60))
		win := Window{Img: wImg, Shift: int(int8(data[9])) % 4, X: int(int8(data[10])), Y: int(int8(data[11]))}
		if data[12]%7 == 0 {
			win.Img = nil
		}
		src := Source{
			Levels: full.Levels[int(data[13])%len(full.Levels):],
			Width:  full.Width,
			Height: full.Height,
			Shift:  int(data[13]) % len(full.Levels),
			Detail: scriptedDetail{win},
		}
		if data[14]%5 == 0 {
			src.Width, src.Height = int(int8(data[15])), int(int8(data[16]))
		}
		m := Mapping{
			Scale:   float64(1+int(data[17])) / float64(1+int(data[18])%60),
			Orient:  codec.Orientation(data[19] % 10),
			Filter:  Filter(data[20] % 3),
			OriginX: float64(int8(data[21])) * 1.5,
			OriginY: float64(int8(data[22])) * 1.5,
		}
		dst := newDst(1+int(data[23])%80, 1+int(data[2])%60, int(int8(data[3]))%5, int(int8(data[4]))%5)
		before := len(dst.Pix)
		Render(dst, src, m, checkerBg())
		if len(dst.Pix) != before {
			t.Fatal("destination resized")
		}
		vis := VisibleRect(m, src.Width, src.Height, dst.Rect.Dx(), dst.Rect.Dy())
		if !vis.Empty() && !vis.In(image.Rect(0, 0, src.Width, src.Height)) {
			t.Fatalf("visible rect %v outside %dx%d", vis, src.Width, src.Height)
		}
	})
}
