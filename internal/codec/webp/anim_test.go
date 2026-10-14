package webp

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"testing"
	"time"

	vpx "github.com/gen2brain/vpx/webp"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func numbered(width, height, index int) *image.NRGBA {
	img := smooth(width, height)
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = uint8(max(0, min(255, int(img.Pix[i])+index*14)))
		img.Pix[i+2] = uint8(max(0, min(255, int(img.Pix[i+2])-index*11)))
	}
	return img
}

func encodeAnimation(t testing.TB, frames []*image.NRGBA, delays []int, loop int) []byte {
	t.Helper()
	images := make([]image.Image, len(frames))
	for i, f := range frames {
		images[i] = f
	}
	var buf bytes.Buffer
	if err := vpx.EncodeAll(&buf, &vpx.WEBP{Image: images, Delay: delays, LoopCount: loop}, optionsLossless()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAnimationFramesDelaysAndLoopCount(t *testing.T) {
	var sources []*image.NRGBA
	for i := 0; i < 5; i++ {
		sources = append(sources, numbered(20, 14, i))
	}
	delays := []int{40, 100, 250, 7, 0}
	wantDelays := []time.Duration{40 * time.Millisecond, 100 * time.Millisecond, 250 * time.Millisecond, 100 * time.Millisecond, 100 * time.Millisecond}
	data := encodeAnimation(t, sources, delays, 3)
	info, err := probe(data)
	if err != nil || !info.Animated || info.Frames != 5 || info.Width != 20 || info.Height != 14 {
		t.Fatalf("%+v %v", info, err)
	}
	img, err := decodeAllFrames(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Frames) != 5 || img.Loop != 3 || img.Scale != 1 || img.Info.Frames != 5 || !img.Info.Animated {
		t.Fatalf("%d frames, loop %d, info %+v", len(img.Frames), img.Loop, img.Info)
	}
	for i, f := range img.Frames {
		if f.Delay != wantDelays[i] {
			t.Fatalf("frame %d delay %v want %v", i, f.Delay, wantDelays[i])
		}
		if !testimg.Identical(f.Pix, sources[i]) {
			t.Fatalf("frame %d differs by %d", i, testimg.MaxDelta(f.Pix, sources[i]))
		}
	}
}

func TestAnimationLoopsForeverWhenZeroOrAbsent(t *testing.T) {
	frame := frameChunk(0, 0, 20, 14, 50, frameNoBlend, losslessChunks(t, numbered(20, 14, 0))...)
	second := frameChunk(0, 0, 20, 14, 50, frameNoBlend, losslessChunks(t, numbered(20, 14, 1))...)
	for name, data := range map[string][]byte{
		"zero":   riff(vp8x(flagAnimation, 20, 14), animChunk(0), frame, second),
		"absent": riff(vp8x(flagAnimation, 20, 14), frame, second),
	} {
		img, err := decodeAllFrames(data)
		if err != nil || img.Loop != 0 || len(img.Frames) != 2 {
			t.Fatalf("%s: %+v %v", name, img, err)
		}
	}
}

func TestFirstFrameOfAnAnimation(t *testing.T) {
	var sources []*image.NRGBA
	for i := 0; i < 4; i++ {
		sources = append(sources, numbered(18, 12, i))
	}
	data := encodeAnimation(t, sources, []int{330, 40, 40, 40}, 2)
	first, err := decodeWith(data, codec.Options{Limits: codec.DefaultLimits(), Frames: codec.FirstFrame})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Frames) != 1 || first.Frames[0].Delay != 330*time.Millisecond || first.Loop != 2 {
		t.Fatalf("%+v", first)
	}
	if !first.Info.Animated || first.Info.Frames != 4 || first.Info.Width != 18 {
		t.Fatalf("first frame must still describe the whole animation: %+v", first.Info)
	}
	if !testimg.Identical(first.First(), sources[0]) {
		t.Fatalf("first frame differs by %d", testimg.MaxDelta(first.First(), sources[0]))
	}
	def, err := decode(data)
	if err != nil || len(def.Frames) != 1 {
		t.Fatalf("default frame selection: %+v %v", def, err)
	}
}

func TestSingleFrameAnimationDecodesLikeAStill(t *testing.T) {
	source := numbered(16, 16, 2)
	data := encodeAnimation(t, []*image.NRGBA{source}, []int{80}, 1)
	for _, frames := range []codec.FrameSel{codec.FirstFrame, codec.AllFrames} {
		img, err := decodeWith(data, codec.Options{Limits: codec.DefaultLimits(), Frames: frames})
		if err != nil || len(img.Frames) != 1 || img.Info.Animated || img.Info.Frames != 1 || img.Loop != 1 {
			t.Fatalf("%+v %v", img, err)
		}
		if !testimg.Identical(img.First(), source) {
			t.Fatal("pixels differ")
		}
	}
}

func TestAnimationWithAlphaKeepsTransparency(t *testing.T) {
	var sources []*image.NRGBA
	for i := 0; i < 3; i++ {
		sources = append(sources, withAlpha(numbered(40, 24, i)))
	}
	data := encodeAnimation(t, sources, []int{50, 60, 70}, 0)
	img, err := decodeAllFrames(data)
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range img.Frames {
		if !alphaIdentical(f.Pix, sources[i]) {
			t.Fatalf("frame %d: alpha differs", i)
		}
		if d := maxPremultipliedDelta(f.Pix, sources[i]); d > 1.5 {
			t.Fatalf("frame %d: premultiplied delta %.2f", i, d)
		}
		if !equalVisible(opaqueOnly(f.Pix), opaqueOnly(sources[i])) {
			t.Fatalf("frame %d: opaque pixels differ", i)
		}
	}
}

func TestAnimationAlphaFlagComesFromTheExtendedHeader(t *testing.T) {
	frame := frameChunk(0, 0, 12, 8, 60, frameNoBlend, losslessChunks(t, numbered(12, 8, 0))...)
	for flags, want := range map[byte]bool{flagAnimation: false, flagAnimation | flagAlpha: true} {
		data := riff(vp8x(flags, 12, 8), animChunk(0), frame, frame)
		info, err := probe(data)
		if err != nil || info.HasAlpha != want || (info.BitDepth == 32) != want {
			t.Fatalf("flags %#x: %+v %v", flags, info, err)
		}
	}
}

func opaqueOnly(src *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(src.Bounds())
	bounds := src.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if c := src.NRGBAAt(x, y); c.A == 255 {
				out.SetNRGBA(x, y, c)
			}
		}
	}
	return out
}

