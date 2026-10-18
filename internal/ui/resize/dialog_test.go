package resize

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

var (
	scales = []float64{1, 1.2, 1.33, 1.6}
	langs  = i18n.Supported()
)

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

type outcome struct {
	size image.Point
	ok   bool
}

type rig struct {
	t       testing.TB
	eng     *engine.Engine
	d       *Dialog
	results []outcome
}

func newRig(t testing.TB, w, h int, dark bool) *rig {
	t.Helper()
	return newRigWith(t, w, h, dark, image.Pt(4000, 3000), image.Pt(2000, 1500))
}

func newRigWith(t testing.TB, w, h int, dark bool, orig, initial image.Point) *rig {
	t.Helper()
	eng := testutil.NewEngine(w, h)
	t.Cleanup(eng.Stop)
	name := "Win11 Light"
	if dark {
		name = "Win11 Dark"
	}
	eng.SetTheme(widget.ThemeByName(name))
	backdrop := widget.NewPanel(color.RGBA{R: 32, G: 32, B: 32, A: 255})
	backdrop.SetBounds(image.Rect(0, 0, w, h))
	eng.SetRoot(backdrop)
	r := &rig{t: t, eng: eng}
	r.d = Show(eng, orig, initial, lim, func(s image.Point, ok bool) { r.results = append(r.results, outcome{s, ok}) })
	eng.Flush()
	t.Cleanup(func() { eng.CloseModal(nil) })
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

func (r *rig) press(code widget.KeyCode) { r.key(code, 0) }

func (r *rig) typeText(s string) {
	for _, ch := range s {
		r.eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyCode(ch), Rune: ch, Pressed: true})
		r.eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyCode(ch), Rune: ch, Pressed: false})
	}
}

func centre(rc image.Rectangle) (int, int) {
	return (rc.Min.X + rc.Max.X) / 2, (rc.Min.Y + rc.Max.Y) / 2
}

func (r *rig) click(x, y int) {
	r.eng.SendMouseMove(x, y)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, false)
}

func (r *rig) clickPart(p Part) {
	r.t.Helper()
	x, y := centre(r.d.Rect(p))
	r.click(x, y)
}

func (r *rig) last() outcome {
	r.t.Helper()
	if len(r.results) == 0 {
		r.t.Fatal("no result")
	}
	return r.results[len(r.results)-1]
}

func TestOpensOnTheWidthFieldWithItSelected(t *testing.T) {
	r := newRig(t, 760, 480, true)
	if r.d.Focused() != PartWidth {
		t.Fatalf("focus %d", r.d.Focused())
	}
	r.typeText("1000")
	if m := r.d.Model(); m.WidthText() != "1000" || m.HeightText() != "750" {
		t.Fatalf("typing replaced the selection: %s x %s", m.WidthText(), m.HeightText())
	}
	r.press(widget.KeyEnter)
	if got := r.last(); !got.ok || got.size != image.Pt(1000, 750) {
		t.Fatalf("result %+v", got)
	}
	if !r.d.Done() {
		t.Fatal("not closed")
	}
}

func TestEscapeCancels(t *testing.T) {
	r := newRig(t, 760, 480, true)
	r.press(widget.KeyEscape)
	if got := r.last(); got.ok {
		t.Fatalf("result %+v", got)
	}
}

func TestEditingKeys(t *testing.T) {
	r := newRig(t, 760, 480, true)
	r.press(widget.KeyBackspace)
	if r.d.Model().WidthText() != "" || r.d.Enabled(PartApply) {
		t.Fatalf("width %q apply=%v", r.d.Model().WidthText(), r.d.Enabled(PartApply))
	}
	if r.d.ResultText() != i18n.T("resize.invalid") {
		t.Fatalf("text %q", r.d.ResultText())
	}
	r.press(widget.KeyEnter)
	if len(r.results) != 0 {
		t.Fatal("Enter applied an invalid size")
	}
	r.typeText("640")
	r.press(widget.KeyLeft)
	r.press(widget.KeyBackspace)
	if r.d.Model().WidthText() != "60" {
		t.Fatalf("width %q", r.d.Model().WidthText())
	}
	r.press(widget.KeyHome)
	r.press(widget.KeyDelete)
	if r.d.Model().WidthText() != "0" {
		t.Fatalf("width %q", r.d.Model().WidthText())
	}
	r.press(widget.KeyEnd)
	r.typeText("x5")
	if r.d.Model().WidthText() != "05" {
		t.Fatalf("letters must be ignored: %q", r.d.Model().WidthText())
	}
	r.key(widget.KeyA, widget.ModCtrl)
	r.typeText("800")
	if r.d.Model().WidthText() != "800" {
		t.Fatalf("select all: %q", r.d.Model().WidthText())
	}
	r.press(widget.KeyUp)
	if r.d.Model().WidthText() != "801" {
		t.Fatalf("up: %q", r.d.Model().WidthText())
	}
	r.key(widget.KeyDown, widget.ModShift)
	if r.d.Model().WidthText() != "791" {
		t.Fatalf("shift+down: %q", r.d.Model().WidthText())
	}
}

