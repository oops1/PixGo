package printdlg

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/print"
	"github.com/oops1/PixGo/internal/testutil"
)

var (
	scales = []float64{1, 1.2, 1.6}
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

func eachCase(t *testing.T, f func(t *testing.T, k float64, lang string)) {
	t.Helper()
	for _, lang := range langs {
		for _, k := range scales {
			t.Run(fmt.Sprintf("%s/k%.2f", lang, k), func(t *testing.T) {
				withScale(t, k, lang)
				f(t, k, lang)
			})
		}
	}
}

func gradient(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 150, A: 255})
		}
	}
	return img
}

func testEnv() Env {
	return Env{
		Pic:     print.Picture{W: 4000, H: 3000, DPI: 300},
		Preview: gradient(200, 150),
		Printers: []Printer{
			{Name: "Office Laser", Description: "Laser"},
			{Name: "Photo Inkjet", Default: true},
			{Name: "Microsoft Print to PDF"},
		},
		System: true,
	}
}

func testValues() Values {
	return Values{Printer: "Photo Inkjet", Copies: 1, Options: print.DefaultOptions()}
}

type outcome struct {
	values Values
	ok     bool
}

type rig struct {
	t       testing.TB
	eng     *engine.Engine
	d       *Dialog
	results []outcome
	systems []Values
	applyFn func(string, int)
}

func themeFor(dark bool) *widget.Theme {
	name := "Win11 Light"
	if dark {
		name = "Win11 Dark"
	}
	return widget.ThemeByName(name)
}

func newRig(t testing.TB, env Env, v Values, w, h int) *rig {
	t.Helper()
	return newRigThemed(t, env, v, w, h, true)
}

func newRigThemed(t testing.TB, env Env, v Values, w, h int, dark bool) *rig {
	t.Helper()
	eng := testutil.NewEngine(w, h)
	t.Cleanup(eng.Stop)
	eng.SetTheme(themeFor(dark))
	backdrop := widget.NewPanel(color.RGBA{R: 32, G: 32, B: 32, A: 255})
	backdrop.SetBounds(image.Rect(0, 0, w, h))
	eng.SetRoot(backdrop)
	r := &rig{t: t, eng: eng}
	r.d = Show(eng, env, v, func(v Values, ok bool) { r.results = append(r.results, outcome{v, ok}) },
		WithSystemDialog(func(v Values, apply func(string, int)) {
			r.systems = append(r.systems, v)
			r.applyFn = apply
		}))
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

func (r *rig) typeRune(ch rune) {
	code := widget.KeyUnknown
	if ch >= '0' && ch <= '9' {
		code = widget.KeyCode(ch)
	}
	r.eng.SendKeyEvent(widget.KeyEvent{Code: code, Rune: ch, Pressed: true})
	r.eng.SendKeyEvent(widget.KeyEvent{Code: code, Rune: ch, Pressed: false})
}

func (r *rig) typeText(s string) {
	for _, ch := range s {
		r.typeRune(ch)
	}
}

func (r *rig) click(x, y int) {
	r.eng.SendMouseMove(x, y)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, false)
}

func centre(rc image.Rectangle) (int, int) {
	return (rc.Min.X + rc.Max.X) / 2, (rc.Min.Y + rc.Max.Y) / 2
}

func (r *rig) clickPart(p Part) {
	r.t.Helper()
	x, y := centre(r.d.Rect(p))
	r.click(x, y)
}

func (r *rig) last() outcome {
	r.t.Helper()
	if len(r.results) == 0 {
		r.t.Fatal("the dialog gave no result")
	}
	return r.results[len(r.results)-1]
}

func (r *rig) focus(p Part) {
	r.t.Helper()
	r.d.form.enter(p)
	r.d.form.keyNav = true
}

func (r *rig) choose(p Part, label string) {
	r.t.Helper()
	items, _ := r.d.form.itemsOf(p)
	for i, it := range items {
		if it == label {
			r.d.form.choose(p, i)
			return
		}
	}
	r.t.Fatalf("no %q in %v", label, items)
}
