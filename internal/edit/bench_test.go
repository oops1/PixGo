package edit

import (
	"context"
	"image"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func benchResample(b *testing.B, src *image.NRGBA, w, h int) {
	b.Helper()
	b.SetBytes(int64(src.Bounds().Dx() * src.Bounds().Dy() * 4))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Resample(context.Background(), src, w, h); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResampleDown2xOpaque(b *testing.B) {
	benchResample(b, noisy(4000, 3000, 1, false), 2000, 1500)
}

func BenchmarkResampleDown2xAlpha(b *testing.B) {
	benchResample(b, noisy(4000, 3000, 1, true), 2000, 1500)
}

func BenchmarkResampleDown5xOpaque(b *testing.B) {
	benchResample(b, noisy(6000, 4000, 1, false), 1200, 800)
}

func BenchmarkResampleDown30xBoxPrefilter(b *testing.B) {
	benchResample(b, noisy(6000, 4000, 1, false), 200, 133)
}

func BenchmarkResampleUp2xOpaque(b *testing.B) {
	benchResample(b, noisy(2000, 1500, 1, false), 4000, 3000)
}

func BenchmarkApplyRotateCropResize(b *testing.B) {
	src := noisy(4000, 3000, 1, false)
	plan := Plan{Orientation: codec.OrientRotate90, Crop: image.Rect(100, 200, 2900, 3800), Size: image.Pt(1400, 1800)}
	b.SetBytes(int64(4000 * 3000 * 4))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Apply(context.Background(), src, plan); err != nil {
			b.Fatal(err)
		}
	}
}
