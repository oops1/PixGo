package jxl

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	gjxl "github.com/gen2brain/jxl"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
	"github.com/oops1/PixGo/internal/imaging"
)

func readTestdata(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func probe(data []byte, lim codec.Limits) (codec.Info, error) {
	return Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
}

func decode(data []byte, o codec.Options) (*codec.Image, error) {
	return Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), o)
}

func decodeAll(data []byte) (*codec.Image, error) {
	return decode(data, codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames})
}

func encode(t testing.TB, img image.Image, opts gjxl.EncodeOptions) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gjxl.Encode(&buf, img, opts); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func libFrames(t testing.TB, data []byte) []*image.NRGBA {
	t.Helper()
	result, err := gjxl.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var out []*image.NRGBA
	for _, img := range result.Image {
		out = append(out, convert.NRGBA(img))
	}
	return out
}

func buildStill(t testing.TB, width, height int, orientation uint32) []byte {
	t.Helper()
	spec := baseHeader(uint32(width), uint32(height))
	spec.orientation = orientation
	_, entries, payload := stillFrame(t, encodeLossless(t, width, height, 21))
	frame := frameSpec{modular: true, groupSizeShift: 1, last: true, entries: entries, data: payload[:sumOf(entries)]}
	return streamOf(spec, frame.write(spec, uint32(width), uint32(height)))
}

func TestSniffRecognisesBothSignatures(t *testing.T) {
	d := Decoder{}
	tests := []struct {
		name string
		head []byte
		want bool
	}{
		{"bare codestream", []byte{0xff, 0x0a, 0x00, 0x01}, true},
		{"container", containerSignature, true},
		{"container with more", contained(encodeLossless(t, 8, 8, 1))[:200], true},
		{"jpeg", []byte{0xff, 0xd8, 0xff, 0xe0}, false},
		{"one byte", []byte{0xff}, false},
		{"almost the container signature", []byte("\x00\x00\x00\x0cJXL \x0d\x0a\x87\x0b"), false},
		{"isobmff", []byte("\x00\x00\x00\x18ftypheic"), false},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := d.Sniff(tt.head); got != tt.want {
				t.Fatalf("Sniff = %v", got)
			}
		})
	}
	if d.ID() != codec.JXL || d.Name() != "JPEG XL" || len(d.Extensions()) != 1 || d.Extensions()[0] != ".jxl" {
		t.Fatal("identity")
	}
}

func TestEncodedPicturesRoundTripAtEverySize(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {2, 3}, {17, 9}, {33, 21}, {64, 64}, {161, 121}, {300, 7}} {
		for _, alpha := range []bool{false, true} {
			src := testimg.Noise(size[0], size[1], uint64(size[0]), alpha)
			data := encode(t, src, gjxl.EncodeOptions{Lossless: true, Effort: 1})
			info, err := probe(data, codec.DefaultLimits())
			if err != nil {
				t.Fatalf("%v alpha=%v: %v", size, alpha, err)
			}
			hasAlpha := alpha && !allOpaque(src)
			want := codec.Info{Format: codec.JXL, FormatName: "JPEG XL", Width: size[0], Height: size[1], Frames: 1, HasAlpha: hasAlpha, Orientation: codec.OrientNormal, BitDepth: 24}
			if hasAlpha {
				want.BitDepth = 32
			}
			if info != want {
				t.Fatalf("%v alpha=%v: %+v, want %+v", size, alpha, info, want)
			}
			img, err := decodeAll(data)
			if err != nil {
				t.Fatalf("%v alpha=%v: %v", size, alpha, err)
			}
			if img.Info != info || len(img.Frames) != 1 || img.Scale != 1 || img.Frames[0].Delay != 0 {
				t.Fatalf("%v: %+v", size, img.Info)
			}
			if !testimg.Identical(img.First(), src) {
				t.Fatalf("%v alpha=%v: lossless picture changed, delta %d", size, alpha, testimg.MaxDelta(img.First(), src))
			}
		}
	}
}

func allOpaque(img *image.NRGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0xff {
			return false
		}
	}
	return true
}

func TestLossyPicturesStayClose(t *testing.T) {
	src := testimg.Photo(160, 120)
	data := encode(t, src, gjxl.EncodeOptions{Quality: 90})
	img, err := decodeAll(data)
	if err != nil {
		t.Fatal(err)
	}
	if delta := testimg.MaxDelta(img.First(), src); delta > 40 {
		t.Fatalf("worst channel error %d", delta)
	}
	reference := libFrames(t, data)
	if !testimg.Identical(img.First(), reference[0]) {
		t.Fatal("differs from the library")
	}
}

