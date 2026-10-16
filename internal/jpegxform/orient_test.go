package jpegxform

import (
	"image"
	"image/color"
	"math/rand"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/imaging"
)

func nrgbaOf(img image.Image) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := rgbaAt(img, x, y)
			out.SetNRGBA(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255})
		}
	}
	return out
}

func TestTransformMatchesWhatTheViewerBakesForEveryOrientation(t *testing.T) {
	for _, sc := range []string{"gray", "420", "444", "422"} {
		var factors [][2]int
		for _, c := range samplingCases {
			if c.name == sc {
				factors = c.factors
			}
		}
		s := samplingSpec(96, 64, factors...)
		data := buildJPEG(t, s, mildWall(rand.New(rand.NewSource(50)), s))
		stored := nrgbaOf(stdDecode(t, data))
		for o := codec.OrientNormal; o <= codec.OrientRotate270; o++ {
			out := mustTransform(t, data, FromOrientation(o), Options{})

			got := nrgbaOf(stdDecode(t, out))

			want := imaging.Orient(stored, o)
			if got.Bounds() != want.Bounds() {
				t.Fatalf("%s orientation %d: %v, want %v", sc, o, got.Bounds(), want.Bounds())
			}
			worst := 0
			for i := range got.Pix {
				worst = max(worst, absInt(int(got.Pix[i])-int(want.Pix[i])))
			}
			if worst > 4 {
				t.Errorf("%s orientation %d: pixels differ by %d", sc, o, worst)
			}
		}
	}
}

func TestOrientationsComposeLikeTheViewerRotatesPictures(t *testing.T) {
	s := samplingSpec(96, 64, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
	wall := randomWall(rand.New(rand.NewSource(51)), s)
	data := buildJPEG(t, s, wall)
	for a := codec.OrientNormal; a <= codec.OrientRotate270; a++ {
		for b := codec.OrientNormal; b <= codec.OrientRotate270; b++ {
			twice := mustTransform(t, mustTransform(t, data, FromOrientation(a), Options{}), FromOrientation(b), Options{})
			once := mustTransform(t, data, FromOrientation(a.Then(b)), Options{})

			_, g1 := parseCoefficients(t, twice)
			_, g2 := parseCoefficients(t, once)
			if !gridsEqual(g1, g2) {
				t.Fatalf("%d then %d is not %d", a, b, a.Then(b))
			}
		}
	}
}
