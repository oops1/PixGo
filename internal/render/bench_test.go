package render

import (
	"image"
	"image/color"
	"math/rand"
	"sync"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

var benchSrc struct {
	once        sync.Once
	opaque      Source
	translucent Source
}

func boxHalve(src *image.NRGBA) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := (w+1)/2, (h+1)/2
	out := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y1 := min(2*y+1, h-1)
		for x := 0; x < nw; x++ {
			x1 := min(2*x+1, w-1)
			o := out.PixOffset(x, y)
			for c := 0; c < 4; c++ {
				s := int(src.Pix[src.PixOffset(2*x, 2*y)+c]) + int(src.Pix[src.PixOffset(x1, 2*y)+c]) +
					int(src.Pix[src.PixOffset(2*x, y1)+c]) + int(src.Pix[src.PixOffset(x1, y1)+c])
				out.Pix[o+c] = uint8((s + 2) / 4)
			}
		}
	}
	return out
}

func benchSources() (Source, Source) {
	benchSrc.once.Do(func() {
		rng := rand.New(rand.NewSource(11))
		l0 := image.NewNRGBA(image.Rect(0, 0, 6000, 4000))
		rng.Read(l0.Pix)
		for i := 3; i < len(l0.Pix); i += 4 {
			l0.Pix[i] = 255
		}
		lv := []*image.NRGBA{l0}
		for len(lv) < 4 {
			lv = append(lv, boxHalve(lv[len(lv)-1]))
		}
		benchSrc.opaque = Source{Levels: lv, Width: 6000, Height: 4000}

		t0 := image.NewNRGBA(l0.Rect)
		copy(t0.Pix, l0.Pix)
		for y := 0; y < 4000; y++ {
			for x := 0; x < 6000; x++ {
				if (x/300+y/300)%3 == 0 {
					t0.Pix[t0.PixOffset(x, y)+3] = uint8(x % 251)
				}
			}
		}
		tl := []*image.NRGBA{t0}
		for len(tl) < 4 {
			tl = append(tl, boxHalve(tl[len(tl)-1]))
		}
		benchSrc.translucent = Source{Levels: tl, Width: 6000, Height: 4000}
		for _, s := range []Source{benchSrc.opaque, benchSrc.translucent} {
			for _, l := range s.Levels {
				IsOpaque(l)
			}
		}
	})
	return benchSrc.opaque, benchSrc.translucent
}

func benchBg() Background {
	return Background{
		Canvas:  color.RGBA{20, 20, 20, 255},
		Kind:    BgChecker,
		Checker: [2]color.RGBA{{255, 255, 255, 255}, {204, 204, 204, 255}},
		Cell:    8,
	}
}

func runRender(b *testing.B, src Source, m Mapping) {
	dst := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	bg := benchBg()
	Render(dst, src, m, bg)
	b.ReportAllocs()
	b.SetBytes(int64(len(dst.Pix)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Render(dst, src, m, bg)
	}
}

func fitMapping(o codec.Orientation) Mapping {
	ow, oh := o.Size(6000, 4000)
	const scale = 0.27
	return Mapping{Scale: scale, Orient: o, OriginX: float64(ow)/2 - 960/scale, OriginY: float64(oh)/2 - 540/scale}
}

func BenchmarkFit(b *testing.B) {
	for _, tc := range []struct {
		name string
		o    codec.Orientation
	}{{"Normal", codec.OrientNormal}, {"Rotate90", codec.OrientRotate90}, {"FlipH", codec.OrientFlipH}} {
		b.Run("Opaque"+tc.name, func(b *testing.B) {
			s, _ := benchSources()
			runRender(b, s, fitMapping(tc.o))
		})
		b.Run("Translucent"+tc.name, func(b *testing.B) {
			_, s := benchSources()
			runRender(b, s, fitMapping(tc.o))
		})
	}
}

func BenchmarkScale1(b *testing.B) {
	for _, tc := range []struct {
		name  string
		o     codec.Orientation
		ox    float64
		scale float64
	}{
		{"Normal", codec.OrientNormal, 100, 1},
		{"NormalHalfPixelSnapped", codec.OrientNormal, 100.5, 1},
		{"NormalScale1001", codec.OrientNormal, 100.5, 1.001},
		{"Rotate90", codec.OrientRotate90, 100, 1},
		{"FlipH", codec.OrientFlipH, 100, 1},
	} {
		m := Mapping{Scale: tc.scale, Orient: tc.o, OriginX: tc.ox, OriginY: 200}
		b.Run("Opaque"+tc.name, func(b *testing.B) {
			s, _ := benchSources()
			runRender(b, s, m)
		})
		b.Run("Translucent"+tc.name, func(b *testing.B) {
			_, s := benchSources()
			runRender(b, s, m)
		})
	}
}

func BenchmarkScale3(b *testing.B) {
	for _, tc := range []struct {
		name string
		o    codec.Orientation
	}{{"Normal", codec.OrientNormal}, {"Rotate90", codec.OrientRotate90}} {
		m := Mapping{Scale: 3, Orient: tc.o, OriginX: 1000.3, OriginY: 700.7, Filter: FilterBilinear}
		b.Run("Opaque"+tc.name, func(b *testing.B) {
			s, _ := benchSources()
			runRender(b, s, m)
		})
		b.Run("Translucent"+tc.name, func(b *testing.B) {
			_, s := benchSources()
			runRender(b, s, m)
		})
	}
}

func BenchmarkScale6Nearest(b *testing.B) {
	s, _ := benchSources()
	runRender(b, s, Mapping{Scale: 6, OriginX: 1000.3, OriginY: 700.7})
}

func BenchmarkPanStrip(b *testing.B) {
	s, _ := benchSources()
	dst := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	bg := benchBg()
	m := fitMapping(codec.OrientNormal)
	Render(dst, s, m, bg)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Shift(dst, 20, 0)
		RenderRect(dst, image.Rect(0, 0, 20, 1080), s, m, bg)
	}
}

func BenchmarkShift(b *testing.B) {
	dst := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	b.SetBytes(int64(len(dst.Pix)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Shift(dst, 17, 9)
	}
}
