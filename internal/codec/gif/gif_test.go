package gif

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	stdgif "image/gif"
	"math/rand"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

var (
	paletteColors = color.Palette{color.RGBA{}, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}, color.RGBA{0, 255, 0, 255}}
	gridColors    = map[byte]color.NRGBA{
		'.': {},
		'R': {R: 255, A: 255},
		'B': {B: 255, A: 255},
		'G': {G: 255, A: 255},
	}
)

const (
	indexTransparent = 0
	indexRed         = 1
	indexBlue        = 2
	indexGreen       = 3
)

func solidFrame(rect image.Rectangle, index uint8) *image.Paletted {
	frame := image.NewPaletted(rect, paletteColors)
	for i := range frame.Pix {
		frame.Pix[i] = index
	}
	return frame
}

func encodeAnimation(t testing.TB, width, height int, loopCount int, frames []*image.Paletted, delays []int, disposals []byte) []byte {
	t.Helper()
	animation := &stdgif.GIF{
		Image:     frames,
		Delay:     delays,
		Disposal:  disposals,
		LoopCount: loopCount,
		Config:    image.Config{ColorModel: paletteColors, Width: width, Height: height},
	}
	var buf bytes.Buffer
	if err := stdgif.EncodeAll(&buf, animation); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodeAll(t testing.TB, data []byte) *codec.Image {
	t.Helper()
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func requireGrid(t *testing.T, name string, pix *image.NRGBA, rows ...string) {
	t.Helper()
	if pix.Bounds().Dy() != len(rows) {
		t.Fatalf("%s: height %d, want %d", name, pix.Bounds().Dy(), len(rows))
	}
	for y, row := range rows {
		if pix.Bounds().Dx() != len(row) {
			t.Fatalf("%s: width %d, want %d", name, pix.Bounds().Dx(), len(row))
		}
		for x := range row {
			if have, want := pix.NRGBAAt(x, y), gridColors[row[x]]; have != want {
				t.Fatalf("%s: pixel (%d,%d) = %v, want %v (%c)", name, x, y, have, want, row[x])
			}
		}
	}
}

func TestSniffAndExtensions(t *testing.T) {
	d := Decoder{}
	if !d.Sniff([]byte("GIF87a....")) || !d.Sniff([]byte("GIF89a")) {
		t.Fatal("signature not recognised")
	}
	for _, head := range [][]byte{nil, []byte("GIF89"), []byte("GIF88a"), []byte("\x89PNG\r\n\x1a\n")} {
		if d.Sniff(head) {
			t.Fatalf("%q sniffed as GIF", head)
		}
	}
	if d.ID() != codec.GIF || d.Name() != "GIF" || len(d.Extensions()) != 1 || d.Extensions()[0] != ".gif" {
		t.Fatal("identity")
	}
}

func TestPlayCountAndFrameDelay(t *testing.T) {
	for loopCount, want := range map[int]int{0: LoopForever, -1: 1, 1: 2, 2: 3, 65535: 65536} {
		if got := PlayCount(loopCount); got != want {
			t.Fatalf("PlayCount(%d) = %d, want %d", loopCount, got, want)
		}
	}
	for centiseconds, want := range map[int]time.Duration{
		0: 100 * time.Millisecond, 1: 100 * time.Millisecond, 2: 20 * time.Millisecond,
		7: 70 * time.Millisecond, 100: time.Second, 65535: 655350 * time.Millisecond,
	} {
		if got := FrameDelay(centiseconds); got != want {
			t.Fatalf("FrameDelay(%d) = %v, want %v", centiseconds, got, want)
		}
	}
}

func TestDisposalNone(t *testing.T) {
	for _, disposal := range []byte{0, stdgif.DisposalNone} {
		bluePatch := solidFrame(image.Rect(1, 1, 3, 3), indexBlue)
		greenPatch := solidFrame(image.Rect(2, 2, 4, 4), indexGreen)
		greenPatch.SetColorIndex(2, 2, indexTransparent)
		data := encodeAnimation(t, 4, 4, 0,
			[]*image.Paletted{solidFrame(image.Rect(0, 0, 4, 4), indexRed), bluePatch, greenPatch},
			[]int{5, 5, 5}, []byte{disposal, disposal, disposal})
		img := decodeAll(t, data)
		if len(img.Frames) != 3 {
			t.Fatalf("%d frames", len(img.Frames))
		}
		requireGrid(t, "none/0", img.Frames[0].Pix, "RRRR", "RRRR", "RRRR", "RRRR")
		requireGrid(t, "none/1", img.Frames[1].Pix, "RRRR", "RBBR", "RBBR", "RRRR")
		requireGrid(t, "none/2", img.Frames[2].Pix, "RRRR", "RBBR", "RBBG", "RRGG")
	}
}

func TestDisposalRestoreToBackground(t *testing.T) {
	data := encodeAnimation(t, 4, 4, 0,
		[]*image.Paletted{
			solidFrame(image.Rect(0, 0, 4, 4), indexRed),
			solidFrame(image.Rect(1, 1, 3, 3), indexBlue),
			solidFrame(image.Rect(3, 3, 4, 4), indexGreen),
		},
		[]int{5, 5, 5}, []byte{stdgif.DisposalNone, stdgif.DisposalBackground, stdgif.DisposalNone})
	img := decodeAll(t, data)
	requireGrid(t, "background/0", img.Frames[0].Pix, "RRRR", "RRRR", "RRRR", "RRRR")
	requireGrid(t, "background/1", img.Frames[1].Pix, "RRRR", "RBBR", "RBBR", "RRRR")
	requireGrid(t, "background/2", img.Frames[2].Pix, "RRRR", "R..R", "R..R", "RRRG")
}

func TestDisposalRestoreToPrevious(t *testing.T) {
	data := encodeAnimation(t, 4, 4, 0,
		[]*image.Paletted{
			solidFrame(image.Rect(0, 0, 4, 4), indexRed),
			solidFrame(image.Rect(1, 1, 3, 3), indexBlue),
			solidFrame(image.Rect(0, 0, 1, 1), indexGreen),
		},
		[]int{5, 5, 5}, []byte{stdgif.DisposalNone, stdgif.DisposalPrevious, stdgif.DisposalNone})
	img := decodeAll(t, data)
	requireGrid(t, "previous/1", img.Frames[1].Pix, "RRRR", "RBBR", "RBBR", "RRRR")
	requireGrid(t, "previous/2", img.Frames[2].Pix, "GRRR", "RRRR", "RRRR", "RRRR")
}

func TestDisposalMixedSequence(t *testing.T) {
	data := encodeAnimation(t, 4, 4, 0,
		[]*image.Paletted{
			solidFrame(image.Rect(0, 0, 2, 2), indexRed),
			solidFrame(image.Rect(2, 0, 4, 2), indexBlue),
			solidFrame(image.Rect(0, 2, 2, 4), indexGreen),
			solidFrame(image.Rect(2, 2, 4, 4), indexRed),
		},
		[]int{5, 5, 5, 5}, []byte{stdgif.DisposalPrevious, stdgif.DisposalBackground, stdgif.DisposalNone, stdgif.DisposalNone})
	img := decodeAll(t, data)
	requireGrid(t, "mixed/0", img.Frames[0].Pix, "RR..", "RR..", "....", "....")
	requireGrid(t, "mixed/1", img.Frames[1].Pix, "..BB", "..BB", "....", "....")
	requireGrid(t, "mixed/2", img.Frames[2].Pix, "....", "....", "GG..", "GG..")
	requireGrid(t, "mixed/3", img.Frames[3].Pix, "....", "....", "GGRR", "GGRR")
}

func TestFramesAreIndependentBuffers(t *testing.T) {
	data := encodeAnimation(t, 4, 4, 0,
		[]*image.Paletted{solidFrame(image.Rect(0, 0, 4, 4), indexRed), solidFrame(image.Rect(0, 0, 4, 4), indexBlue)},
		[]int{5, 5}, nil)
	img := decodeAll(t, data)
	img.Frames[0].Pix.SetNRGBA(0, 0, color.NRGBA{9, 9, 9, 9})
	requireGrid(t, "second frame", img.Frames[1].Pix, "BBBB", "BBBB", "BBBB", "BBBB")
}

func TestDelaysAndLoopCounts(t *testing.T) {
	frames := []*image.Paletted{
		solidFrame(image.Rect(0, 0, 2, 2), indexRed),
		solidFrame(image.Rect(0, 0, 2, 2), indexBlue),
		solidFrame(image.Rect(0, 0, 2, 2), indexGreen),
		solidFrame(image.Rect(0, 0, 2, 2), indexRed),
	}
	data := encodeAnimation(t, 2, 2, 0, frames, []int{0, 1, 2, 250}, nil)
	img := decodeAll(t, data)
	want := []time.Duration{100 * time.Millisecond, 100 * time.Millisecond, 20 * time.Millisecond, 2500 * time.Millisecond}
	for i, frame := range img.Frames {
		if frame.Delay != want[i] {
			t.Fatalf("frame %d delay %v, want %v", i, frame.Delay, want[i])
		}
	}
	for loopCount, wantLoop := range map[int]int{0: LoopForever, 3: 4, 1: 2} {
		data := encodeAnimation(t, 2, 2, loopCount, frames, []int{5, 5, 5, 5}, nil)
		if got := decodeAll(t, data).Loop; got != wantLoop {
			t.Fatalf("loop count %d: Loop = %d, want %d", loopCount, got, wantLoop)
		}
	}
	noExtension := encodeAnimation(t, 2, 2, -1, frames, []int{5, 5, 5, 5}, nil)
	if got := decodeAll(t, noExtension).Loop; got != 1 {
		t.Fatalf("no loop extension: Loop = %d, want 1", got)
	}
}

func TestProbeCountsFramesAndReadsMetadata(t *testing.T) {
	frames := []*image.Paletted{
		solidFrame(image.Rect(0, 0, 6, 5), indexRed),
		solidFrame(image.Rect(0, 0, 6, 5), indexBlue),
		solidFrame(image.Rect(0, 0, 6, 5), indexGreen),
	}
	data := encodeAnimation(t, 6, 5, 7, frames, []int{5, 5, 5}, nil)
	info, err := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := codec.Info{Format: codec.GIF, FormatName: "GIF", Width: 6, Height: 5, Frames: 3, Animated: true, HasAlpha: true, Orientation: codec.OrientNormal, BitDepth: 8}
	if info != want {
		t.Fatalf("got %+v, want %+v", info, want)
	}
	blocks, err := scanBlocks(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil || blocks.loopCount != 7 || blocks.frames != 3 || blocks.width != 6 || blocks.height != 5 {
		t.Fatalf("scan %+v %v", blocks, err)
	}
}

func TestProbeStaticImageWithoutTransparency(t *testing.T) {
	opaque := color.Palette{color.RGBA{1, 2, 3, 255}, color.RGBA{4, 5, 6, 255}}
	frame := image.NewPaletted(image.Rect(0, 0, 3, 2), opaque)
	var buf bytes.Buffer
	if err := stdgif.Encode(&buf, frame, nil); err != nil {
		t.Fatal(err)
	}
	info, err := Decoder{}.Probe(context.Background(), bytes.NewReader(buf.Bytes()), int64(buf.Len()), codec.DefaultLimits())
	if err != nil || info.Frames != 1 || info.Animated || info.HasAlpha || info.Width != 3 || info.Height != 2 {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestFirstFrameDecodesOnlyTheFirstFrameOnFullCanvas(t *testing.T) {
	data := encodeAnimation(t, 4, 4, 0,
		[]*image.Paletted{solidFrame(image.Rect(1, 1, 3, 2), indexBlue), solidFrame(image.Rect(0, 0, 4, 4), indexRed)},
		[]int{9, 5}, nil)
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits(), Frames: codec.FirstFrame})
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Frames) != 1 || img.Info.Frames != 2 || !img.Info.Animated {
		t.Fatalf("frames %d info %+v", len(img.Frames), img.Info)
	}
	requireGrid(t, "first", img.First(), "....", ".BB.", "....", "....")
	if img.Frames[0].Delay != 90*time.Millisecond || img.Loop != LoopForever {
		t.Fatalf("delay %v loop %d", img.Frames[0].Delay, img.Loop)
	}
}

func TestFirstFrameWorksWithoutTheRestOfTheFile(t *testing.T) {
	frames := []*image.Paletted{solidFrame(image.Rect(0, 0, 4, 4), indexRed), solidFrame(image.Rect(0, 0, 4, 4), indexBlue)}
	data := encodeAnimation(t, 4, 4, 0, frames, []int{5, 5}, nil)
	firstFrameEnd := bytes.LastIndex(data, []byte{0x21, 0xf9})
	cut := data[:firstFrameEnd]
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(cut), int64(len(cut)), codec.Options{Limits: codec.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	requireGrid(t, "first", img.First(), "RRRR", "RRRR", "RRRR", "RRRR")
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(cut), int64(len(cut)), codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames}); err == nil {
		t.Fatal("all frames of a cut file must fail")
	}
}

func rawGIF(width, height, frames int, lzw []byte) []byte {
	out := []byte("GIF89a")
	out = append(out, byte(width), byte(width>>8), byte(height), byte(height>>8), 0x80, 0, 0)
	out = append(out, 0, 0, 0, 255, 255, 255)
	out = append(out, 0x21, 0xff, 11)
	out = append(out, "NETSCAPE2.0"...)
	out = append(out, 3, 1, 5, 0, 0)
	for i := 0; i < frames; i++ {
		out = append(out, 0x21, 0xf9, 4, 0, 10, 0, 0, 0)
		out = append(out, 0x2c, 0, 0, 0, 0, byte(width), byte(width>>8), byte(height), byte(height>>8), 0)
		out = append(out, 2)
		for rest := lzw; len(rest) > 0; {
			n := len(rest)
			if n > 255 {
				n = 255
			}
			out = append(out, byte(n))
			out = append(out, rest[:n]...)
			rest = rest[n:]
		}
		out = append(out, 0)
	}
	return append(out, 0x3b)
}

func TestScannerDoesNotDecodePixelData(t *testing.T) {
	garbage := make([]byte, 700)
	rand.New(rand.NewSource(1)).Read(garbage)
	data := rawGIF(8, 8, 1000, garbage)
	info, err := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if info.Frames != 1000 || !info.Animated {
		t.Fatalf("%+v", info)
	}
	blocks, _ := scanBlocks(context.Background(), bytes.NewReader(data), int64(len(data)))
	if blocks.loopCount != 5 || blocks.firstDelayCS != 10 {
		t.Fatalf("%+v", blocks)
	}
	_, err = Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames})
	testimg.RequireCodecError(t, err)
}

