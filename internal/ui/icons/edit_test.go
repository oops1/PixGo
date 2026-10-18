package icons

import (
	"image/color"
	"testing"
)

var editIcons = []string{"edit", "crop", "flip-h", "flip-v", "resize", "reset", "aspect-swap"}

func TestEditIconsExistAndAreSquare(t *testing.T) {
	for _, name := range editIcons {
		if !Has(name) {
			t.Errorf("icon %q is missing", name)
			continue
		}
		if s := SizeOf(name); s.W != 20 || s.H != 20 {
			t.Errorf("icon %q is %dx%d, the toolbar icons are 20x20", name, s.W, s.H)
		}
	}
}

func TestEditIconsPaintInkInsideTheirBox(t *testing.T) {
	for _, name := range editIcons {
		s := SizeOf(name)
		c := newCanvas(s.W+8, s.H+8, color.RGBA{0, 0, 0, 255})
		Draw(c, name, 4, 4, color.RGBA{255, 255, 255, 255})
		ink, outside := 0, 0
		for y := 0; y < s.H+8; y++ {
			for x := 0; x < s.W+8; x++ {
				if c.img.RGBAAt(x, y).R == 0 {
					continue
				}
				if x >= 4 && x < 4+s.W && y >= 4 && y < 4+s.H {
					ink++
				} else {
					outside++
				}
			}
		}
		cover := float64(ink) / float64(s.W*s.H)
		if cover < 0.05 || cover > 0.6 {
			t.Errorf("%s: coverage %.2f", name, cover)
		}
		if outside != 0 {
			t.Errorf("%s: %d pixels outside the box", name, outside)
		}
	}
}

func TestEditIconsAreDistinct(t *testing.T) {
	render := func(name string) []byte {
		s := SizeOf(name)
		c := newCanvas(s.W, s.H, color.RGBA{0, 0, 0, 255})
		Draw(c, name, 0, 0, color.RGBA{255, 255, 255, 255})
		return append([]byte(nil), c.img.Pix...)
	}
	seen := map[string]string{}
	for _, name := range editIcons {
		key := string(render(name))
		if other, dup := seen[key]; dup {
			t.Errorf("%s draws the same as %s", name, other)
		}
		seen[key] = name
	}
}

func TestFlipIconsAreMirrorsOfEachOther(t *testing.T) {
	h, v := SizeOf("flip-h"), SizeOf("flip-v")
	ch, cv := newCanvas(h.W, h.H, color.RGBA{}), newCanvas(v.W, v.H, color.RGBA{})
	Draw(ch, "flip-h", 0, 0, color.RGBA{255, 255, 255, 255})
	Draw(cv, "flip-v", 0, 0, color.RGBA{255, 255, 255, 255})
	diff := 0
	for y := 0; y < h.H; y++ {
		for x := 0; x < h.W; x++ {
			a, b := ch.img.RGBAAt(x, y).R, cv.img.RGBAAt(y, x).R
			if d := int(a) - int(b); d > 40 || d < -40 {
				diff++
			}
		}
	}
	if diff > 20 {
		t.Fatalf("flip-v is not the transpose of flip-h (%d pixels differ)", diff)
	}
}
