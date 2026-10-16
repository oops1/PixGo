package jpegxform

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"math/rand"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func TestIdentityKeepsEveryCoefficientOfARandomWall(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, sc := range samplingCases {
		for _, style := range []tableStyle{flatTables, skewedTables} {
			for _, restart := range []int{0, 1, 3, 7} {
				t.Run(fmt.Sprintf("%s/style%d/restart%d", sc.name, style, restart), func(t *testing.T) {
					s := samplingSpec(100, 70, sc.factors...)
					s.style, s.restart = style, restart
					wall := randomWall(rng, s)
					data := buildJPEG(t, s, wall)

					_, parsed := parseCoefficients(t, data)
					if !gridsEqual(parsed, wall) {
						t.Fatal("the decoder lost a coefficient of the source")
					}
					out := mustTransform(t, data, Identity, Options{})
					_, again := parseCoefficients(t, out)

					if !gridsEqual(again, wall) {
						t.Fatal("the identity transform changed a coefficient")
					}
				})
			}
		}
	}
}

func TestEveryOperationMovesTheCoefficientsLikeTheOrientationSays(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for _, sc := range samplingCases {
		for _, op := range allOps {
			t.Run(fmt.Sprintf("%s/%v", sc.name, op), func(t *testing.T) {
				s := samplingSpec(96, 64, sc.factors...)
				wall := randomWall(rng, s)
				data := buildJPEG(t, s, wall)

				perfect, err := Perfect(bytes.NewReader(data), int64(len(data)), op)
				if err != nil || !perfect {
					t.Fatalf("Perfect = %v, %v", perfect, err)
				}
				out := mustTransform(t, data, op, Options{})

				mx, my := s.mcus()
				checkPlanes(t, data, out, op, mx, my)
			})
		}
	}
}

func TestTrimCropsExactlyThePartialBlocksThatWouldWrap(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	widthOps := map[Op]bool{FlipH: true, Rotate180: true, Rotate270: true, Transverse: true}
	heightOps := map[Op]bool{FlipV: true, Rotate180: true, Rotate90: true, Transverse: true}
	for _, sc := range samplingCases {
		for _, op := range allOps {
			t.Run(fmt.Sprintf("%s/%v", sc.name, op), func(t *testing.T) {
				s := samplingSpec(101, 75, sc.factors...)
				wall := randomWall(rng, s)
				data := buildJPEG(t, s, wall)
				hm, vm := s.hmaxVmax()
				mcuW, mcuH := 8*hm, 8*vm
				cropW, cropH := s.width, s.height
				if widthOps[op] {
					cropW -= cropW % mcuW
				}
				if heightOps[op] {
					cropH -= cropH % mcuH
				}
				wantPerfect := cropW == s.width && cropH == s.height

				perfect, err := Perfect(bytes.NewReader(data), int64(len(data)), op)
				if err != nil || perfect != wantPerfect {
					t.Fatalf("Perfect = %v, %v; want %v", perfect, err, wantPerfect)
				}
				_, err = runTransform(data, op, Options{})
				if wantPerfect != (err == nil) || (!wantPerfect && err != ErrImperfect) {
					t.Fatalf("without Trim: %v", err)
				}
				out := mustTransform(t, data, op, Options{Trim: true})

				hout, _ := parseCoefficients(t, out)
				wantW, wantH := cropW, cropH
				if op.Transposes() {
					wantW, wantH = cropH, cropW
				}
				if hout.fr.width != wantW || hout.fr.height != wantH {
					t.Fatalf("output %dx%d, want %dx%d", hout.fr.width, hout.fr.height, wantW, wantH)
				}
				if s.width-cropW >= mcuW || s.height-cropH >= mcuH || s.width-cropW > 31 || s.height-cropH > 15 {
					t.Fatalf("trimmed %d and %d pixels", s.width-cropW, s.height-cropH)
				}
				checkPlanes(t, data, out, op, cropW/mcuW+bool2int(cropW%mcuW != 0), cropH/mcuH+bool2int(cropH%mcuH != 0))
			})
		}
	}
}

