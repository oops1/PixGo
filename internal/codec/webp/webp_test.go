package webp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math/rand"
	"runtime"
	"testing"
	"time"

	xwebp "golang.org/x/image/webp"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

var realFiles = []string{
	"blue-purple-pink.lossy.webp",
	"gopher-doc.1bpp.lossless.webp",
	"gopher-doc.with-alpha.lossless.webp",
	"yellow_rose.lossy-with-alpha.webp",
}

func TestSniffAndExtensions(t *testing.T) {
	d := Decoder{}
	if !d.Sniff(riff(vp8(1, 1))) {
		t.Fatal("WebP not recognised")
	}
	for _, head := range [][]byte{nil, []byte("RIFF"), []byte("RIFF\x00\x00\x00\x00WAVE"), []byte("RIFX\x00\x00\x00\x00WEBP"), []byte("GIF89a")} {
		if d.Sniff(head) {
			t.Fatalf("%q sniffed as WebP", head)
		}
	}
	if d.ID() != codec.WebP || d.Name() != "WebP" || len(d.Extensions()) != 1 || d.Extensions()[0] != ".webp" {
		t.Fatal("identity")
	}
}

func TestProbeAgreesWithStandardLibraryOnRealFiles(t *testing.T) {
	for _, name := range realFiles {
		data := readTestdata(t, name)
		config, err := xwebp.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		info, err := probe(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.Width != config.Width || info.Height != config.Height || info.Format != codec.WebP || info.FormatName != "WebP" || info.Frames != 1 || info.Animated || info.Orientation != codec.OrientNormal || info.Vector || info.Preview {
			t.Fatalf("%s: %+v vs %v", name, info, config)
		}
	}
}

func TestAlphaDetectionOnRealFiles(t *testing.T) {
	for name, want := range map[string]bool{
		"blue-purple-pink.lossy.webp":         false,
		"gopher-doc.1bpp.lossless.webp":       false,
		"gopher-doc.with-alpha.lossless.webp": true,
		"yellow_rose.lossy-with-alpha.webp":   true,
	} {
		info, err := probe(readTestdata(t, name))
		if err != nil || info.HasAlpha != want {
			t.Fatalf("%s: %+v %v", name, info, err)
		}
		wantDepth := 24
		if want {
			wantDepth = 32
		}
		if info.BitDepth != wantDepth {
			t.Fatalf("%s: depth %d", name, info.BitDepth)
		}
	}
}

func TestBareBitstreamHeaders(t *testing.T) {
	info, err := probe(riff(vp8(640, 480)))
	if err != nil || info.Width != 640 || info.Height != 480 || info.HasAlpha || info.BitDepth != 24 {
		t.Fatalf("lossy: %+v %v", info, err)
	}
	info, err = probe(riff(vp8l(1000, 20, true)))
	if err != nil || info.Width != 1000 || info.Height != 20 || !info.HasAlpha || info.BitDepth != 32 {
		t.Fatalf("lossless: %+v %v", info, err)
	}
	info, err = probe(riff(vp8l(10000, 9999, false)))
	if err != nil || info.Width != 10000 || info.Height != 9999 || info.HasAlpha {
		t.Fatalf("largest lossless: %+v %v", info, err)
	}
}

func TestExtendedHeaderStillImage(t *testing.T) {
	info, err := probe(riff(vp8x(flagAlpha, 300, 200), chunk("ALPH", []byte{0, 1, 2}), vp8(300, 200)))
	if err != nil || info.Width != 300 || info.Height != 200 || !info.HasAlpha || info.Animated || info.Frames != 1 {
		t.Fatalf("%+v %v", info, err)
	}
	info, err = probe(riff(vp8x(0, 300, 200), vp8(300, 200)))
	if err != nil || info.HasAlpha {
		t.Fatalf("%+v %v", info, err)
	}
	info, err = probe(riff(vp8x(flagAlpha, 300, 200), vp8l(300, 200, false)))
	if err != nil || !info.HasAlpha {
		t.Fatalf("alpha flag with VP8L: %+v %v", info, err)
	}
	if _, err := probe(riff(vp8x(0, 300, 200), vp8(301, 200))); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("canvas mismatch: %v", err)
	}
}

