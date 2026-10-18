package raw

import (
	"bytes"
	"context"
	"image"
	"math"
	"math/rand"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
	"github.com/oops1/PixGo/internal/rawdev"
	"github.com/oops1/PixGo/internal/rawdev/rawdevtest"
)

var (
	rggbPattern = [4]uint8{0, 1, 1, 2}
	bggrPattern = [4]uint8{2, 1, 1, 0}
	grbgPattern = [4]uint8{1, 0, 2, 1}
	gbrgPattern = [4]uint8{1, 2, 0, 1}
)

const previewW, previewH = 48, 32

func baseSpec(tb testing.TB, w, h int, cfa [4]uint8, bps int) *dngSpec {
	tb.Helper()
	white := float64(int(1)<<uint(bps) - 1)
	black := math.Round(float64(int(1)<<uint(bps)) / 32)
	m := rawdevtest.Bayer(w, h, cfa, [4]float64{black, black, black, black}, white, rawdevtest.SceneAt)
	cm := rawdevtest.ColorMatrix
	neutral := rawdevtest.Neutral()
	return &dngSpec{
		w: w, h: h, spp: 1, bps: bps, cfa: cfa, pix: m.Pix,
		black: []float64{black}, white: []float64{white},
		matrix1: &cm, illum1: 21, neutral: neutral[:],
		preview: jpegOf(tb, previewW, previewH, navy),
	}
}

func linearSpec(tb testing.TB, w, h, bps int) *dngSpec {
	tb.Helper()
	white := float64(int(1)<<uint(bps) - 1)
	black := math.Round(float64(int(1)<<uint(bps)) / 32)
	cm := rawdevtest.ColorMatrix
	neutral := rawdevtest.Neutral()
	return &dngSpec{
		w: w, h: h, spp: 3, bps: bps, pix: rawdevtest.Linear(w, h, black, white, rawdevtest.SceneAt),
		black: []float64{black, black, black}, white: []float64{white, white, white},
		matrix1: &cm, illum1: 21, neutral: neutral[:],
		preview: jpegOf(tb, previewW, previewH, navy),
	}
}

func isPreview(img *codec.Image) bool {
	b := img.First().Bounds()
	return img.Info.Preview && b.Dx() == previewW && b.Dy() == previewH
}

type measure struct {
	max  float64
	mean float64
}

func deepOrPix(img *codec.Image) (get func(x, y, c int) float64, w, h int) {
	f := img.Frames[0]
	if f.Deep != nil {
		b := f.Deep.Bounds()
		return func(x, y, c int) float64 {
			o := y*f.Deep.Stride + 8*x + 2*c
			return float64(uint16(f.Deep.Pix[o])<<8|uint16(f.Deep.Pix[o+1])) / 257
		}, b.Dx(), b.Dy()
	}
	b := f.Pix.Bounds()
	return func(x, y, c int) float64 { return float64(f.Pix.Pix[y*f.Pix.Stride+4*x+c]) }, b.Dx(), b.Dy()
}

func compareScene(tb testing.TB, img *codec.Image, origin image.Point, ev float64, margin int) measure {
	tb.Helper()
	get, w, h := deepOrPix(img)
	var m measure
	n := 0
	for y := margin; y < h-margin; y++ {
		for x := margin; x < w-margin; x++ {
			s := rawdevtest.SceneAt(origin.X+x, origin.Y+y)
			for c := 0; c < 3; c++ {
				want := 255 * rawdev.ToneCurve(s[c]*math.Exp2(ev), ev)
				d := math.Abs(get(x, y, c) - want)
				m.max = math.Max(m.max, d)
				m.mean += d
				n++
			}
		}
	}
	if n > 0 {
		m.mean /= float64(n)
	}
	return m
}

func developed(tb testing.TB, data []byte, opts ...func(*codec.Options)) *codec.Image {
	tb.Helper()
	o := codec.Options{Limits: codec.DefaultLimits()}
	for _, f := range opts {
		f(&o)
	}
	img, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), o)
	if err != nil {
		tb.Fatal(err)
	}
	return img
}

func requireDeveloped(tb testing.TB, data []byte, w, h int, tolerance float64, origin image.Point, ev float64) {
	tb.Helper()
	info, err := probeData(data)
	if err != nil {
		tb.Fatal(err)
	}
	if info.Format != codec.RAW || info.FormatName != "DNG" || info.Preview || info.Width != w || info.Height != h || info.Frames != 1 {
		tb.Fatalf("probe: %+v", info)
	}
	img := developed(tb, data)
	if img.Info.Preview || img.Info.Width != w || img.Info.Height != h || img.Scale != 1 || !img.Color.Empty() {
		tb.Fatalf("decode: %+v scale %d", img.Info, img.Scale)
	}
	if img.Frames[0].Pix == nil || img.Frames[0].Deep != nil {
		tb.Fatal("expected an 8 bit picture without Deep")
	}
	if b := img.First().Bounds(); b.Dx() != w || b.Dy() != h {
		tb.Fatalf("picture %v", b)
	}
	m := compareScene(tb, img, origin, ev, 2)
	if m.max > tolerance || m.mean > tolerance/4 {
		tb.Fatalf("max %.1f mean %.2f levels (limit %.1f)", m.max, m.mean, tolerance)
	}
}