func TestSixteenBitPicturesAreReducedToEightBits(t *testing.T) {
	wide := image.NewNRGBA64(image.Rect(0, 0, 24, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			wide.SetNRGBA64(x, y, color.NRGBA64{R: uint16(x * 2600), G: uint16(y * 4000), B: uint16(x*y*100 + 77), A: 65535})
		}
	}
	data := encode(t, wide, gjxl.EncodeOptions{Lossless: true})
	info, err := probe(data, codec.DefaultLimits())
	if err != nil || info.BitDepth != 48 {
		t.Fatalf("%+v %v", info, err)
	}
	img, err := decodeAll(data)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			c := wide.NRGBA64At(x, y)
			if got := img.First().NRGBAAt(x, y); got.R != uint8(c.R>>8) || got.G != uint8(c.G>>8) || got.B != uint8(c.B>>8) || got.A != 255 {
				t.Fatalf("(%d,%d) = %v from %v", x, y, got, c)
			}
		}
	}
}

func TestStreamsOfTheLibraryDecodeLikeTheLibrary(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		height   int
		bitDepth int
	}{
		{"dct_small.jxl", 129, 77, 24},
		{"dct_gray.jxl", 333, 211, 8},
		{"tree_plain.jxl", 2000, 1200, 24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := readTestdata(t, tt.name)
			info, err := probe(data, codec.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			if info.Width != tt.width || info.Height != tt.height || info.BitDepth != tt.bitDepth || info.Frames != 1 || info.HasAlpha {
				t.Fatalf("%+v", info)
			}
			img, err := decodeAll(data)
			if err != nil {
				t.Fatal(err)
			}
			if !testimg.Identical(img.First(), libFrames(t, data)[0]) {
				t.Fatal("differs from the library")
			}
		})
	}
	gray, err := decodeAll(readTestdata(t, "dct_gray.jxl"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(gray.First().Pix); i += 4 {
		p := gray.First().Pix[i : i+4]
		if p[0] != p[1] || p[1] != p[2] || p[3] != 255 {
			t.Fatalf("grey picture has colour %v", p)
		}
	}
}

func TestContainersDecodeLikeBareCodestreams(t *testing.T) {
	stream := readTestdata(t, "dct_small.jxl")
	want, err := decodeAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"jxlc":          contained(stream),
		"jxlp":          boxed(append([][]byte{containerSignature, ftypBox()}, partial(stream, 3)...)...),
		"with metadata": contained(stream, box("Exif", []byte("\x00\x00\x00\x00II*\x00")), box("xml ", []byte("<x/>"))),
	} {
		info, err := probe(data, codec.DefaultLimits())
		if err != nil || info != want.Info {
			t.Fatalf("%s: %+v %v", name, info, err)
		}
		got, err := decodeAll(data)
		if err != nil || !testimg.Identical(got.First(), want.First()) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	level10 := boxed(containerSignature, ftypBox(), box("jxll", []byte{10}), box("jxlc", stream))
	for _, run := range []func() error{
		func() error { _, err := probe(level10, codec.DefaultLimits()); return err },
		func() error { _, err := decode(level10, codec.Options{}); return err },
	} {
		if err := run(); !errors.Is(err, codec.ErrUnsupported) {
			t.Fatalf("level 10: %v", err)
		}
	}
}

func TestOrientationIsAppliedByTheLibraryAndReportedAsNormal(t *testing.T) {
	base, err := decode(buildStill(t, 24, 10, 1), codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	reference, err := decode(encodeLossless(t, 24, 10, 21), codec.Options{})
	if err != nil || !testimg.Identical(base.First(), reference.First()) {
		t.Fatalf("test stream differs from the encoder's: %v", err)
	}
	for orientation := uint32(1); orientation <= 8; orientation++ {
		data := buildStill(t, 24, 10, orientation)
		info, err := probe(data, codec.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		wantWidth, wantHeight := 24, 10
		if orientation >= 5 {
			wantWidth, wantHeight = 10, 24
		}
		if info.Width != wantWidth || info.Height != wantHeight || info.Orientation != codec.OrientNormal {
			t.Fatalf("orientation %d: %+v", orientation, info)
		}
		img, err := decode(data, codec.Options{})
		if err != nil {
			t.Fatalf("orientation %d: %v", orientation, err)
		}
		if img.Info != info || img.First().Bounds().Dx() != wantWidth || img.First().Bounds().Dy() != wantHeight {
			t.Fatalf("orientation %d: %v %+v", orientation, img.First().Bounds(), img.Info)
		}
		want := imaging.Orient(base.First(), codec.Orientation(orientation))
		if !testimg.Identical(img.First(), want) {
			t.Fatalf("orientation %d: pixels differ from the expected rotation", orientation)
		}
	}
}

func TestAnimationCarriesDelaysLoopAndHiddenFrames(t *testing.T) {
	spec := animationSpec{width: 20, height: 12, loops: 3, clips: []clipSpec{{duration: 3}, {hidden: true}, {duration: 5}, {duration: 1}}}
	data := buildAnimation(t, spec)
	info, err := probe(data, codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if info.Frames != 3 || !info.Animated || info.Width != 20 || info.Height != 12 {
		t.Fatalf("%+v", info)
	}
	img, err := decodeAll(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Frames) != 3 || img.Loop != 3 || img.Info != info {
		t.Fatalf("%d frames, loop %d, %+v", len(img.Frames), img.Loop, img.Info)
	}
	for i, want := range []time.Duration{30 * time.Millisecond, 50 * time.Millisecond, 10 * time.Millisecond} {
		if img.Frames[i].Delay != want {
			t.Fatalf("frame %d lasts %v, want %v", i, img.Frames[i].Delay, want)
		}
	}
	reference := libFrames(t, data)
	if len(reference) != 3 {
		t.Fatalf("library shows %d frames", len(reference))
	}
	for i := range reference {
		if !testimg.Identical(img.Frames[i].Pix, reference[i]) {
			t.Fatalf("frame %d differs from the library", i)
		}
	}
	if testimg.Identical(img.Frames[0].Pix, img.Frames[1].Pix) {
		t.Fatal("frames are identical, the test proves nothing")
	}
}

func TestAnimationTimingFollowsTheTickRate(t *testing.T) {
	data := buildAnimation(t, animationSpec{width: 8, height: 8, ticks: [2]uint32{24000, 1001}, clips: []clipSpec{{duration: 1}, {duration: 24}}})
	img, err := decodeAll(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{41708333 * time.Nanosecond, 1001 * time.Millisecond}
	for i, w := range want {
		if d := img.Frames[i].Delay - w; d < -time.Microsecond || d > time.Microsecond {
			t.Fatalf("frame %d lasts %v, want %v", i, img.Frames[i].Delay, w)
		}
	}
	if img.Loop != 0 {
		t.Fatalf("loop %d", img.Loop)
	}
}

func TestFirstFrameOfAnAnimationIsEnough(t *testing.T) {
	data := buildAnimation(t, animationSpec{width: 20, height: 12, clips: []clipSpec{{duration: 3}, {duration: 5}, {duration: 1}}})
	first, err := decode(data, codec.Options{Limits: codec.DefaultLimits(), Frames: codec.FirstFrame})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Frames) != 1 || first.Info.Frames != 3 || !first.Info.Animated {
		t.Fatalf("%d frames, %+v", len(first.Frames), first.Info)
	}
	all, err := decodeAll(data)
	if err != nil || !testimg.Identical(first.First(), all.First()) {
		t.Fatalf("%v", err)
	}
}

func TestAnimationsWithAPreviewAndAnOrientation(t *testing.T) {
	data := buildAnimation(t, animationSpec{width: 20, height: 12, orientation: 6, preview: true, clips: []clipSpec{{duration: 2}, {duration: 2}}})
	info, err := probe(data, codec.DefaultLimits())
	if err != nil || info.Width != 12 || info.Height != 20 || info.Frames != 2 || info.Orientation != codec.OrientNormal {
		t.Fatalf("%+v %v", info, err)
	}
	img, err := decodeAll(data)
	if err != nil {
		t.Fatal(err)
	}
	reference := libFrames(t, data)
	if len(img.Frames) != 2 || len(reference) != 2 {
		t.Fatalf("%d frames, library %d", len(img.Frames), len(reference))
	}
	for i := range reference {
		if !testimg.Identical(img.Frames[i].Pix, reference[i]) || img.Frames[i].Pix.Bounds().Dx() != 12 {
			t.Fatalf("frame %d", i)
		}
	}
}

func TestFrameLimitsApplyToAnimations(t *testing.T) {
	data := buildAnimation(t, animationSpec{width: 20, height: 12, clips: []clipSpec{{duration: 1}, {duration: 1}, {duration: 1}, {duration: 1}}})
	lim := codec.DefaultLimits()
	lim.MaxFrames = 3
	for _, frames := range []codec.FrameSel{codec.AllFrames, codec.FirstFrame} {
		if _, err := probe(data, lim); !errors.Is(err, codec.ErrTooLarge) {
			t.Fatalf("probe: %v", err)
		}
		if _, err := decode(data, codec.Options{Limits: lim, Frames: frames}); !errors.Is(err, codec.ErrTooLarge) {
			t.Fatalf("%d: %v", frames, err)
		}
	}
	lim = codec.DefaultLimits()
	lim.MaxFramePixels = 20 * 12 * 3
	if _, err := probe(data, lim); err != nil {
		t.Fatalf("probe does not look at the frame budget: %v", err)
	}
	if _, err := decode(data, codec.Options{Limits: lim, Frames: codec.AllFrames}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("frame pixels: %v", err)
	}
}

func TestLimitsAreCheckedBeforeDecoding(t *testing.T) {
	data := readTestdata(t, "dct_small.jxl")
	tests := []struct {
		name string
		lim  codec.Limits
	}{
		{"pixels", codec.Limits{MaxPixels: 129*77 - 1}},
		{"file size", codec.Limits{MaxFileBytes: int64(len(data)) - 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := probe(data, tt.lim); !errors.Is(err, codec.ErrTooLarge) {
				t.Fatalf("probe: %v", err)
			}
			if _, err := decode(data, codec.Options{Limits: tt.lim}); !errors.Is(err, codec.ErrTooLarge) {
				t.Fatalf("decode: %v", err)
			}
		})
	}
	if _, err := probe(data, codec.Limits{MaxPixels: 129 * 77}); err != nil {
		t.Fatalf("exact fit: %v", err)
	}
}

func allocated(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func TestDecompressionBombsAreRefusedWithoutAllocatingTheirSize(t *testing.T) {
	tiny := func(width, height uint32) []byte {
		spec := baseHeader(width, height)
		frame := frameSpec{allDefault: true, entries: []uint32{1}}
		return streamOf(spec, frame.write(spec, width, height))
	}
	frames := animatedSpec(64, 64)
	oneFrame := frameSpec{modular: true, groupSizeShift: 1, duration: 1}.write(frames, 64, 64)
	last := frameSpec{modular: true, groupSizeShift: 1, last: true}.write(frames, 64, 64)
	refusedByProbe := map[string][]byte{
		"billions of pixels":  tiny(1<<30, 1<<30),
		"just over the limit": tiny(10001, 10000),
		"too many frames":     streamOf(frames, bytes.Repeat(oneFrame, 10001), last),
	}
	for name, data := range refusedByProbe {
		for _, run := range []func() error{
			func() error { _, err := probe(data, codec.DefaultLimits()); return err },
			func() error { _, err := decode(data, codec.Options{}); return err },
		} {
			var err error
			spent := allocated(func() { err = run() })
			if !errors.Is(err, codec.ErrTooLarge) {
				t.Fatalf("%s: %v", name, err)
			}
			if spent > 8<<20 {
				t.Fatalf("%s: refusing allocated %d bytes", name, spent)
			}
		}
	}
	heavy := tiny(9000, 9000)
	if _, err := probe(heavy, codec.DefaultLimits()); err != nil {
		t.Fatalf("a legal size must be probed: %v", err)
	}
	var err error
	if spent := allocated(func() { _, err = decode(heavy, codec.Options{}) }); !errors.Is(err, codec.ErrTooLarge) || spent > 8<<20 {
		t.Fatalf("working set: %v after %d bytes", err, spent)
	}
	if _, err := decode(readTestdata(t, "tree_plain.jxl"), codec.Options{Limits: codec.Limits{MaxPixels: 1 << 20}}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("48 bytes asking for 2.4 megapixels: %v", err)
	}
}

func TestFramesTheLibraryRefusesAreTooLarge(t *testing.T) {
	spec := baseHeader(20, 12)
	frame := frameSpec{modular: true, groupSizeShift: 1, last: true, cropped: true, originX: 0, originY: 0, frameWidth: 3000, frameHeight: 3000, entries: []uint32{4}}.write(spec, 20, 12)
	data := streamOf(spec, frame)
	lim := codec.DefaultLimits()
	lim.MaxPixels = 1 << 20
	if _, err := probe(data, lim); err != nil {
		t.Fatal(err)
	}
	if _, err := decode(data, codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("%v", err)
	}
}

func TestAnimationsThatCannotBeMeasuredAreUnsupported(t *testing.T) {
	icc := animatedSpec(20, 12)
	icc.colorAllDefault, icc.embeddedProfile = false, true
	frame := frameSpec{modular: true, groupSizeShift: 1, last: true}
	permuted := frame
	permuted.permuted = true
	plain := animatedSpec(20, 12)
	for name, data := range map[string][]byte{
		"embedded profile": streamOf(icc, frame.write(icc, 20, 12)),
		"permuted table":   streamOf(plain, permuted.write(plain, 20, 12)),
	} {
		for _, run := range []func() error{
			func() error { _, err := probe(data, codec.DefaultLimits()); return err },
			func() error { _, err := decode(data, codec.Options{}); return err },
		} {
			if err := run(); !errors.Is(err, codec.ErrUnsupported) {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
}

func TestStillPicturesWithAnEmbeddedProfileAreProbed(t *testing.T) {
	spec := baseHeader(20, 12)
	spec.colorAllDefault, spec.embeddedProfile = false, true
	data := streamOf(spec, frameSpec{modular: true, groupSizeShift: 1, last: true}.write(spec, 20, 12))
	info, err := probe(data, codec.DefaultLimits())
	if err != nil || info.Width != 20 || info.Frames != 1 {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestEveryTruncationFailsCleanly(t *testing.T) {
	files := map[string][]byte{
		"lossy":     readTestdata(t, "dct_small.jxl"),
		"lossless":  encodeLossless(t, 24, 20, 31),
		"animation": buildAnimation(t, animationSpec{width: 16, height: 16, clips: []clipSpec{{duration: 1}, {duration: 2}, {duration: 1}}}),
		"boxed":     boxed(append([][]byte{containerSignature, ftypBox()}, partial(encodeLossless(t, 16, 16, 32), 3)...)...),
	}
	for name, data := range files {
		step := max(1, len(data)/1200)
		for cut := 0; cut < len(data); cut += step {
			part := data[:cut]
			_, probeErr := probe(part, codec.DefaultLimits())
			_, decodeErr := decodeAll(part)
			for _, err := range []error{probeErr, decodeErr} {
				if err != nil && !errors.Is(err, codec.ErrCorrupt) && !errors.Is(err, codec.ErrUnsupported) && !errors.Is(err, codec.ErrTooLarge) {
					t.Fatalf("%s cut %d: unclassified error %v", name, cut, err)
				}
			}
			if decodeErr == nil {
				t.Fatalf("%s cut at %d of %d decoded", name, cut, len(data))
			}
		}
	}
}

func TestDamagedFilesNeverPanic(t *testing.T) {
	random := rand.New(rand.NewSource(9))
	files := [][]byte{
		readTestdata(t, "dct_small.jxl"),
		readTestdata(t, "dct_gray.jxl"),
		encodeLossless(t, 20, 14, 33),
		buildAnimation(t, animationSpec{width: 16, height: 16, clips: []clipSpec{{duration: 1}, {duration: 2}}}),
		contained(encodeLossless(t, 16, 16, 34)),
	}
	for _, data := range files {
		positions := make([]int, 0, 500)
		for i := 0; i < len(data) && i < 150; i++ {
			positions = append(positions, i)
		}
		for i := 0; i < 250; i++ {
			positions = append(positions, random.Intn(len(data)))
		}
		for _, position := range positions {
			for _, bit := range []uint{0, 3, 7} {
				damaged := append([]byte(nil), data...)
				damaged[position] ^= 1 << bit
				if _, err := probe(damaged, testimg.SmallLimits()); err != nil {
					testimg.RequireCodecError(t, err)
				}
				if _, err := decode(damaged, codec.Options{Limits: testimg.SmallLimits(), Frames: codec.AllFrames}); err != nil {
					testimg.RequireCodecError(t, err)
				}
			}
		}
	}
}

func TestGarbageAfterTheSignatureIsCorrupt(t *testing.T) {
	random := rand.New(rand.NewSource(4))
	for _, signature := range [][]byte{codestreamSignature, containerSignature} {
		for _, length := range []int{0, 1, 7, 8, 40, 500, 5000} {
			junk := make([]byte, length)
			random.Read(junk)
			data := append(append([]byte{}, signature...), junk...)
			if _, err := probe(data, codec.DefaultLimits()); err != nil {
				testimg.RequireCodecError(t, err)
			}
			if _, err := decode(data, codec.Options{}); err == nil {
				t.Fatalf("%d bytes of junk after %q decoded", length, signature)
			} else {
				testimg.RequireCodecError(t, err)
			}
		}
	}
	if _, err := probe([]byte("PK\x03\x04 plainly not an image"), codec.DefaultLimits()); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("%v", err)
	}
}

type panickingReader struct {
	data  []byte
	calls int
	after int
}

func (r *panickingReader) ReadAt(p []byte, off int64) (int, error) {
	r.calls++
	if r.calls > r.after {
		panic("reader exploded")
	}
	return bytes.NewReader(r.data).ReadAt(p, off)
}

func TestPanicsInTheReaderBecomeCorruptErrors(t *testing.T) {
	data := readTestdata(t, "dct_small.jxl")
	for after := 0; after < 8; after++ {
		_, err := (Decoder{}).Decode(context.Background(), &panickingReader{data: data, after: after}, int64(len(data)), codec.Options{})
		if err != nil && !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("panic after %d reads: %v", after, err)
		}
		if after < 3 && err == nil {
			t.Fatalf("a reader that panics after %d reads decoded", after)
		}
		if _, err := (Decoder{}).Probe(context.Background(), &panickingReader{data: data, after: after}, int64(len(data)), codec.DefaultLimits()); err != nil && !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("probe, panic after %d reads: %v", after, err)
		}
	}
}

func TestCanceledContextStopsEveryStage(t *testing.T) {
	data := readTestdata(t, "dct_small.jxl")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Decoder{}).Probe(ctx, bytes.NewReader(data), int64(len(data)), codec.DefaultLimits()); !errors.Is(err, context.Canceled) {
		t.Fatalf("probe: %v", err)
	}
	if _, err := (Decoder{}).Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("decode: %v", err)
	}
}

type cancelingReader struct {
	data   []byte
	cancel context.CancelFunc
	reads  int
	after  int
}

func (r *cancelingReader) ReadAt(p []byte, off int64) (int, error) {
	r.reads++
	if r.reads == r.after {
		r.cancel()
	}
	return bytes.NewReader(r.data).ReadAt(p, off)
}

func TestCancellationDuringReadingIsReportedAsCancellation(t *testing.T) {
	for _, data := range [][]byte{readTestdata(t, "dct_small.jxl"), buildAnimation(t, animationSpec{width: 16, height: 16, clips: []clipSpec{{duration: 1}, {duration: 2}}})} {
		for after := 1; after < 10; after++ {
			ctx, cancel := context.WithCancel(context.Background())
			reader := &cancelingReader{data: data, cancel: cancel, after: after}
			_, err := (Decoder{}).Decode(ctx, reader, int64(len(data)), codec.Options{Frames: codec.AllFrames})
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel at read %d: %v", after, err)
			}
		}
	}
}

func TestDecodersAreSafeToShareBetweenGoroutines(t *testing.T) {
	data := buildAnimation(t, animationSpec{width: 20, height: 12, clips: []clipSpec{{duration: 3}, {duration: 5}}})
	want, err := decodeAll(data)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := decodeAll(data)
			if err != nil || len(got.Frames) != len(want.Frames) || !testimg.Identical(got.Frames[1].Pix, want.Frames[1].Pix) {
				failures <- "mismatch"
			}
		}()
	}
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
}

func TestParallelDecodingGivesTheSamePictures(t *testing.T) {
	data := buildAnimation(t, animationSpec{width: 40, height: 30, clips: []clipSpec{{duration: 3}, {duration: 5}}})
	serial, err := decodeAll(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, threads := range []int{-3, 0, 1, 2, 8} {
		got, err := (Decoder{Threads: threads}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Frames: codec.AllFrames})
		if err != nil || got.Info != serial.Info || len(got.Frames) != len(serial.Frames) {
			t.Fatalf("%d threads: %v", threads, err)
		}
		for i := range got.Frames {
			if !testimg.Identical(got.Frames[i].Pix, serial.Frames[i].Pix) {
				t.Fatalf("%d threads: frame %d differs", threads, i)
			}
		}
	}
}
