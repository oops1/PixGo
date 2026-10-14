package saveopts

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

func inside(inner, outer image.Rectangle) bool { return !inner.Empty() && inner.In(outer) }

func TestLayoutKeepsEveryControlInsideTheDialogAtEveryScale(t *testing.T) {
	eachCase(t, func(t *testing.T, kind Kind, k float64, lang string) {
		l := compute(kind, 0, 0)
		body := image.Rectangle{Max: l.size}

		parts := map[string]image.Rectangle{"caption": l.caption, "slider": l.slider, "hint": l.hint, "save": l.save, "cancel": l.cancel}
		if kind == WebP {
			parts["check"] = l.check
		}
		for name, r := range parts {
			if !inside(r, body) {
				t.Errorf("%s %v leaves the dialog %v", name, r, body)
			}
		}
		if kind == JPEG && !l.check.Empty() {
			t.Errorf("a JPEG dialog has no checkbox, got %v", l.check)
		}
		order := []string{"check", "caption", "slider", "hint", "save"}
		for i := 0; i < len(order); i++ {
			for j := i + 1; j < len(order); j++ {
				a, ok1 := parts[order[i]]
				b, ok2 := parts[order[j]]
				if ok1 && ok2 && a.Overlaps(b) {
					t.Errorf("%s %v overlaps %s %v", order[i], a, order[j], b)
				}
			}
		}
		if l.save.Overlaps(l.cancel) || l.save.Max.X > l.cancel.Min.X {
			t.Errorf("buttons %v and %v collide or are out of order", l.save, l.cancel)
		}
		if l.save.Min.Y != l.cancel.Min.Y || l.save.Dy() != l.cancel.Dy() {
			t.Errorf("buttons are not aligned: %v %v", l.save, l.cancel)
		}
		if l.size.Y-l.save.Max.Y < l.m.padBottom {
			t.Errorf("the bottom padding %d is below %d", l.size.Y-l.save.Max.Y, l.m.padBottom)
		}
		if l.cancel.Max.X != l.size.X-l.m.padX {
			t.Errorf("buttons are not flush with the right padding: %d vs %d", l.cancel.Max.X, l.size.X-l.m.padX)
		}
	})
}

func TestLayoutTextAlwaysFitsItsBox(t *testing.T) {
	eachCase(t, func(t *testing.T, kind Kind, k float64, lang string) {
		l := compute(kind, 0, 0)

		label := bodyFont.Measure(nil, i18n.T("save.quality"))
		if need := label + l.m.s(8) + l.value.Dx(); need > l.caption.Dx() {
			t.Errorf("caption needs %d, the row offers %d", need, l.caption.Dx())
		}
		if kind == WebP {
			if need := l.m.boxSize + l.m.boxGap + bodyFont.Measure(nil, i18n.T("save.lossless")); need > l.check.Dx() {
				t.Errorf("checkbox row needs %d, has %d", need, l.check.Dx())
			}
		}
		for _, line := range l.hintText {
			if w := hintFont.Measure(nil, line); w > l.hint.Dx() {
				t.Errorf("hint line %q is %d wide in %d", line, w, l.hint.Dx())
			}
			if strings.HasSuffix(line, "…") {
				t.Errorf("hint line %q is cut", line)
			}
		}
		for name, c := range map[string]struct {
			text string
			box  image.Rectangle
		}{
			"save":   {i18n.T("save.options.save"), l.save},
			"cancel": {i18n.T("dialog.cancel"), l.cancel},
		} {
			if w := bodyFont.Measure(nil, c.text); w+2*l.m.btnPad > c.box.Dx() {
				t.Errorf("%s: %q is %d wide plus padding in a %d wide button", name, c.text, w, c.box.Dx())
			}
		}
		if h := textHeight(bodyFont); h+2 > l.save.Dy() {
			t.Errorf("the button is %d high for %d high text", l.save.Dy(), h)
		}
	})
}

