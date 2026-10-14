package saveopts

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

var (
	scales = []float64{1, 1.2, 1.33, 1.6}
	langs  = []string{"ru", "en"}
	kinds  = []Kind{JPEG, WebP}
)

func kindName(k Kind) string {
	if k == WebP {
		return "webp"
	}
	return "jpeg"
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

func eachCase(t *testing.T, f func(t *testing.T, kind Kind, k float64, lang string)) {
	t.Helper()
	for _, kind := range kinds {
		for _, lang := range langs {
			for _, k := range scales {
				t.Run(fmt.Sprintf("%s/%s/k%.2f", kindName(kind), lang, k), func(t *testing.T) {
					withScale(t, k, lang)
					f(t, kind, k, lang)
				})
			}
		}
	}
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
}

func themeFor(dark bool) *widget.Theme {
	name := "Win11 Light"
	if dark {
		name = "Win11 Dark"
	}
	return widget.ThemeByName(name)
}

func newRig(t testing.TB, kind Kind, initial Values, w, h int) *rig {
	t.Helper()
	return newRigThemed(t, kind, initial, w, h, true)
}

func newRigThemed(t testing.TB, kind Kind, initial Values, w, h int, dark bool) *rig {
	t.Helper()
	eng := testutil.NewEngine(w, h)
	eng.SetTheme(themeFor(dark))
	backdrop := widget.NewPanel(color.RGBA{R: 32, G: 32, B: 32, A: 255})
	backdrop.SetBounds(image.Rect(0, 0, w, h))
	eng.SetRoot(backdrop)
	r := &rig{t: t, eng: eng}
	r.d = Show(eng, kind, initial, func(v Values, ok bool) { r.results = append(r.results, outcome{v, ok}) })
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

func (r *rig) trackX(q int) int {
	x0, x1, _ := r.d.form.track()
	return x0 + int(math.Round(float64(q-MinQuality)/float64(MaxQuality-MinQuality)*float64(x1-x0)))
}

func (r *rig) clickQuality(q int) {
	_, _, cy := r.d.form.track()
	r.click(r.trackX(q), cy)
}

func (r *rig) last() outcome {
	r.t.Helper()
	if len(r.results) == 0 {
		r.t.Fatal("the dialog gave no result")
	}
	return r.results[len(r.results)-1]
}

func sameImage(a, b *image.RGBA, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}
