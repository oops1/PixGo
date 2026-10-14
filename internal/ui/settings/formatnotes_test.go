package settings

import (
	"fmt"
	"image"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

func notedFormats() []Format {
	return []Format{
		{ID: "jpeg", Name: "JPEG", Extensions: []string{"JPG", "JPEG"}, Available: true, Enabled: true},
		{ID: "apng", Name: i18n.T("settings.format.apng"), Extensions: []string{"APNG"}, Optional: true, Available: true, Enabled: true},
		{ID: "avif", Name: i18n.T("settings.format.avif"), Extensions: []string{"AVIF"}, Optional: true, Available: true, Enabled: true},
		{ID: "heic", Name: i18n.T("settings.format.heic"), Extensions: []string{"HEIC", "HEIF"}, Note: i18n.T("settings.format.heic.note"), Optional: true, Available: true, Enabled: true},
		{ID: "raw", Name: i18n.T("settings.format.raw"), Extensions: []string{"CR2", "NEF", "ARW", "DNG"}, Note: i18n.T("settings.format.raw.note"), Optional: true, Available: true, Enabled: false},
		{ID: "jxl", Name: i18n.T("settings.format.jxl"), Extensions: []string{"JXL"}, Optional: true},
	}
}

func newNotedRig(t testing.TB, lang string, w, h int) *rig {
	t.Helper()
	r := newScaledRig(t, lang, w, h)
	r.src.formats = notedFormats()
	r.page.Refresh()
	r.page.SetSection(SectionFormats)
	return r
}

func notesOf(r *rig) []*item {
	var out []*item
	for i := 1; i < len(r.page.items); i++ {
		if it := &r.page.items[i]; it.kind == kindNote && r.page.items[i-1].kind == kindFormat {
			out = append(out, it)
		}
	}
	return out
}

func indexOfItem(r *rig, id string) int {
	for i := range r.page.items {
		if r.page.items[i].id == id {
			return i
		}
	}
	return -1
}

func TestFormatNoteIsShownRightUnderItsRow(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"format:heic", "settings.format.heic.note"},
		{"format:raw", "settings.format.raw.note"},
	}
	for _, lang := range scaleLangs {
		for _, tc := range tests {
			t.Run(lang+"/"+tc.id, func(t *testing.T) {
				r := newNotedRig(t, lang, 700, 600)

				at := indexOfItem(r, tc.id)
				if at < 0 || at+1 >= len(r.page.items) {
					t.Fatalf("row %s not found", tc.id)
				}
				row, note := r.page.items[at], r.page.items[at+1]

				if note.kind != kindNote {
					t.Fatalf("the item after %s is %v, want its note", tc.id, note.kind)
				}
				if got, want := joinLines(note.lines), joinLines(wrapLines(noteFont, i18n.T(tc.want), r.page.geo.xt-r.page.geo.x0, noteLinesLong)); got != want {
					t.Errorf("note = %q, want %q", got, want)
				}
				if note.r.Min.Y < row.r.Max.Y || note.r.Min.X != row.r.Min.X {
					t.Errorf("note %v must start under the row %v", note.r, row.r)
				}
			})
		}
	}
}

func TestOnlyRowsWithANoteGetOne(t *testing.T) {
	r := newNotedRig(t, "ru", 700, 600)

	var withNote []string
	for i := range r.page.items {
		if r.page.items[i].kind == kindFormat && i+1 < len(r.page.items) && r.page.items[i+1].kind == kindNote {
			withNote = append(withNote, r.page.items[i].id)
		}
	}

	if strings.Join(withNote, ",") != "format:heic,format:raw" {
		t.Fatalf("rows followed by a note: %v", withNote)
	}
}

func TestRowsWithoutANoteKeepTheMockupPitch(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionFormats)

	first, second := r.item("format:jpeg"), r.item("format:png")

	if got, want := second.r.Min.Y-first.r.Min.Y, dim.formatRowHeight+dim.formatGap; got != want {
		t.Fatalf("pitch %d, want %d", got, want)
	}
}

func TestANoteMovesTheFollowingRowsDownByItsHeight(t *testing.T) {
	plain := newNotedRig(t, "ru", 700, 600)
	for i := range plain.src.formats {
		plain.src.formats[i].Note = ""
	}
	plain.page.Refresh()
	noted := newNotedRig(t, "ru", 700, 600)

	shift := noted.item("format:jxl").r.Min.Y - plain.item("format:jxl").r.Min.Y

	want := 0
	for _, n := range notesOf(noted) {
		want += n.r.Dy() + int(2*dim.k+.5)
	}
	if shift != want || shift <= 0 {
		t.Fatalf("the rows after the notes moved by %d, want %d", shift, want)
	}
}

func TestFormatNotesAreNeverCutAtEveryScale(t *testing.T) {
	eachScale(t, func(t *testing.T, k float64, lang string) {
		for _, size := range []image.Point{scaledSize(k, 420, 320), scaledSize(k, 457, 430), {1100, 800}} {
			r := newNotedRig(t, lang, size.X, size.Y)

			notes := notesOf(r)
			if len(notes) < 2 {
				t.Fatalf("%v: %d notes", size, len(notes))
			}
			for _, n := range notes {
				if len(n.lines) == 0 || len(n.lines) > noteLinesLong {
					t.Errorf("%v: note has %d lines", size, len(n.lines))
				}
				for _, l := range n.lines {
					if hasEllipsis(l) {
						t.Errorf("%v: line %q is cut", size, l)
					}
				}
			}
			assertLayout(t, r, fmt.Sprintf("%v/formats", size))
		}
	})
}

