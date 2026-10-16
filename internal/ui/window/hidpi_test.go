package window

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

func newScaledRig(t *testing.T, k float64) *rig {
	t.Helper()
	i18n.SetLang("en")
	h := newHost()
	h.reg = accessRegistry(h)
	eng := testutil.NewEngineScaled(770, 504, k)
	rec := &recordingEngine{Engine: eng}
	r := &rig{t: t, eng: eng, rec: rec, host: h, ts: &timers{}, now: time.Unix(100, 0)}
	r.root = New(h, rec, WindowOps{Close: func() { r.ops = append(r.ops, "close") }})
	r.root.OnMore = func(image.Rectangle) {}
	r.root.After = r.ts.after
	eng.SetRootFullCanvas(r.root)
	eng.Flush()
	eng.RenderOnce()
	return r
}

func (r *rig) physicalClick(l Layout, k ZoneKind, scale float64) {
	x, y := l.Center(k)
	px, py := int(math.Round(float64(x)*scale)), int(math.Round(float64(y)*scale))
	r.eng.SendMouseMove(px, py)
	r.eng.SendMouseButton(px, py, widget.MouseLeft, true)
	r.eng.SendMouseButton(px, py, widget.MouseLeft, false)
}

func TestMouseHitAreasAreLogicalWhateverTheScale(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 1.75, 2} {
		r := newScaledRig(t, scale)
		l := r.root.Layout()
		for k, want := range map[ZoneKind]commands.ID{
			ZoneToolDelete: "file.delete", ZoneToolZoomIn: "view.zoomIn", ZoneToolFullscreen: "view.fullscreen",
			ZoneBack: "app.back", ZoneInfo: "view.info", ZonePrev: "nav.prev", ZoneNext: "nav.next",
		} {
			r.host.ran = nil
			r.physicalClick(l, k, scale)
			if len(r.host.ran) != 1 || r.host.ran[0] != want {
				t.Errorf("scale %v: clicking zone %d ran %v, want %s", scale, k, r.host.ran, want)
			}
		}
		r.physicalClick(l, ZoneClose, scale)
		if len(r.ops) != 1 || r.ops[0] != "close" {
			t.Errorf("scale %v: close button ran %v", scale, r.ops)
		}
	}
}

func TestRingIsDrawnOnThePhysicalGridAtFractionalScales(t *testing.T) {
	for _, scale := range []float64{1.25, 1.5, 1.75, 2} {
		r := newScaledRig(t, scale)
		r.key(widget.KeyTab, 0)
		z := r.root.Layout().Rect(ZoneToolGallery)
		plate := z.Inset(2)
		x0 := int(math.Round(float64(plate.Min.X) * scale))
		y := int(math.Round(float64(z.Min.Y+z.Dy()/2) * scale))
		r.root.Refresh()
		r.eng.Flush()
		img := r.eng.RenderOnce()
		focus := r.root.Theme.Focus
		width := int(math.Round(2 * scale))
		for i := 1; i <= width; i++ {
			if got := img.RGBAAt(x0-i, y); got != focus {
				t.Errorf("scale %v: ring pixel %d left of the button is %v, want %v", scale, i, got, focus)
			}
		}
		if got := img.RGBAAt(x0-width-1, y); got == focus {
			t.Errorf("scale %v: ring is wider than %d pixels", scale, width)
		}
	}
}

func TestTooltipAndToastStayInsideTheWindowAtEveryScale(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 1.75, 2} {
		r := newScaledRig(t, scale)
		l := r.root.Layout()
		for _, k := range []ZoneKind{ZoneToolDelete, ZoneToolZoomLabel, ZoneInfo, ZoneClose, ZonePrev} {
			x, y := l.Center(k)
			r.root.OnMouseMove(x, y)
			r.ts.fireAll()
			rect := r.root.tipRect(nil)
			if rect.Empty() {
				t.Errorf("scale %v: no tooltip for zone %d", scale, k)
				continue
			}
			if !rect.In(image.Rect(0, 0, 770, 504)) {
				t.Errorf("scale %v: tooltip %v of zone %d leaves the window", scale, rect, k)
			}
			r.root.hideTip()
		}
		r.root.OnMouseMove(l.Rect(ZoneToolDelete).Min.X+4, l.Rect(ZoneToolDelete).Min.Y+4)
		r.ts.fireAll()
		r.root.Toast("File saved")
		r.root.Refresh()
		r.eng.Flush()
		img := r.eng.RenderOnce()
		if dir := testutil.OutDir(); dir != "" {
			testutil.SavePNG(dir+"/tip_"+scaleName(scale)+".png", img)
		}
		if toast := r.root.toastRect(r.root.fullOverlayLayout()); !toast.In(image.Rect(0, 0, 770, 504)) {
			t.Errorf("scale %v: toast %v leaves the window", scale, toast)
		}
	}
}

func scaleName(k float64) string {
	return map[float64]string{1: "1.00", 1.25: "1.25", 1.5: "1.50", 1.75: "1.75", 2: "2.00"}[k]
}