func TestDNGLayoutsAreDeveloped(t *testing.T) {
	type layout struct {
		name string
		make func() *dngSpec
		tol  float64
	}
	bayer := func(w, h, bps int, f func(*dngSpec)) func() *dngSpec {
		return func() *dngSpec {
			s := baseSpec(t, w, h, rggbPattern, bps)
			f(s)
			return s
		}
	}
	lin := func(w, h, bps int, f func(*dngSpec)) func() *dngSpec {
		return func() *dngSpec {
			s := linearSpec(t, w, h, bps)
			f(s)
			return s
		}
	}
	none := func(*dngSpec) {}
	cases := []layout{
		{"uncompressed 16 bit", bayer(70, 50, 16, none), 3},
		{"uncompressed 16 bit big endian", bayer(70, 50, 16, func(s *dngSpec) { s.order = big }), 3},
		{"uncompressed 14 bit packed", bayer(70, 50, 14, none), 3},
		{"uncompressed 12 bit packed", bayer(70, 50, 12, none), 4},
		{"uncompressed 10 bit packed", bayer(70, 50, 10, none), 6},
		{"uncompressed 8 bit", bayer(70, 50, 8, none), 16},
		{"uncompressed strips of 7 rows", bayer(70, 50, 12, func(s *dngSpec) { s.rowsPerStrip = 7 }), 4},
		{"uncompressed tiles with edges", bayer(150, 100, 12, func(s *dngSpec) { s.tileW, s.tileH = 64, 48 }), 4},
		{"lossless one strip", bayer(70, 50, 14, func(s *dngSpec) { s.compression = compressionJPEG }), 3},
		{"lossless strips", bayer(70, 50, 14, func(s *dngSpec) { s.compression = compressionJPEG; s.rowsPerStrip = 16 }), 3},
		{"lossless tiles", bayer(150, 100, 14, func(s *dngSpec) { s.compression = compressionJPEG; s.tileW, s.tileH = 64, 32 }), 3},
		{"lossless two components per row", bayer(150, 100, 14, func(s *dngSpec) { s.compression = compressionJPEG; s.ljComps = 2; s.tileW, s.tileH = 64, 32 }), 3},
		{"lossless 12 bit predictor 6", bayer(70, 50, 12, func(s *dngSpec) { s.compression = compressionJPEG; s.ljPredictor = 6 }), 4},
		{"lossless 16 bit long codes", bayer(70, 50, 16, func(s *dngSpec) {
			s.compression = compressionJPEG
			s.ljPredictor = 4
			s.ljLengths = rawdevtestLong()
		}), 3},
		{"deflate", bayer(70, 50, 14, func(s *dngSpec) { s.compression = compressionDeflate }), 3},
		{"deflate horizontal predictor", bayer(70, 50, 14, func(s *dngSpec) { s.compression = compressionDeflate; s.predictor = predictorHorizontal }), 3},
		{"deflate predictor for pairs", bayer(70, 50, 16, func(s *dngSpec) { s.compression = compressionDeflate; s.predictor = predictorHorizontal2 }), 3},
		{"deflate predictor for quads in tiles", bayer(150, 100, 12, func(s *dngSpec) {
			s.compression = compressionDeflate
			s.predictor = predictorHorizontal4
			s.tileW, s.tileH = 64, 64
		}), 4},
		{"Adobe deflate", bayer(70, 50, 14, func(s *dngSpec) { s.compression = compressionAdobe }), 3},
		{"raw data in IFD0", bayer(70, 50, 14, func(s *dngSpec) { s.rawInIFD0 = true; s.preview = nil }), 3},
		{"no preview", bayer(70, 50, 14, func(s *dngSpec) { s.preview = nil }), 3},
		{"linear raw 16 bit", lin(70, 50, 16, none), 3},
		{"linear raw lossless", lin(70, 50, 14, func(s *dngSpec) { s.compression = compressionJPEG }), 3},
		{"linear raw deflate", lin(70, 50, 12, func(s *dngSpec) { s.compression = compressionDeflate; s.predictor = predictorHorizontal }), 4},
		{"linear raw 12 bit tiles", lin(100, 70, 12, func(s *dngSpec) { s.tileW, s.tileH = 32, 32 }), 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec := c.make()
			requireDeveloped(t, spec.build(), spec.w, spec.h, c.tol, image.Point{}, 0)
		})
	}
}

