package fonts

import (
	"testing"

	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"

	"github.com/oops1/PixGo/internal/i18n"
)

func TestEveryCatalogueRuneHasAGlyphInEveryFace(t *testing.T) {
	prev := i18n.Lang()
	t.Cleanup(func() { i18n.SetLang(prev) })
	runes := map[rune]string{}
	for _, l := range i18n.Supported() {
		i18n.SetLang(l)
		for _, k := range i18n.Keys(l) {
			for _, r := range i18n.T(k) {
				if _, ok := runes[r]; !ok {
					runes[r] = l + " " + k
				}
			}
		}
		for _, loc := range []i18n.Locale{i18n.LocaleFor(l)} {
			for _, s := range []string{loc.Group, loc.Decimal, loc.DateLayout, loc.TimeLayout} {
				for _, r := range s {
					if _, ok := runes[r]; !ok {
						runes[r] = l + " locale"
					}
				}
			}
			for _, u := range loc.ByteUnits {
				for _, r := range u {
					if _, ok := runes[r]; !ok {
						runes[r] = l + " unit"
					}
				}
			}
		}
	}
	for _, face := range []string{UI, UILight, UISemiBold, UIBold} {
		data, err := Data(face)
		if err != nil {
			t.Fatal(err)
		}
		f, err := opentype.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		var b sfnt.Buffer
		for r, where := range runes {
			if r == '\n' {
				continue
			}
			if g, err := f.GlyphIndex(&b, r); err != nil || g == 0 {
				t.Errorf("%s has no glyph for %q U+%04X (%s)", face, r, r, where)
			}
		}
	}
}