func TestSliderRowIsTallEnoughForTheThumbAtEveryScale(t *testing.T) {
	eachCase(t, func(t *testing.T, kind Kind, k float64, lang string) {
		l := compute(kind, 0, 0)

		if l.slider.Dy() < 2*l.m.thumbR+2 {
			t.Errorf("slider row %d high, the thumb is %d", l.slider.Dy(), 2*l.m.thumbR)
		}
		if l.slider.Dx()-2*l.m.thumbR < l.m.s(150) {
			t.Errorf("track is only %d long", l.slider.Dx()-2*l.m.thumbR)
		}
	})
}

func TestDialogGrowsWithTheTextScale(t *testing.T) {
	for _, kind := range kinds {
		var prev image.Point
		for _, k := range scales {
			withScale(t, k, "ru")
			size := compute(kind, 0, 0).size
			if size.X <= prev.X || size.Y <= prev.Y {
				t.Errorf("%s: k=%.2f gives %v, not larger than %v", kindName(kind), k, size, prev)
			}
			prev = size
		}
	}
}

func TestScaleOneUsesTheBaseMetrics(t *testing.T) {
	withScale(t, 1, "ru")
	l := compute(JPEG, 0, 0)

	if l.m.padX != 18 || l.m.btnMinW != 88 || l.m.btnMinH != 30 || l.m.boxSize != 16 || l.m.thumbR != 8 {
		t.Fatalf("base metrics changed: %+v", l.m)
	}
	if l.size.X < 300+2*18 {
		t.Fatalf("dialog is %d wide", l.size.X)
	}
}

func TestNarrowCanvasShrinksTheDialogWithoutBreakingIt(t *testing.T) {
	for _, maxW := range []int{900, 396, 300, 240, 100} {
		eachCase(t, func(t *testing.T, kind Kind, k float64, lang string) {
			l := compute(kind, maxW, 0)
			floor := 2*l.m.padX + l.m.s(160)

			if l.size.X > max(maxW, floor) {
				t.Errorf("maxW %d: dialog is %d wide", maxW, l.size.X)
			}
			body := image.Rectangle{Max: l.size}
			for name, r := range map[string]image.Rectangle{"slider": l.slider, "hint": l.hint, "save": l.save, "cancel": l.cancel, "caption": l.caption} {
				if !inside(r, body) {
					t.Errorf("maxW %d: %s %v leaves the dialog %v", maxW, name, r, body)
				}
			}
			if l.save.Overlaps(l.cancel) {
				t.Errorf("maxW %d: buttons overlap", maxW)
			}
			if len(l.hintText) > hintMaxLines {
				t.Errorf("maxW %d: %d hint lines", maxW, len(l.hintText))
			}
			if maxW >= 396 {
				for name, c := range map[string]struct {
					text string
					box  image.Rectangle
				}{"save": {i18n.T("save.options.save"), l.save}, "cancel": {i18n.T("dialog.cancel"), l.cancel}} {
					if w := bodyFont.Measure(nil, c.text); w > c.box.Dx() {
						t.Errorf("maxW %d: %s label %q is %d wide in a %d wide button", maxW, name, c.text, w, c.box.Dx())
					}
				}
				if need := bodyFont.Measure(nil, i18n.T("save.quality")) + l.value.Dx(); need > l.caption.Dx() {
					t.Errorf("maxW %d: the caption row needs %d of %d", maxW, need, l.caption.Dx())
				}
			}
			for _, line := range l.hintText {
				if w := hintFont.Measure(nil, line); w > l.hint.Dx() {
					t.Errorf("maxW %d: line %q is %d wide in %d", maxW, line, w, l.hint.Dx())
				}
			}
		})
	}
}

func TestNarrowWindowsAtTheLargestTextStillKeepTheHintWhole(t *testing.T) {
	for _, lang := range langs {
		for _, kind := range kinds {
			t.Run(lang+"/"+kindName(kind), func(t *testing.T) {
				withScale(t, 1.6, lang)

				l := compute(kind, 420-2*screenMargin, 0)

				if len(l.hintText) == 0 {
					t.Fatal("no hint")
				}
				for _, line := range l.hintText {
					if strings.HasSuffix(line, "…") {
						t.Errorf("hint line %q is cut", line)
					}
				}
			})
		}
	}
}