func rawdevtestLong() [17]int {
	return rawdevtest.LengthsLong
}

func TestEveryColorFilterPatternIsDeveloped(t *testing.T) {
	for name, cfa := range map[string][4]uint8{"RGGB": rggbPattern, "BGGR": bggrPattern, "GRBG": grbgPattern, "GBRG": gbrgPattern} {
		for _, order := range []byteOrder{little, big} {
			spec := baseSpec(t, 90, 60, cfa, 14)
			spec.order = order
			spec.compression = compressionJPEG
			requireDeveloped(t, spec.build(), 90, 60, 3, image.Point{}, 0)
			_ = name
		}
	}
}

func TestBlackLevelPatterns(t *testing.T) {
	w, h, bps := 80, 60, 14
	white := float64(int(1)<<uint(bps) - 1)
	black := [4]float64{600, 480, 520, 700}
	m := rawdevtest.Bayer(w, h, rggbPattern, black, white, rawdevtest.SceneAt)
	spec := baseSpec(t, w, h, rggbPattern, bps)
	spec.pix = m.Pix
	spec.black = black[:]
	spec.blackDim = [2]int{2, 2}
	requireDeveloped(t, spec.build(), w, h, 3, image.Point{}, 0)

	single := baseSpec(t, w, h, grbgPattern, bps)
	single.black = []float64{512}
	requireDeveloped(t, single.build(), w, h, 3, image.Point{}, 0)

	missing := baseSpec(t, w, h, rggbPattern, bps)
	missing.black = nil
	missing.pix = rawdevtest.Bayer(w, h, rggbPattern, [4]float64{}, white, rawdevtest.SceneAt).Pix
	requireDeveloped(t, missing.build(), w, h, 3, image.Point{}, 0)

	rowCol := baseSpec(t, w, h, rggbPattern, bps)
	rowCol.black = []float64{512, 512}
	rowCol.blackDim = [2]int{2, 1}
	requireDeveloped(t, rowCol.build(), w, h, 3, image.Point{}, 0)

	large := baseSpec(t, w, h, rggbPattern, bps)
	large.black = make([]float64, 16)
	for i := range large.black {
		large.black[i] = 512
	}
	large.blackDim = [2]int{4, 4}
	requireDeveloped(t, large.build(), w, h, 3, image.Point{}, 0)
}

func TestLinearRawBlackLevelsPerChannel(t *testing.T) {
	spec := linearSpec(t, 60, 40, 14)
	spec.black = []float64{512, 512, 512}
	requireDeveloped(t, spec.build(), 60, 40, 3, image.Point{}, 0)
}

func TestLinearizationTable(t *testing.T) {
	w, h := 70, 50
	curve := func(v float64) float64 { return 4095 * math.Pow(v/16383, 0.6) }
	back := func(v float64) float64 { return 16383 * math.Pow(v/4095, 1/0.6) }
	spec := baseSpec(t, w, h, rggbPattern, 12)
	black := 512.0
	white := 16383.0
	spec.pix = rawdevtest.Bayer(w, h, rggbPattern, [4]float64{black, black, black, black}, white, rawdevtest.SceneAt).Pix
	for i, v := range spec.pix {
		spec.pix[i] = uint16(math.Round(curve(float64(v))))
	}
	spec.table = make([]uint16, 4096)
	for i := range spec.table {
		spec.table[i] = uint16(math.Round(back(float64(i))))
	}
	spec.black = []float64{black}
	spec.white = []float64{white}
	requireDeveloped(t, spec.build(), w, h, 4, image.Point{}, 0)
}

func TestActiveAreaAndDefaultCrop(t *testing.T) {
	w, h := 100, 80
	for _, c := range []struct {
		name   string
		area   []int
		origin []float64
		size   []float64
		rect   image.Rectangle
	}{
		{"active area only", []int{6, 10, 74, 90}, nil, nil, image.Rect(0, 0, 80, 68)},
		{"crop only", nil, []float64{8, 4}, []float64{60, 50}, image.Rect(8, 4, 68, 54)},
		{"both", []int{6, 10, 74, 90}, []float64{8, 4}, []float64{60, 50}, image.Rect(8, 4, 68, 54)},
		{"odd crop origin", nil, []float64{7, 5}, []float64{61, 45}, image.Rect(7, 5, 68, 50)},
		{"fractional crop", nil, []float64{8.2, 4.4}, []float64{59.7, 49.6}, image.Rect(8, 4, 68, 54)},
	} {
		t.Run(c.name, func(t *testing.T) {
			spec := baseSpec(t, w, h, bggrPattern, 14)
			spec.area, spec.cropOrigin, spec.cropSize = c.area, c.origin, c.size
			origin := image.Point{}
			if c.area != nil {
				origin = image.Pt(c.area[1], c.area[0])
			}
			if c.area != nil {
				sub := rawdevtest.Bayer(w, h, bggrPattern, [4]float64{512, 512, 512, 512}, 16383, func(x, y int) [3]float64 {
					return rawdevtest.SceneAt(x-origin.X, y-origin.Y)
				})
				spec.pix = sub.Pix
			}
			data := spec.build()
			info := mustProbe(t, data)
			if info.Width != c.rect.Dx() || info.Height != c.rect.Dy() {
				t.Fatalf("probe %dx%d, want %v", info.Width, info.Height, c.rect)
			}
			requireDeveloped(t, data, c.rect.Dx(), c.rect.Dy(), 3, c.rect.Min, 0)
		})
	}
}

