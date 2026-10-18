package jpegdec

import (
	"bytes"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"testing"
)

func inkSpecs() map[string]encSpec {
	four := func(h, v int) []encComp {
		return []encComp{{1, h, v, 0}, {2, 1, 1, 1}, {3, 1, 1, 1}, {4, h, v, 0}}
	}
	specs := map[string]encSpec{
		"cmyk":              {width: 45, height: 38, comps: four(1, 1), adobe: 1, seed: 11},
		"cmyk-2x2":          {width: 45, height: 38, comps: four(2, 2), adobe: 1, seed: 12},
		"cmyk-restart":      {width: 45, height: 38, comps: four(2, 2), adobe: 1, restart: 3, seed: 13},
		"cmyk-separate":     {width: 45, height: 38, comps: four(2, 2), adobe: 1, separateScans: true, seed: 14},
		"ycck":              {width: 45, height: 38, comps: four(1, 1), adobe: 3, seed: 15},
		"ycck-2x2":          {width: 45, height: 38, comps: four(2, 2), adobe: 3, restart: 5, seed: 16},
		"ycck-separate":     {width: 45, height: 38, comps: four(2, 2), adobe: 3, separateScans: true, seed: 17},
		"cmyk-odd":          {width: 17, height: 33, comps: four(2, 2), adobe: 1, seed: 18},
		"ycck-noise":        {width: 96, height: 80, comps: four(2, 2), adobe: 3, whiteNoise: true, unitQuant: true, seed: 19},
		"cmyk-single-block": {width: 7, height: 9, comps: four(1, 1), adobe: 1, seed: 20},
	}
	return specs
}

func inkFiles(t testing.TB) map[string][]byte {
	files := map[string][]byte{"video-001.cmyk.jpeg": corpus(t)["video-001.cmyk.jpeg"]}
	for name, spec := range inkSpecs() {
		files[name] = encodeSpec(spec)
	}
	return files
}

func sameInk(a, b *image.CMYK) string {
	if a.Bounds() != b.Bounds() {
		return "bounds " + a.Bounds().String() + " vs " + b.Bounds().String()
	}
	w := a.Bounds().Dx() * 4
	for y := 0; y < a.Bounds().Dy(); y++ {
		if !bytes.Equal(a.Pix[y*a.Stride:y*a.Stride+w], b.Pix[y*b.Stride:y*b.Stride+w]) {
			for x := 0; x < a.Bounds().Dx(); x++ {
				if a.CMYKAt(x, y) != b.CMYKAt(x, y) {
					return "pixel (" + itoa(x) + "," + itoa(y) + ")"
				}
			}
		}
	}
	return ""
}

func inkToNRGBA(src *image.CMYK) *image.NRGBA {
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			p := src.CMYKAt(b.Min.X+x, b.Min.Y+y)
			r, g, bl := color.CMYKToRGB(p.C, p.M, p.Y, p.K)
			out.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: bl, A: 255})
		}
	}
	return out
}

func TestInkSamplesMatchTheStandardLibrary(t *testing.T) {
	for name, data := range inkFiles(t) {
		img, err := stdjpeg.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want, ok := img.(*image.CMYK)
		if !ok {
			t.Fatalf("%s: standard library returned %T", name, img)
		}
		for _, workers := range []int{1, 4} {
			d := open(t, data, Options{Workers: workers})
			if !d.Ink() {
				t.Fatalf("%s: not reported as ink", name)
			}
			got, err := d.DecodeInk(t.Context(), 0)
			d.Close()
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if diff := sameInk(got, want); diff != "" {
				t.Errorf("%s workers %d: %s", name, workers, diff)
			}
		}
	}
}