func TestFrameBudgetIsCheckedBeforeDecoding(t *testing.T) {
	data := rawGIF(100, 100, 1000, []byte{1, 2, 3})
	lim := codec.DefaultLimits()
	lim.MaxFramePixels = 1_000_000
	_, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim, Frames: codec.AllFrames})
	if !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("pixels: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxFrames = 500
	_, err = Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim, Frames: codec.AllFrames})
	if !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("frames: %v", err)
	}
}

func TestSingleFrameLimitsOnAnimation(t *testing.T) {
	frames := []*image.Paletted{
		solidFrame(image.Rect(0, 0, 10, 10), indexRed), solidFrame(image.Rect(0, 0, 10, 10), indexBlue), solidFrame(image.Rect(0, 0, 10, 10), indexGreen),
	}
	data := encodeAnimation(t, 10, 10, 0, frames, []int{5, 5, 5}, nil)
	lim := codec.DefaultLimits()
	lim.MaxFrames = 2
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim, Frames: codec.AllFrames}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("all frames: %v", err)
	}
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim, Frames: codec.FirstFrame}); err != nil {
		t.Fatalf("first frame: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxPixels = 99
	if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("pixels: %v", err)
	}
}

func TestHugeCanvasIsRejectedQuickly(t *testing.T) {
	data := rawGIF(65535, 65535, 1, []byte{1, 2, 3})
	testimg.Within(t, 10*time.Second, func() {
		_, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames})
		if !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("decode: %v", err)
		}
		_, err = Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
		if !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("probe: %v", err)
		}
	})
}