func TestTabWalksTheControls(t *testing.T) {
	r := newRig(t, 760, 480, true)
	var seen []Part
	for i := 0; i < 8; i++ {
		r.press(widget.KeyTab)
		seen = append(seen, r.d.Focused())
	}
	want := []Part{PartHeight, PartPixels, PartPercent, PartKeep, PartApply, PartCancel, PartWidth, PartHeight}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Fatalf("order %v want %v", seen, want)
	}
	r.key(widget.KeyTab, widget.ModShift)
	if r.d.Focused() != PartWidth {
		t.Fatalf("shift-tab: %d", r.d.Focused())
	}
}

func TestTabSkipsApplyWhenInvalid(t *testing.T) {
	r := newRig(t, 760, 480, true)
	r.press(widget.KeyBackspace)
	for i := 0; i < 7; i++ {
		r.press(widget.KeyTab)
		if r.d.Focused() == PartApply {
			t.Fatal("tab reached a disabled button")
		}
	}
}

func TestUnitSwitchByMouseAndKeys(t *testing.T) {
	r := newRig(t, 760, 480, true)
	r.clickPart(PartPercent)
	m := r.d.Model()
	if m.Unit() != UnitPercent || m.WidthText() != "50" {
		t.Fatalf("unit %v width %s", m.Unit(), m.WidthText())
	}
	r.clickPart(PartWidth)
	r.key(widget.KeyA, widget.ModCtrl)
	r.typeText("10")
	r.press(widget.KeyEnter)
	if got := r.last(); got.size != image.Pt(400, 300) {
		t.Fatalf("result %+v", got)
	}
}

func TestUnitRadioKeys(t *testing.T) {
	r := newRig(t, 760, 480, true)
	r.press(widget.KeyTab)
	r.press(widget.KeyTab)
	if r.d.Focused() != PartPixels {
		t.Fatalf("focus %d", r.d.Focused())
	}
	r.press(widget.KeyRight)
	if r.d.Model().Unit() != UnitPercent {
		t.Fatal("right did not select percent")
	}
	r.press(widget.KeyLeft)
	if r.d.Model().Unit() != UnitPixels {
		t.Fatal("left did not select pixels")
	}
}

func TestKeepProportionsCheckBox(t *testing.T) {
	r := newRig(t, 760, 480, true)
	r.clickPart(PartKeep)
	if r.d.Model().Keep() {
		t.Fatal("click did not turn it off")
	}
	r.clickPart(PartWidth)
	r.key(widget.KeyA, widget.ModCtrl)
	r.typeText("3000")
	if r.d.Model().HeightText() != "1500" {
		t.Fatalf("height moved with the lock off: %s", r.d.Model().HeightText())
	}
	r.press(widget.KeyEnter)
	if got := r.last(); got.size != image.Pt(3000, 1500) {
		t.Fatalf("result %+v", got)
	}
}

func TestClickPlacesTheCaret(t *testing.T) {
	r := newRig(t, 760, 480, true)
	rc := r.d.Rect(PartHeight)
	r.click(rc.Min.X+r.d.form.fieldPad()+2, (rc.Min.Y+rc.Max.Y)/2)
	if r.d.Focused() != PartHeight {
		t.Fatalf("focus %d", r.d.Focused())
	}
	r.typeText("9")
	if r.d.Model().HeightText() != "91500" {
		t.Fatalf("the caret was not at the start: %s", r.d.Model().HeightText())
	}
}