func TestInvalidCropFallsBackToTheActiveArea(t *testing.T) {
	for _, c := range []struct{ origin, size []float64 }{
		{[]float64{-5, 0}, []float64{40, 40}},
		{[]float64{0, 0}, []float64{500, 40}},
		{[]float64{50, 40}, []float64{60, 50}},
		{[]float64{0, 0}, []float64{0, 0}},
	} {
		spec := baseSpec(t, 70, 50, rggbPattern, 14)
		spec.cropOrigin, spec.cropSize = c.origin, c.size
		requireDeveloped(t, spec.build(), 70, 50, 3, image.Point{}, 0)
	}
}

func TestBaselineExposure(t *testing.T) {
	for _, ev := range []float64{-1, 0.5, 1, 2} {
		spec := baseSpec(t, 70, 50, rggbPattern, 14)
		spec.exposure, spec.hasExposure = ev, true
		requireDeveloped(t, spec.build(), 70, 50, 3, image.Point{}, ev)
	}
}

func TestWhitePointAndMatrixVariants(t *testing.T) {
	cm := rawdevtest.ColorMatrix
	other := cm
	for i := range other {
		other[i] *= 1.15
	}
	t.Run("two matrices", func(t *testing.T) {
		spec := baseSpec(t, 70, 50, rggbPattern, 14)
		spec.matrix1, spec.illum1 = &other, 17
		spec.matrix2, spec.illum2 = &cm, 21
		requireDeveloped(t, spec.build(), 70, 50, 6, image.Point{}, 0)
	})
	t.Run("white xy", func(t *testing.T) {
		spec := baseSpec(t, 70, 50, rggbPattern, 14)
		spec.neutral = nil
		spec.whiteXY = []float64{0.3127, 0.3290}
		requireDeveloped(t, spec.build(), 70, 50, 3, image.Point{}, 0)
	})
	t.Run("forward matrix", func(t *testing.T) {
		spec := baseSpec(t, 70, 50, rggbPattern, 14)
		spec.forward1 = forwardFor(cm)
		requireDeveloped(t, spec.build(), 70, 50, 4, image.Point{}, 0)
	})
}

func forwardFor(cm [9]float64) *[9]float64 {
	m := rawdev.Matrix(cm)
	inv, _ := m.Inverse()
	n := rawdevtest.Neutral()
	toXYZ := inv.Mul(rawdev.Matrix{n[0], 0, 0, 0, n[1], 0, 0, 0, n[2]})
	bradford := rawdev.Matrix{0.8951, 0.2664, -0.1614, -0.7502, 1.7135, 0.0367, 0.0389, -0.0685, 1.0296}
	binv, _ := bradford.Inverse()
	src := bradford.Apply([3]float64{0.95047, 1, 1.08883})
	dst := bradford.Apply([3]float64{0.96422, 1, 0.82521})
	scale := rawdev.Matrix{dst[0] / src[0], 0, 0, 0, dst[1] / src[1], 0, 0, 0, dst[2] / src[2]}
	out := binv.Mul(scale).Mul(bradford).Mul(toXYZ)
	res := [9]float64(out)
	return &res
}

func TestOrientationIsReportedAndPicturesAreNotRotated(t *testing.T) {
	for _, o := range []uint16{1, 3, 6, 8} {
		spec := baseSpec(t, 70, 50, rggbPattern, 14)
		spec.orientation = o
		data := spec.build()
		info := mustProbe(t, data)
		img := mustDecode(t, data)
		if uint16(info.Orientation) != o || uint16(img.Info.Orientation) != o {
			t.Fatalf("orientation %d: probe %v decode %v", o, info.Orientation, img.Info.Orientation)
		}
		if info.Width != 70 || info.Height != 50 {
			t.Fatalf("size %dx%d", info.Width, info.Height)
		}
	}
}

func TestBitDepthOfTheInfo(t *testing.T) {
	for _, bps := range []int{10, 12, 14, 16} {
		info := mustProbe(t, baseSpec(t, 30, 20, rggbPattern, bps).build())
		if info.BitDepth != 3*bps {
			t.Fatalf("%d bit raw reports %d", bps, info.BitDepth)
		}
	}
}

