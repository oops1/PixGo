package settings

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func benchFrame(b *testing.B, s Section) {
	r := newRig(b, mockW, mockH)
	r.page.SetSection(s)
	r.render()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.eng.Invalidate()
		r.eng.RenderOnce()
	}
}

func BenchmarkFrameGeneral(b *testing.B)     { benchFrame(b, SectionGeneral) }
func BenchmarkFrameView(b *testing.B)        { benchFrame(b, SectionView) }
func BenchmarkFramePerformance(b *testing.B) { benchFrame(b, SectionPerformance) }
func BenchmarkFrameAbout(b *testing.B)       { benchFrame(b, SectionAbout) }

func BenchmarkScrollStep(b *testing.B) {
	r := newRig(b, mockW, mockH)
	r.render()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dy := wheelNotchPixels
		if i%2 == 1 {
			dy = -wheelNotchPixels
		}
		r.page.OnMouseWheelPixelsMod(300, 200, 0, dy, 0)
		r.eng.RenderOnce()
	}
}

func BenchmarkHoverMove(b *testing.B) {
	r := newRig(b, mockW, mockH)
	r.render()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.page.OnMouseMove(200+i%200, 240+i%80)
		r.eng.RenderOnce()
	}
}

func BenchmarkRebuild(b *testing.B) {
	r := newRig(b, mockW, mockH)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.page.rebuild()
	}
}

func BenchmarkKeyToggle(b *testing.B) {
	r := newRig(b, mockW, mockH)
	r.focusOn("rememberWindow")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.page.HandleKey(widget.KeyEvent{Code: widget.KeySpace, Pressed: true})
	}
}