func TestShowUsesTheCanvasWidthWhenTheHostKnowsIt(t *testing.T) {
	for _, k := range scales {
		for _, kind := range kinds {
			t.Run(fmt.Sprintf("%s/k%.2f", kindName(kind), k), func(t *testing.T) {
				withScale(t, k, "ru")
				r := newRig(t, kind, Values{Quality: 70}, 420, 320)

				b := r.d.Widget().Bounds()

				if b.Dx() > 420-2*screenMargin || b.Min.X < screenMargin-1 || b.Max.X > 420 {
					t.Fatalf("dialog %v on a 420 wide canvas", b)
				}
				for _, p := range []Part{PartSlider, PartSave, PartCancel} {
					if !inside(r.d.Rect(p), b) {
						t.Errorf("%v %v leaves the dialog %v", p, r.d.Rect(p), b)
					}
				}
			})
		}
	}
}

func TestEveryControlIsClickableAtEveryScale(t *testing.T) {
	eachCase(t, func(t *testing.T, kind Kind, k float64, lang string) {
		r := newRig(t, kind, Values{Quality: 50}, 900, 600)

		for _, p := range []Part{PartCheck, PartSlider, PartSave, PartCancel} {
			if p == PartCheck && kind == JPEG {
				continue
			}
			x, y := centre(r.d.Rect(p))
			if got := r.d.form.hitAt(x, y); got != p {
				t.Errorf("the centre of %v hits %v", p, got)
			}
		}
		x, y := centre(r.d.Rect(PartSlider))
		r.click(x, y)
		if q := r.d.Values().Quality; q < 45 || q > 56 {
			t.Errorf("a click in the middle of the track gave %d", q)
		}
	})
}

func TestRenderingAtEveryScaleStaysInsideTheDialog(t *testing.T) {
	eachCase(t, func(t *testing.T, kind Kind, k float64, lang string) {
		r := newRig(t, kind, Values{Quality: 50}, 900, 600)
		r.press(widget.KeyF1)
		img := r.render()
		b := r.d.Widget().Bounds()
		blank := newRig(t, kind, Values{Quality: 50}, 900, 600).render()

		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				if image.Pt(x, y).In(b.Inset(-12)) {
					continue
				}
				if img.RGBAAt(x, y) != blank.RGBAAt(x, y) {
					t.Fatalf("pixel (%d,%d) differs outside the dialog %v", x, y, b)
				}
			}
		}
	})
}

func TestWrapSplitsAtWordsAndRespectsTheWidth(t *testing.T) {
	text := "Режим «Без потерь» сохраняет каждый пиксель, но файл получается больше"
	for _, width := range []int{400, 250, 160, 90} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			lines := wrap(hintFont, text, width, 20)

			if strings.Join(lines, " ") != text {
				t.Fatalf("words lost or reordered: %q", lines)
			}
			for _, l := range lines {
				if w := hintFont.Measure(nil, l); w > width && len(strings.Fields(l)) > 1 {
					t.Errorf("line %q is %d wide for %d", l, w, width)
				}
			}
		})
	}
}

func TestWrapBreaksAWordLongerThanTheLine(t *testing.T) {
	long := strings.Repeat("W", 60)

	lines := wrap(hintFont, long, 80, 40)

	if len(lines) < 2 || strings.Join(lines, "") != long {
		t.Fatalf("lines = %q", lines)
	}
	for _, l := range lines {
		if hintFont.Measure(nil, l) > 80 {
			t.Errorf("line %q is too wide", l)
		}
	}
}

func TestWrapCutsTheLastLineWhenThereIsNoRoom(t *testing.T) {
	text := strings.Repeat("слово ", 80)

	lines := wrap(hintFont, text, 120, 3)

	if len(lines) != 3 || !strings.HasSuffix(lines[2], "…") {
		t.Fatalf("lines = %q", lines)
	}
	if hintFont.Measure(nil, lines[2]) > 120 {
		t.Fatalf("the ellipsis line is too wide: %q", lines[2])
	}
}

