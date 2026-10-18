package edit

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/imaging"
	"github.com/oops1/PixGo/internal/render"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/theme"
	"github.com/oops1/PixGo/internal/ui/view"
	"github.com/oops1/PixGo/internal/viewer"
)

func gradient(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x * 255 / max(1, w-1)), uint8(y * 255 / max(1, h-1)), 128, 255})
		}
	}
	return img
}

func source(w, h int) render.Source {
	return render.Source{Levels: imaging.BuildPyramid(gradient(w, h), 64), Width: w, Height: h}
}

type calls struct {
	saveCopy, cancel, resize, changed int
	aspectMenu, saveMenu              []image.Rectangle
}

type rig struct {
	t      testing.TB
	eng    *engine.Engine
	canvas *view.Canvas
	ed     *Editor
	calls  *calls
	w, h   int
}

func newRig(t testing.TB, w, h, iw, ih int) *rig {
	t.Helper()
	return newRigThemed(t, w, h, iw, ih, theme.Dark())
}

func newRigThemed(t testing.TB, w, h, iw, ih int, th *theme.Theme) *rig {
	t.Helper()
	i18n.SetLang("en")
	eng := testutil.NewEngine(w, h)
	canvas := view.New(eng)
	canvas.Background = render.Background{Canvas: th.Canvas, Kind: render.BgSolid, Fill: th.Canvas}
	canvas.SetImage(source(iw, ih), codec.OrientNormal, viewer.ZoomFit)
	ed := New(eng, canvas)
	ed.Theme = th
	c := &calls{}
	ed.On = Handlers{
		AspectMenu: func(a image.Rectangle) { c.aspectMenu = append(c.aspectMenu, a) },
		SaveMenu:   func(a image.Rectangle) { c.saveMenu = append(c.saveMenu, a) },
		SaveCopy:   func() { c.saveCopy++ },
		Cancel:     func() { c.cancel++ },
		Resize:     func() { c.resize++ },
		Changed:    func() { c.changed++ },
	}
	eng.SetRootFullCanvas(ed)
	ed.SetBounds(image.Rect(0, 0, w, h))
	r := &rig{t: t, eng: eng, canvas: canvas, ed: ed, calls: c, w: w, h: h}
	eng.Flush()
	if !ed.Begin() {
		t.Fatal("Begin refused")
	}
	eng.SetFocus(ed)
	r.render()
	t.Cleanup(func() { i18n.SetLang("en") })
	return r
}

func (r *rig) render() *image.RGBA {
	r.eng.Invalidate()
	r.eng.Flush()
	src := r.eng.RenderOnce()
	out := image.NewRGBA(src.Bounds())
	copy(out.Pix, src.Pix)
	return out
}

func (r *rig) key(code widget.KeyCode, mod widget.KeyMod) {
	r.eng.SendKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
	r.eng.SendKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: false})
}

func (r *rig) click(x, y int) {
	r.eng.SendMouseMove(x, y)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, false)
}

func (r *rig) dragFrom(x0, y0, x1, y1 int) {
	r.eng.SendMouseMove(x0, y0)
	r.eng.SendMouseButton(x0, y0, widget.MouseLeft, true)
	r.eng.SendMouseMove((x0+x1)/2, (y0+y1)/2)
	r.eng.SendMouseMove(x1, y1)
	r.eng.SendMouseButton(x1, y1, widget.MouseLeft, false)
}

func centre(rc image.Rectangle) (int, int) {
	return (rc.Min.X + rc.Max.X) / 2, (rc.Min.Y + rc.Max.Y) / 2
}

func (r *rig) clickPart(p Part) {
	r.t.Helper()
	rc := r.ed.BarRect(p)
	if rc.Empty() {
		r.t.Fatalf("part %d has no rect", p)
	}
	x, y := centre(rc)
	r.click(x, y)
}

func (r *rig) setCrop(rc image.Rectangle) {
	r.t.Helper()
	r.ed.Crop().SetRect(rc)
	if r.ed.Crop().Rect() != rc {
		r.t.Fatalf("crop %v refused, have %v", rc, r.ed.Crop().Rect())
	}
}

func withScale(t testing.TB, k float64, lang string) {
	t.Helper()
	oldScale, oldLang := fonts.TextScale(), i18n.Lang()
	fonts.SetTextScale(k)
	i18n.SetLang(lang)
	t.Cleanup(func() {
		fonts.SetTextScale(oldScale)
		i18n.SetLang(oldLang)
	})
}