func compositingSpecs(lossy bool) []frameSpec {
	return []frameSpec{
		{img: numbered(24, 16, 0), milliseconds: 40, blend: true, lossy: lossy},
		{x: 4, y: 2, img: withAlpha(numbered(10, 8, 1)), milliseconds: 50, blend: true, dispose: true, lossy: lossy},
		{x: 8, y: 6, img: withAlpha(numbered(12, 8, 2)), milliseconds: 60, blend: true, lossy: lossy},
		{img: withAlpha(numbered(24, 16, 3)), milliseconds: 70, blend: false, lossy: lossy},
		{x: 2, y: 2, img: withAlpha(numbered(8, 8, 4)), milliseconds: 80, blend: false, dispose: true, lossy: lossy},
		{img: withAlpha(numbered(24, 16, 5)), milliseconds: 90, blend: true, lossy: lossy},
		{x: 6, y: 4, img: withAlpha(numbered(16, 10, 6)), milliseconds: 100, blend: true, dispose: true, lossy: lossy},
		{x: 0, y: 0, img: withAlpha(numbered(24, 16, 7)), milliseconds: 110, blend: true, lossy: lossy},
	}
}

func TestLosslessCompositingFollowsBlendAndDisposeFlags(t *testing.T) {
	specs := compositingSpecs(false)
	data := buildAnimation(t, 24, 16, 0, specs)
	img, err := decodeAllFrames(data)
	if err != nil {
		t.Fatal(err)
	}
	want := composeReference(24, 16, specs)
	if len(img.Frames) != len(want) {
		t.Fatalf("%d frames, want %d", len(img.Frames), len(want))
	}
	for i, f := range img.Frames {
		if d := maxPremultipliedDelta(f.Pix, want[i]); d > 4 {
			t.Fatalf("frame %d: premultiplied delta %.2f against the reference composition", i, d)
		}
		if f.Delay != time.Duration(specs[i].milliseconds)*time.Millisecond {
			t.Fatalf("frame %d delay %v", i, f.Delay)
		}
	}
}

func TestCompositingReferenceIsSensitiveToTheFlags(t *testing.T) {
	specs := compositingSpecs(false)
	img, err := decodeAllFrames(buildAnimation(t, 24, 16, 0, specs))
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*frameSpec){
		"dispose flipped": func(s *frameSpec) { s.dispose = !s.dispose },
		"blend flipped":   func(s *frameSpec) { s.blend = !s.blend },
	} {
		flipped := append([]frameSpec(nil), specs...)
		for i := range flipped {
			mutate(&flipped[i])
		}
		want := composeReference(24, 16, flipped)
		worst := 0.0
		for i, f := range img.Frames {
			worst = max(worst, maxPremultipliedDelta(f.Pix, want[i]))
		}
		if worst < 20 {
			t.Fatalf("%s: the specs do not exercise the flag (delta %.1f)", name, worst)
		}
	}
}