func TestTooBigIsShownAndBlocksApply(t *testing.T) {
	r := newRig(t, 760, 480, true)
	r.typeText("20000")
	if r.d.Enabled(PartApply) {
		t.Fatal("apply enabled")
	}
	if got := r.d.ResultText(); got == i18n.T("resize.invalid") || got == "" {
		t.Fatalf("text %q", got)
	}
	r.clickPart(PartApply)
	if len(r.results) != 0 {
		t.Fatal("applied")
	}
}

func TestApplyButtonAndCancelButton(t *testing.T) {
	r := newRig(t, 760, 480, true)
	r.clickPart(PartApply)
	if got := r.last(); !got.ok || got.size != image.Pt(2000, 1500) {
		t.Fatalf("%+v", got)
	}
	c := newRig(t, 760, 480, true)
	c.clickPart(PartCancel)
	if got := c.last(); got.ok {
		t.Fatalf("%+v", got)
	}
}

func TestEverythingFitsEveryScaleAndLanguage(t *testing.T) {
	for _, lang := range langs {
		for _, k := range scales {
			t.Run(fmt.Sprintf("%s/k%.2f", lang, k), func(t *testing.T) {
				withScale(t, k, lang)
				r := newRig(t, 420, 320, true)
				b := r.d.Widget().Bounds()
				if b.Min.Y < 0 || b.Max.Y > 320 || b.Min.X < 0 || b.Max.X > 420 {
					t.Fatalf("dialog %v does not fit", b)
				}
				var seen []image.Rectangle
				for _, p := range order {
					rc := r.d.Rect(p)
					if rc.Empty() || !rc.In(b) {
						t.Errorf("part %d %v outside %v", p, rc, b)
					}
					for _, o := range seen {
						if rc.Overlaps(o) {
							t.Errorf("part %d %v overlaps %v", p, rc, o)
						}
					}
					seen = append(seen, rc)
				}
				if r.d.Rect(PartApply).Max.Y > b.Max.Y {
					t.Error("buttons cut off")
				}
			})
		}
	}
}

func TestSnapshots(t *testing.T) {
	out := testutil.OutDir()
	if out == "" {
		t.Skip("PIXGO_SNAPSHOTS is not set")
	}
	for _, lang := range langs {
		for _, dark := range []bool{true, false} {
			withScale(t, 1, lang)
			r := newRig(t, 760, 480, dark)
			r.press(widget.KeyTab)
			r.press(widget.KeyTab)
			name := fmt.Sprintf("resize_%s_%v.png", lang, map[bool]string{true: "dark", false: "light"}[dark])
			testutil.SavePNG(out+"/"+name, r.render())
		}
	}
}

func TestAccessTreeAndActions(t *testing.T) {
	r := newRig(t, 760, 480, true)
	infos := r.d.form.AccessChildren()
	testutil.RequireActionIDs(t, infos)
	for _, in := range infos {
		if in.Name == "" {
			t.Errorf("unnamed node %+v", in)
		}
	}
	f := r.d.form
	if !f.AccessAction("unit.percent", widget.AccessSelect) || r.d.Model().Unit() != UnitPercent {
		t.Fatal("select percent")
	}
	if !f.AccessAction("unit.pixels", widget.AccessInvoke) || r.d.Model().Unit() != UnitPixels {
		t.Fatal("invoke pixels")
	}
	if !f.AccessAction("keep", widget.AccessToggle) || r.d.Model().Keep() {
		t.Fatal("toggle keep")
	}
	if !f.AccessAction("height", widget.AccessFocus) || r.d.Focused() != PartHeight {
		t.Fatal("focus height")
	}
	if f.AccessAction("height", widget.AccessInvoke) || f.AccessAction("nope", widget.AccessInvoke) || f.AccessAction("width", widget.AccessToggle) {
		t.Fatal("unexpected success")
	}
	if !f.AccessAction("apply", widget.AccessInvoke) {
		t.Fatal("invoke apply")
	}
	if got := r.last(); !got.ok {
		t.Fatalf("%+v", got)
	}
}

func TestAccessValuesFollowTheInput(t *testing.T) {
	r := newRig(t, 760, 480, true)
	r.typeText("640")
	var width, result string
	for _, in := range r.d.form.AccessChildren() {
		switch in.ID {
		case "width":
			width = in.Value
		case "result":
			result = in.Name
		}
	}
	if width != "640" || result != i18n.T("resize.result", 640, 480) {
		t.Fatalf("width %q result %q", width, result)
	}
}
