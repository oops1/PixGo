package rawdev

import (
	"context"
	"errors"
	"image"
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/par"
)

type errStats struct {
	max  float64
	mean float64
}

func compare8(t testing.TB, img *image.NRGBA, s synthetic, origin image.Point, gain float64, margin int) errStats {
	t.Helper()
	var st errStats
	count := 0
	b := img.Bounds()
	for y := margin; y < b.Dy()-margin; y++ {
		for x := margin; x < b.Dx()-margin; x++ {
			want := expected8(s.scene(origin.X+x, origin.Y+y), gain)
			got := img.Pix[y*img.Stride+4*x:]
			for c := 0; c < 3; c++ {
				d := math.Abs(float64(got[c]) - want[c])
				st.max = math.Max(st.max, d)
				st.mean += d
				count++
			}
			if got[3] != 255 {
				t.Fatalf("alpha %d at %d,%d", got[3], x, y)
			}
		}
	}
	if count > 0 {
		st.mean /= float64(count)
	}
	return st
}

func TestDevelopRecoversThePictureForEveryPattern(t *testing.T) {
	for name, cfa := range map[string][4]uint8{"RGGB": rggb, "BGGR": bggr, "GRBG": grbg, "GBRG": gbrg} {
		s := makeSynthetic(synthOptions{W: 160, H: 120, CFA: cfa})
		img, deep, err := Develop(context.Background(), s.mosaic, s.params, 1, false)
		if err != nil || deep != nil || img == nil {
			t.Fatalf("%s: %v", name, err)
		}
		st := compare8(t, img, s, image.Point{}, 1, 2)
		t.Logf("%s: max %.1f mean %.3f levels of 255", name, st.max, st.mean)
		if st.max > 2 || st.mean > 0.4 {
			t.Errorf("%s: max %.1f mean %.3f", name, st.max, st.mean)
		}
	}
}

func TestDeepOutputMatchesEightBitOutput(t *testing.T) {
	s := makeSynthetic(synthOptions{W: 64, H: 48, CFA: rggb})
	img, _, err := Develop(context.Background(), s.mosaic, s.params, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	_, deep, err := Develop(context.Background(), s.mosaic, s.params, 1, true)
	if err != nil || deep == nil {
		t.Fatal(err)
	}
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			for c := 0; c < 3; c++ {
				v16 := uint32(deep.Pix[y*deep.Stride+8*x+2*c])<<8 | uint32(deep.Pix[y*deep.Stride+8*x+2*c+1])
				if want := uint8((v16 + 128) / 257); img.Pix[y*img.Stride+4*x+c] != want {
					t.Fatalf("pixel %d,%d channel %d: %d vs %d", x, y, c, img.Pix[y*img.Stride+4*x+c], want)
				}
			}
			if deep.Pix[y*deep.Stride+8*x+6] != 255 || deep.Pix[y*deep.Stride+8*x+7] != 255 {
				t.Fatal("deep alpha")
			}
		}
	}
	var maxErr float64
	for y := 2; y < 46; y++ {
		for x := 2; x < 62; x++ {
			want := expected8(sceneAt(x, y), 1)
			for c := 0; c < 3; c++ {
				v16 := float64(uint32(deep.Pix[y*deep.Stride+8*x+2*c])<<8 | uint32(deep.Pix[y*deep.Stride+8*x+2*c+1]))
				maxErr = math.Max(maxErr, math.Abs(v16-want[c]*257))
			}
		}
	}
	t.Logf("deep max error %.0f of 65535", maxErr)
	if maxErr > 3*257 {
		t.Fatalf("deep error %.0f", maxErr)
	}
}

