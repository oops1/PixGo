package confirm

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

type rig struct {
	t       testing.TB
	eng     *engine.Engine
	d       *Dialog
	results []Result
}

func newRig(t testing.TB, w, h int, cfg Config) *rig {
	t.Helper()
	i18n.SetLang("en")
	eng := testutil.NewEngine(w, h)
	eng.SetTheme(widget.ThemeByName("Win11 Dark"))
	backdrop := widget.NewPanel(color.RGBA{R: 32, G: 32, B: 32, A: 255})
	backdrop.SetBounds(image.Rect(0, 0, w, h))
	eng.SetRoot(backdrop)
	r := &rig{t: t, eng: eng}
	r.d = Show(eng, cfg, func(res Result) { r.results = append(r.results, res) })
	eng.Flush()
	t.Cleanup(func() { eng.CloseModal(nil) })
	return r
}

func (r *rig) key(code widget.KeyCode, mod widget.KeyMod) {
	r.eng.SendKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
	r.eng.SendKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: false})
}

func (r *rig) click(p Part) {
	rc := r.d.Rect(p)
	x, y := (rc.Min.X+rc.Max.X)/2, (rc.Min.Y+rc.Max.Y)/2
	r.eng.SendMouseMove(x, y)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, false)
}

var cfg = Config{Title: "Replace the original?", Message: "The file \"photo.jpg\" will be replaced with the edited picture. This cannot be undone.", Yes: "Replace", Check: "Don't ask again"}

func TestEnterConfirms(t *testing.T) {
	r := newRig(t, 600, 400, cfg)
	r.key(widget.KeyEnter, 0)
	if len(r.results) != 1 || !r.results[0].Yes || r.results[0].Checked {
		t.Fatalf("results %+v", r.results)
	}
	if !r.d.Done() {
		t.Fatal("not closed")
	}
}

func TestEscapeCancels(t *testing.T) {
	r := newRig(t, 600, 400, cfg)
	r.key(widget.KeyEscape, 0)
	if len(r.results) != 1 || r.results[0].Yes {
		t.Fatalf("results %+v", r.results)
	}
}

func TestCheckBoxIsReportedWithTheAnswer(t *testing.T) {
	r := newRig(t, 600, 400, cfg)
	r.click(PartCheck)
	if !r.d.Checked() {
		t.Fatal("click did not check")
	}
	r.click(PartYes)
	if len(r.results) != 1 || !r.results[0].Yes || !r.results[0].Checked {
		t.Fatalf("results %+v", r.results)
	}
}

func TestKeyboardWalk(t *testing.T) {
	r := newRig(t, 600, 400, cfg)
	if r.d.Focused() != PartYes {
		t.Fatalf("focus %d", r.d.Focused())
	}
	r.key(widget.KeyTab, 0)
	if r.d.Focused() != PartCancel {
		t.Fatalf("focus %d", r.d.Focused())
	}
	r.key(widget.KeyTab, 0)
	if r.d.Focused() != PartCheck {
		t.Fatalf("focus %d", r.d.Focused())
	}
	r.key(widget.KeySpace, 0)
	if !r.d.Checked() {
		t.Fatal("space did not check")
	}
	r.key(widget.KeyTab, widget.ModShift)
	r.key(widget.KeyEnter, 0)
	if len(r.results) != 1 || r.results[0].Yes {
		t.Fatalf("Enter on Cancel must cancel: %+v", r.results)
	}
}

func TestWithoutACheckBox(t *testing.T) {
	c := cfg
	c.Check = ""
	r := newRig(t, 600, 400, c)
	if !r.d.Rect(PartCheck).Empty() {
		t.Fatal("check box shown")
	}
	r.key(widget.KeyTab, 0)
	r.key(widget.KeyTab, 0)
	if r.d.Focused() != PartYes {
		t.Fatalf("focus %d", r.d.Focused())
	}
}

func TestFitsEveryScale(t *testing.T) {
	for _, lang := range []string{"en", "ru"} {
		for _, k := range []float64{1, 1.33, 1.6} {
			t.Run(fmt.Sprintf("%s/%.2f", lang, k), func(t *testing.T) {
				old := fonts.TextScale()
				fonts.SetTextScale(k)
				t.Cleanup(func() { fonts.SetTextScale(old) })
				r := newRig(t, 420, 320, cfg)
				b := r.d.Widget().Bounds()
				if b.Min.X < 0 || b.Max.X > 420 || b.Min.Y < 0 || b.Max.Y > 320 {
					t.Fatalf("dialog %v", b)
				}
				for _, p := range []Part{PartCheck, PartYes, PartCancel} {
					if rc := r.d.Rect(p); rc.Empty() || !rc.In(b) {
						t.Errorf("part %d %v outside %v", p, rc, b)
					}
				}
			})
		}
	}
}

func TestAccess(t *testing.T) {
	r := newRig(t, 600, 400, cfg)
	infos := r.d.form.AccessChildren()
	testutil.RequireActionIDs(t, infos)
	if !r.d.form.AccessAction("check", widget.AccessToggle) || !r.d.Checked() {
		t.Fatal("toggle")
	}
	if !r.d.form.AccessAction("cancel", widget.AccessFocus) || r.d.Focused() != PartCancel {
		t.Fatal("focus")
	}
	if r.d.form.AccessAction("yes", widget.AccessToggle) || r.d.form.AccessAction("none", widget.AccessInvoke) {
		t.Fatal("unexpected success")
	}
	if !r.d.form.AccessAction("yes", widget.AccessInvoke) || len(r.results) != 1 || !r.results[0].Yes || !r.results[0].Checked {
		t.Fatalf("invoke: %+v", r.results)
	}
}
