package colorm

import (
	"image"
	"testing"
)

func FuzzParseICC(f *testing.F) {
	for _, seed := range [][]byte{
		srgbV2(), srgbV4(), displayP3(), adobeRGB(), grayProfile(2.2), cmykProfile(3, true, false), lutDisplay(), perceptualPair(),
		labProfile(mft1Lab(5, srgbModel)), labProfile(mft2Lab(5, srgbModel)),
		buildICC(4, "mntr", "RGB ", "XYZ ", []testTag{{"A2B0", mft2XYZMatrix(srgbD50, srgbDecode)}}),
		nil, make([]byte, 200),
	} {
		f.Add(seed)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 37)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := ParseICC(data)
		if err != nil {
			return
		}
		_ = p.Description
		_ = p.Channels()
		for _, in := range []Intent{RelativeColorimetric, Perceptual} {
			tf, err := NewTransform(p, SRGB(), Options{Intent: in})
			if err != nil {
				continue
			}
			cp := image.NewNRGBA(img.Rect)
			copy(cp.Pix, img.Pix)
			tf.Apply(cp)
			if p.Channels() == 4 {
				out := image.NewNRGBA(image.Rect(0, 0, 1, 1))
				_ = tf.ConvertCMYK(out, image.NewCMYK(image.Rect(0, 0, 1, 1)))
			}
			if rev, err := NewTransform(SRGB(), p, Options{Intent: in}); err == nil {
				rev.Apply(cp)
			}
		}
	})
}