func TestLossyCompositingStaysCloseToTheReferenceComposition(t *testing.T) {
	specs := compositingSpecs(true)
	data := buildAnimation(t, 24, 16, 0, specs)
	img, err := decodeAllFrames(data)
	if err != nil {
		t.Fatal(err)
	}
	want := composeReference(24, 16, specs)
	if len(img.Frames) != len(want) {
		t.Fatalf("%d frames, want %d", len(img.Frames), len(want))
	}
	for i, f := range img.Frames {
		if quality := psnr(f.Pix, want[i], 250); quality < 28 {
			t.Fatalf("frame %d: PSNR %.1f dB against the reference composition", i, quality)
		}
	}
}

func TestDisposedAreaBecomesTransparentForTheNextFrame(t *testing.T) {
	red := solid(8, 8, color.NRGBA{R: 255, A: 255})
	green := solid(4, 4, color.NRGBA{G: 255, A: 255})
	specs := []frameSpec{
		{img: red, milliseconds: 50, blend: true, dispose: true},
		{x: 2, y: 2, img: green, milliseconds: 50, blend: true},
	}
	img, err := decodeAllFrames(buildAnimation(t, 8, 8, 0, specs))
	if err != nil {
		t.Fatal(err)
	}
	second := img.Frames[1].Pix
	if second.NRGBAAt(0, 0).A != 0 || second.NRGBAAt(7, 7).A != 0 {
		t.Fatalf("disposed background survived: %v %v", second.NRGBAAt(0, 0), second.NRGBAAt(7, 7))
	}
	if second.NRGBAAt(3, 3) != (color.NRGBA{G: 255, A: 255}) {
		t.Fatalf("inner frame: %v", second.NRGBAAt(3, 3))
	}
	kept := []frameSpec{
		{img: red, milliseconds: 50, blend: true},
		{x: 2, y: 2, img: green, milliseconds: 50, blend: true},
	}
	img, err = decodeAllFrames(buildAnimation(t, 8, 8, 0, kept))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Frames[1].Pix.NRGBAAt(0, 0); got != (color.NRGBA{R: 255, A: 255}) {
		t.Fatalf("background was not kept: %v", got)
	}
}

func TestAnimationPremultiplicationRoundTrip(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 256, 254))
	for alpha := 1; alpha < 255; alpha++ {
		for value := 0; value < 256; value++ {
			source.SetNRGBA(value, alpha-1, color.NRGBA{R: uint8(value), G: uint8(value), B: uint8(value), A: uint8(alpha)})
		}
	}
	data := encodeAnimation(t, []*image.NRGBA{source, source}, []int{50, 50}, 0)
	img, err := decodeAllFrames(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range img.Frames {
		for alpha := 1; alpha < 255; alpha++ {
			for value := 0; value < 256; value++ {
				got := f.Pix.NRGBAAt(value, alpha-1)
				if got.A != uint8(alpha) || got.R != got.G || got.G != got.B {
					t.Fatalf("alpha %d value %d: %v", alpha, value, got)
				}
				if (value == 0 || value == 255) && int(got.R) != value {
					t.Fatalf("alpha %d: saturated %d became %d", alpha, value, got.R)
				}
				if bound := 255/alpha + 1; int(got.R) > value+bound || int(got.R) < value-bound {
					t.Fatalf("alpha %d: %d became %d", alpha, value, got.R)
				}
			}
		}
	}
}

func TestAnimationFrameBudget(t *testing.T) {
	var sources []*image.NRGBA
	for i := 0; i < 3; i++ {
		sources = append(sources, numbered(16, 16, i))
	}
	data := encodeAnimation(t, sources, []int{50, 50, 50}, 0)
	decodeLimited := func(lim codec.Limits, frames codec.FrameSel) error {
		_, err := decodeWith(data, codec.Options{Limits: lim, Frames: frames})
		return err
	}
	lim := codec.DefaultLimits()
	lim.MaxFrames = 2
	if err := decodeLimited(lim, codec.AllFrames); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("frame count: %v", err)
	}
	if err := decodeLimited(lim, codec.FirstFrame); err != nil {
		t.Fatalf("first frame is not subject to the frame count: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxFramePixels = 16 * 16 * 2
	if err := decodeLimited(lim, codec.AllFrames); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("total pixels: %v", err)
	}
	lim.MaxFramePixels = 16 * 16 * 3
	if err := decodeLimited(lim, codec.AllFrames); err != nil {
		t.Fatalf("exact budget: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxPixels = 100
	if err := decodeLimited(lim, codec.FirstFrame); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("canvas: %v", err)
	}
}

func TestCancellationBetweenFrames(t *testing.T) {
	var sources []*image.NRGBA
	delays := make([]int, 30)
	for i := range delays {
		sources = append(sources, numbered(12, 10, i))
		delays[i] = 40
	}
	data := encodeAnimation(t, sources, delays, 0)
	options := codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames}
	full := &countingReader{data: data}
	if _, err := (Decoder{}).Decode(context.Background(), full, int64(len(data)), options); err != nil {
		t.Fatal(err)
	}
	total := full.reads.Load()
	for _, after := range []int64{total / 4, total / 2, total - 12, total - 2} {
		ctx, cancel := context.WithCancel(context.Background())
		reader := &countingReader{data: data, after: after, action: cancel}
		_, err := (Decoder{}).Decode(ctx, reader, int64(len(data)), options)
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled at read %d of %d: %v", after, total, err)
		}
		if reads := reader.reads.Load(); reads != after {
			t.Fatalf("decoding went on to read %d after cancellation at %d", reads, after)
		}
	}
}

