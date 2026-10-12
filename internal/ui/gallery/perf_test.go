package gallery

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func benchRig(b *testing.B, n int, thumbs bool) *rig {
	b.Helper()
	src := mockSource()
	if n != len(src.items) {
		src.items = manyItems(n)
	}
	if thumbs {
		for i := 0; i < min(n, 64); i++ {
			src.images[src.items[i].Path] = solid(96, 64, color.RGBA{uint8(i * 3), 120, 200, 255})
		}
	}
	return newRig(b, src, mockW, mockH)
}

func BenchmarkFrameTwelveItems(b *testing.B) {
	r := benchRig(b, 12, true)
	r.render()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.eng.Invalidate()
		r.eng.RenderOnce()
	}
}

func BenchmarkFrameFiveThousandItems(b *testing.B) {
	r := benchRig(b, 5000, true)
	r.render()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.eng.Invalidate()
		r.eng.RenderOnce()
	}
}

func BenchmarkFrameFiveThousandItemsScrolled(b *testing.B) {
	r := benchRig(b, 5000, false)
	r.page.setScroll(r.page.grid.MaxScroll() / 2)
	r.render()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.eng.Invalidate()
		r.eng.RenderOnce()
	}
}

func BenchmarkScrollStepFiveThousandItems(b *testing.B) {
	r := benchRig(b, 5000, false)
	r.render()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dy := wheelNotchPixels
		if i%2 == 1 {
			dy = -dy
		}
		r.page.OnMouseWheelPixelsMod(300, 200, 0, dy, widget.ModNone)
		r.eng.RenderOnce()
	}
}

func BenchmarkRefreshFiveThousandItems(b *testing.B) {
	r := benchRig(b, 5000, false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.page.Refresh()
	}
}