func TestMetadataChunksDoNotDisturbTheProbe(t *testing.T) {
	icc := chunk("ICCP", bytes.Repeat([]byte{7}, 501))
	xmp := chunk("XMP ", []byte("<x:xmpmeta/>"))
	unknown := chunk("ZZZZ", []byte{1, 2, 3})
	for name, data := range map[string][]byte{
		"icc first":        riff(vp8x(0x20, 90, 60), icc, vp8(90, 60), xmp),
		"xmp and unknown":  riff(vp8x(0x24, 90, 60), unknown, vp8(90, 60), xmp, unknown),
		"all before image": riff(vp8x(0x2c, 90, 60), icc, xmp, exifChunk(testimg.OrientationTIFF(false, 8), true), vp8(90, 60)),
	} {
		info, err := probe(data)
		if err != nil || info.Width != 90 || info.Height != 60 || info.Frames != 1 {
			t.Fatalf("%s: %+v %v", name, info, err)
		}
	}
}

func TestAnimatedHeaders(t *testing.T) {
	frame := frameChunk(0, 0, 500, 400, 40, 0)
	data := riff(vp8x(flagAnimation|flagAlpha, 500, 400), animChunk(0), frame, frame, frame)
	info, err := probe(data)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Animated || info.Frames != 3 || info.Width != 500 || info.Height != 400 || !info.HasAlpha || info.BitDepth != 32 {
		t.Fatalf("%+v", info)
	}
	single, err := probe(riff(vp8x(flagAnimation, 5, 5), animChunk(0), frameChunk(0, 0, 5, 5, 40, 0)))
	if err != nil || single.Animated || single.Frames != 1 {
		t.Fatalf("single frame animation: %+v %v", single, err)
	}
}

func TestAnimationLayoutLoopAndFirstDelay(t *testing.T) {
	for _, tc := range []struct {
		name  string
		data  []byte
		loop  int
		delay time.Duration
	}{
		{"three plays", riff(vp8x(flagAnimation, 8, 8), animChunk(3), frameChunk(0, 0, 8, 8, 250, 0), frameChunk(0, 0, 8, 8, 40, 0)), 3, 250 * time.Millisecond},
		{"forever", riff(vp8x(flagAnimation, 8, 8), animChunk(0), frameChunk(0, 0, 8, 8, 33, 0)), 0, 33 * time.Millisecond},
		{"largest loop count", riff(vp8x(flagAnimation, 8, 8), animChunk(65535), frameChunk(0, 0, 8, 8, 120, 0)), 65535, 120 * time.Millisecond},
		{"no ANIM chunk loops forever", riff(vp8x(flagAnimation, 8, 8), frameChunk(0, 0, 8, 8, 70, 0)), 0, 70 * time.Millisecond},
		{"zero delay", riff(vp8x(flagAnimation, 8, 8), animChunk(1), frameChunk(0, 0, 8, 8, 0, 0)), 1, 100 * time.Millisecond},
		{"ten milliseconds", riff(vp8x(flagAnimation, 8, 8), animChunk(1), frameChunk(0, 0, 8, 8, 10, 0)), 1, 100 * time.Millisecond},
		{"eleven milliseconds", riff(vp8x(flagAnimation, 8, 8), animChunk(1), frameChunk(0, 0, 8, 8, 11, 0)), 1, 11 * time.Millisecond},
		{"longest delay", riff(vp8x(flagAnimation, 8, 8), animChunk(1), frameChunk(0, 0, 8, 8, 1<<24-1, 0)), 1, (1<<24 - 1) * time.Millisecond},
	} {
		found, err := walkChunks(context.Background(), bytes.NewReader(tc.data), int64(len(tc.data)))
		if err != nil || found.loop != tc.loop || found.firstDelay != tc.delay {
			t.Fatalf("%s: %+v %v", tc.name, found, err)
		}
	}
}