func TestBlackLevelsPerPositionAndLinearizationTable(t *testing.T) {
	black := [4]float64{600, 480, 520, 700}
	s := makeSynthetic(synthOptions{W: 96, H: 64, CFA: rggb, Black: black})
	img, _, err := Develop(context.Background(), s.mosaic, s.params, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if st := compare8(t, img, s, image.Point{}, 1, 2); st.max > 3 {
		t.Fatalf("per position black levels: %+v", st)
	}
	curve := func(v float64) float64 { return 4095 * math.Pow(v/16383, 0.6) }
	back := func(v float64) float64 { return 16383 * math.Pow(v/4095, 1/0.6) }
	s = makeSynthetic(synthOptions{W: 96, H: 64, CFA: rggb, Table: back, Inverse: curve})
	img, _, err = Develop(context.Background(), s.mosaic, s.params, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	st := compare8(t, img, s, image.Point{}, 1, 2)
	t.Logf("linearization table: max %.1f mean %.3f", st.max, st.mean)
	if st.max > 4 || st.mean > 1 {
		t.Fatalf("linearization table: %+v", st)
	}
}

func TestActiveAreaAndCropSelectThePixels(t *testing.T) {
	for _, crop := range []image.Rectangle{image.Rect(10, 6, 90, 70), image.Rect(11, 7, 91, 71), image.Rect(0, 0, 100, 80), image.Rect(97, 77, 100, 80)} {
		s := makeSynthetic(synthOptions{W: 100, H: 80, CFA: grbg, Pad: 6, Crop: crop})
		img, _, err := Develop(context.Background(), s.mosaic, s.params, 1, false)
		if err != nil {
			t.Fatalf("%v: %v", crop, err)
		}
		if img.Bounds().Dx() != crop.Dx() || img.Bounds().Dy() != crop.Dy() {
			t.Fatalf("%v: size %v", crop, img.Bounds())
		}
		margin := 0
		if crop.Dx() > 10 {
			margin = 2
		}
		if st := compare8(t, img, s, crop.Min, 1, margin); st.max > 3 {
			t.Errorf("%v: %+v", crop, st)
		}
	}
}

func TestBinnedOutputIsTheAverageOfTheFullOne(t *testing.T) {
	s := makeSynthetic(synthOptions{W: 130, H: 94, CFA: bggr})
	for _, scale := range []int{2, 4, 8} {
		img, _, err := Develop(context.Background(), s.mosaic, s.params, scale, false)
		if err != nil {
			t.Fatal(err)
		}
		ow, oh := OutputSize(s.params.Crop, scale)
		if img.Bounds().Dx() != ow || img.Bounds().Dy() != oh || ow != (130+scale-1)/scale {
			t.Fatalf("scale %d: %v", scale, img.Bounds())
		}
		var worst float64
		for oy := 1; oy < oh-1; oy++ {
			for ox := 1; ox < ow-1; ox++ {
				var want [3]float64
				for j := 0; j < scale; j++ {
					for i := 0; i < scale; i++ {
						e := expected8(s.scene(ox*scale+i, oy*scale+j), 1)
						for c := 0; c < 3; c++ {
							want[c] += e[c] / float64(scale*scale)
						}
					}
				}
				for c := 0; c < 3; c++ {
					got := float64(img.Pix[oy*img.Stride+4*ox+c])
					worst = math.Max(worst, math.Abs(got-want[c]))
				}
			}
		}
		t.Logf("scale %d: max deviation from the box average %.1f levels", scale, worst)
		if worst > 7 {
			t.Errorf("scale %d deviates by %.1f", scale, worst)
		}
	}
}

func TestBinningAFlatColourIsExact(t *testing.T) {
	colour := func(x, y int) [3]float64 { return [3]float64{0.55, 0.2, 0.35} }
	s := makeSynthetic(synthOptions{W: 67, H: 45, CFA: gbrg, Scene: colour})
	want := expected8(colour(0, 0), 1)
	for _, scale := range []int{1, 2, 4, 8} {
		img, _, err := Develop(context.Background(), s.mosaic, s.params, scale, false)
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				for c := 0; c < 3; c++ {
					if d := math.Abs(float64(img.Pix[y*img.Stride+4*x+c]) - want[c]); d > 1.01 {
						t.Fatalf("scale %d at %d,%d channel %d: %d want %.1f", scale, x, y, c, img.Pix[y*img.Stride+4*x+c], want[c])
					}
				}
			}
		}
	}
}

func flat(v float64) func(x, y int) [3]float64 {
	return func(x, y int) [3]float64 { return [3]float64{v, v, v} }
}