func TestMalformedStructures(t *testing.T) {
	valid := rawGIF(4, 4, 1, []byte{1, 2, 3})
	cases := map[string][]byte{
		"empty":              nil,
		"header only":        []byte("GIF89a"),
		"bad signature":      append([]byte("GIF99a"), valid[6:]...),
		"zero size":          rawGIF(0, 0, 1, []byte{1}),
		"no frames":          append(append([]byte(nil), valid[:13+6+3+14+3]...), 0x3b),
		"unknown block":      append(append([]byte(nil), valid[:len(valid)-1]...), 0x55, 0x3b),
		"truncated subblock": valid[:len(valid)-3],
		"bad graphic block":  bytes.Replace(valid, []byte{0x21, 0xf9, 4}, []byte{0x21, 0xf9, 9}, 1),
	}
	for name, data := range cases {
		_, probeErr := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
		_, decodeErr := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames})
		if decodeErr == nil {
			t.Fatalf("%s: decode succeeded", name)
		}
		testimg.RequireCodecError(t, decodeErr)
		if probeErr != nil {
			testimg.RequireCodecError(t, probeErr)
		}
	}
}

func TestEveryTruncatedPrefix(t *testing.T) {
	frames := []*image.Paletted{
		solidFrame(image.Rect(0, 0, 5, 4), indexRed), solidFrame(image.Rect(1, 1, 3, 3), indexBlue),
	}
	data := encodeAnimation(t, 5, 4, 2, frames, []int{5, 5}, []byte{stdgif.DisposalNone, stdgif.DisposalBackground})
	for n := 0; n < len(data); n++ {
		prefix := data[:n]
		if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(prefix), int64(n), codec.DefaultLimits()); err != nil {
			testimg.RequireCodecError(t, err)
		}
		for _, sel := range []codec.FrameSel{codec.FirstFrame, codec.AllFrames} {
			_, err := Decoder{}.Decode(context.Background(), bytes.NewReader(prefix), int64(n), codec.Options{Limits: codec.DefaultLimits(), Frames: sel})
			if err != nil {
				testimg.RequireCodecError(t, err)
			}
			if err == nil && sel == codec.AllFrames {
				t.Fatalf("all frames: prefix %d/%d decoded", n, len(data))
			}
		}
	}
}

