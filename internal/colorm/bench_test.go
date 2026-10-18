package colorm

import (
	"image"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

const (
	benchW = 6000
	benchH = 4000
)

var (
	cicp2020PQ  = codec.CICP{Primaries: 9, Transfer: 16, FullRange: true}
	cicp2020HLG = codec.CICP{Primaries: 9, Transfer: 18, FullRange: true}
)

func benchImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, benchW, benchH))
	var s uint32 = 12345
	for i := range img.Pix {
		s = s*1664525 + 1013904223
		img.Pix[i] = uint8(s >> 24)
	}
	return img
}

func benchDeep() *image.NRGBA64 {
	img := image.NewNRGBA64(image.Rect(0, 0, benchW, benchH))
	var s uint32 = 54321
	for i := range img.Pix {
		s = s*1664525 + 1013904223
		img.Pix[i] = uint8(s >> 24)
	}
	return img
}

func benchApply(b *testing.B, src, dst *Profile, o Options) {
	tf, err := NewTransform(src, dst, o)
	if err != nil {
		b.Fatal(err)
	}
	img := benchImage()
	b.SetBytes(int64(len(img.Pix)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tf.Apply(img)
	}
}

func benchConvert(b *testing.B, src, dst *Profile, o Options) {
	tf, err := NewTransform(src, dst, o)
	if err != nil {
		b.Fatal(err)
	}
	deep := benchDeep()
	out := image.NewNRGBA(deep.Rect)
	b.SetBytes(int64(len(deep.Pix)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tf.Convert(out, deep)
	}
}

func mustP(b *testing.B, data []byte) *Profile {
	p, err := ParseICC(data)
	if err != nil {
		b.Fatal(err)
	}
	return p
}

func BenchmarkApply24MpxP3ToSRGB(b *testing.B) {
	benchApply(b, mustP(b, displayP3()), SRGB(), Options{})
}

func BenchmarkApply24MpxAdobeToSRGB(b *testing.B) {
	benchApply(b, mustP(b, adobeRGB()), SRGB(), Options{})
}

func BenchmarkApply24MpxSoftClip(b *testing.B) {
	benchApply(b, mustP(b, displayP3()), SRGB(), Options{SoftClip: true})
}

func BenchmarkApply24MpxLUT(b *testing.B) {
	benchApply(b, mustP(b, labProfile(mft2Lab(17, srgbModel))), mustP(b, displayP3()), Options{})
}

func BenchmarkApply24MpxPQ8Bit(b *testing.B) {
	p, _ := FromCICP(cicp2020PQ)
	benchApply(b, p, SRGB(), Options{})
}

func BenchmarkConvert24MpxDeepP3(b *testing.B) {
	benchConvert(b, mustP(b, displayP3()), SRGB(), Options{})
}

func BenchmarkConvert24MpxDeepSRGB(b *testing.B) { benchConvert(b, SRGB(), SRGB(), Options{}) }

func BenchmarkConvert24MpxDeepLUT(b *testing.B) {
	benchConvert(b, mustP(b, labProfile(mft2Lab(17, srgbModel))), mustP(b, displayP3()), Options{})
}

func BenchmarkConvert24MpxPQ(b *testing.B) {
	p, _ := FromCICP(cicp2020PQ)
	benchConvert(b, p, SRGB(), Options{})
}

func BenchmarkConvert24MpxHLG(b *testing.B) {
	p, _ := FromCICP(cicp2020HLG)
	benchConvert(b, p, SRGB(), Options{})
}

func BenchmarkBuildTransformP3(b *testing.B) {
	p := mustP(b, displayP3())
	for i := 0; i < b.N; i++ {
		buildTransform(p, SRGB(), Options{HDR: HDRConfig{}.withDefaults()})
	}
}

func BenchmarkBuildTransformLUT(b *testing.B) {
	p := mustP(b, labProfile(mft2Lab(17, srgbModel)))
	for i := 0; i < b.N; i++ {
		buildTransform(p, SRGB(), Options{HDR: HDRConfig{}.withDefaults()})
	}
}

func BenchmarkBuildTransformPQ(b *testing.B) {
	p, _ := FromCICP(cicp2020PQ)
	for i := 0; i < b.N; i++ {
		buildTransform(p, SRGB(), Options{HDR: HDRConfig{}.withDefaults()})
	}
}