func TestFormatNotesStayInsideTheColumnAndBelowTheirRowAtEveryScale(t *testing.T) {
	eachScale(t, func(t *testing.T, k float64, lang string) {
		size := scaledSize(k, 420, 320)
		r := newNotedRig(t, lang, size.X, size.Y)
		g := r.page.geo

		for _, id := range []string{"format:heic", "format:raw"} {
			at := indexOfItem(r, id)
			row, note := r.page.items[at], r.page.items[at+1]
			if note.r.Min.X < g.x0 || note.r.Max.X > g.xt {
				t.Errorf("%s: note %v leaves the column %d..%d", id, note.r, g.x0, g.xt)
			}
			for _, l := range note.lines {
				if w := measure(noteFont, l); w > g.xt-g.x0 {
					t.Errorf("%s: line %q is %d wide, the column offers %d", id, l, w, g.xt-g.x0)
				}
			}
			if gap := note.r.Min.Y - row.r.Max.Y; gap < 0 {
				t.Errorf("%s: the note overlaps its row by %d", id, -gap)
			}
			if at+2 < len(r.page.items) && r.page.items[at+2].r.Min.Y < note.r.Max.Y {
				t.Errorf("%s: the next item %v starts inside the note %v", id, r.page.items[at+2].r, note.r)
			}
		}
	})
}

func TestFormatNotesAreDrawnAtEveryScale(t *testing.T) {
	eachScale(t, func(t *testing.T, k float64, lang string) {
		size := scaledSize(k, 457, 430)
		r := newNotedRig(t, lang, size.X, size.Y)
		with := r.render()
		for i := range r.src.formats {
			r.src.formats[i].Note = ""
		}
		r.page.Refresh()
		without := r.render()

		note := notesOf(r)
		if len(note) != 0 {
			t.Fatalf("notes stayed after clearing them: %d", len(note))
		}
		rect := image.Rect(r.page.geo.x0, r.page.geo.view.Min.Y, r.page.geo.xt, r.page.geo.view.Max.Y)
		if sameImage(with, without, rect) {
			t.Fatal("the notes left no pixels")
		}
	})
}

func TestNotesAreAnnouncedToAssistiveTechnology(t *testing.T) {
	r := newNotedRig(t, "ru", 700, 600)

	var seen int
	for _, a := range r.page.AccessChildren() {
		if a.Role == widget.RoleLabel && (strings.Contains(a.Name, "HEVC") || strings.Contains(a.Name, "предварительный просмотр")) {
			seen++
		}
	}

	if seen != 2 {
		t.Fatalf("%d of the 2 notes are announced", seen)
	}
}

func TestNotesDoNotAddFocusStops(t *testing.T) {
	plain := newNotedRig(t, "ru", 700, 600)
	for i := range plain.src.formats {
		plain.src.formats[i].Note = ""
	}
	plain.page.Refresh()
	noted := newNotedRig(t, "ru", 700, 600)

	if got, want := len(noted.page.stops()), len(plain.page.stops()); got != want {
		t.Fatalf("%d focus stops with notes, %d without", got, want)
	}
}

func TestSwitchingANotedFormatStillReachesTheStore(t *testing.T) {
	r := newNotedRig(t, "ru", 700, 600)

	r.clickItem("format:heic")
	r.clickItem("format:raw")

	if r.src.cfg.Formats["heic"].Enabled || !r.src.cfg.Formats["raw"].Enabled {
		t.Fatalf("heic %v raw %v", r.src.cfg.Formats["heic"].Enabled, r.src.cfg.Formats["raw"].Enabled)
	}
}

func TestUnavailableFormatKeepsItsNoteOutWhenTheSourceSendsNone(t *testing.T) {
	r := newNotedRig(t, "en", 700, 600)

	jxl := r.item("format:jxl")
	at := indexOfItem(r, "format:jxl")

	if !jxl.disabled || at+1 < len(r.page.items) && r.page.items[at+1].kind == kindNote && r.page.items[at+1].r.Min.Y < jxl.r.Max.Y+int(2*dim.k) {
		t.Fatalf("an unavailable row must stay a plain greyed row: %+v", jxl)
	}
}

func TestFormatNoteSnapshots(t *testing.T) {
	out := testutil.OutDir()
	if out == "" {
		t.Skip("PIXGO_SNAPSHOTS is not set")
	}
	for _, k := range []float64{1, 1.2, 1.6} {
		for _, lang := range scaleLangs {
			t.Run(fmt.Sprintf("k%.2f/%s", k, lang), func(t *testing.T) {
				withScale(t, k)
				size := scaledSize(k, 520, 430)
				r := newNotedRig(t, lang, size.X, size.Y)
				testutil.SavePNG(filepath.Join(out, fmt.Sprintf("formats_notes_k%02d_%s.png", int(k*10+.5), lang)), r.render())
			})
		}
	}
}

func TestOptionalFormatNamesAreNeverCutAtEveryScale(t *testing.T) {
	eachScale(t, func(t *testing.T, k float64, lang string) {
		for _, size := range []image.Point{scaledSize(k, 420, 320), scaledSize(k, 457, 430), {1100, 800}} {
			r := newNotedRig(t, lang, size.X, size.Y)

			for i := range r.page.items {
				it := &r.page.items[i]
				if it.kind != kindFormat {
					continue
				}
				if hasEllipsis(it.name) || hasEllipsis(it.detail) {
					t.Errorf("%v: row %s shows %q / %q", size, it.id, it.name, it.detail)
				}
				if it.change != nil && it.r.Min.X+measure(bodyFont, it.name) > it.box.Min.X {
					t.Errorf("%v: the name of %s runs into its switch", size, it.id)
				}
			}
		}
	})
}
