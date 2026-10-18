package saveopts

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

func TestGPSCheckBoxIsOptIn(t *testing.T) {
	for _, kind := range kinds {
		plain := newRig(t, kind, Values{Quality: 70}, 760, 420)
		if plain.d.Enabled(PartStripGPS) || !plain.d.Rect(PartStripGPS).Empty() {
			t.Errorf("%s: the GPS box appeared without the flag", kindName(kind))
		}
		with := newRig(t, kind|GPS, Values{Quality: 70}, 760, 420)
		if !with.d.Enabled(PartStripGPS) || with.d.Rect(PartStripGPS).Empty() {
			t.Errorf("%s: the GPS box is missing", kindName(kind))
		}
		if with.d.Kind() != kind {
			t.Errorf("Kind() = %v, want %v", with.d.Kind(), kind)
		}
	}
}

func TestGPSCheckBoxDefaultsOffAndToggles(t *testing.T) {
	r := newRig(t, JPEG|GPS, Values{Quality: 70}, 760, 420)
	if r.d.Values().StripGPS {
		t.Fatal("must be off by default")
	}
	r.clickPart(PartStripGPS)
	if !r.d.Values().StripGPS {
		t.Fatal("click did not turn it on")
	}
	r.press(widget.KeyEnter)
	if got := r.last(); !got.ok || !got.values.StripGPS || got.values.Quality != 70 {
		t.Fatalf("result %+v", got)
	}
}

func TestGPSCheckBoxStartsOnFromTheSavedChoice(t *testing.T) {
	r := newRig(t, WebP|GPS, Values{Quality: 70, StripGPS: true}, 760, 420)
	if !r.d.Values().StripGPS {
		t.Fatal("initial value lost")
	}
	off := newRig(t, WebP, Values{Quality: 70, StripGPS: true}, 760, 420)
	if off.d.Values().StripGPS {
		t.Fatal("a dialog without the box cannot report GPS stripping")
	}
}

func TestGPSCheckBoxSpaceTogglesAndTabReachesIt(t *testing.T) {
	r := newRig(t, JPEG|GPS, Values{Quality: 70}, 760, 420)
	if r.d.Focused() != PartStripGPS && r.d.Focused() != PartSlider {
		t.Fatalf("focus %d", r.d.Focused())
	}
	for i := 0; i < 6 && r.d.Focused() != PartStripGPS; i++ {
		r.press(widget.KeyTab)
	}
	if r.d.Focused() != PartStripGPS {
		t.Fatal("tab never reached the GPS box")
	}
	r.press(widget.KeySpace)
	if !r.d.Values().StripGPS {
		t.Fatal("space did not toggle")
	}
	r.press(widget.KeySpace)
	if r.d.Values().StripGPS {
		t.Fatal("space did not toggle back")
	}
}

func TestLosslessWatchReportsTheSwitch(t *testing.T) {
	eng := testutil.NewEngine(760, 420)
	t.Cleanup(eng.Stop)
	backdrop := widget.NewPanel(color.RGBA{R: 32, G: 32, B: 32, A: 255})
	backdrop.SetBounds(image.Rect(0, 0, 760, 420))
	eng.SetRoot(backdrop)
	var seen []bool
	d := Show(eng, JPEGLossless|GPS, Values{Quality: 70, Lossless: true}, func(Values, bool) {}, WithLosslessWatch(func(on bool) { seen = append(seen, on) }))
	t.Cleanup(func() { eng.CloseModal(nil) })
	eng.Flush()
	if len(seen) != 1 || !seen[0] {
		t.Fatalf("the initial state was not reported: %v", seen)
	}
	x, y := centre(d.Rect(PartCheck))
	eng.SendMouseMove(x, y)
	eng.SendMouseButton(x, y, widget.MouseLeft, true)
	eng.SendMouseButton(x, y, widget.MouseLeft, false)
	if len(seen) != 2 || seen[1] {
		t.Fatalf("the switch was not reported: %v", seen)
	}
}

func TestGPSCheckBoxAccessibility(t *testing.T) {
	r := newRig(t, JPEG|GPS, Values{Quality: 70}, 760, 420)
	infos := r.d.form.AccessChildren()
	testutil.RequireActionIDs(t, infos)
	var found bool
	for _, in := range infos {
		if in.ID == partIDs[PartStripGPS] {
			found = true
			if in.Role != widget.RoleCheckBox || in.Name != i18n.T("save.stripGPS") {
				t.Fatalf("node %+v", in)
			}
		}
	}
	if !found {
		t.Fatal("no node for the GPS box")
	}
	if !r.d.form.AccessAction(partIDs[PartStripGPS], widget.AccessToggle) || !r.d.Values().StripGPS {
		t.Fatal("toggle")
	}
	if !r.d.form.AccessAction(partIDs[PartStripGPS], widget.AccessInvoke) || r.d.Values().StripGPS {
		t.Fatal("invoke")
	}
	plain := newRig(t, JPEG, Values{Quality: 70}, 760, 420)
	if plain.d.form.AccessAction(partIDs[PartStripGPS], widget.AccessToggle) {
		t.Fatal("a dialog without the box accepted the action")
	}
}

func TestGPSDialogFitsEveryScale(t *testing.T) {
	for _, kind := range kinds {
		for _, lang := range langs {
			for _, k := range scales {
				t.Run(fmt.Sprintf("%s/%s/k%.2f", kindName(kind), lang, k), func(t *testing.T) {
					withScale(t, k, lang)
					r := newRig(t, kind|GPS, Values{Quality: 70}, 420, 320)
					if b := r.d.Widget().Bounds(); b.Min.Y < 0 || b.Max.Y > 320 || b.Min.X < 0 || b.Max.X > 420 {
						t.Fatalf("the dialog %v does not fit the canvas", b)
					}
					box := r.d.Rect(PartStripGPS)
					if box.Empty() {
						t.Fatal("no GPS rect")
					}
					for _, p := range []Part{PartCheck, PartSlider, PartSave, PartCancel} {
						o := r.d.Rect(p)
						if !o.Empty() && o.Overlaps(box) {
							t.Errorf("part %d %v overlaps the GPS box %v", p, o, box)
						}
					}
				})
			}
		}
	}
}