func TestWrapOfNothingGivesNoLines(t *testing.T) {
	for _, s := range []string{"", "   ", "\n\t"} {
		if lines := wrap(hintFont, s, 100, 3); len(lines) != 0 {
			t.Errorf("wrap(%q) = %q", s, lines)
		}
	}
}

func TestShortCanvasSqueezesTheGapsSoTheButtonsStayVisible(t *testing.T) {
	titleH := 34
	for _, canvasH := range []int{320, 280, 240} {
		eachCase(t, func(t *testing.T, kind Kind, k float64, lang string) {
			room := canvasH - 2*screenMargin - titleH
			natural := compute(kind, 420-2*screenMargin, 0)

			l := compute(kind, 420-2*screenMargin, room)

			if natural.size.Y > room && l.size.Y >= natural.size.Y {
				t.Errorf("canvas %d: the dialog was not squeezed (%d of %d)", canvasH, l.size.Y, natural.size.Y)
			}
			if natural.size.Y <= room && l.size != natural.size {
				t.Errorf("canvas %d: a dialog that fits was changed: %v vs %v", canvasH, l.size, natural.size)
			}
			if l.size.Y > max(room, natural.size.Y-natural.gapTotal+int(float64(natural.gapTotal)*minGapShare)+4) {
				t.Errorf("canvas %d: height %d, room %d", canvasH, l.size.Y, room)
			}
			body := image.Rectangle{Max: l.size}
			parts := map[string]image.Rectangle{"caption": l.caption, "slider": l.slider, "hint": l.hint, "save": l.save, "cancel": l.cancel}
			if kind == WebP {
				parts["check"] = l.check
			}
			order := []string{"check", "caption", "slider", "hint", "save"}
			for name, r := range parts {
				if !inside(r, body) {
					t.Errorf("canvas %d: %s %v leaves the dialog %v", canvasH, name, r, body)
				}
			}
			for i := 0; i < len(order); i++ {
				for j := i + 1; j < len(order); j++ {
					a, ok1 := parts[order[i]]
					b, ok2 := parts[order[j]]
					if ok1 && ok2 && a.Overlaps(b) {
						t.Errorf("canvas %d: %s %v overlaps %s %v", canvasH, order[i], a, order[j], b)
					}
				}
			}
		})
	}
}

func TestSqueezeNeverTouchesTheTextSizes(t *testing.T) {
	eachCase(t, func(t *testing.T, kind Kind, k float64, lang string) {
		natural := compute(kind, 396, 0)
		tight := compute(kind, 396, 150)

		if natural.size.X != tight.size.X || natural.save.Dy() != tight.save.Dy() || natural.slider.Dy() != tight.slider.Dy() || natural.caption.Dy() != tight.caption.Dy() {
			t.Fatalf("squeezing changed control sizes: %v vs %v", natural.size, tight.size)
		}
		if tight.squeeze < minGapShare || tight.squeeze > 1 {
			t.Fatalf("squeeze factor %v out of range", tight.squeeze)
		}
	})
}

func TestShowFitsTheShortestWindowAtEveryScale(t *testing.T) {
	for _, kind := range kinds {
		for _, lang := range langs {
			for _, k := range scales {
				t.Run(fmt.Sprintf("%s/%s/k%.2f", kindName(kind), lang, k), func(t *testing.T) {
					withScale(t, k, lang)
					r := newRig(t, kind, Values{Quality: 70}, 420, 320)

					b := r.d.Widget().Bounds()

					if b.Min.Y < 0 || b.Max.Y > 320 || b.Min.X < 0 || b.Max.X > 420 {
						t.Fatalf("the dialog %v does not fit the 420 x 320 canvas", b)
					}
					for _, p := range []Part{PartSave, PartCancel, PartSlider} {
						if !inside(r.d.Rect(p), b) {
							t.Errorf("%v %v leaves the dialog %v", p, r.d.Rect(p), b)
						}
					}
				})
			}
		}
	}
}