func bool2int(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestPerfectRuleMatchesJpegtranForEveryOperation(t *testing.T) {
	type size struct{ w, h int }
	aligned := size{64, 48}
	badW := size{70, 48}
	badH := size{64, 50}
	badBoth := size{70, 50}
	tests := []struct {
		op                          Op
		aligned, onlyW, onlyH, both bool
	}{
		{Identity, true, true, true, true},
		{FlipH, true, false, true, false},
		{FlipV, true, true, false, false},
		{Rotate90, true, true, false, false},
		{Rotate180, true, false, false, false},
		{Rotate270, true, false, true, false},
		{Transpose, true, true, true, true},
		{Transverse, true, false, false, false},
	}
	for _, tc := range tests {
		for i, sz := range []size{aligned, badW, badH, badBoth} {
			want := []bool{tc.aligned, tc.onlyW, tc.onlyH, tc.both}[i]
			s := samplingSpec(sz.w, sz.h, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
			data := buildJPEG(t, s, randomWall(rand.New(rand.NewSource(4)), s))

			got, err := Perfect(bytes.NewReader(data), int64(len(data)), tc.op)

			if err != nil || got != want {
				t.Errorf("%v on %dx%d: %v, %v; want %v", tc.op, sz.w, sz.h, got, err, want)
			}
		}
	}
}

func TestSmallerThanOneBlockCannotBeTrimmed(t *testing.T) {
	s := samplingSpec(10, 40, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
	data := buildJPEG(t, s, randomWall(rand.New(rand.NewSource(5)), s))

	_, err := runTransform(data, FlipH, Options{Trim: true})

	if err == nil || !errors.Is(err, ErrImperfect) {
		t.Fatalf("got %v", err)
	}
	if _, err := runTransform(data, Transpose, Options{}); err != nil {
		t.Fatalf("a transpose never needs trimming: %v", err)
	}
}

func TestOutputWithAnyOperationDecodesWithTheStandardLibrary(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	for _, sc := range samplingCases {
		t.Run(sc.name, func(t *testing.T) {
			s := samplingSpec(96, 64, sc.factors...)
			for _, op := range allOps {
				data := buildJPEG(t, s, randomWall(rng, s))
				out := mustTransform(t, data, op, Options{})

				img, err := decodeStdErr(out)

				if err != nil {
					t.Fatalf("%v: %v", op, err)
				}
				wantW, wantH := op.Orientation().Size(96, 64)
				if b := img.Bounds(); b.Dx() != wantW || b.Dy() != wantH {
					t.Fatalf("%v: %dx%d", op, b.Dx(), b.Dy())
				}
			}
		})
	}
}

func TestCorpusMatchesTheStandardDecoderWithinTheIDCTRounding(t *testing.T) {
	var worst int
	var exact, total int
	for _, name := range supportedCorpus(t) {
		data := readCorpus(t, name)
		in := stdDecode(t, data)
		hdr, err := parseHeaderOnly(data)
		if err != nil {
			t.Fatal(err)
		}
		for _, op := range allOps {
			out := mustTransform(t, data, op, Options{Trim: true})
			img, err := decodeStdErr(out)
			if err != nil {
				t.Fatalf("%s %v: %v", name, op, err)
			}
			cw, ch, _ := edgeCrop(&hdr.fr, op)
			want := orientImage(cropImage(in, cw, ch), op)

			d := compareImages(t, img, want)

			if op == Identity && d.max != 0 {
				t.Errorf("%s: the identity changed pixels (max %d)", name, d.max)
			}
			if d.max > 4 || d.mean() > 0.1 {
				t.Errorf("%s %v: max %d mean %.3f", name, op, d.max, d.mean())
			}
			worst = max(worst, d.max)
			exact += d.exact
			total += d.total
		}
	}
	t.Logf("worst difference %d; %.2f%% of the samples are bit-exact", worst, 100*float64(exact)/float64(total))
	if float64(exact) < 0.9*float64(total) {
		t.Errorf("only %d of %d samples are exact", exact, total)
	}
}

func TestOptimisedTablesKeepTheFileWithinTwoPercentOfTheSource(t *testing.T) {
	for _, name := range supportedCorpus(t) {
		data := readCorpus(t, name)
		for _, op := range allOps {
			out := mustTransform(t, data, op, Options{Trim: true})
			if limit := len(data) + len(data)/50; len(out) > limit {
				t.Errorf("%s %v: %d bytes from %d", name, op, len(out), len(data))
			}
		}
		out := mustTransform(t, data, Identity, Options{})
		t.Logf("%s: %d -> %d bytes (%.1f%%)", name, len(data), len(out), 100*float64(len(out))/float64(len(data)))
	}
}

func TestEveryEXIFOrientationIsTheTransformThatMakesThePictureUpright(t *testing.T) {
	for o := codec.OrientNormal; o <= codec.OrientRotate270; o++ {
		op := FromOrientation(o)
		if op.Orientation() != o {
			t.Errorf("orientation %d -> %v -> %d", o, op, op.Orientation())
		}
	}
	if FromOrientation(0) != Identity || FromOrientation(9) != Identity || Op(200).Orientation() != codec.OrientNormal {
		t.Error("invalid values must map to the identity")
	}
	want := map[codec.Orientation]Op{
		1: Identity, 2: FlipH, 3: Rotate180, 4: FlipV, 5: Transpose, 6: Rotate90, 7: Transverse, 8: Rotate270,
	}
	for o, op := range want {
		if FromOrientation(o) != op {
			t.Errorf("orientation %d is %v, want %v", o, FromOrientation(o), op)
		}
	}
}

func TestTransposingOperationsSwapSamplingAndTransposeTheQuantTables(t *testing.T) {
	s := samplingSpec(96, 64, [2]int{2, 1}, [2]int{1, 1}, [2]int{1, 1})
	data := buildJPEG(t, s, randomWall(rand.New(rand.NewSource(7)), s))

	out := mustTransform(t, data, Rotate90, Options{})

	hin, _ := parseCoefficients(t, data)
	hout, _ := parseCoefficients(t, out)
	if c := hout.fr.comps[0]; c.h != 1 || c.v != 2 {
		t.Fatalf("luma sampling %dx%d, want 1x2", c.h, c.v)
	}
	if hout.fr.width != 64 || hout.fr.height != 96 {
		t.Fatalf("size %dx%d", hout.fr.width, hout.fr.height)
	}
	for id := 0; id < 2; id++ {
		var natIn, natOut [64]uint16
		for k := 0; k < 64; k++ {
			natIn[zigzag[k]] = hin.quant[id].values[k]
			natOut[zigzag[k]] = hout.quant[id].values[k]
		}
		for r := 0; r < 8; r++ {
			for c := 0; c < 8; c++ {
				if natOut[r*8+c] != natIn[c*8+r] {
					t.Fatalf("table %d entry (%d,%d) is %d, want %d", id, r, c, natOut[r*8+c], natIn[c*8+r])
				}
			}
		}
	}
}

func TestComponentOrderIdsAndTableSharingSurviveTheTransform(t *testing.T) {
	s := samplingSpec(64, 48, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
	s.comps[0].id, s.comps[1].id, s.comps[2].id = 'R', 'G', 'B'
	s.comps[2].tq = 1
	s.dcTables = []int{0, 1, 1}
	s.acTables = []int{0, 1, 1}
	s.scanOrder = []int{2, 0, 1}
	wall := randomWall(rand.New(rand.NewSource(8)), s)
	data := buildJPEG(t, s, wall)

	out := mustTransform(t, data, Identity, Options{})

	h, again := parseCoefficients(t, out)
	if h.fr.comps[0].id != 'R' || h.fr.comps[1].id != 'G' || h.fr.comps[2].id != 'B' {
		t.Fatalf("component ids %v", h.fr.comps)
	}
	if h.scan[0] != 2 || h.scan[1] != 0 || h.scan[2] != 1 {
		t.Fatalf("scan order %v", h.scan)
	}
	if !gridsEqual(again, wall) {
		t.Fatal("coefficients changed")
	}
	if h.fr.comps[1].dcTable != h.fr.comps[2].dcTable || h.fr.comps[0].dcTable == h.fr.comps[1].dcTable {
		t.Fatalf("table sharing lost: %+v", h.fr.comps)
	}
}

func TestEightBitBaselineWithWideQuantTablesBecomesExtendedSequential(t *testing.T) {
	s := samplingSpec(48, 32, [2]int{1, 1}, [2]int{1, 1}, [2]int{1, 1})
	s.wideQuant = true
	for i := range s.quant {
		for j := range s.quant[i] {
			s.quant[i][j] += 300
		}
	}
	data := buildJPEG(t, s, randomWall(rand.New(rand.NewSource(9)), s))

	out := mustTransform(t, data, Rotate90, Options{})

	h, _ := parseCoefficients(t, out)
	if h.fr.marker != markerSOF1 || !h.quant[0].wide {
		t.Fatalf("marker %#x wide %v", h.fr.marker, h.quant[0].wide)
	}
}

func TestSourceWithFourTablePairsWritesExtendedSequential(t *testing.T) {
	s := samplingSpec(48, 32, [2]int{1, 1}, [2]int{1, 1}, [2]int{1, 1}, [2]int{1, 1})
	s.dcTables = []int{0, 1, 2, 3}
	s.acTables = []int{0, 1, 2, 3}
	s.sof = markerSOF1
	wall := randomWall(rand.New(rand.NewSource(10)), s)
	data := buildJPEG(t, s, wall)

	out := mustTransform(t, data, FlipH, Options{})

	h, _ := parseCoefficients(t, out)
	if h.fr.marker != markerSOF1 {
		t.Fatalf("marker %#x", h.fr.marker)
	}
	if _, err := decodeStdErr(out); err != nil {
		t.Fatal(err)
	}
}

func TestStandardEncoderFilesOfManySizesRotateLikeThePixels(t *testing.T) {
	rng := rand.New(rand.NewSource(60))
	sizes := [][2]int{{1, 1}, {7, 9}, {8, 8}, {16, 16}, {17, 33}, {31, 15}, {64, 48}, {100, 77}, {129, 130}, {257, 33}}
	for _, sz := range sizes {
		for _, gray := range []bool{false, true} {
			var src image.Image
			if gray {
				g := image.NewGray(image.Rect(0, 0, sz[0], sz[1]))
				for i := range g.Pix {
					g.Pix[i] = uint8(rng.Intn(60) + i*3%150)
				}
				src = g
			} else {
				c := image.NewRGBA(image.Rect(0, 0, sz[0], sz[1]))
				for y := 0; y < sz[1]; y++ {
					for x := 0; x < sz[0]; x++ {
						c.SetRGBA(x, y, color.RGBA{uint8(x * 255 / sz[0]), uint8(y * 255 / sz[1]), uint8(rng.Intn(40) + 100), 255})
					}
				}
				src = c
			}
			var enc bytes.Buffer
			if err := stdjpeg.Encode(&enc, src, &stdjpeg.Options{Quality: 60 + rng.Intn(40)}); err != nil {
				t.Fatal(err)
			}
			data := enc.Bytes()
			in := stdDecode(t, data)
			hdr, err := parseHeaderOnly(data)
			if err != nil {
				t.Fatal(err)
			}
			for _, op := range allOps {
				out, err := runTransform(data, op, Options{Trim: true})
				cw, ch, _ := edgeCrop(&hdr.fr, op)
				if cw == 0 || ch == 0 {
					if !errors.Is(err, ErrImperfect) {
						t.Fatalf("%dx%d gray=%v %v: %v", sz[0], sz[1], gray, op, err)
					}
					continue
				}
				if err != nil {
					t.Fatalf("%dx%d gray=%v %v: %v", sz[0], sz[1], gray, op, err)
				}
				got := stdDecode(t, out)
				want := orientImage(cropImage(in, cw, ch), op)
				if d := compareImages(t, got, want); d.max > 6 {
					t.Errorf("%dx%d gray=%v %v: pixels differ by %d", sz[0], sz[1], gray, op, d.max)
				}
			}
		}
	}
}