func TestAnimationTruncatedAtEveryByte(t *testing.T) {
	specs := []frameSpec{
		{img: numbered(16, 10, 0), milliseconds: 40, blend: true},
		{x: 2, y: 2, img: withAlpha(numbered(8, 6, 1)), milliseconds: 50, blend: true, lossy: true},
		{x: 4, y: 0, img: withAlpha(numbered(10, 10, 2)), milliseconds: 60, blend: true, dispose: true},
		{img: numbered(16, 10, 3), milliseconds: 70, blend: false, lossy: true},
	}
	data := buildAnimation(t, 16, 10, 0, specs)
	whole, err := decodeAllFrames(data)
	if err != nil || len(whole.Frames) != len(specs) {
		t.Fatalf("%v", err)
	}
	shorter := 0
	for n := 0; n < len(data); n++ {
		prefix := data[:n]
		for _, frames := range []codec.FrameSel{codec.FirstFrame, codec.AllFrames} {
			img, err := decodeWith(prefix, codec.Options{Limits: codec.DefaultLimits(), Frames: frames})
			if err != nil {
				testimg.RequireCodecError(t, err)
				continue
			}
			if frames == codec.AllFrames && len(img.Frames) < len(specs) {
				shorter++
			}
			for i, f := range img.Frames {
				if !testimg.Identical(f.Pix, whole.Frames[i].Pix) || f.Delay != whole.Frames[i].Delay {
					t.Fatalf("prefix %d: frame %d differs from the complete decode", n, i)
				}
			}
		}
		if _, err := probe(prefix); err != nil {
			testimg.RequireCodecError(t, err)
		}
	}
	if shorter == 0 {
		t.Fatal("no truncation at a frame boundary produced a shorter animation")
	}
}

func TestAnimationMetadataChunksAreSkipped(t *testing.T) {
	inner := losslessChunks(t, numbered(12, 8, 0))
	other := losslessChunks(t, numbered(12, 8, 1))
	iccp := chunk("ICCP", bytes.Repeat([]byte{9}, 301))
	data := riff(
		vp8x(flagAnimation|0x20|0x08|0x04, 12, 8),
		iccp,
		animChunk(2),
		frameChunk(0, 0, 12, 8, 60, frameNoBlend, inner...),
		chunk("ZZZZ", []byte{1, 2, 3}),
		frameChunk(0, 0, 12, 8, 60, frameNoBlend, other...),
		exifChunk(testimg.OrientationTIFF(false, 3), true),
		chunk("XMP ", []byte("<x:xmpmeta/>")),
	)
	img, err := decodeAllFrames(data)
	if err != nil || len(img.Frames) != 2 || img.Loop != 2 || img.Info.Orientation != codec.OrientRotate180 {
		t.Fatalf("%+v %v", img, err)
	}
}

func TestFrameOutsideTheCanvasIsCorrupt(t *testing.T) {
	inner := losslessChunks(t, numbered(10, 8, 0))
	data := riff(vp8x(flagAnimation, 24, 16), animChunk(0), frameChunk(20, 4, 10, 8, 50, 0, inner...))
	if _, err := probe(data); err != nil {
		t.Fatal(err)
	}
	for _, frames := range []codec.FrameSel{codec.FirstFrame, codec.AllFrames} {
		if _, err := decodeWith(data, codec.Options{Limits: codec.DefaultLimits(), Frames: frames}); !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("frames=%d: %v", frames, err)
		}
	}
}

func TestFrameWithDifferentSizeThanItsBitstreamIsCorrupt(t *testing.T) {
	inner := losslessChunks(t, numbered(10, 8, 0))
	data := riff(vp8x(flagAnimation, 24, 16), animChunk(0), frameChunk(0, 0, 12, 8, 50, 0, inner...))
	if _, err := decodeAllFrames(data); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("%v", err)
	}
}
