package render

import (
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"testing"
)

func stabilityDigest() string {
	rng := rand.New(rand.NewSource(99))
	h := sha256.New()
	for _, translucent := range []bool{false, true} {
		src := randomSource(rng, 173, 131, 5, translucent)
		for _, o := range allOrients {
			ow, oh := o.Size(src.Width, src.Height)
			for _, sc := range []float64{0.07, 0.13, 0.27, 0.5, 0.999, 1, 1.0004, 1.7, 3, 6} {
				for _, f := range []Filter{FilterAuto, FilterBilinear, FilterNearest} {
					for _, bg := range []Background{solidBg(), checkerBg()} {
						m := Mapping{
							Scale:   sc,
							Orient:  o,
							Filter:  f,
							OriginX: rng.Float64()*float64(ow) - 20,
							OriginY: rng.Float64()*float64(oh) - 20,
						}
						dst := newDst(97, 71, 3, 2)
						fillGarbage(dst, 41)
						Render(dst, src, m, bg)
						h.Write(dst.Pix)
					}
				}
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestRenderOutputIsBitStable(t *testing.T) {
	got := stabilityDigest()
	const want = "c1ff52aedccf827074451eaa76138a885012ca8243e1c798a17566e8895944aa"
	if got != want {
		t.Fatalf("render output changed: digest %s", got)
	}
}
