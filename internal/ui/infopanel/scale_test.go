package infopanel

import (
	"fmt"
	"image"
	"image/color"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/meta"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/theme"
)

var textScales = []float64{1.0, 1.2, 1.33, 1.6}

const (
	richName = "IMG_20240914_182703_BURST000_COVER_edited_final_v2_with_a_really_long_name_for_the_panel.jpg"
	richPath = `D:\Photos\2024\Summer trip to the Carpathian mountains\Day 03 - Hoverla ascent\Edited originals\` + richName
)

func setScale(t *testing.T, k float64) {
	t.Helper()
	old := fonts.TextScale()
	t.Cleanup(func() { fonts.SetTextScale(old) })
	fonts.SetTextScale(k)
}

func setLang(t *testing.T, l string) {
	t.Helper()
	old := i18n.Lang()
	t.Cleanup(func() { i18n.SetLang(old) })
	i18n.SetLang(l)
}

func richModel() Model {
	taken := time.Date(2024, 9, 14, 18, 27, 3, 0, time.UTC)
	md := &meta.Metadata{
		Exif: &meta.Exif{
			Make: "Canon", Model: "Canon EOS R5 Mark II", Lens: "RF 24-70mm F2.8 L IS USM with Extender RF 1.4x",
			Exposure: meta.Rational{Num: 1, Den: 250}, FNumber: meta.Rational{Num: 40, Den: 10}, ISO: 12800,
			FocalLength: meta.Rational{Num: 24, Den: 1}, TakenAt: taken, Flash: &meta.Flash{Fired: true},
			Software: "Adobe Photoshop Lightroom Classic 13.2 (Windows)",
		},
		GPS: &meta.GPS{Lat: 48.160271, Lon: 24.500162, Alt: 2061.4, HasAlt: true},
		ICC: &meta.ICC{Description: "Display P3 - Calibrated Wide Gamut Reference Profile", ColorSpace: "RGB"},
	}
	return BuildModel(Input{
		Path: richPath, Name: richName, FileSize: 9_160_000, Modified: taken, Created: taken, HasCreated: true,
		Info: codec.Info{FormatName: "JPEG (JFIF)", Width: 6000, Height: 4000, Orientation: codec.OrientNormal}, Meta: md,
	})
}

func shortNameModel() Model {
	m := richModel()
	m.Name = "mountain_sunrise_2024.jpg"
	return m
}

func noteModel() Model {
	return BuildModel(Input{Path: `D:\a.png`, Name: "a.png", Info: codec.Info{FormatName: "PNG", Width: 2, Height: 2}, Meta: &meta.Metadata{}})
}

func newPanelAt(t *testing.T, m Model, w, h int) (*image.RGBA, *Panel) {
	t.Helper()
	eng := testutil.NewEngine(w, h)
	p := New(eng, theme.Dark())
	host := &frameHost{child: p}
	eng.SetRootFullCanvas(host)
	p.SetBounds(image.Rect(0, 0, w, h))
	p.SetModel(m)
	p.SetThumbnail(photo(600, 400), codec.OrientNormal)
	eng.Flush()
	return eng.RenderOnce(), p
}

func tallPanel(t *testing.T, m Model, w int) (*image.RGBA, *Panel) {
	t.Helper()
	_, probe := newPanelAt(t, m, w, 200)
	return newPanelAt(t, m, w, probe.ContentHeight())
}

type box struct {
	what string
	r    image.Rectangle
}

func textBox(f gfx.Font, s string, x, base int) image.Rectangle {
	m := fonts.MetricsOf(f.Name, f.Size(), 96)
	return image.Rect(x, base-m.Ascent, x+f.Measure(nil, s), base+m.Descent)
}

func headerBoxes(p *Panel) []box {
	pl := p.place()
	var out []box
	out = append(out, box{"header", textBox(headerFont, i18n.T("info.title"), headerX, pl.headerBase)})
	for i, l := range pl.nameLines {
		out = append(out, box{fmt.Sprintf("name%d", i), textBox(nameFont, l, textX, pl.nameBase+i*pl.nameStep)})
	}
	if p.model.Format != "" {
		out = append(out, box{"format", textBox(subFont, subFont.Fit(nil, p.model.Format, pl.textW), textX, pl.formatBase)})
	}
	if p.model.FileSize != "" {
		out = append(out, box{"size", textBox(subFont, subFont.Fit(nil, p.model.FileSize, pl.textW), textX, pl.sizeBase)})
	}
	return out
}

func cardBoxes(cp cardPos) []box {
	var out []box
	if cp.card.Title != "" {
		tf := titleFont
		if cp.card.Kind == CardSizes || cp.card.Kind == CardFile {
			tf = compactTitleFont
		}
		out = append(out, box{"title " + cp.card.Title, textBox(tf, tf.Fit(nil, cp.card.Title, cp.innerW), cp.rect.Min.X+cardPad, cp.rect.Min.Y+cp.title)})
	}
	for _, rp := range cp.rows {
		for i, l := range rp.labelLines {
			out = append(out, box{"label " + l, textBox(labelFont, l, cp.rect.Min.X+cardPad, cp.rect.Min.Y+rp.baseline+i*rp.labelStep)})
		}
		for i, l := range rp.valueLines {
			out = append(out, box{"value " + l, textBox(rp.valueFont, l, cp.rect.Min.X+rp.valueX-2, cp.rect.Min.Y+rp.baseline+i*rp.valueStep)})
		}
		if rp.row.Copy != "" {
			out = append(out, box{"copy", rp.copyRect.Add(cp.rect.Min)})
		}
	}
	return out
}

func TestLayoutAtScaleOneKeepsMockupNumbers(t *testing.T) {
	setScale(t, 1)
	pl := layoutModel(mockupModel(), 326, 1)
	wantTop := []int{116, 188, 288}
	wantBottom := []int{186, 286, 456}
	for i, cp := range pl.cards {
		if cp.rect.Min.Y != wantTop[i] || cp.rect.Max.Y != wantBottom[i] || cp.rect.Min.X != cardX || cp.rect.Max.X != 317 {
			t.Errorf("card %d at %v, want y %d..%d", i, cp.rect, wantTop[i], wantBottom[i])
		}
	}
	camera := pl.cards[2]
	for i, want := range []int{40, 59, 79, 98, 118, 137, 156} {
		if camera.rows[i].baseline != want || camera.rows[i].valueX != 139 {
			t.Errorf("camera row %d baseline %d valueX %d, want %d 139", i, camera.rows[i].baseline, camera.rows[i].valueX, want)
		}
	}
	if pl.height != 468 || pl.headerBase != 26 || pl.nameBase != 59 || pl.formatBase != 80 || pl.sizeBase != 99 {
		t.Errorf("height %d header %d name %d format %d size %d", pl.height, pl.headerBase, pl.nameBase, pl.formatBase, pl.sizeBase)
	}
}

func TestWrapTextRespectsWidthAndLines(t *testing.T) {
	setScale(t, 1)
	f := gfx.UI(8.5)
	rng := rand.New(rand.NewSource(7))
	alphabet := []rune("abcdefghijklmnopqrstuvwxyzАБВГДЕЖЗИКЛМН0123456789 _-\\/.")
	for n := 0; n < 400; n++ {
		rs := make([]rune, rng.Intn(120))
		for i := range rs {
			rs[i] = alphabet[rng.Intn(len(alphabet))]
		}
		width := 30 + rng.Intn(260)
		maxLines := 1 + rng.Intn(5)
		lines := wrapText(f, string(rs), width, maxLines, false)
		if len(lines) > maxLines {
			t.Fatalf("%d lines for max %d: %q", len(lines), maxLines, lines)
		}
		for _, l := range lines {
			if l == "" || strings.HasPrefix(l, " ") || strings.HasSuffix(l, " ") {
				t.Fatalf("bad line %q in %q", l, lines)
			}
			if w := f.Measure(nil, l); w > width && len([]rune(l)) > 1 {
				t.Fatalf("line %q is %d px, wider than %d", l, w, width)
			}
		}
		if len(lines) > 0 && len(lines) < maxLines && strings.HasSuffix(lines[len(lines)-1], "…") {
			t.Fatalf("ellipsis before the line limit: %q", lines)
		}
		if len(lines) < maxLines {
			got := strings.ReplaceAll(strings.Join(lines, ""), " ", "")
			if want := strings.ReplaceAll(strings.TrimSpace(string(rs)), " ", ""); got != want {
				t.Fatalf("text changed: %q -> %q", want, got)
			}
		}
	}
}

func TestWrapTextBreaksPathsAtSeparators(t *testing.T) {
	setScale(t, 1)
	f := gfx.UI(7.75)
	lines := wrapText(f, `D:\Photos\Nature\Mountains\Carpathians\mountain.jpg`, 110, 6, false)
	if len(lines) < 2 {
		t.Fatalf("path was not wrapped: %q", lines)
	}
	for _, l := range lines[:len(lines)-1] {
		if !strings.HasSuffix(l, `\`) {
			t.Errorf("line %q does not end at a separator", l)
		}
	}
	if got := wrapText(f, "short", 200, 3, false); len(got) != 1 || got[0] != "short" {
		t.Errorf("short text: %q", got)
	}
	if got := wrapText(f, "   ", 200, 3, false); len(got) != 0 {
		t.Errorf("blank text: %q", got)
	}
	cut := wrapText(f, strings.Repeat("word ", 60), 120, 2, false)
	if len(cut) != 2 || !strings.HasSuffix(cut[1], "…") {
		t.Errorf("limit not marked with an ellipsis: %q", cut)
	}
	tail := wrapText(f, richPath, 110, 3, true)
	if len(tail) != 3 || !strings.HasPrefix(tail[2], "…") || !strings.HasSuffix(richPath, strings.TrimPrefix(tail[2], "…")) || f.Measure(nil, tail[2]) > 110 {
		t.Errorf("the tail of a long path was not kept: %q", tail)
	}
}

func TestLayoutGrowsWithTheTextScale(t *testing.T) {
	setLang(t, "ru")
	m := richModel()
	prev := 0
	for _, k := range textScales {
		setScale(t, k)
		h := layoutModel(m, 326, k).height
		if h <= prev {
			t.Errorf("k=%.2f height %d did not grow past %d", k, h, prev)
		}
		prev = h
	}
}

func modelNamed(name string) Model {
	switch name {
	case "short":
		return shortNameModel()
	case "note":
		return noteModel()
	}
	return richModel()
}

func TestPanelFitsEveryTextScale(t *testing.T) {
	for _, lang := range []string{"ru", "en"} {
		for _, k := range textScales {
			for _, name := range []string{"rich", "short", "note"} {
				t.Run(fmt.Sprintf("%s/k%.2f/%s", lang, k, name), func(t *testing.T) {
					setLang(t, lang)
					setScale(t, k)
					m := modelNamed(name)
					checkLayout(t, m, 326)
					checkPixels(t, m, 326)
				})
			}
		}
	}
}

func checkLayout(t *testing.T, m Model, w int) {
	t.Helper()
	_, p := tallPanel(t, m, w)
	pl := p.place()
	if pl.height != p.Bounds().Dy() {
		t.Fatalf("content height %d != bounds %d", pl.height, p.Bounds().Dy())
	}
	boxes := headerBoxes(p)
	thumb := image.Rect(thumbX, thumbY, thumbX+thumbW, thumbY+thumbH)
	boxes = append(boxes, box{"thumb", thumb})
	firstTop := 1 << 30
	if len(pl.cards) > 0 {
		firstTop = pl.cards[0].rect.Min.Y
	}
	for _, b := range boxes {
		if b.r.Max.X > w || b.r.Min.X < 0 || b.r.Min.Y < 0 || b.r.Max.Y > firstTop {
			t.Errorf("%s %v leaves the header block (cards start at %d, width %d)", b.what, b.r, firstTop, w)
		}
		if b.what != "header" && b.what != "thumb" && b.r.Max.X > textX+pl.textW+textSlack {
			t.Errorf("%s %v is wider than the text column", b.what, b.r)
		}
	}
	if h := boxes[0].r; h.Max.X > closeBoxX-8 {
		t.Errorf("header %v runs into the close button", h)
	}
	disjoint(t, boxes)
	prevBottom := 0
	for _, cp := range pl.cards {
		if cp.rect.Min.X < 0 || cp.rect.Max.X > w {
			t.Errorf("card %v leaves the panel width %d", cp.rect, w)
		}
		if cp.rect.Min.Y < prevBottom {
			t.Errorf("card %v overlaps the previous one ending at %d", cp.rect, prevBottom)
		}
		prevBottom = cp.rect.Max.Y
		cb := cardBoxes(cp)
		disjoint(t, cb)
		inner := image.Rect(cp.rect.Min.X+cardPad, cp.rect.Min.Y, cp.rect.Max.X-cardPad+textSlack, cp.rect.Max.Y)
		for _, b := range cb {
			if b.what != "copy" && !b.r.In(inner) {
				t.Errorf("%s %v is outside the card interior %v", b.what, b.r, inner)
			}
		}
		for _, rp := range cp.rows {
			checkRow(t, cp, rp)
		}
	}
	if len(pl.cards) > 0 && pl.height != pl.cards[len(pl.cards)-1].rect.Max.Y+bottomMargin {
		t.Errorf("height %d does not end with the last card %v", pl.height, pl.cards[len(pl.cards)-1].rect)
	}
	checkTruncation(t, m, pl)
}

func disjoint(t *testing.T, boxes []box) {
	t.Helper()
	for i := range boxes {
		for j := i + 1; j < len(boxes); j++ {
			if boxes[i].r.Overlaps(boxes[j].r) {
				t.Errorf("%s %v overlaps %s %v", boxes[i].what, boxes[i].r, boxes[j].what, boxes[j].r)
			}
		}
	}
}

func checkRow(t *testing.T, cp cardPos, rp rowPos) {
	t.Helper()
	if cp.card.Kind != CardNote && rp.valueX-cardPad-valuePad < 0 {
		t.Errorf("row %q: negative label column", rp.row.Label)
	}
	if rp.row.Copy != "" && !rp.copyRect.In(image.Rect(0, 0, cp.rect.Dx(), cp.rect.Dy())) {
		t.Errorf("row %q: copy button %v leaves the card %v", rp.row.Label, rp.copyRect, cp.rect)
	}
	if !rp.row.Link {
		return
	}
	if !rp.linkRect.In(image.Rect(0, 0, cp.rect.Dx(), cp.rect.Dy())) {
		t.Errorf("row %q: link rect %v leaves the card %v", rp.row.Label, rp.linkRect, cp.rect)
	}
	if rp.row.Copy != "" && rp.linkRect.Overlaps(rp.copyRect) {
		t.Errorf("row %q: link %v overlaps the copy button %v", rp.row.Label, rp.linkRect, rp.copyRect)
	}
	for i, l := range rp.valueLines {
		b := textBox(rp.valueFont, l, rp.valueX-2, rp.baseline+i*rp.valueStep)
		if b.Min.X < rp.linkRect.Min.X-2 || b.Max.X > rp.linkRect.Max.X+textSlack || b.Min.Y < rp.linkRect.Min.Y || b.Max.Y > rp.linkRect.Max.Y+2 {
			t.Errorf("row %q: link line %d %v is not covered by the hit rect %v", rp.row.Label, i, b, rp.linkRect)
		}
	}
}

func squash(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), "…", "") }

func checkTruncation(t *testing.T, m Model, pl placed) {
	t.Helper()
	if len(pl.nameLines) > 0 {
		last := pl.nameLines[len(pl.nameLines)-1]
		if !strings.HasSuffix(last, "…") && squash(strings.Join(pl.nameLines, "")) != squash(m.Name) {
			t.Errorf("name lost text without an ellipsis: %q for %q", pl.nameLines, m.Name)
		}
		if strings.HasSuffix(last, "…") && len(pl.nameLines) != maxNameLines {
			t.Errorf("name ellipsised before the line limit: %q", pl.nameLines)
		}
	}
	for _, cp := range pl.cards {
		for _, rp := range cp.rows {
			labelLimit, valueLimit := maxLabelLines, maxValueLines
			if cp.card.Kind == CardNote {
				labelLimit = maxNoteLines
			}
			if rp.row.Link || rp.row.Copy != "" {
				valueLimit = maxPathLines
			}
			checkLines(t, rp.row.Label, rp.labelLines, rp.row.Label, labelLimit)
			checkLines(t, rp.row.Label, rp.valueLines, rp.row.Value, valueLimit)
		}
	}
}

func checkLines(t *testing.T, row string, lines []string, src string, limit int) {
	t.Helper()
	if len(lines) == 0 {
		if strings.TrimSpace(src) != "" {
			t.Errorf("row %q: %q vanished", row, src)
		}
		return
	}
	last := lines[len(lines)-1]
	ell := strings.HasSuffix(last, "…") || strings.HasPrefix(last, "…")
	if !ell && squash(strings.Join(lines, "")) != squash(src) {
		t.Errorf("row %q lost text without an ellipsis: %q for %q", row, lines, src)
	}
	if ell && len(lines) != limit {
		t.Errorf("row %q ellipsised at %d of %d lines: %q", row, len(lines), limit, lines)
	}
}

func differs(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) > tol || d(a.G, b.G) > tol || d(a.B, b.B) > tol
}

func checkPixels(t *testing.T, m Model, w int) {
	t.Helper()
	img, p := tallPanel(t, m, w)
	pl := p.place()
	th := p.Theme
	pad := func(r image.Rectangle) image.Rectangle { return r.Inset(-3) }

	head := headerBoxes(p)
	head = append(head,
		box{"thumb", image.Rect(thumbX, thumbY, thumbX+thumbW, thumbY+thumbH)},
		box{"close", image.Rect(closeBoxX-8, closeBoxY-8, closeBoxX-8+closeHit, closeBoxY-8+closeHit)},
	)
	for _, b := range head {
		if b.what == "thumb" || b.what == "close" {
			continue
		}
		if ink := inkAgainst(img, b.r.Inset(-1).Intersect(img.Bounds()), th.Panel); ink.Empty() {
			t.Errorf("%s %v has no ink", b.what, b.r)
		}
	}
	top := 0
	if len(pl.cards) > 0 {
		top = pl.cards[0].rect.Min.Y - 2
	} else {
		top = img.Bounds().Dy()
	}
	strayInk(t, img, image.Rect(0, 0, w, top), th.Panel, head, pad, "header block")

	for _, cp := range pl.cards {
		boxes := cardBoxes(cp)
		for _, b := range boxes {
			if b.what == "copy" {
				continue
			}
			if ink := inkAgainst(img, b.r.Inset(-1).Intersect(img.Bounds()), th.PanelCard); ink.Empty() {
				t.Errorf("%s %v has no ink", b.what, b.r)
			}
		}
		strayInk(t, img, cp.rect.Inset(4), th.PanelCard, boxes, pad, fmt.Sprintf("card %q", cp.card.Title))
		if ink := inkAgainst(img, cp.rect.Inset(3), th.PanelCard); !ink.Empty() && (ink.Max.X > cp.rect.Max.X-cardPad+textSlack+2 || ink.Min.X < cp.rect.Min.X+cardPad-2) {
			t.Errorf("card %q: ink %v crosses the card padding (rect %v)", cp.card.Title, ink, cp.rect)
		}
	}
}

func inkAgainst(img *image.RGBA, r image.Rectangle, bg color.RGBA) image.Rectangle {
	out := image.Rectangle{}
	r = r.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if differs(img.RGBAAt(x, y), bg, 6) {
				out = out.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return out
}

func strayInk(t *testing.T, img *image.RGBA, area image.Rectangle, bg color.RGBA, known []box, pad func(image.Rectangle) image.Rectangle, what string) {
	t.Helper()
	area = area.Intersect(img.Bounds())
	for y := area.Min.Y; y < area.Max.Y; y++ {
	px:
		for x := area.Min.X; x < area.Max.X; x++ {
			if !differs(img.RGBAAt(x, y), bg, 6) {
				continue
			}
			for _, b := range known {
				if image.Pt(x, y).In(pad(b.r)) {
					continue px
				}
			}
			t.Errorf("%s: stray ink at (%d,%d) outside every known box", what, x, y)
			return
		}
	}
}

func TestLastCardIsFullyReachableAtEveryTextScale(t *testing.T) {
	for _, lang := range []string{"ru", "en"} {
		for _, k := range textScales {
			t.Run(fmt.Sprintf("%s/k%.2f", lang, k), func(t *testing.T) {
				setLang(t, lang)
				setScale(t, k)
				_, p := newPanelAt(t, richModel(), 326, 454)
				if p.ContentHeight() <= p.Bounds().Dy() {
					t.Fatalf("rich content %d fits %d: nothing to scroll", p.ContentHeight(), p.Bounds().Dy())
				}
				p.OnMouseWheelPixelsMod(100, 100, 0, 1e6, 0)
				want := p.ContentHeight() - p.Bounds().Dy()
				if p.ScrollY() != want {
					t.Fatalf("scroll %d, want %d", p.ScrollY(), want)
				}
				p.OnMouseWheelPixelsMod(100, 100, 0, 1e6, 0)
				if p.ScrollY() != want {
					t.Fatalf("scrolled past the end: %d > %d", p.ScrollY(), want)
				}
				cards := p.place().cards
				last := cards[len(cards)-1]
				bottom := p.Bounds().Min.Y + last.rect.Max.Y - p.ScrollY()
				if bottom != p.Bounds().Max.Y-bottomMargin {
					t.Errorf("last card ends at %d, want %d", bottom, p.Bounds().Max.Y-bottomMargin)
				}
				top := p.Bounds().Min.Y + last.rect.Min.Y - p.ScrollY()
				if top < p.Bounds().Min.Y {
					t.Errorf("last card starts above the viewport at %d", top)
				}
				img := renderAgain(t, p)
				for _, b := range cardBoxes(last) {
					if b.what == "copy" {
						continue
					}
					r := b.r.Add(image.Pt(0, -p.ScrollY()))
					if !r.In(p.Bounds()) {
						t.Errorf("%s %v is outside the viewport %v", b.what, r, p.Bounds())
						continue
					}
					if ink := inkAgainst(img, r.Inset(-1), p.Theme.PanelCard); ink.Empty() {
						t.Errorf("%s %v has no ink after scrolling to the end", b.what, r)
					}
				}
			})
		}
	}
}

func renderAgain(t *testing.T, p *Panel) *image.RGBA {
	t.Helper()
	type renderer interface {
		Flush()
		RenderOnce() *image.RGBA
	}
	r, ok := p.eng.(renderer)
	if !ok {
		t.Fatal("engine cannot render")
	}
	p.invalidate()
	r.Flush()
	return r.RenderOnce()
}

func TestClicksFollowTheScaledRects(t *testing.T) {
	for _, lang := range []string{"ru", "en"} {
		for _, k := range textScales {
			for _, scroll := range []int{0, 90, 1 << 20} {
				t.Run(fmt.Sprintf("%s/k%.2f/scroll%d", lang, k, scroll), func(t *testing.T) {
					setLang(t, lang)
					setScale(t, k)
					_, p := newPanelAt(t, richModel(), 326, 454)
					p.OnMouseWheelPixelsMod(100, 100, 0, float64(scroll), 0)
					var closed, revealed int
					var copied, opened string
					p.OnClose = func() { closed++ }
					p.OnReveal = func() { revealed++ }
					p.OnCopy = func(s string) { copied = s }
					p.OnOpenURL = func(s string) { opened = s }
					reset := func() { closed, revealed, copied, opened = 0, 0, "", "" }
					view := p.Bounds()
					seen := map[string]bool{}
					for _, cp := range p.place().cards {
						for _, rp := range cp.rows {
							org := image.Pt(view.Min.X+cp.rect.Min.X, view.Min.Y+cp.rect.Min.Y-p.ScrollY())
							if rp.row.Copy != "" {
								reset()
								pt := rp.copyRect.Add(org).Min.Add(image.Pt(rp.copyRect.Dx()/2, rp.copyRect.Dy()/2))
								if pt.In(view) {
									click(p, pt.X, pt.Y)
									if copied != richPath || revealed != 0 {
										t.Errorf("copy button at %v: copied=%q revealed=%d", pt, copied, revealed)
									}
									seen["copy"] = true
								}
							}
							if rp.row.Link {
								for i := range rp.valueLines {
									reset()
									pt := image.Pt(org.X+rp.valueX+10, org.Y+rp.baseline+i*rp.valueStep-4)
									if !pt.In(view) {
										continue
									}
									click(p, pt.X, pt.Y)
									if rp.row.URL == "" && (revealed != 1 || opened != "") || rp.row.URL != "" && (opened != rp.row.URL || revealed != 0) {
										t.Errorf("link %q line %d at %v: revealed=%d opened=%q", rp.row.Label, i, pt, revealed, opened)
									}
									seen[rp.row.Label] = true
								}
							}
							if !rp.row.Link && rp.row.Copy == "" && len(rp.valueLines) > 0 {
								reset()
								pt := image.Pt(org.X+rp.valueX+10, org.Y+rp.baseline-4)
								if pt.In(view) && pt.Y > view.Min.Y+closeBoxY+closeHit {
									click(p, pt.X, pt.Y)
									if closed+revealed != 0 || copied != "" || opened != "" {
										t.Errorf("plain row %q at %v triggered an action", rp.row.Label, pt)
									}
									seen["plain"] = true
								}
								reset()
								lp := image.Pt(org.X+cardPad+2, org.Y+rp.baseline-4)
								if lp.In(view) && lp.Y > view.Min.Y+closeBoxY+closeHit {
									click(p, lp.X, lp.Y)
									if closed+revealed != 0 || copied != "" || opened != "" {
										t.Errorf("label %q at %v triggered an action", rp.row.Label, lp)
									}
								}
							}
						}
					}
					if scroll == 0 && (!seen["copy"] || len(seen) < 2) {
						t.Errorf("nothing clickable was visible at the top: %v", seen)
					}
					if scroll > 1<<10 && !seen[i18n.T("info.openMap")] {
						t.Errorf("the map link was not reachable at the end: %v", seen)
					}
				})
			}
		}
	}
}

func TestHitRectsCoverTheDrawnLinkText(t *testing.T) {
	for _, lang := range []string{"ru", "en"} {
		for _, k := range textScales {
			t.Run(fmt.Sprintf("%s/k%.2f", lang, k), func(t *testing.T) {
				setLang(t, lang)
				setScale(t, k)
				img, p := tallPanel(t, richModel(), 326)
				found := 0
				for _, cp := range p.place().cards {
					for _, rp := range cp.rows {
						if !rp.row.Link {
							continue
						}
						rect := rp.linkRect.Add(cp.rect.Min)
						text := image.Rectangle{}
						for i, l := range rp.valueLines {
							text = text.Union(textBox(rp.valueFont, l, cp.rect.Min.X+rp.valueX-2, cp.rect.Min.Y+rp.baseline+i*rp.valueStep).Inset(-2))
						}
						ink := inkAgainst(img, text.Intersect(cp.rect.Inset(3)), p.Theme.PanelCard)
						if ink.Empty() {
							t.Errorf("link %q: no text found in %v", rp.row.Label, text)
							continue
						}
						found++
						if ink.Min.Y < rect.Min.Y || ink.Max.Y > rect.Max.Y || ink.Min.X < rect.Min.X-3 || ink.Max.X > rect.Max.X+textSlack+2 {
							t.Errorf("link %q: drawn text %v sticks out of the hit rect %v", rp.row.Label, ink, rect)
						}
						if rp.row.Copy != "" {
							icon := inkAgainst(img, rp.copyRect.Add(cp.rect.Min), p.Theme.PanelCard)
							if icon.Empty() || !icon.In(rp.copyRect.Add(cp.rect.Min)) {
								t.Errorf("copy icon ink %v is not inside its hit rect %v", icon, rp.copyRect.Add(cp.rect.Min))
							}
						}
					}
				}
				if found != 2 {
					t.Errorf("found %d link rows, want 2", found)
				}
			})
		}
	}
}

func TestWheelStepFollowsTheTextScale(t *testing.T) {
	for _, k := range textScales {
		setScale(t, k)
		_, p := newPanelAt(t, richModel(), 326, 454)
		p.OnMouseButton(widget.MouseEvent{X: 100, Y: 100, Button: widget.MouseWheelDown})
		if want := scaled(scrollStep, k); p.ScrollY() != want {
			t.Errorf("k=%.2f: one tick scrolled %d, want %d", k, p.ScrollY(), want)
		}
	}
}

func TestTextScaleChangeRelaysOutTheCachedPanel(t *testing.T) {
	setLang(t, "ru")
	setScale(t, 1)
	_, p := newPanelAt(t, richModel(), 326, 454)
	h1 := p.ContentHeight()
	fonts.SetTextScale(1.4)
	if h2 := p.ContentHeight(); h2 <= h1 {
		t.Errorf("height stayed %d after the text scale grew (was %d)", h2, h1)
	}
	fonts.SetTextScale(1)
	if h3 := p.ContentHeight(); h3 != h1 {
		t.Errorf("height %d after returning to scale 1, want %d", h3, h1)
	}
}

func TestPanelStaysUsableAtOtherWidths(t *testing.T) {
	setLang(t, "ru")
	for _, k := range []float64{1, 1.33, 1.6} {
		for _, w := range []int{240, 300, 420} {
			t.Run(fmt.Sprintf("k%.2f/w%d", k, w), func(t *testing.T) {
				setScale(t, k)
				checkLayout(t, richModel(), w)
			})
		}
	}
}

func TestSnapshotsAtScale(t *testing.T) {
	out := testutil.OutDir()
	if out == "" {
		t.Skip("PIXGO_SNAPSHOTS is not set")
	}
	for _, lang := range []string{"ru", "en"} {
		for _, k := range []float64{1.2, 1.6} {
			setLang(t, lang)
			setScale(t, k)
			tag := fmt.Sprintf("infopanel_k%02d_%s", int(k*10+.5), lang)
			full, _ := tallPanel(t, richModel(), 326)
			testutil.SavePNG(filepath.Join(out, tag+"_full.png"), full)
			top, p := newPanelAt(t, richModel(), 326, 454)
			testutil.SavePNG(filepath.Join(out, tag+"_top.png"), top)
			p.OnMouseWheelPixelsMod(100, 100, 0, 1e6, 0)
			testutil.SavePNG(filepath.Join(out, tag+"_end.png"), renderAgain(t, p))
		}
	}
}