func TestInkSamplesConvertedNaivelyEqualTheRGBDecode(t *testing.T) {
	for name, data := range inkFiles(t) {
		d := open(t, data, Options{Workers: 2})
		for shift := 0; shift <= MaxShift; shift++ {
			ink, err := d.DecodeInk(t.Context(), shift)
			if err != nil {
				t.Fatalf("%s shift %d: %v", name, shift, err)
			}
			rgb, err := d.Decode(t.Context(), shift)
			if err != nil {
				t.Fatalf("%s shift %d: %v", name, shift, err)
			}
			if !samePixels(inkToNRGBA(ink), rgb) {
				t.Errorf("%s shift %d: %s", name, shift, firstDifference(inkToNRGBA(ink), rgb))
			}
		}
		d.Close()
	}
}

func TestInkRegionsEqualCropsOfTheWholeLevel(t *testing.T) {
	for name, data := range inkFiles(t) {
		d := open(t, data, Options{Workers: 3})
		for shift := 0; shift <= 2; shift++ {
			whole, err := d.DecodeInk(t.Context(), shift)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			b := whole.Bounds()
			for _, r := range []image.Rectangle{
				image.Rect(0, 0, b.Dx(), b.Dy()),
				image.Rect(b.Dx()/3, b.Dy()/4, b.Dx()-b.Dx()/5, b.Dy()-1),
				image.Rect(b.Dx()-1, b.Dy()-1, b.Dx(), b.Dy()),
			} {
				r = r.Intersect(b)
				if r.Empty() {
					continue
				}
				got, err := d.RegionInk(t.Context(), shift, r)
				if err != nil {
					if d.CanRegion() {
						t.Errorf("%s shift %d %v: %v", name, shift, r, err)
					}
					continue
				}
				want := image.NewCMYK(image.Rect(0, 0, r.Dx(), r.Dy()))
				for y := 0; y < r.Dy(); y++ {
					for x := 0; x < r.Dx(); x++ {
						want.SetCMYK(x, y, whole.CMYKAt(r.Min.X+x, r.Min.Y+y))
					}
				}
				if diff := sameInk(got, want); diff != "" {
					t.Errorf("%s shift %d %v: %s", name, shift, r, diff)
				}
			}
		}
		d.Close()
	}
}

func TestInkIsRefusedForFilesThatAreNotCMYK(t *testing.T) {
	files := corpus(t)
	for _, name := range []string{"video-001.jpeg", "video-005.gray.q50.jpeg", "video-001.rgb.jpeg", "video-001.progressive.jpeg"} {
		d := open(t, files[name], Options{})
		if d.Ink() {
			t.Errorf("%s reported as ink", name)
		}
		if _, err := d.DecodeInk(t.Context(), 0); err == nil {
			t.Errorf("%s: DecodeInk accepted a file with no ink", name)
		}
		if _, err := d.RegionInk(t.Context(), 0, image.Rect(0, 0, 4, 4)); err == nil {
			t.Errorf("%s: RegionInk accepted a file with no ink", name)
		}
		d.Close()
	}
}

func TestAskingForInkDoesNotChangeTheRGBDecode(t *testing.T) {
	for name, data := range inkFiles(t) {
		want, wantErr := referenceNRGBA(t, data)
		if wantErr != nil {
			t.Fatalf("%s: %v", name, wantErr)
		}
		d := open(t, data, Options{Workers: 2})
		if _, err := d.DecodeInk(t.Context(), 0); err != nil {
			t.Fatal(err)
		}
		got, err := d.Decode(t.Context(), 0)
		d.Close()
		if err != nil || !samePixels(got, want) {
			t.Errorf("%s: %v %s", name, err, firstDifference(got, want))
		}
	}
}

func FuzzDecodeInk(f *testing.F) {
	for _, data := range inkFiles(f) {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		d, err := Open(t.Context(), bytes.NewReader(data), int64(len(data)), Options{Workers: 1, MaxPixels: 1 << 22})
		if err != nil {
			return
		}
		defer d.Close()
		if !d.Ink() {
			return
		}
		img, err := d.DecodeInk(t.Context(), 1)
		if err != nil {
			return
		}
		naive, err := d.Decode(t.Context(), 1)
		if err != nil {
			return
		}
		if !samePixels(inkToNRGBA(img), naive) {
			t.Fatal("ink and RGB decodes disagree")
		}
	})
}
