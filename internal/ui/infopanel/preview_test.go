package infopanel

import (
	"fmt"
	"strings"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/meta"
)

func TestFormatLineMarksPreviewsAndOnlyThem(t *testing.T) {
	tests := []struct {
		name string
		lang string
		info codec.Info
		want string
	}{
		{"ordinary file", "ru", codec.Info{FormatName: "JPEG (JFIF)"}, "JPEG (JFIF)"},
		{"ordinary file in English", "en", codec.Info{FormatName: "PNG"}, "PNG"},
		{"raw preview in Russian", "ru", codec.Info{FormatName: "RAW", Preview: true}, "RAW (предпросмотр)"},
		{"raw preview in English", "en", codec.Info{FormatName: "RAW", Preview: true}, "RAW (preview)"},
		{"longer name keeps the whole name", "ru", codec.Info{FormatName: "Canon CR2", Preview: true}, "Canon CR2 (предпросмотр)"},
		{"a preview without a name shows nothing", "ru", codec.Info{Preview: true}, ""},
		{"no name and no preview", "en", codec.Info{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setLang(t, tc.lang)

			m := BuildModel(Input{Path: "a.cr2", Name: "a.cr2", Info: tc.info})

			if m.Format != tc.want {
				t.Fatalf("format line %q, want %q", m.Format, tc.want)
			}
		})
	}
}

func TestPreviewMarkChangesNothingElseInTheModel(t *testing.T) {
	plain := BuildModel(Input{Path: "a.cr2", Name: "a.cr2", FileSize: 1000, Info: codec.Info{FormatName: "RAW", Width: 40, Height: 30}, Meta: &meta.Metadata{}})
	marked := BuildModel(Input{Path: "a.cr2", Name: "a.cr2", FileSize: 1000, Info: codec.Info{FormatName: "RAW", Width: 40, Height: 30, Preview: true}, Meta: &meta.Metadata{}})

	if plain.Path != marked.Path || plain.Name != marked.Name || plain.FileSize != marked.FileSize || len(plain.Cards) != len(marked.Cards) {
		t.Fatalf("the preview flag changed more than the format line: %+v vs %+v", plain, marked)
	}
	for i := range plain.Cards {
		if fmt.Sprint(plain.Cards[i]) != fmt.Sprint(marked.Cards[i]) {
			t.Errorf("card %d changed: %v vs %v", i, plain.Cards[i], marked.Cards[i])
		}
	}
	if plain.Format == marked.Format {
		t.Error("the format line must differ")
	}
}

func TestPreviewMarkIsKeptWhenTheFormatNameHasToBeShortened(t *testing.T) {
	for _, lang := range i18n.Supported() {
		for _, k := range textScales {
			for _, name := range []string{"RAW", "Canon CR2", "Nikon NEF", "Sony Alpha ARW (ILCE-7M4)", "Hasselblad 3FR extended"} {
				t.Run(fmt.Sprintf("%s/k%.2f/%s", lang, k, name), func(t *testing.T) {
					setLang(t, lang)
					setScale(t, k)
					m := BuildModel(Input{Path: `D:.cr2`, Name: "a.cr2", FileSize: 1 << 20, Info: codec.Info{FormatName: name, Width: 40, Height: 30, Preview: true}})
					_, p := newPanelAt(t, m, 326, 600)
					pl := p.place()

					drawn := fitFormat(subFont, p.model.Format, p.model.FormatNote, pl.textW)

					if !strings.HasSuffix(drawn, i18n.T("info.previewMark")) {
						t.Fatalf("the preview mark is lost: %q", drawn)
					}
					if w := subFont.Measure(nil, drawn); w > pl.textW {
						t.Fatalf("%q is %d wide for %d px", drawn, w, pl.textW)
					}
					if subFont.Measure(nil, m.Format) <= pl.textW && drawn != m.Format {
						t.Fatalf("a line that fits was changed: %q", drawn)
					}
				})
			}
		}
	}
}

func TestFitFormatCases(t *testing.T) {
	setLang(t, "en")
	setScale(t, 1)
	const mark = "(preview)"
	wide := subFont.Measure(nil, "Canon CR2 "+mark) + 40
	tests := []struct {
		name   string
		format string
		note   string
		width  int
		want   string
	}{
		{"fits", "Canon CR2 " + mark, mark, wide, "Canon CR2 " + mark},
		{"no note, long text is left to DrawFit", "Canon CR2 " + mark, "", 30, "Canon CR2 " + mark},
		{"note that is not a suffix", "Canon CR2", mark, 30, "Canon CR2"},
		{"shortens the name, keeps the mark", "Canon EOS R5 Mark II " + mark, mark, subFont.Measure(nil, "Canon EO… "+mark) + 2, ""},
		{"no room even for an ellipsis", "Canon CR2 " + mark, mark, subFont.Measure(nil, " "+mark) + 2, "Canon CR2 " + mark},
		{"the whole line is the note", mark, mark, 5, mark},
		{"empty", "", "", 100, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fitFormat(subFont, tc.format, tc.note, tc.width)

			if tc.want != "" && got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if tc.want == "" && tc.format != "" && (!strings.HasSuffix(got, mark) || !strings.Contains(got, "…") || subFont.Measure(nil, got) > tc.width) {
				t.Fatalf("got %q (%d px) for %d px", got, subFont.Measure(nil, got), tc.width)
			}
		})
	}
}

func TestPanelDrawsThePreviewMarkAtEveryScale(t *testing.T) {
	for _, k := range textScales {
		t.Run(fmt.Sprintf("k%.2f", k), func(t *testing.T) {
			setLang(t, "ru")
			setScale(t, k)
			plain := BuildModel(Input{Path: `D:.cr2`, Name: "a.cr2", FileSize: 1 << 20, Info: codec.Info{FormatName: "RAW", Width: 40, Height: 30}})
			marked := BuildModel(Input{Path: `D:.cr2`, Name: "a.cr2", FileSize: 1 << 20, Info: codec.Info{FormatName: "RAW", Width: 40, Height: 30, Preview: true}})
			a, p := newPanelAt(t, plain, 326, 600)
			b, _ := newPanelAt(t, marked, 326, 600)
			pl := p.place()
			line := textBox(subFont, marked.Format, textX, pl.formatBase)

			changed := false
			for y := line.Min.Y; y < line.Max.Y && !changed; y++ {
				for x := line.Min.X; x < line.Max.X; x++ {
					if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
						changed = true
						break
					}
				}
			}

			if !changed {
				t.Fatal("the format line looks the same with and without the preview mark")
			}
		})
	}
}