func TestScaleBinsTheMosaic(t *testing.T) {
	spec := baseSpec(t, 150, 100, rggbPattern, 14)
	spec.compression = compressionJPEG
	spec.tileW, spec.tileH = 64, 32
	data := spec.build()
	for _, scale := range []int{2, 4, 8} {
		img := developed(t, data, func(o *codec.Options) { o.Scale = scale })
		lw, lh := (150+scale-1)/scale, (100+scale-1)/scale
		if img.Scale != scale || img.Info.Width != 150 || img.Info.Height != 100 || img.Info.Preview {
			t.Fatalf("scale %d: image scale %d info %+v", scale, img.Scale, img.Info)
		}
		if b := img.First().Bounds(); b.Dx() != lw || b.Dy() != lh {
			t.Fatalf("scale %d: %v want %dx%d", scale, b, lw, lh)
		}
		var worst float64
		get, w, h := deepOrPix(img)
		for y := 1; y < h-1; y++ {
			for x := 1; x < w-1; x++ {
				for c := 0; c < 3; c++ {
					var want float64
					for j := 0; j < scale; j++ {
						for i := 0; i < scale; i++ {
							want += 255 * rawdev.ToneCurve(rawdevtest.SceneAt(x*scale+i, y*scale+j)[c], 0) / float64(scale*scale)
						}
					}
					worst = math.Max(worst, math.Abs(get(x, y, c)-want))
				}
			}
		}
		t.Logf("scale %d: max %.1f levels from the box average", scale, worst)
		if worst > 8 {
			t.Fatalf("scale %d deviates by %.1f", scale, worst)
		}
	}
	if img := developed(t, data, func(o *codec.Options) { o.Scale = 3 }); img.Scale != 2 {
		t.Fatalf("scale 3 gives %d", img.Scale)
	}
	if img := developed(t, data, func(o *codec.Options) { o.Scale = 100 }); img.Scale != 8 {
		t.Fatalf("scale 100 gives %d", img.Scale)
	}
	if img := developed(t, data, func(o *codec.Options) { o.Scale = 1 }); img.Scale != 1 {
		t.Fatalf("scale 1 gives %d", img.Scale)
	}
}

func TestDeepOptionReturnsSixteenBitPicture(t *testing.T) {
	spec := baseSpec(t, 70, 50, rggbPattern, 14)
	data := spec.build()
	img := developed(t, data, func(o *codec.Options) { o.Deep = true })
	f := img.Frames[0]
	if f.Deep == nil || f.Pix != nil {
		t.Fatalf("deep %v pix %v", f.Deep != nil, f.Pix != nil)
	}
	if b := f.Deep.Bounds(); b.Dx() != 70 || b.Dy() != 50 || !img.Color.Empty() || img.Info.Preview {
		t.Fatalf("%v %+v", b, img.Info)
	}
	m := compareScene(t, img, image.Point{}, 0, 2)
	t.Logf("deep: max %.2f mean %.3f levels of 255", m.max, m.mean)
	if m.max > 3 {
		t.Fatalf("deep error %v", m)
	}
}

func setDevelop(t *testing.T, on bool) {
	t.Helper()
	prev := Decoder{}.Develops()
	Decoder{}.SetDevelop(on)
	t.Cleanup(func() { Decoder{}.SetDevelop(prev) })
}

func TestSwitchingDevelopmentOffShowsThePreview(t *testing.T) {
	data := baseSpec(t, 70, 50, rggbPattern, 14).build()
	setDevelop(t, false)
	info := mustProbe(t, data)
	if !info.Preview || info.FormatName != "RAW" || info.Width != previewW || info.Height != previewH {
		t.Fatalf("probe %+v", info)
	}
	img := mustDecode(t, data)
	if !isPreview(img) {
		t.Fatalf("decode %+v", img.Info)
	}
	Decoder{}.SetDevelop(true)
	if mustProbe(t, data).Preview {
		t.Fatal("development did not come back")
	}
}

func TestDevelopmentDefaultsToOn(t *testing.T) {
	if !(Decoder{}).Develops() {
		t.Fatal("DNG development is off by default")
	}
}

func TestOtherRawFormatsKeepTheEmbeddedPreview(t *testing.T) {
	for _, s := range allSamples(t) {
		img := mustDecode(t, s.data)
		if !img.Info.Preview || img.Info.FormatName != "RAW" {
			t.Fatalf("%s: %+v", s.name, img.Info)
		}
	}
}