func TestOrientationFromExifChunk(t *testing.T) {
	for _, bigEndian := range []bool{false, true} {
		for v := uint16(1); v <= 8; v++ {
			tiff := testimg.OrientationTIFF(bigEndian, v)
			for name, data := range map[string][]byte{
				"after image":     riff(vp8x(0x08, 64, 32), vp8(64, 32), exifChunk(tiff, false)),
				"before image":    riff(vp8x(0x08, 64, 32), exifChunk(tiff, false), vp8(64, 32)),
				"with Exif\\0\\0": riff(vp8x(0x08, 64, 32), vp8(64, 32), exifChunk(tiff, true)),
				"animation":       riff(vp8x(flagAnimation|0x08, 64, 32), animChunk(0), frameChunk(0, 0, 64, 32, 50, 0), frameChunk(0, 0, 64, 32, 50, 0), exifChunk(tiff, true)),
			} {
				info, err := probe(data)
				if err != nil || info.Orientation != codec.Orientation(v) {
					t.Fatalf("%s bigEndian=%v v=%d: %+v %v", name, bigEndian, v, info, err)
				}
			}
		}
	}
	for name, data := range map[string][]byte{
		"invalid value":   riff(vp8x(0x08, 64, 32), vp8(64, 32), exifChunk(testimg.OrientationTIFF(false, 99), false)),
		"junk exif":       riff(vp8x(0x08, 64, 32), vp8(64, 32), chunk("EXIF", []byte("junk"))),
		"tiny exif":       riff(vp8x(0x08, 64, 32), vp8(64, 32), chunk("EXIF", []byte("II"))),
		"exif in a plain": riff(vp8(64, 32), exifChunk(testimg.OrientationTIFF(false, 6), false)),
	} {
		info, err := probe(data)
		if err != nil || info.Orientation != codec.OrientNormal {
			t.Fatalf("%s: %+v %v", name, info, err)
		}
	}
	first := riff(vp8x(0x08, 64, 32), vp8(64, 32), exifChunk(testimg.OrientationTIFF(false, 6), false), exifChunk(testimg.OrientationTIFF(false, 3), false))
	if info, err := probe(first); err != nil || info.Orientation != codec.OrientRotate90 {
		t.Fatalf("first EXIF chunk wins: %+v %v", info, err)
	}
}

func TestOrientationSurvivesDecodingARealImage(t *testing.T) {
	source := smooth(48, 32)
	file := bitstreamChunks(t, vpxEncode(t, source, optionsLossless()))
	data := riff(append([][]byte{vp8x(0x08, 48, 32)}, append(file, exifChunk(testimg.OrientationTIFF(false, 6), true))...)...)
	decoded, err := decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Info.Orientation != codec.OrientRotate90 || !testimg.Identical(decoded.First(), source) {
		t.Fatalf("%+v", decoded.Info)
	}
}

func TestRIFFSizeVariants(t *testing.T) {
	data := riff(vp8(77, 33))
	withTrailing := append(append([]byte(nil), data...), bytes.Repeat([]byte{0xee}, 40)...)
	if info, err := probe(withTrailing); err != nil || info.Width != 77 {
		t.Fatalf("trailing data: %+v %v", info, err)
	}
	oversize := append([]byte(nil), data...)
	binary.LittleEndian.PutUint32(oversize[4:], 1<<30)
	if info, err := probe(oversize); err != nil || info.Width != 77 {
		t.Fatalf("oversized RIFF length: %+v %v", info, err)
	}
	tiny := append([]byte(nil), data...)
	binary.LittleEndian.PutUint32(tiny[4:], 2)
	if _, err := probe(tiny); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("RIFF length 2: %v", err)
	}
}

