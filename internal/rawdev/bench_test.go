package rawdev

import (
	"context"
	"image"
	"math/rand"
	"testing"

	"github.com/oops1/PixGo/internal/rawdev/rawdevtest"
)

func benchMosaic(w, h int) (Mosaic, Params) {
	s := makeSynthetic(synthOptions{W: 64, H: 64, CFA: rggb})
	pix := make([]uint16, w*h)
	rng := rand.New(rand.NewSource(1))
	for i := range pix {
		pix[i] = uint16(512 + rng.Intn(15000))
	}
	p := s.params
	p.Crop = image.Rect(0, 0, w, h)
	return Mosaic{Pix: pix, Stride: w, Samples: 1, W: w, H: h}, p
}

func BenchmarkDevelop24MpxEightBit(b *testing.B) {
	m, p := benchMosaic(6000, 4000)
	b.SetBytes(6000 * 4000 * 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := Develop(context.Background(), m, p, 1, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDevelop24MpxDeep(b *testing.B) {
	m, p := benchMosaic(6000, 4000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := Develop(context.Background(), m, p, 1, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDevelop24MpxBinned(b *testing.B) {
	m, p := benchMosaic(6000, 4000)
	for _, scale := range []int{2, 4, 8} {
		b.Run(string(rune('0'+scale)), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, _, err := Develop(context.Background(), m, p, scale, false); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLJ92Decode(b *testing.B) {
	w, h := 1024, 1024
	rng := rand.New(rand.NewSource(2))
	src := randomSamples(rng, w, h, 1, 14)
	data := rawdevtest.EncodeLJ92(w, h, 1, 14, src, rawdevtest.LJOptions{Lengths: rawdevtest.LengthsLong})
	d, err := ParseLJ92(data)
	if err != nil {
		b.Fatal(err)
	}
	dst := make([]uint16, w*h)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := d.Decode(context.Background(), dst, w); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(w*h)*float64(b.N)/b.Elapsed().Seconds()/1e6, "Msamples/s")
}