func TestUnsupportedVariantsFallBackToThePreview(t *testing.T) {
	mutate := map[string]func(*dngSpec){
		"float samples":      func(s *dngSpec) { s.sampleFmt = 3 },
		"lossy compression":  func(s *dngSpec) { s.override = []entry{shortEntry(tagCompression, 34892)} },
		"floating predictor": func(s *dngSpec) { s.compression = compressionDeflate; s.predictor = 34894 },
		"predictor 3":        func(s *dngSpec) { s.compression = compressionDeflate; s.predictor = 3 },
		"planar":             func(s *dngSpec) { s.override = []entry{shortEntry(0x011c, 2)} },
		"six by six pattern": func(s *dngSpec) {
			s.override = []entry{{tag: 0x828d, typ: typeShort, count: 2, inline: []byte{6, 0, 6, 0}}}
		},
		"no pattern":  func(s *dngSpec) { s.skipCFA = true },
		"bad pattern": func(s *dngSpec) { s.cfa = [4]uint8{0, 0, 0, 0} },
		"four colors": func(s *dngSpec) {
			s.extra = []entry{{tag: 0xc616, typ: typeByte, count: 4, inline: []byte{0, 1, 2, 1}}}
		},
		"staggered layout":    func(s *dngSpec) { s.extra = []entry{shortEntry(0xc617, 2)} },
		"no color matrix":     func(s *dngSpec) { s.matrix1 = nil },
		"white below black":   func(s *dngSpec) { s.white = []float64{100} },
		"seven bits":          func(s *dngSpec) { s.bps = 7 },
		"seventeen bits":      func(s *dngSpec) { s.bps = 17 },
		"unknown compression": func(s *dngSpec) { s.override = []entry{shortEntry(tagCompression, 99)} },
		"zero neutral":        func(s *dngSpec) { s.neutral = []float64{0.5, 0, 0.5} },
	}
	for name, f := range mutate {
		t.Run(name, func(t *testing.T) {
			spec := baseSpec(t, 70, 50, rggbPattern, 14)
			if name == "seven bits" || name == "seventeen bits" {
				spec.pix = make([]uint16, 70*50)
			}
			f(spec)
			data := spec.build()
			info := mustProbe(t, data)
			if !info.Preview || info.Width != previewW {
				t.Fatalf("probe %+v", info)
			}
			if img := mustDecode(t, data); !isPreview(img) {
				t.Fatalf("decode %+v", img.Info)
			}
			spec.preview = nil
			if _, err := decodeData(spec.build()); err == nil {
				t.Fatal("decoded without preview or support")
			} else {
				testimg.RequireCodecError(t, err)
			}
		})
	}
}

func TestDamagedSensorDataFallsBackToThePreview(t *testing.T) {
	spec := baseSpec(t, 70, 50, rggbPattern, 14)
	spec.compression = compressionJPEG
	spec.rowsPerStrip = 10
	good := spec.build()
	rawStart := bytes.Index(good, []byte{0xff, 0xd8, 0xff, 0xc4})
	if rawStart < 0 {
		t.Fatal("no lossless stream")
	}
	frame := rawStart + bytes.Index(good[rawStart:], []byte{0xff, 0xc3})
	wrongHeight := append([]byte(nil), good...)
	wrongHeight[frame+5], wrongHeight[frame+6] = 0, 9
	if img := mustDecode(t, wrongHeight); !isPreview(img) {
		t.Fatalf("a block of the wrong size was accepted: %+v", img.Info)
	}
	garbage := append([]byte(nil), good...)
	for i := rawStart + 30; i < rawStart+60; i++ {
		garbage[i] = 0xff
	}
	if img, err := decodeData(garbage); err != nil {
		testimg.RequireCodecError(t, err)
	} else if img.First() == nil {
		t.Fatal("no picture")
	}
	spec2 := baseSpec(t, 70, 50, rggbPattern, 14)
	data := spec2.build()
	trunc := data[:len(data)-len(spec2.preview)-200]
	if img, err := decodeData(trunc); err == nil && img.Info.Preview {
		t.Fatal("truncated file claims a preview")
	} else if err != nil {
		testimg.RequireCodecError(t, err)
	}
}

func TestPixelLimitsApplyToTheDevelopedPicture(t *testing.T) {
	spec := baseSpec(t, 70, 50, rggbPattern, 14)
	data := spec.build()
	lim := codec.DefaultLimits()
	lim.MaxPixels = 70*50 - 1
	info, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
	if err != nil || !info.Preview {
		t.Fatalf("over the limit the preview is shown: %+v %v", info, err)
	}
	lim.MaxPixels = 70 * 50
	if info, err = (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim); err != nil || info.Preview {
		t.Fatalf("at the limit: %+v %v", info, err)
	}
	lim.MaxFileBytes = int64(len(data) - 1)
	if _, err = (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim); err == nil {
		t.Fatal("file over the byte limit accepted")
	}
}

func TestDevelopmentHonoursCancellation(t *testing.T) {
	spec := baseSpec(t, 200, 150, rggbPattern, 14)
	spec.compression = compressionJPEG
	spec.tileW, spec.tileH = 32, 32
	data := spec.build()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Decoder{}).Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{}); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
	for after := 1; after < 12; after++ {
		ctx, cancel := context.WithCancel(context.Background())
		r := &cancelReader{r: bytes.NewReader(data), after: after, cancel: cancel}
		_, err := (Decoder{}).Decode(ctx, r, int64(len(data)), codec.Options{})
		if err != nil && err != context.Canceled {
			testimg.RequireCodecError(t, err)
		}
		cancel()
	}
}