func TestExposureLiftsMidtonesAndKeepsWhiteAtWhite(t *testing.T) {
	for _, ev := range []float64{-1, 0, 0.5, 1, 2} {
		quarter := makeSynthetic(synthOptions{W: 32, H: 32, CFA: rggb, Scene: flat(0.25)})
		quarter.params.Exposure = ev
		img, _, err := Develop(context.Background(), quarter.mosaic, quarter.params, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		want := 255 * ToneCurve(0.25*math.Exp2(ev), ev)
		got := float64(img.Pix[16*img.Stride+4*16])
		if math.Abs(got-want) > 2 {
			t.Errorf("ev %v: quarter grey %v want %.1f", ev, got, want)
		}
		white := makeSynthetic(synthOptions{W: 32, H: 32, CFA: rggb, Scene: flat(1)})
		white.params.Exposure = ev
		img, _, err = Develop(context.Background(), white.mosaic, white.params, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		px := img.Pix[16*img.Stride+4*16:]
		wantWhite := 255.0
		if ev < 0 {
			wantWhite = 255 * ToneCurve(math.Exp2(ev), 0)
		}
		for c := 0; c < 3; c++ {
			if math.Abs(float64(px[c])-wantWhite) > 2 {
				t.Errorf("ev %v: white %v want %.0f", ev, px[:3], wantWhite)
			}
		}
	}
}

func TestToneCurveIsMonotoneContinuousAndBounded(t *testing.T) {
	for _, ev := range []float64{0, 0.5, 1, 2, 4} {
		prev := -1.0
		for i := 0; i <= 4000; i++ {
			x := float64(i) / 1000
			v := ToneCurve(x, ev)
			if v < prev-1e-12 || v < 0 || v > 1 {
				t.Fatalf("ev %v x %v: %v after %v", ev, x, v, prev)
			}
			if i > 0 && v-prev > 0.02 {
				t.Fatalf("ev %v jump at %v", ev, x)
			}
			prev = v
		}
		if got := ToneCurve(math.Exp2(ev), ev); math.Abs(got-1) > 1e-9 {
			t.Fatalf("ev %v: white maps to %v", ev, got)
		}
	}
	if ToneCurve(0.5, 0) != srgbEncode(0.5) {
		t.Fatal("identity below the shoulder at ev 0")
	}
}

func TestSaturatedHighlightsGoWhite(t *testing.T) {
	s := makeSynthetic(synthOptions{W: 32, H: 32, CFA: rggb, Scene: flat(1.6)})
	img, _, err := Develop(context.Background(), s.mosaic, s.params, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, off := range []int{0, 4, 4 * img.Bounds().Dx()} {
		px := img.Pix[off:]
		if px[0] != 255 || px[1] != 255 || px[2] != 255 {
			t.Fatalf("clipped highlight %v", px[:3])
		}
	}
}

func TestWhiteBalanceMakesNeutralsNeutral(t *testing.T) {
	grey := func(x, y int) [3]float64 { v := 0.2 + 0.5*float64(x)/200; return [3]float64{v, v, v} }
	s := makeSynthetic(synthOptions{W: 200, H: 16, CFA: gbrg, Scene: grey})
	img, _, err := Develop(context.Background(), s.mosaic, s.params, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	for x := 3; x < 197; x++ {
		px := img.Pix[8*img.Stride+4*x:]
		if d := max(int(px[0]), int(px[1]), int(px[2])) - min(int(px[0]), int(px[1]), int(px[2])); d > 2 {
			t.Fatalf("x %d: %v is not neutral", x, px[:3])
		}
	}
}

func linearRaw(w, h int, black, white float64) (Mosaic, Params, func(x, y int) [3]float64) {
	pix := make([]uint16, w*h*3)
	n := cameraNeutral(testCamera)
	top := math.Max(n[0], math.Max(n[1], n[2]))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cam := testCamera.Apply(srgbToXYZD65.Apply(sceneAt(x, y)))
			for c := 0; c < 3; c++ {
				pix[(y*w+x)*3+c] = uint16(math.Round(black + math.Min(math.Max(cam[c]/top, 0), 1)*(white-black)))
			}
		}
	}
	r, _ := cameraColor(testCamera).Resolve()
	p := Params{Crop: image.Rect(0, 0, w, h), Render: r}
	for i := range p.Black {
		p.Black[i], p.White[i] = black, white
	}
	return Mosaic{Pix: pix, Stride: w * 3, Samples: 3, W: w, H: h}, p, sceneAt
}

func TestLinearRawSkipsTheDemosaic(t *testing.T) {
	m, p, scene := linearRaw(70, 50, 256, 4095)
	img, _, err := Develop(context.Background(), m, p, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	var worst float64
	for y := 0; y < 50; y++ {
		for x := 0; x < 70; x++ {
			want := expected8(scene(x, y), 1)
			for c := 0; c < 3; c++ {
				worst = math.Max(worst, math.Abs(float64(img.Pix[y*img.Stride+4*x+c])-want[c]))
			}
		}
	}
	t.Logf("linear raw: max %.1f", worst)
	if worst > 2 {
		t.Fatalf("linear raw error %.1f", worst)
	}
	for _, scale := range []int{2, 4, 8} {
		_, small, err := Develop(context.Background(), m, p, scale, true)
		if err != nil || small == nil {
			t.Fatalf("scale %d: %v", scale, err)
		}
		if w, h := OutputSize(p.Crop, scale); small.Bounds().Dx() != w || small.Bounds().Dy() != h {
			t.Fatalf("scale %d: %v", scale, small.Bounds())
		}
	}
}

func TestParallelAndSerialAgree(t *testing.T) {
	s := makeSynthetic(synthOptions{W: 210, H: 150, CFA: rggb})
	defer par.Set(par.Configured())
	par.Set(1)
	a, _, err := Develop(context.Background(), s.mosaic, s.params, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	par.Set(8)
	b, _, err := Develop(context.Background(), s.mosaic, s.params, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(a.Pix) != string(b.Pix) {
		t.Fatal("results differ between 1 and 8 threads")
	}
}

func TestBadParametersAreRejected(t *testing.T) {
	good := makeSynthetic(synthOptions{W: 32, H: 32, CFA: rggb})
	mutations := map[string]func(*Mosaic, *Params){
		"crop outside":      func(m *Mosaic, p *Params) { p.Crop = image.Rect(0, 0, 33, 32) },
		"empty crop":        func(m *Mosaic, p *Params) { p.Crop = image.Rect(5, 5, 5, 9) },
		"negative crop":     func(m *Mosaic, p *Params) { p.Crop = image.Rect(-1, 0, 8, 8) },
		"white below black": func(m *Mosaic, p *Params) { p.White[0] = 100 },
		"NaN white":         func(m *Mosaic, p *Params) { p.White[1] = math.NaN() },
		"bad pattern":       func(m *Mosaic, p *Params) { p.CFA = [4]uint8{0, 0, 1, 2} },
		"pattern colour":    func(m *Mosaic, p *Params) { p.CFA = [4]uint8{0, 1, 1, 3} },
		"short pixels":      func(m *Mosaic, p *Params) { m.Pix = m.Pix[:100] },
		"huge table":        func(m *Mosaic, p *Params) { p.Table = make([]uint16, 70000) },
		"NaN matrix":        func(m *Mosaic, p *Params) { p.Render.Matrix[4] = math.NaN() },
		"zero gain":         func(m *Mosaic, p *Params) { p.Render.Gain[2] = 0 },
		"four samples":      func(m *Mosaic, p *Params) { m.Samples = 4 },
		"stride too small":  func(m *Mosaic, p *Params) { m.Stride = 5 },
		"origin beyond":     func(m *Mosaic, p *Params) { m.X = 1000 },
	}
	for name, mutate := range mutations {
		m, p := good.mosaic, good.params
		mutate(&m, &p)
		if _, _, err := Develop(context.Background(), m, p, 1, false); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, _, err := Develop(context.Background(), good.mosaic, good.params, 3, false); !errors.Is(err, ErrUnsupported) {
		t.Errorf("scale 3: %v", err)
	}
}

func TestCancellation(t *testing.T) {
	s := makeSynthetic(synthOptions{W: 64, H: 64, CFA: rggb})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := Develop(ctx, s.mosaic, s.params, 1, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestTinyPicturesAndEdges(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {2, 2}, {3, 5}, {5, 3}, {1, 9}, {9, 1}} {
		s := makeSynthetic(synthOptions{W: size[0], H: size[1], CFA: rggb})
		for _, scale := range []int{1, 2, 4, 8} {
			img, _, err := Develop(context.Background(), s.mosaic, s.params, scale, false)
			if err != nil {
				t.Fatalf("%v scale %d: %v", size, scale, err)
			}
			ow, oh := OutputSize(s.params.Crop, scale)
			if img.Bounds().Dx() != ow || img.Bounds().Dy() != oh {
				t.Fatalf("%v scale %d: %v", size, scale, img.Bounds())
			}
		}
	}
}
