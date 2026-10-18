package apng

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func sixteenBitFrame(w, h int, seed int, alpha func(x, y int) uint16) *image.NRGBA64 {
	img := image.NewNRGBA64(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint16((x*4099 + y*7919 + seed*12289) % 65521)
			img.SetNRGBA64(x, y, color.NRGBA64{R: v, G: v ^ 0x5555, B: uint16(seed*20011) + uint16(x), A: alpha(x, y)})
		}
	}
	return img
}

type refCanvas struct {
	w, h int
	pix  [][4]float64
}

func newRef(w, h int) *refCanvas { return &refCanvas{w: w, h: h, pix: make([][4]float64, w*h)} }

func (c *refCanvas) clone() []([4]float64) { return append([][4]float64(nil), c.pix...) }

func (c *refCanvas) draw(img *image.NRGBA64, x0, y0 int, over bool) {
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			p := img.NRGBA64At(x, y)
			s := [4]float64{float64(p.R), float64(p.G), float64(p.B), float64(p.A)}
			d := &c.pix[(y0+y)*c.w+x0+x]
			if !over {
				*d = s
				continue
			}
			sa, da := s[3]/65535, d[3]/65535
			outA := sa + da*(1-sa)
			if outA == 0 {
				*d = [4]float64{}
				continue
			}
			for k := 0; k < 3; k++ {
				d[k] = (s[k]*sa + d[k]*da*(1-sa)) / outA
			}
			d[3] = outA * 65535
		}
	}
}

func (c *refCanvas) clear(x0, y0, w, h int) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			c.pix[y*c.w+x] = [4]float64{}
		}
	}
}

func fullButOne(x, y int) uint16 {
	if x == 0 && y == 0 {
		return 0xfffe
	}
	return 0xffff
}

func TestSixteenBitAnimationComposesInSixteenBits(t *testing.T) {
	const w, h = 14, 10
	opaque := func(x, y int) uint16 {
		if x == 0 && y == 0 {
			return 0xfffe
		}
		return 0xffff
	}
	half := func(x, y int) uint16 { return uint16(0x2000 + x*0x1000) }
	f0 := sixteenBitFrame(w, h, 1, opaque)
	f1 := sixteenBitFrame(6, 5, 2, half)
	f2 := sixteenBitFrame(7, 4, 3, half)
	anim := animation{frames: []frameSpec{
		{png: encodeStd(t, f0), num: 1, den: 10},
		{png: encodeStd(t, f1), x: 3, y: 2, num: 1, den: 10, blend: blendOver, dispose: disposeBackground},
		{png: encodeStd(t, f2), x: 5, y: 4, num: 1, den: 10, blend: blendOver, dispose: disposePrevious},
		{png: encodeStd(t, f0), num: 1, den: 10},
	}}
	data := anim.bytes(t)
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames, Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Frames) != 4 {
		t.Fatalf("%d frames", len(img.Frames))
	}
	for i, f := range img.Frames {
		if f.Deep == nil || f.Pix != nil {
			t.Fatalf("frame %d is not deep", i)
		}
	}
	ref := newRef(w, h)
	var shown [][][4]float64
	ref.draw(f0, 0, 0, false)
	shown = append(shown, ref.clone())
	ref.draw(f1, 3, 2, true)
	shown = append(shown, ref.clone())
	ref.clear(3, 2, 6, 5)
	saved := ref.clone()
	ref.draw(f2, 5, 4, true)
	shown = append(shown, ref.clone())
	ref.pix = saved
	ref.draw(f0, 0, 0, false)
	shown = append(shown, ref.clone())
	for i, f := range img.Frames {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				p := f.Deep.NRGBA64At(x, y)
				got := [4]float64{float64(p.R), float64(p.G), float64(p.B), float64(p.A)}
				want := shown[i][y*w+x]
				for k := 0; k < 4; k++ {
					tolerance := 2.0
					if want[3] == 0 && k < 3 {
						continue
					}
					if math.Abs(got[k]-want[k]) > tolerance {
						t.Fatalf("frame %d at %d,%d channel %d: %v, want %v", i, x, y, k, got, want)
					}
				}
			}
		}
	}
}

func TestSixteenBitFirstFrameOnlyAndWithoutDeep(t *testing.T) {
	f0 := sixteenBitFrame(9, 7, 4, fullButOne)
	f1 := sixteenBitFrame(9, 7, 5, fullButOne)
	data := animation{frames: []frameSpec{{png: encodeStd(t, f0), num: 1, den: 10}, {png: encodeStd(t, f1), num: 1, den: 10}}}.bytes(t)
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits(), Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Frames) != 1 || !bytes.Equal(img.Frames[0].Deep.Pix, f0.Pix) {
		t.Fatal("first frame differs")
	}
	plain := mustDecodeAll(t, data)
	if plain.Frames[0].Pix == nil || plain.Frames[0].Deep != nil {
		t.Fatal("Deep leaked into the default path")
	}
	for i, fr := range plain.Frames {
		want := []*image.NRGBA64{f0, f1}[i]
		got := fr.Pix.NRGBAAt(2, 3)
		w := want.NRGBA64At(2, 3)
		if uint16(got.R) != w.R>>8 || uint16(got.G) != w.G>>8 || uint16(got.B) != w.B>>8 {
			t.Fatalf("frame %d: %v vs %v", i, got, w)
		}
	}
}

func TestAnimationEightBitStaysEightBitWithDeep(t *testing.T) {
	a := testimg.Photo(10, 8)
	b := testimg.Photo(10, 8)
	b.Pix[0] ^= 0xff
	data := animation{frames: []frameSpec{{png: encodeStd(t, a), num: 1, den: 10}, {png: encodeStd(t, b), num: 1, den: 10}}}.bytes(t)
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames, Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range img.Frames {
		if f.Deep != nil || f.Pix == nil {
			t.Fatal("8 bit animation became deep")
		}
	}
}

func TestAnimationReportsColor(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 64)
	frame := sixteenBitFrame(6, 6, 1, fullButOne)
	var icc []byte
	icc = append(icc, "p"...)
	icc = append(icc, 0, 0)
	icc = append(icc, deflate(profile)...)
	anim := animation{
		frames: []frameSpec{{png: encodeStd(t, frame), num: 1, den: 10}, {png: encodeStd(t, frame), num: 1, den: 10}},
		before: []rawChunk{{kind: "cICP", body: []byte{9, 16, 0, 1}}, {kind: "iCCP", body: icc}},
	}
	img := mustDecodeAll(t, anim.bytes(t))
	if !bytes.Equal(img.Color.ICC, profile) || img.Color.CICP == nil || img.Color.CICP.Transfer != 16 {
		t.Fatalf("%d bytes %+v", len(img.Color.ICC), img.Color.CICP)
	}
}