func TestHostileTileAndStripTables(t *testing.T) {
	base := func() *dngSpec {
		s := baseSpec(t, 70, 50, rggbPattern, 14)
		s.compression = compressionJPEG
		s.tileW, s.tileH = 32, 32
		return s
	}
	cases := map[string]func(*dngSpec){
		"huge tiles":        func(s *dngSpec) { s.override = []entry{longEntry(0x0142, 60000), longEntry(0x0143, 60000)} },
		"tiny tiles":        func(s *dngSpec) { s.tileW, s.tileH = 1, 1 },
		"zero tile width":   func(s *dngSpec) { s.override = []entry{longEntry(0x0142, 0)} },
		"huge dimensions":   func(s *dngSpec) { s.override = []entry{longEntry(0x0100, 0xfffffff0), longEntry(0x0101, 0xfffffff0)} },
		"negative-looking":  func(s *dngSpec) { s.override = []entry{longEntry(0x0100, 0xffffffff)} },
		"short offsets":     func(s *dngSpec) { s.drop = []uint16{0x0144} },
		"short counts":      func(s *dngSpec) { s.drop = []uint16{0x0145} },
		"huge rows":         func(s *dngSpec) { s.tileW = 0; s.override = []entry{longEntry(0x0116, 0xffffffff)} },
		"zero rows":         func(s *dngSpec) { s.tileW = 0; s.override = []entry{longEntry(0x0116, 0)} },
		"bad bits":          func(s *dngSpec) { s.override = []entry{shortEntry(0x0102, 0)} },
		"huge black":        func(s *dngSpec) { s.black = make([]float64, 70000); s.blackDim = [2]int{1, 1} },
		"huge table":        func(s *dngSpec) { s.table = make([]uint16, 65535) },
		"empty table":       func(s *dngSpec) { s.extra = []entry{{tag: 0xc618, typ: typeShort, count: 0}} },
		"area outside":      func(s *dngSpec) { s.area = []int{0, 0, 5000, 5000} },
		"area inverted":     func(s *dngSpec) { s.area = []int{40, 60, 10, 10} },
		"nan neutral":       func(s *dngSpec) { s.neutral = []float64{math.NaN(), 1, 1} },
		"huge exposure":     func(s *dngSpec) { s.exposure, s.hasExposure = 1e9, true },
		"zero white":        func(s *dngSpec) { s.white = []float64{0} },
		"fractional matrix": func(s *dngSpec) { m := [9]float64{}; s.matrix1 = &m },
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			spec := base()
			f(spec)
			data := func() (d []byte) {
				defer func() {
					if recover() != nil {
						d = nil
					}
				}()
				return spec.build()
			}()
			if data == nil {
				return
			}
			testimg.Within(t, 5e9, func() {
				if info, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), testimg.SmallLimits()); err != nil {
					testimg.RequireCodecError(t, err)
				} else if info.Width <= 0 || info.Height <= 0 {
					t.Fatalf("probe %+v", info)
				}
				if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: testimg.SmallLimits()}); err != nil {
					testimg.RequireCodecError(t, err)
				}
			})
		})
	}
}

func dngSamples(tb testing.TB) []sample {
	var out []sample
	build := func(name string, f func(*dngSpec), w, h int) {
		s := baseSpec(tb, w, h, rggbPattern, 12)
		f(s)
		out = append(out, sample{name, s.build()})
	}
	build("plain", func(s *dngSpec) {}, 20, 12)
	build("lossless", func(s *dngSpec) { s.compression = compressionJPEG }, 20, 12)
	build("lossless tiles", func(s *dngSpec) { s.compression = compressionJPEG; s.tileW, s.tileH = 8, 8 }, 20, 12)
	build("deflate", func(s *dngSpec) { s.compression = compressionDeflate; s.predictor = predictorHorizontal }, 20, 12)
	build("big endian", func(s *dngSpec) { s.order = big; s.compression = compressionJPEG; s.ljComps = 2 }, 20, 12)
	lin := linearSpec(tb, 16, 10, 12)
	lin.compression = compressionJPEG
	out = append(out, sample{"linear", lin.build()})
	return out
}

func TestEveryTruncatedDNGIsRejectedOrYieldsAConsistentPicture(t *testing.T) {
	for _, s := range dngSamples(t) {
		for n := 0; n < len(s.data); n += 3 {
			data := s.data[:n]
			info, err := probeData(data)
			if err != nil {
				testimg.RequireCodecError(t, err)
			}
			img, err := decodeData(data)
			if err != nil {
				testimg.RequireCodecError(t, err)
				continue
			}
			b := img.First().Bounds()
			if err == nil && info.Width > 0 && !img.Info.Preview && (b.Dx() != info.Width || b.Dy() != info.Height) {
				t.Fatalf("%s at %d: probe %dx%d, decode %v", s.name, n, info.Width, info.Height, b)
			}
		}
	}
}