func TestCancelledContext(t *testing.T) {
	data := encodeAnimation(t, 4, 4, 0, []*image.Paletted{solidFrame(image.Rect(0, 0, 4, 4), indexRed), solidFrame(image.Rect(0, 0, 4, 4), indexBlue)}, []int{5, 5}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, sel := range []codec.FrameSel{codec.FirstFrame, codec.AllFrames} {
		if _, err := (Decoder{}).Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{Frames: sel}); !errors.Is(err, context.Canceled) {
			t.Fatalf("selection %d: %v", sel, err)
		}
	}
}

func TestCancellationBetweenFrames(t *testing.T) {
	var frames []*image.Paletted
	delays := []int{}
	for i := 0; i < 20; i++ {
		frames = append(frames, solidFrame(image.Rect(0, 0, 8, 8), indexRed))
		delays = append(delays, 5)
	}
	animation := &stdgif.GIF{Image: frames, Delay: delays, Config: image.Config{ColorModel: paletteColors, Width: 8, Height: 8}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := composeFrames(ctx, animation, 8, 8); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestCorruptedBytesNeverPanic(t *testing.T) {
	frames := []*image.Paletted{
		solidFrame(image.Rect(0, 0, 8, 8), indexRed), solidFrame(image.Rect(2, 2, 6, 6), indexBlue), solidFrame(image.Rect(0, 0, 3, 3), indexGreen),
	}
	data := encodeAnimation(t, 8, 8, 0, frames, []int{5, 5, 5}, []byte{3, 2, 1})
	rng := rand.New(rand.NewSource(4))
	for i := 0; i < 600; i++ {
		mutated := append([]byte(nil), data...)
		for k := 0; k < 1+rng.Intn(4); k++ {
			mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
		}
		_, err := Decoder{}.Decode(context.Background(), bytes.NewReader(mutated), int64(len(mutated)), codec.Options{Limits: testimg.SmallLimits(), Frames: codec.AllFrames})
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
	}
}

func FuzzDecode(f *testing.F) {
	frames := []*image.Paletted{solidFrame(image.Rect(0, 0, 6, 6), indexRed), solidFrame(image.Rect(1, 1, 4, 4), indexBlue)}
	f.Add(encodeAnimation(f, 6, 6, 0, frames, []int{5, 5}, []byte{2, 3}))
	f.Add(rawGIF(4, 4, 2, []byte{1, 2, 3, 4, 5}))
	f.Add([]byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"))
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := testimg.SmallLimits()
		ctx := context.Background()
		scanBlocks(ctx, bytes.NewReader(data), int64(len(data)))
		Decoder{}.Probe(ctx, bytes.NewReader(data), int64(len(data)), lim)
		for _, sel := range []codec.FrameSel{codec.FirstFrame, codec.AllFrames} {
			img, err := Decoder{}.Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim, Frames: sel})
			if err == nil && (img == nil || img.First() == nil) {
				t.Fatal("success without pixels")
			}
			if err != nil {
				testimg.RequireCodecError(t, err)
			}
		}
	})
}
