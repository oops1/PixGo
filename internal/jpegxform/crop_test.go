package jpegxform

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math/rand"
	"testing"

	"github.com/oops1/PixGo/internal/meta/exif"
)

func outputDims(t testing.TB, data []byte, op Op) (int, int) {
	t.Helper()
	h, err := parseHeaderOnly(data)
	if err != nil {
		t.Fatal(err)
	}
	g := geometryOf(&h.fr, op)
	return g.outW, g.outH
}

func sampleCrops(w, h int) []image.Rectangle {
	return []image.Rectangle{
		image.Rect(0, 0, w, h),
		image.Rect(1, 1, w-1, h-1),
		image.Rect(w/3, h/3, w*2/3, h*2/3),
		image.Rect(0, 0, min(10, w), min(10, h)),
		image.Rect(w-1, h-1, w, h),
		image.Rect(w-17, 0, w, h),
		image.Rect(0, h/2, w, h/2+1),
		image.Rect(w/2, 0, w/2+1, h),
		image.Rect(-50, -50, w+50, h+50),
		image.Rect(17, 13, 47, 33),
		image.Rect(w, h, 0, 0),
	}
}

func colorsEqual(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func TestCropPixelsEqualTheUncroppedTransform(t *testing.T) {
	for _, name := range supportedCorpus(t) {
		data := readCorpus(t, name)
		if _, err := decodeStdErr(data); err != nil {
			continue
		}
		for _, op := range allOps {
			var whole image.Rectangle
			ref := mustTransform(t, data, op, Options{Trim: true, Applied: &whole})
			refImg := stdDecode(t, ref)
			w, h := outputDims(t, data, op)
			for _, want := range sampleCrops(w, h) {
				t.Run(fmt.Sprintf("%s/%v/%v", name, op, want), func(t *testing.T) {
					checkCropAgainst(t, data, op, want, whole, refImg)
				})
			}
		}
	}
}

func checkCropAgainst(t *testing.T, data []byte, op Op, want, whole image.Rectangle, refImg image.Image) {
	t.Helper()
	var got image.Rectangle
	out, err := runTransform(data, op, Options{Trim: true, Crop: want, Applied: &got})
	w, h := outputDims(t, data, op)
	clamped := want.Canon().Intersect(image.Rect(0, 0, w, h))
	if clamped.Empty() {
		if !errors.Is(err, ErrCrop) {
			t.Fatalf("an empty crop must be refused, got %v", err)
		}
		return
	}
	if errors.Is(err, ErrCrop) {
		if clamped.Max.X > whole.Min.X && clamped.Max.Y > whole.Min.Y {
			t.Fatalf("crop %v lies inside the transformable area %v but was refused", clamped, whole)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if got.Max != clamped.Max {
		t.Fatalf("far corner %v, want the requested %v", got.Max, clamped.Max)
	}
	if got.Min.X > clamped.Min.X && got.Min.X != whole.Min.X || got.Min.Y > clamped.Min.Y && got.Min.Y != whole.Min.Y {
		t.Fatalf("near corner %v moved right/down of the request %v", got.Min, clamped.Min)
	}
	if !got.In(whole) {
		t.Fatalf("result %v outside the transformable area %v", got, whole)
	}
	img := stdDecode(t, out)
	if img.Bounds().Dx() != got.Dx() || img.Bounds().Dy() != got.Dy() {
		t.Fatalf("decoded %v, reported %v", img.Bounds(), got)
	}
	refImgB := refImg.Bounds()
	for y := 0; y < got.Dy(); y++ {
		for x := 0; x < got.Dx(); x++ {
			rx, ry := refImgB.Min.X+got.Min.X-whole.Min.X+x, refImgB.Min.Y+got.Min.Y-whole.Min.Y+y
			if !colorsEqual(img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y), refImg.At(rx, ry)) {
				t.Fatalf("pixel %d,%d differs from the uncropped transform", x, y)
			}
		}
	}
	snapped, err := CropFor(bytes.NewReader(data), int64(len(data)), op, want)
	if err != nil || snapped != got {
		t.Fatalf("CropFor = %v, %v; Transform used %v", snapped, err, got)
	}
}

func TestCropSnapsTheCornerToTheOutputBlockGrid(t *testing.T) {
	cases := []struct {
		name    string
		factors [][2]int
		w, h    int
		op      Op
		want    image.Rectangle
		got     image.Rectangle
	}{
		{"420 identity", [][2]int{{2, 2}, {1, 1}, {1, 1}}, 96, 64, Identity, image.Rect(21, 5, 50, 40), image.Rect(16, 0, 50, 40)},
		{"420 on the grid", [][2]int{{2, 2}, {1, 1}, {1, 1}}, 96, 64, Identity, image.Rect(32, 16, 80, 48), image.Rect(32, 16, 80, 48)},
		{"420 last pixel", [][2]int{{2, 2}, {1, 1}, {1, 1}}, 96, 64, Identity, image.Rect(95, 63, 96, 64), image.Rect(80, 48, 96, 64)},
		{"422 identity", [][2]int{{2, 1}, {1, 1}, {1, 1}}, 96, 64, Identity, image.Rect(31, 15, 60, 30), image.Rect(16, 8, 60, 30)},
		{"422 rotate 90 swaps the cell", [][2]int{{2, 1}, {1, 1}, {1, 1}}, 96, 64, Rotate90, image.Rect(31, 31, 60, 60), image.Rect(24, 16, 60, 60)},
		{"422 transpose swaps the cell", [][2]int{{2, 1}, {1, 1}, {1, 1}}, 96, 64, Transpose, image.Rect(31, 31, 60, 60), image.Rect(24, 16, 60, 60)},
		{"444 identity", [][2]int{{1, 1}, {1, 1}, {1, 1}}, 40, 40, Identity, image.Rect(9, 9, 20, 20), image.Rect(8, 8, 20, 20)},
		{"gray identity", [][2]int{{1, 1}}, 40, 40, Identity, image.Rect(15, 7, 20, 20), image.Rect(8, 0, 20, 20)},
		{"411 identity", [][2]int{{4, 1}, {1, 1}, {1, 1}}, 128, 64, Identity, image.Rect(63, 15, 100, 30), image.Rect(32, 8, 100, 30)},
		{"420 flip h on a full grid", [][2]int{{2, 2}, {1, 1}, {1, 1}}, 96, 64, FlipH, image.Rect(20, 20, 70, 50), image.Rect(16, 16, 70, 50)},
		{"clipped to the picture", [][2]int{{2, 2}, {1, 1}, {1, 1}}, 96, 64, Identity, image.Rect(40, 40, 500, 500), image.Rect(32, 32, 96, 64)},
		{"reversed corners", [][2]int{{2, 2}, {1, 1}, {1, 1}}, 96, 64, Identity, image.Rect(50, 40, 21, 5), image.Rect(16, 0, 50, 40)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := samplingSpec(c.w, c.h, c.factors...)
			data := buildJPEG(t, s, randomWall(rand.New(rand.NewSource(5)), s))
			got, err := CropFor(bytes.NewReader(data), int64(len(data)), c.op, c.want)
			if err != nil || got != c.got {
				t.Fatalf("CropFor = %v, %v; want %v", got, err, c.got)
			}
			var applied image.Rectangle
			out := mustTransform(t, data, c.op, Options{Crop: c.want, Applied: &applied})
			if applied != c.got {
				t.Fatalf("applied %v, want %v", applied, c.got)
			}
			cfg, err := decodeConfigOf(out)
			if err != nil || cfg != c.got.Size() {
				t.Fatalf("output %v %v, want %v", cfg, err, c.got.Size())
			}
		})
	}
}

func decodeConfigOf(data []byte) (image.Point, error) {
	h, err := parseHeaderOnly(data)
	if err != nil {
		return image.Point{}, err
	}
	return image.Pt(h.fr.width, h.fr.height), nil
}

func mirrorOf(op Op, x, y, w, h int) (int, int) {
	u, v := op.Orientation().ToSource(float64(x)+0.5, float64(y)+0.5, float64(w), float64(h))
	return int(u), int(v)
}

func TestCropMatchesThePixelDomainOnAlignedPictures(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	for _, sc := range samplingCases {
		for _, op := range allOps {
			t.Run(fmt.Sprintf("%s/%v", sc.name, op), func(t *testing.T) {
				s := samplingSpec(128, 96, sc.factors...)
				data := buildJPEG(t, s, mildWall(rng, s))
				if _, err := decodeStdErr(data); err != nil {
					t.Skip("the standard decoder does not read this layout")
				}
				src := stdDecode(t, data)
				w, h := outputDims(t, data, op)
				for _, want := range []image.Rectangle{image.Rect(0, 0, w, h), image.Rect(37, 21, w-9, h-5), image.Rect(w-30, h-20, w, h), image.Rect(3, 70, 33, 71)} {
					want = want.Intersect(image.Rect(0, 0, w, h))
					if want.Empty() {
						continue
					}
					var got image.Rectangle
					out := mustTransform(t, data, op, Options{Crop: want, Applied: &got})
					img := stdDecode(t, out)
					worst := 0
					for y := 0; y < got.Dy(); y++ {
						for x := 0; x < got.Dx(); x++ {
							sx, sy := mirrorOf(op, got.Min.X+x, got.Min.Y+y, src.Bounds().Dx(), src.Bounds().Dy())
							r1, g1, b1, _ := img.At(x, y).RGBA()
							r2, g2, b2, _ := src.At(sx, sy).RGBA()
							for _, d := range []int{int(r1>>8) - int(r2>>8), int(g1>>8) - int(g2>>8), int(b1>>8) - int(b2>>8)} {
								worst = max(worst, max(d, -d))
							}
						}
					}
					if worst > 4 {
						t.Fatalf("crop %v: the pixels differ from the flipped source by %d levels", got, worst)
					}
				}
			})
		}
	}
}

func TestCropOfAnImperfectPictureNeedsTrimOnlyWhereItTouchesTheEdge(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	factors := [][2]int{{2, 2}, {1, 1}, {1, 1}}
	s := samplingSpec(101, 75, factors...)
	data := buildJPEG(t, s, randomWall(rng, s))
	type pad struct{ x, y int }
	pads := map[Op]pad{
		Identity: {0, 0}, FlipH: {5, 0}, FlipV: {0, 11}, Rotate180: {5, 11},
		Rotate90: {11, 0}, Rotate270: {0, 5}, Transpose: {0, 0}, Transverse: {11, 5},
	}
	for _, op := range allOps {
		t.Run(op.String(), func(t *testing.T) {
			p := pads[op]
			w, h := outputDims(t, data, op)
			if p.x > 0 {
				if _, err := runTransform(data, op, Options{Crop: image.Rect(p.x-1, 0, w, h)}); err != ErrImperfect {
					t.Fatalf("crop reaching into the partial block: %v", err)
				}
			}
			if p.y > 0 {
				if _, err := runTransform(data, op, Options{Crop: image.Rect(0, p.y-1, w, h)}); err != ErrImperfect {
					t.Fatalf("crop reaching into the partial block: %v", err)
				}
			}
			clean := image.Rect(p.x, p.y, w, h)
			var got image.Rectangle
			out, err := runTransform(data, op, Options{Crop: clean, Applied: &got})
			if err != nil {
				t.Fatalf("a crop clear of the partial block must not need Trim: %v", err)
			}
			if got != clean {
				t.Fatalf("applied %v, want %v", got, clean)
			}
			if _, err := decodeStdErr(out); err != nil {
				t.Fatalf("output does not decode: %v", err)
			}
			if _, trimmed, err := CropForTrim(bytes.NewReader(data), int64(len(data)), op, image.Rect(0, 0, w, h)); err != nil || trimmed != (p.x > 0 || p.y > 0) {
				t.Fatalf("CropForTrim trimmed=%v err=%v", trimmed, err)
			}
			if _, err := runTransform(data, op, Options{Crop: image.Rect(0, 0, w, h), Trim: true}); err != nil {
				t.Fatalf("with Trim the whole picture is fine: %v", err)
			}
		})
	}
}

func TestCropRefusesRectanglesOutsideThePicture(t *testing.T) {
	s := samplingSpec(64, 48, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
	data := buildJPEG(t, s, randomWall(rand.New(rand.NewSource(1)), s))
	for _, op := range allOps {
		w, h := outputDims(t, data, op)
		for _, want := range []image.Rectangle{
			{}, image.Rect(5, 5, 5, 20), image.Rect(w, 0, w+10, h), image.Rect(0, h, w, h+3),
			image.Rect(-20, -20, 0, 0), image.Rect(-9, -9, -1, -1),
		} {
			if _, err := CropFor(bytes.NewReader(data), int64(len(data)), op, want); !errors.Is(err, ErrCrop) {
				t.Errorf("%v %v: CropFor err %v", op, want, err)
			}
			if want.Empty() && want == (image.Rectangle{}) {
				continue
			}
			if _, err := runTransform(data, op, Options{Crop: want}); !errors.Is(err, ErrCrop) && !want.Empty() {
				t.Errorf("%v %v: Transform err %v", op, want, err)
			}
		}
	}
	if _, err := CropFor(bytes.NewReader(data), int64(len(data)), Op(99), image.Rect(0, 0, 4, 4)); err == nil {
		t.Fatal("invalid operation")
	}
	if _, err := CropFor(bytes.NewReader([]byte("nope")), 4, Identity, image.Rect(0, 0, 4, 4)); err == nil {
		t.Fatal("garbage")
	}
}

func TestOversizedCropIsClippedAndEqualsTheWholePicture(t *testing.T) {
	s := samplingSpec(64, 48, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
	data := buildJPEG(t, s, randomWall(rand.New(rand.NewSource(2)), s))
	for _, op := range allOps {
		whole := mustTransform(t, data, op, Options{})
		big := mustTransform(t, data, op, Options{Crop: image.Rect(-100, -100, 1000, 1000)})
		a, ga := parseCoefficients(t, whole)
		b, gb := parseCoefficients(t, big)
		if a.fr.width != b.fr.width || a.fr.height != b.fr.height || !gridsEqual(ga, gb) {
			t.Fatalf("%v: a crop covering everything must equal the plain transform", op)
		}
	}
}

func TestCropKeepsEveryCoefficientOfTheSelectedBlocks(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for _, sc := range samplingCases {
		s := samplingSpec(128, 128, sc.factors...)
		wall := randomWall(rng, s)
		data := buildJPEG(t, s, wall)
		for _, op := range allOps {
			whole := mustTransform(t, data, op, Options{})
			_, gw := parseCoefficients(t, whole)
			var got image.Rectangle
			out := mustTransform(t, data, op, Options{Crop: image.Rect(30, 17, 77, 61), Applied: &got})
			hc, gc := parseCoefficients(t, out)
			srcHeader, _ := parseCoefficients(t, data)
			g := geometryOf(&srcHeader.fr, op)
			ox, oy := got.Min.X/g.cellX, got.Min.Y/g.cellY
			for ci := range gc {
				c := hc.fr.comps[ci]
				for by := 0; by < gc[ci].high; by++ {
					for bx := 0; bx < gc[ci].wide; bx++ {
						srcBX, srcBY := ox*c.h+bx, oy*c.v+by
						if srcBX >= gw[ci].wide || srcBY >= gw[ci].high {
							continue
						}
						if !equalBlocks(gc[ci].block(bx, by), gw[ci].block(srcBX, srcBY)) {
							t.Fatalf("%s %v component %d block %d,%d differs from the whole transform", sc.name, op, ci, bx, by)
						}
					}
				}
			}
		}
	}
}

func TestCropShrinksTheFile(t *testing.T) {
	s := samplingSpec(160, 128, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
	data := buildJPEG(t, s, randomWall(rand.New(rand.NewSource(4)), s))
	whole := mustTransform(t, data, Identity, Options{})
	part := mustTransform(t, data, Identity, Options{Crop: image.Rect(0, 0, 40, 32)})
	if len(part)*6 > len(whole) {
		t.Fatalf("cropped file %d bytes, whole %d", len(part), len(whole))
	}
}

type appendOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

func gpsTIFF(order appendOrder, width, height uint32) []byte {
	out := make([]byte, 0, 256)
	out = append(out, 'I', 'I')
	if order == binary.BigEndian {
		out[0], out[1] = 'M', 'M'
	}
	out = order.AppendUint16(out, 42)
	out = order.AppendUint32(out, 8)
	entry := func(tag, typ uint16, count, value uint32, short bool) {
		out = order.AppendUint16(out, tag)
		out = order.AppendUint16(out, typ)
		out = order.AppendUint32(out, count)
		if short {
			out = order.AppendUint16(out, uint16(value))
			out = append(out, 0, 0)
			return
		}
		out = order.AppendUint32(out, value)
	}
	out = order.AppendUint16(out, 4)
	entry(tagImageWidth, typeLong, 1, width, false)
	entry(tagImageLength, typeLong, 1, height, false)
	entry(tagOrientation, typeShort, 1, 6, true)
	entry(0x8825, typeLong, 1, 8+2+4*12+4, false)
	out = order.AppendUint32(out, 0)
	out = order.AppendUint16(out, 1)
	entry(0x0001, 2, 2, 'N', false)
	out[len(out)-4], out[len(out)-3] = 'N', 0
	return order.AppendUint32(out, 0)
}

func TestCropRewritesTheMetadataSegments(t *testing.T) {
	for _, order := range []appendOrder{binary.LittleEndian, binary.BigEndian} {
		thumb := tinyJPEGThumbnail(t)
		spec := exifSpec{order: order, orientation: 6, width: 96, height: 64, pixelX: 96, pixelY: 64, resX: 300, resY: 72, thumbnail: thumb}
		s := samplingSpec(96, 64, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
		xmp := `<x tiff:Orientation="6" tiff:ImageWidth="96" tiff:ImageLength="64" exif:PixelXDimension="96" exif:PixelYDimension="64" exif:GPSLatitude="1,2N"/>`
		s.segments = []segment{exifSegmentOf(buildTIFF(spec)), xmpSegment(xmp)}
		data := buildJPEG(t, s, randomWall(rand.New(rand.NewSource(6)), s))

		var got image.Rectangle
		out := mustTransform(t, data, Rotate90, Options{Crop: image.Rect(20, 10, 60, 90), Applied: &got})
		_, tree := readBackExif(t, out)
		if o := uintOf(t, tree.IFD0, tagOrientation); o != 1 {
			t.Errorf("orientation %d", o)
		}
		if w, h := uintOf(t, tree.IFD0, tagImageWidth), uintOf(t, tree.IFD0, tagImageLength); int(w) != got.Dx() || int(h) != got.Dy() {
			t.Errorf("IFD0 size %dx%d, picture %v", w, h, got)
		}
		if x, y := uintOf(t, tree.Exif, tagPixelXDim), uintOf(t, tree.Exif, tagPixelYDim); int(x) != got.Dx() || int(y) != got.Dy() {
			t.Errorf("pixel dimensions %dx%d, picture %v", x, y, got)
		}
		if x, y := rationalOf(t, tree.IFD0, tagXResolution), rationalOf(t, tree.IFD0, tagYResolution); x != 72 || y != 300 {
			t.Errorf("resolution %d x %d, want it swapped by the rotation", x, y)
		}
		tiff, _ := readBackExif(t, out)
		if _, ok := exif.JPEGThumbnail(tiff); ok || bytes.Contains(tiff, thumb) {
			t.Errorf("the thumbnail of the whole picture must not stay in a crop")
		}
		h, err := parseHeaderOnly(out)
		if err != nil {
			t.Fatal(err)
		}
		var xmpOut []byte
		for _, seg := range h.segments {
			if isXMP(seg.payload) {
				xmpOut = seg.payload[len(xmpPrefix):]
			}
		}
		for _, want := range []string{`tiff:Orientation="1"`, fmt.Sprintf(`tiff:ImageWidth="%d"`, got.Dx()), fmt.Sprintf(`exif:PixelYDimension="%d"`, got.Dy()), "GPSLatitude"} {
			if !bytes.Contains(xmpOut, []byte(want)) {
				t.Errorf("XMP lacks %s: %s", want, xmpOut)
			}
		}
	}
}

func TestStripGPSRemovesLocationFromEveryMetadataSegment(t *testing.T) {
	for _, withCrop := range []bool{false, true} {
		s := samplingSpec(96, 64, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
		s.segments = []segment{exifSegmentOf(gpsTIFF(binary.LittleEndian, 96, 64)), xmpSegment(`<x exif:GPSLatitude="1,2N" exif:ColorSpace="1"><exif:GPSLongitude>3</exif:GPSLongitude></x>`)}
		data := buildJPEG(t, s, randomWall(rand.New(rand.NewSource(7)), s))
		o := Options{StripGPS: true}
		if withCrop {
			o.Crop = image.Rect(16, 16, 80, 48)
		}
		kept := mustTransform(t, data, Identity, Options{Crop: o.Crop})
		if _, tree := readBackExif(t, kept); !withCrop && tree.GPS == nil {
			t.Fatal("the fixture has no GPS")
		}
		out := mustTransform(t, data, Identity, o)
		_, tree := readBackExif(t, out)
		if tree.GPS != nil {
			t.Fatalf("crop=%v: the GPS directory survived", withCrop)
		}
		if bytes.Contains(out, []byte("GPSLatitude")) || bytes.Contains(out, []byte("GPSLongitude")) {
			t.Fatalf("crop=%v: GPS properties survived in XMP", withCrop)
		}
		if !bytes.Contains(out, []byte(`exif:ColorSpace="1"`)) {
			t.Fatal("other XMP properties must stay")
		}
	}
}

func TestWholePictureStripGPSKeepsThePixels(t *testing.T) {
	s := samplingSpec(96, 64, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
	s.segments = []segment{exifSegmentOf(gpsTIFF(binary.BigEndian, 96, 64))}
	wall := randomWall(rand.New(rand.NewSource(10)), s)
	data := buildJPEG(t, s, wall)
	out := mustTransform(t, data, Identity, Options{StripGPS: true})
	_, got := parseCoefficients(t, out)
	if !gridsEqual(got, wall) {
		t.Fatal("stripping location must not touch the picture")
	}
}

func TestAppliedIsFilledWithoutACrop(t *testing.T) {
	s := samplingSpec(101, 75, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
	data := buildJPEG(t, s, randomWall(rand.New(rand.NewSource(11)), s))
	for _, tc := range []struct {
		op   Op
		want image.Rectangle
	}{
		{Identity, image.Rect(0, 0, 101, 75)},
		{FlipH, image.Rect(5, 0, 101, 75)},
		{Rotate180, image.Rect(5, 11, 101, 75)},
		{Rotate90, image.Rect(11, 0, 75, 101)},
	} {
		var got image.Rectangle
		mustTransform(t, data, tc.op, Options{Trim: true, Applied: &got})
		if got != tc.want {
			t.Errorf("%v: applied %v, want %v", tc.op, got, tc.want)
		}
	}
}

func TestCropOnProgressiveAndUnsupportedFilesFailsCleanly(t *testing.T) {
	for _, name := range corpusNames(t) {
		data := readCorpus(t, name)
		_, err := runTransform(data, Identity, Options{Crop: image.Rect(0, 0, 8, 8)})
		_, plainErr := runTransform(data, Identity, Options{})
		if (err == nil) != (plainErr == nil) {
			t.Errorf("%s: crop err %v but plain err %v", name, err, plainErr)
		}
	}
}

func TestCropOfImperfectPicturesEqualsTheTrimmedTransform(t *testing.T) {
	rng := rand.New(rand.NewSource(12))
	for _, sc := range samplingCases {
		s := samplingSpec(101, 75, sc.factors...)
		data := buildJPEG(t, s, mildWall(rng, s))
		if _, err := decodeStdErr(data); err != nil {
			continue
		}
		for _, op := range allOps {
			var whole image.Rectangle
			ref := mustTransform(t, data, op, Options{Trim: true, Applied: &whole})
			refImg := stdDecode(t, ref)
			w, h := outputDims(t, data, op)
			for _, want := range sampleCrops(w, h) {
				t.Run(fmt.Sprintf("%s/%v/%v", sc.name, op, want), func(t *testing.T) {
					checkCropAgainst(t, data, op, want, whole, refImg)
				})
			}
		}
	}
}