func TestMalformedContainers(t *testing.T) {
	good := riff(vp8(10, 10))
	anim := animChunk(0)
	cases := map[string][]byte{
		"empty":                    nil,
		"header only":              good[:12],
		"not riff":                 append([]byte("RIFX"), good[4:]...),
		"wave":                     append(append([]byte(nil), good[:8]...), append([]byte("WAVE"), good[12:]...)...),
		"no chunks":                riff(),
		"unknown chunks only":      riff(chunk("JUNK", []byte{1, 2, 3})),
		"unknown chunk first":      riff(chunk("JUNK", []byte{1, 2, 3}), vp8(10, 10)),
		"chunk beyond file":        append(append([]byte(nil), good[:12]...), 'V', 'P', '8', ' ', 0xff, 0xff, 0xff, 0x7f, 1, 2, 3),
		"bad VP8 start code":       riff(chunk("VP8 ", []byte{0x10, 0, 0, 1, 2, 3, 10, 0, 10, 0})),
		"VP8 inter frame":          riff(chunk("VP8 ", []byte{0x11, 0, 0, 0x9d, 1, 0x2a, 10, 0, 10, 0})),
		"VP8 too short":            riff(chunk("VP8 ", []byte{0x10, 0, 0})),
		"bad VP8L signature":       riff(chunk("VP8L", []byte{0x2e, 1, 2, 3, 4})),
		"VP8L too short":           riff(chunk("VP8L", []byte{0x2f, 1})),
		"VP8L future version":      riff(chunk("VP8L", []byte{0x2f, 0, 0, 0, 0x20})),
		"VP8X too short":           riff(chunk("VP8X", []byte{0, 0, 0})),
		"VP8X without image":       riff(vp8x(0, 10, 10)),
		"VP8X with metadata only":  riff(vp8x(0x08, 10, 10), exifChunk(testimg.OrientationTIFF(false, 6), true)),
		"zero VP8 dimensions":      riff(vp8(0, 10)),
		"animation without frames": riff(vp8x(flagAnimation, 10, 10), anim),
		"animation with still":     riff(vp8x(flagAnimation, 10, 10), anim, vp8(10, 10)),
		"ANIM too short":           riff(vp8x(flagAnimation, 10, 10), chunk("ANIM", []byte{0, 0, 0}), frameChunk(0, 0, 10, 10, 40, 0)),
		"ANMF too short":           riff(vp8x(flagAnimation, 10, 10), anim, chunk("ANMF", []byte{1, 2, 3, 4, 5})),
	}
	for name, data := range cases {
		_, probeErr := probe(data)
		_, decodeErr := decode(data)
		if probeErr == nil || decodeErr == nil {
			t.Fatalf("%s: probe=%v decode=%v", name, probeErr, decodeErr)
		}
		testimg.RequireCodecError(t, probeErr)
		testimg.RequireCodecError(t, decodeErr)
	}
}

func TestHeadersWithoutPixelDataFailToDecode(t *testing.T) {
	for name, data := range map[string][]byte{
		"lossy header":        riff(vp8(10, 10)),
		"lossless header":     riff(vp8l(7, 7, true)),
		"extended lossy":      riff(vp8x(0, 20, 20), vp8(20, 20)),
		"extended with alpha": riff(vp8x(flagAlpha, 20, 20), chunk("ALPH", []byte{1, 2, 3, 4}), vp8(20, 20)),
		"animation frame":     riff(vp8x(flagAnimation, 20, 20), animChunk(0), frameChunk(0, 0, 20, 20, 40, 0, vp8(20, 20))),
	} {
		if _, err := probe(data); err != nil {
			t.Fatalf("%s: probe %v", name, err)
		}
		for _, frames := range []codec.FrameSel{codec.FirstFrame, codec.AllFrames} {
			_, err := decodeWith(data, codec.Options{Limits: codec.DefaultLimits(), Frames: frames})
			if !errors.Is(err, codec.ErrCorrupt) {
				t.Fatalf("%s frames=%d: %v", name, frames, err)
			}
		}
	}
}

func TestHugeCanvasRejectedQuickly(t *testing.T) {
	data := riff(vp8x(0, 1<<24, 1<<24), vp8(10, 10))
	testimg.Within(t, 10*time.Second, func() {
		if _, err := probe(data); !errors.Is(err, codec.ErrTooLarge) && !errors.Is(err, codec.ErrCorrupt) {
			t.Errorf("probe: %v", err)
		}
		animated := riff(vp8x(flagAnimation, 1<<24, 1<<24), frameChunk(0, 0, 1, 1, 10, 0))
		if _, err := decode(animated); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("animated decode: %v", err)
		}
		lossless := riff(vp8l(16384, 16384, true))
		lim := codec.DefaultLimits()
		lim.MaxPixels = 1_000_000
		if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(lossless), int64(len(lossless)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("lossless decode: %v", err)
		}
	})
}

func TestDecompressionBombIsRejectedWithoutAllocation(t *testing.T) {
	for name, data := range map[string][]byte{
		"lossless 16383x16383": riff(vp8l(16383, 16383, true)),
		"lossy 16383x16383":    riff(vp8(16383, 16383)),
		"extended canvas":      riff(vp8x(flagAlpha, 16383, 16383), chunk("ALPH", []byte{0, 0}), vp8(16383, 16383)),
		"animated canvas":      riff(vp8x(flagAnimation|flagAlpha, 16383, 16383), animChunk(0), frameChunk(0, 0, 16383, 16383, 40, 0)),
	} {
		for _, frames := range []codec.FrameSel{codec.FirstFrame, codec.AllFrames} {
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			_, err := decodeWith(data, codec.Options{Limits: codec.DefaultLimits(), Frames: frames})
			runtime.ReadMemStats(&after)
			if !errors.Is(err, codec.ErrTooLarge) {
				t.Fatalf("%s: %v", name, err)
			}
			if grown := after.TotalAlloc - before.TotalAlloc; grown > 8<<20 {
				t.Fatalf("%s: allocated %d bytes before refusing", name, grown)
			}
		}
	}
}