func TestRandomDamageToDNGNeverPanics(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for _, s := range dngSamples(t) {
		for i := 0; i < 200; i++ {
			mutated := append([]byte(nil), s.data...)
			for k := 0; k < 1+rng.Intn(8); k++ {
				mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
			}
			if _, err := probeData(mutated); err != nil {
				testimg.RequireCodecError(t, err)
			}
			img, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(mutated), int64(len(mutated)), codec.Options{Limits: testimg.SmallLimits(), Deep: i%2 == 0, Scale: []int{1, 2, 4, 8}[i%4]})
			if err != nil {
				testimg.RequireCodecError(t, err)
			} else if img.Frames[0].Pix == nil && img.Frames[0].Deep == nil {
				t.Fatalf("%s: success without pixels", s.name)
			}
		}
	}
}

func FuzzDNG(f *testing.F) {
	for _, s := range dngSamples(f) {
		f.Add(s.data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := testimg.SmallLimits()
		info, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
		for _, o := range []codec.Options{{Limits: lim}, {Limits: lim, Deep: true, Scale: 4}} {
			img, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), o)
			if err != nil {
				testimg.RequireCodecError(t, err)
				continue
			}
			if len(img.Frames) != 1 || (img.Frames[0].Pix == nil && img.Frames[0].Deep == nil) {
				t.Fatal("success without pixels")
			}
			if err == nil && info.Width > 0 && !img.Info.Preview && o.Scale == 0 {
				if b := img.First().Bounds(); b.Dx() != info.Width || b.Dy() != info.Height {
					t.Fatalf("probe %dx%d decode %v", info.Width, info.Height, b)
				}
			}
		}
	})
}

func dngBenchSpec(tb testing.TB, w, h int, variant string) *dngSpec {
	spec := baseSpec(tb, 64, 64, rggbPattern, 14)
	pix := make([]uint16, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pix[y*w+x] = uint16(1000 + (x*7+y*13)%9000 + (x*y)%97)
		}
	}
	spec.w, spec.h, spec.pix = w, h, pix
	spec.preview = nil
	switch variant {
	case "uncompressed":
	case "lossless tiles":
		spec.compression = compressionJPEG
		spec.tileW, spec.tileH = 512, 512
	case "lossless single strip":
		spec.compression = compressionJPEG
	case "lossless strips":
		spec.compression = compressionJPEG
		spec.rowsPerStrip = 256
	case "deflate tiles":
		spec.compression = compressionDeflate
		spec.predictor = predictorHorizontal
		spec.tileW, spec.tileH = 512, 512
	}
	return spec
}

var benchVariants = []string{"uncompressed", "lossless tiles", "lossless strips", "lossless single strip", "deflate tiles"}

func BenchmarkDevelop24Mpx(b *testing.B) {
	for _, variant := range benchVariants {
		data := dngBenchSpec(b, 6000, 4000, variant).build()
		b.Run(variant, func(b *testing.B) {
			b.ReportMetric(float64(len(data))/(1<<20), "MiB")
			for i := 0; i < b.N; i++ {
				if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFirstPicture24Mpx(b *testing.B) {
	for _, variant := range benchVariants {
		data := dngBenchSpec(b, 6000, 4000, variant).build()
		b.Run(variant, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Scale: 4}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestOversizedTilesAreRefusedBeforeAnyAllocation(t *testing.T) {
	spec := baseSpec(t, 70, 50, rggbPattern, 14)
	spec.compression = compressionJPEG
	spec.tileW, spec.tileH = 32, 32
	spec.override = []entry{longEntry(0x0142, 60000), longEntry(0x0143, 60000)}
	data := spec.build()
	lim := codec.DefaultLimits()
	lim.MaxPixels = 1 << 20
	info, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
	if err != nil || !info.Preview {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestTheLargestFullResolutionRawIFDIsChosen(t *testing.T) {
	spec := baseSpec(t, 70, 50, rggbPattern, 14)
	spec.decoys = []decoy{{w: 20, h: 10}, {w: 400, h: 300, subType: 1}, {w: 30, h: 20}}
	requireDeveloped(t, spec.build(), 70, 50, 3, image.Point{}, 0)
	small := baseSpec(t, 30, 20, rggbPattern, 14)
	small.decoys = []decoy{{w: 70, h: 50}}
	data := small.build()
	if info := mustProbe(t, data); info.Width != 70 || info.Height != 50 {
		t.Fatalf("the larger raw IFD was not chosen: %+v", info)
	}
}