func TestManyTinyChunksFinishQuickly(t *testing.T) {
	var many [][]byte
	many = append(many, vp8x(0, 5, 5))
	junk := chunk("JUNK", nil)
	for i := 0; i < 300_000; i++ {
		many = append(many, junk)
	}
	many = append(many, vp8(5, 5))
	data := riff(many...)
	testimg.Within(t, 10*time.Second, func() {
		if info, err := probe(data); err != nil || info.Width != 5 {
			t.Errorf("%+v %v", info, err)
		}
	})
}

func TestMoreChunksThanTheBudgetIsCorrupt(t *testing.T) {
	junk := chunk("JUNK", nil)
	many := [][]byte{vp8x(0, 5, 5)}
	for i := 0; i < maxChunks+8; i++ {
		many = append(many, junk)
	}
	many = append(many, vp8(5, 5))
	data := riff(many...)
	testimg.Within(t, 20*time.Second, func() {
		if _, err := probe(data); !errors.Is(err, codec.ErrCorrupt) {
			t.Errorf("probe: %v", err)
		}
	})
}

func TestEveryTruncatedPrefixOfRealFiles(t *testing.T) {
	data := readTestdata(t, "gopher-doc.1bpp.lossless.webp")
	for n := 0; n < len(data); n++ {
		prefix := data[:n]
		if _, err := probe(prefix); err != nil {
			testimg.RequireCodecError(t, err)
		}
		_, err := decode(prefix)
		if err == nil && n < len(data)-1 {
			t.Fatalf("prefix %d/%d decoded", n, len(data))
		}
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
	}
	for _, name := range []string{"blue-purple-pink.lossy.webp", "yellow_rose.lossy-with-alpha.webp"} {
		lossy := readTestdata(t, name)
		for n := 0; n < len(lossy); n += 7 {
			if _, err := decode(lossy[:n]); err != nil {
				testimg.RequireCodecError(t, err)
			} else if n < len(lossy)-1 {
				t.Fatalf("%s: prefix %d/%d decoded", name, n, len(lossy))
			}
		}
	}
}

func TestLimitsAndCancellation(t *testing.T) {
	data := readTestdata(t, "blue-purple-pink.lossy.webp")
	lim := codec.DefaultLimits()
	lim.MaxPixels = 10
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("pixels: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxFileBytes = 100
	if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("file: %v", err)
	}
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("file on decode: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Decoder{}).Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := (Decoder{}).Probe(ctx, bytes.NewReader(data), int64(len(data)), codec.Limits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel probe: %v", err)
	}
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{}); err != nil {
		t.Fatalf("zero limits: %v", err)
	}
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), -1, codec.Options{}); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("negative size: %v", err)
	}
}

func TestCancellationIsSeenWhileWalkingChunks(t *testing.T) {
	var many [][]byte
	many = append(many, vp8x(0, 5, 5))
	junk := chunk("JUNK", nil)
	for i := 0; i < 5*contextCheckInterval; i++ {
		many = append(many, junk)
	}
	many = append(many, vp8(5, 5))
	data := riff(many...)
	ctx, cancel := context.WithCancel(context.Background())
	reader := &countingReader{data: data, after: 3 * contextCheckInterval, action: cancel}
	_, err := Decoder{}.Probe(ctx, reader, int64(len(data)), codec.DefaultLimits())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("probe: %v", err)
	}
	if reads := reader.reads.Load(); reads > 4*contextCheckInterval+2 {
		t.Fatalf("walk went on for %d reads after cancellation", reads)
	}
}

func TestCorruptedBytesNeverPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	sources := [][]byte{
		readTestdata(t, "gopher-doc.1bpp.lossless.webp"),
		readTestdata(t, "blue-purple-pink.lossy.webp"),
		readTestdata(t, "yellow_rose.lossy-with-alpha.webp"),
		buildAnimation(t, 24, 16, 2, []frameSpec{
			{img: withAlpha(smooth(24, 16)), milliseconds: 40, blend: true},
			{x: 2, y: 4, img: smooth(10, 6), milliseconds: 60, blend: true, lossy: true},
		}),
	}
	for _, data := range sources {
		for i := 0; i < 150; i++ {
			mutated := append([]byte(nil), data...)
			for k := 0; k < 1+rng.Intn(4); k++ {
				mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
			}
			for _, frames := range []codec.FrameSel{codec.FirstFrame, codec.AllFrames} {
				if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(mutated), int64(len(mutated)), codec.Options{Limits: testimg.SmallLimits(), Frames: frames}); err != nil {
					testimg.RequireCodecError(t, err)
				}
			}
		}
	}
}

func TestConcurrentDecodesAgree(t *testing.T) {
	sources := [][]byte{
		readTestdata(t, "yellow_rose.lossy-with-alpha.webp"),
		readTestdata(t, "gopher-doc.with-alpha.lossless.webp"),
		buildAnimation(t, 24, 16, 2, []frameSpec{
			{img: withAlpha(smooth(24, 16)), milliseconds: 40, blend: true},
			{img: withAlpha(smooth(24, 16)), milliseconds: 40, blend: true, dispose: true},
		}),
	}
	for _, data := range sources {
		want, err := decodeAllFrames(data)
		if err != nil {
			t.Fatal(err)
		}
		results := make(chan *codec.Image, 8)
		for i := 0; i < cap(results); i++ {
			go func() {
				got, err := decodeAllFrames(data)
				if err != nil {
					got = nil
				}
				results <- got
			}()
		}
		for i := 0; i < cap(results); i++ {
			got := <-results
			if got == nil || len(got.Frames) != len(want.Frames) {
				t.Fatalf("concurrent decode failed: %+v", got)
			}
			for f := range got.Frames {
				if !testimg.Identical(got.Frames[f].Pix, want.Frames[f].Pix) {
					t.Fatalf("frame %d differs between concurrent decodes", f)
				}
			}
		}
	}
}

func TestReadFailuresAreReportedAsCorruption(t *testing.T) {
	still := riff(vp8x(0x28, 64, 32), vp8(64, 32), exifChunk(testimg.OrientationTIFF(false, 6), true))
	animated := riff(vp8x(flagAnimation, 8, 8), animChunk(2), frameChunk(0, 0, 8, 8, 40, 0), frameChunk(0, 0, 8, 8, 40, 0))
	for name, data := range map[string][]byte{
		"still":                 still,
		"animation":             animated,
		"lossless header":       riff(vp8l(30, 20, true)),
		"lossy header":          riff(vp8(30, 20)),
		"real lossy with alpha": readTestdata(t, "yellow_rose.lossy-with-alpha.webp"),
	} {
		failures := 0
		for failAt := 1; ; failAt++ {
			reader := &brokenReader{data: data, failAt: failAt}
			_, err := Decoder{}.Probe(context.Background(), reader, int64(len(data)), codec.DefaultLimits())
			if reader.reads < failAt {
				break
			}
			if err == nil {
				continue
			}
			failures++
			if !errors.Is(err, codec.ErrCorrupt) {
				t.Fatalf("%s: read %d failed with %v", name, failAt, err)
			}
		}
		if failures < 3 {
			t.Fatalf("%s: only %d read failures were reported", name, failures)
		}
	}
}

func TestDecodeSkipsMetadataChunks(t *testing.T) {
	source := smooth(30, 20)
	plain := bitstreamChunks(t, vpxEncode(t, source, vpxLossy(70)))
	reference, err := decode(riff(plain...))
	if err != nil {
		t.Fatal(err)
	}
	wrapped := riff(append(append([][]byte{
		vp8x(0x2c, 30, 20),
		chunk("ICCP", bytes.Repeat([]byte{3}, 777)),
	}, plain...), exifChunk(testimg.OrientationTIFF(false, 8), true), chunk("XMP ", []byte("<x:xmpmeta xmlns:x='adobe:ns:meta/'/>")), chunk("ZZZZ", []byte{1}))...)
	img, err := decode(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if !testimg.Identical(img.First(), reference.First()) || img.Info.Orientation != codec.OrientRotate270 {
		t.Fatalf("%+v", img.Info)
	}
}
