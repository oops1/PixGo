package apng

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	stdpng "image/png"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

var (
	red         = color.NRGBA{R: 255, A: 255}
	green       = color.NRGBA{G: 255, A: 255}
	blue        = color.NRGBA{B: 255, A: 255}
	clear0      = color.NRGBA{}
	hiddenGreen = color.NRGBA{G: 255}
	semiBlue    = color.NRGBA{B: 255, A: 128}
	mixed       = color.NRGBA{R: 127, B: 128, A: 255}
	faint       = color.NRGBA{R: 10, G: 20, B: 30, A: 128}
)

var legend = map[rune]color.NRGBA{
	'r': red, 'g': green, 'b': blue, '.': clear0, 't': hiddenGreen, 'm': mixed, 's': faint,
}

func artDiff(img *image.NRGBA, rows ...string) string {
	if img.Bounds().Dy() != len(rows) {
		return fmt.Sprintf("height %d, want %d", img.Bounds().Dy(), len(rows))
	}
	for y, row := range rows {
		if img.Bounds().Dx() != len(row) {
			return fmt.Sprintf("width %d, want %d", img.Bounds().Dx(), len(row))
		}
		var have strings.Builder
		for x := range row {
			c := at(img, x, y)
			found := '?'
			for r, want := range legend {
				if want == c {
					found = r
				}
			}
			have.WriteRune(found)
		}
		if have.String() != row {
			return fmt.Sprintf("row %d is %q, want %q", y, have.String(), row)
		}
	}
	return ""
}

func requireArt(t *testing.T, img *image.NRGBA, rows ...string) {
	t.Helper()
	if msg := artDiff(img, rows...); msg != "" {
		t.Fatal(msg)
	}
}

func rect(c color.NRGBA, w, h, x, y int, dispose, blend byte) frameSpec {
	return frameSpec{png: rgbaPNG(solid(w, h, c)), x: x, y: y, num: 1, den: 10, dispose: dispose, blend: blend}
}

func canvasFrame(c color.NRGBA) frameSpec {
	return rect(c, 4, 4, 0, 0, 0, 0)
}

func TestIdentity(t *testing.T) {
	d := Decoder{}
	if d.ID() != codec.APNG || d.Name() != "APNG" {
		t.Fatalf("%s %s", d.ID(), d.Name())
	}
	if got := strings.Join(d.Extensions(), " "); got != ".apng .png" {
		t.Fatalf("extensions %q", got)
	}
}

func TestSniff(t *testing.T) {
	two := animation{frames: []frameSpec{canvasFrame(red), canvasFrame(green)}}
	plain := chunkList{{kind: "IHDR", body: ihdr(4, 4, 8, 6, 0)}, {kind: "IDAT", body: deflate(make([]byte, 4*17))}, {kind: "IEND"}}
	huge := uint32(0xffffffff)
	cases := []struct {
		name string
		head []byte
		want bool
	}{
		{"control right after the header", two.bytes(t), true},
		{"control after ancillary chunks", animation{frames: two.frames, before: []rawChunk{{kind: "gAMA", body: []byte{0, 1, 2, 3}}, {kind: "tEXt", body: []byte("k\x00v")}}}.bytes(t), true},
		{"control pushed past the sniff window", animation{frames: two.frames, before: []rawChunk{{kind: "iCCP", body: make([]byte, codec.SniffLen)}}}.bytes(t), false},
		{"control behind a profile of three kilobytes", animation{frames: two.frames, before: []rawChunk{{kind: "iCCP", body: make([]byte, 3000)}}}.bytes(t), true},
		{"control after image data", append(plain[:2:2], append(chunkList{{kind: "acTL", body: animationControl(2, 0)}}, plain[2])...).bytes(), false},
		{"plain PNG", plain.bytes(), false},
		{"control of the wrong length", chunkList{{kind: "IHDR", body: ihdr(4, 4, 8, 6, 0)}, {kind: "acTL", body: []byte{0, 0, 0, 2}}, {kind: "IDAT"}}.bytes(), false},
		{"unknown chunk longer than the file hides the control", chunkList{{kind: "IHDR", body: ihdr(4, 4, 8, 6, 0)}, {kind: "tEXt", badSize: &huge}, {kind: "acTL", body: animationControl(2, 0)}}.bytes(), false},
		{"end of image before control", chunkList{{kind: "IHDR", body: ihdr(4, 4, 8, 6, 0)}, {kind: "IEND"}, {kind: "acTL", body: animationControl(2, 0)}}.bytes(), false},
		{"header not first", chunkList{{kind: "tEXt", body: []byte("a\x00b")}, {kind: "IHDR", body: ihdr(4, 4, 8, 6, 0)}, {kind: "acTL", body: animationControl(2, 0)}}.bytes(), false},
		{"header of the wrong length", append(append([]byte(nil), signature...), chunk("IHDR", make([]byte, 12))...), false},
		{"bad signature", append([]byte("\x89PNG\r\n\x1a\x0b"), two.bytes(t)[8:]...), false},
		{"jpeg", []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0}, false},
		{"gif", []byte("GIF89a......................................."), false},
		{"short", two.bytes(t)[:20], false},
		{"empty", nil, false},
	}
	for _, c := range cases {
		head := c.head
		if len(head) > codec.SniffLen {
			head = head[:codec.SniffLen]
		}
		if got := (Decoder{}).Sniff(head); got != c.want {
			t.Errorf("%s: Sniff = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestProbeDescribesTheAnimationWithoutDecoding(t *testing.T) {
	data := animation{
		plays:  3,
		frames: []frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, 0, 0), rect(blue, 1, 1, 3, 3, 0, 1)},
		before: []rawChunk{{kind: "eXIf", body: testimg.OrientationTIFF(false, 6)}},
	}.bytes(t)
	info, err := probe(data)
	if err != nil {
		t.Fatal(err)
	}
	want := codec.Info{Format: codec.APNG, FormatName: "APNG", Width: 4, Height: 4, Frames: 3, Animated: true, HasAlpha: true, Orientation: codec.OrientRotate90, BitDepth: 32}
	if info != want {
		t.Fatalf("%+v, want %+v", info, want)
	}
	img := mustDecodeAll(t, data)
	if img.Info != info || len(img.Frames) != 3 || img.Scale != 1 || img.Loop != 3 {
		t.Fatalf("decode: %+v loop %d frames %d", img.Info, img.Loop, len(img.Frames))
	}
}

func TestProbeDoesNotTouchFrameData(t *testing.T) {
	data := animation{frames: []frameSpec{canvasFrame(red), canvasFrame(green)}}.chunks(t).corruptCRC("fdAT", 0).corruptCRC("IDAT", 0).bytes()
	if _, err := probe(data); err != nil {
		t.Fatalf("probe verified compressed data: %v", err)
	}
	if _, err := decodeAll(data); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("decode: %v", err)
	}
}

func TestLoopCount(t *testing.T) {
	cases := []struct {
		plays uint32
		want  int
	}{{0, 0}, {1, 1}, {7, 7}, {math.MaxUint32, math.MaxInt32}}
	for _, c := range cases {
		img := mustDecodeAll(t, animation{plays: c.plays, frames: []frameSpec{canvasFrame(red), canvasFrame(green)}}.bytes(t))
		if img.Loop != c.want {
			t.Errorf("num_plays %d: Loop = %d, want %d", c.plays, img.Loop, c.want)
		}
	}
}

func TestFrameDelays(t *testing.T) {
	cases := []struct {
		name     string
		num, den uint16
		want     time.Duration
	}{
		{"tenth of a second", 1, 10, 100 * time.Millisecond},
		{"zero numerator is clamped to ten milliseconds", 0, 10, 10 * time.Millisecond},
		{"zero over zero is clamped", 0, 0, 10 * time.Millisecond},
		{"zero denominator means hundredths", 5, 0, 50 * time.Millisecond},
		{"hundredths", 3, 100, 30 * time.Millisecond},
		{"exactly the minimum", 10, 1000, 10 * time.Millisecond},
		{"below the minimum", 1, 1000, 10 * time.Millisecond},
		{"fraction keeps nanoseconds", 1, 30, time.Second / 30},
		{"long delay", 65535, 1, 65535 * time.Second},
		{"huge denominator", 1, 65535, 10 * time.Millisecond},
	}
	for _, c := range cases {
		f := rect(red, 4, 4, 0, 0, 0, 0)
		f.num, f.den = c.num, c.den
		img := mustDecodeAll(t, animation{frames: []frameSpec{f, canvasFrame(green)}}.bytes(t))
		if img.Frames[0].Delay != c.want {
			t.Errorf("%s: %v, want %v", c.name, img.Frames[0].Delay, c.want)
		}
	}
}

func TestCompositionRules(t *testing.T) {
	const (
		none       = 0
		background = 1
		previous   = 2
		source     = 0
		over       = 1
	)
	cases := []struct {
		name   string
		frames []frameSpec
		want   [][]string
	}{
		{
			"dispose none keeps earlier frames",
			[]frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, none, source), rect(blue, 1, 1, 3, 3, none, source)},
			[][]string{{"rrrr", "rrrr", "rrrr", "rrrr"}, {"rrrr", "rggr", "rggr", "rrrr"}, {"rrrr", "rggr", "rggr", "rrrb"}},
		},
		{
			"dispose background clears only the frame rectangle",
			[]frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, background, source), rect(blue, 1, 1, 0, 0, none, source)},
			[][]string{{"rrrr", "rrrr", "rrrr", "rrrr"}, {"rrrr", "rggr", "rggr", "rrrr"}, {"brrr", "r..r", "r..r", "rrrr"}},
		},
		{
			"dispose previous restores the canvas",
			[]frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, previous, source), rect(blue, 1, 1, 0, 0, none, source)},
			[][]string{{"rrrr", "rrrr", "rrrr", "rrrr"}, {"rrrr", "rggr", "rggr", "rrrr"}, {"brrr", "rrrr", "rrrr", "rrrr"}},
		},
		{
			"dispose previous on the first frame acts as background",
			[]frameSpec{rect(red, 4, 4, 0, 0, previous, source), rect(blue, 1, 1, 0, 0, none, source)},
			[][]string{{"rrrr", "rrrr", "rrrr", "rrrr"}, {"b...", "....", "....", "...."}},
		},
		{
			"consecutive previous disposals",
			[]frameSpec{canvasFrame(red), rect(green, 2, 2, 0, 0, previous, source), rect(blue, 2, 2, 2, 2, previous, source), rect(green, 1, 1, 1, 1, none, source)},
			[][]string{{"rrrr", "rrrr", "rrrr", "rrrr"}, {"ggrr", "ggrr", "rrrr", "rrrr"}, {"rrrr", "rrrr", "rrbb", "rrbb"}, {"rrrr", "rgrr", "rrrr", "rrrr"}},
		},
		{
			"previous restores before the next frame overlaps",
			[]frameSpec{canvasFrame(red), rect(green, 3, 3, 0, 0, previous, source), rect(blue, 2, 2, 1, 1, none, source)},
			[][]string{{"rrrr", "rrrr", "rrrr", "rrrr"}, {"gggr", "gggr", "gggr", "rrrr"}, {"rrrr", "rbbr", "rbbr", "rrrr"}},
		},
		{
			"background of a middle frame exposes transparency",
			[]frameSpec{canvasFrame(red), rect(green, 1, 1, 0, 0, background, source), rect(blue, 1, 1, 3, 3, none, source)},
			[][]string{{"rrrr", "rrrr", "rrrr", "rrrr"}, {"grrr", "rrrr", "rrrr", "rrrr"}, {".rrr", "rrrr", "rrrr", "rrrb"}},
		},
		{
			"source blend replaces alpha too",
			[]frameSpec{canvasFrame(red), rect(hiddenGreen, 2, 2, 1, 1, none, source)},
			[][]string{{"rrrr", "rrrr", "rrrr", "rrrr"}, {"rrrr", "rttr", "rttr", "rrrr"}},
		},
		{
			"over blend keeps what lies below a transparent pixel",
			[]frameSpec{canvasFrame(red), rect(hiddenGreen, 2, 2, 1, 1, none, over)},
			[][]string{{"rrrr", "rrrr", "rrrr", "rrrr"}, {"rrrr", "rrrr", "rrrr", "rrrr"}},
		},
		{
			"over blend of an opaque pixel replaces",
			[]frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, none, over)},
			[][]string{{"rrrr", "rrrr", "rrrr", "rrrr"}, {"rrrr", "rggr", "rggr", "rrrr"}},
		},
		{
			"over blend mixes a translucent pixel with an opaque one",
			[]frameSpec{canvasFrame(red), rect(semiBlue, 2, 2, 1, 1, none, over)},
			[][]string{{"rrrr", "rrrr", "rrrr", "rrrr"}, {"rrrr", "rmmr", "rmmr", "rrrr"}},
		},
		{
			"over blend onto transparent keeps the source colour",
			[]frameSpec{canvasFrame(clear0), rect(faint, 2, 2, 1, 1, none, over)},
			[][]string{{"....", "....", "....", "...."}, {"....", ".ss.", ".ss.", "...."}},
		},
		{
			"a single frame",
			[]frameSpec{canvasFrame(blue)},
			[][]string{{"bbbb", "bbbb", "bbbb", "bbbb"}},
		},
	}
	for _, c := range cases {
		for _, split := range []int{1, 3} {
			img := mustDecodeAll(t, animation{pieces: split, frames: c.frames}.bytes(t))
			if len(img.Frames) != len(c.want) {
				t.Fatalf("%s: %d frames, want %d", c.name, len(img.Frames), len(c.want))
			}
			for i, art := range c.want {
				if msg := artDiff(img.Frames[i].Pix, art...); msg != "" {
					t.Fatalf("%s: frame %d (data in %d chunks): %s", c.name, i, split, msg)
				}
				if img.Frames[i].Delay != 100*time.Millisecond {
					t.Fatalf("%s: delay %v", c.name, img.Frames[i].Delay)
				}
			}
		}
	}
}

func TestEveryFrameIsAnIndependentImage(t *testing.T) {
	img := mustDecodeAll(t, animation{frames: []frameSpec{canvasFrame(red), rect(green, 2, 2, 0, 0, 0, 0), rect(blue, 1, 1, 0, 0, 0, 0)}}.bytes(t))
	if img.Frames[0].Pix == img.Frames[1].Pix || img.Frames[1].Pix == img.Frames[2].Pix {
		t.Fatal("frames share a bitmap")
	}
	requireArt(t, img.Frames[0].Pix, "rrrr", "rrrr", "rrrr", "rrrr")
	img.Frames[2].Pix.Pix[0] = 7
	requireArt(t, img.Frames[0].Pix, "rrrr", "rrrr", "rrrr", "rrrr")
	if img.First() != img.Frames[0].Pix {
		t.Fatal("First is not the first frame")
	}
}

func referenceOver(dst, src color.NRGBA) color.NRGBA {
	as, ad := float64(src.A)/255, float64(dst.A)/255
	ao := as + ad*(1-as)
	if ao == 0 {
		return color.NRGBA{}
	}
	mix := func(s, d uint8) uint8 {
		return uint8(math.Round((float64(s)*as + float64(d)*ad*(1-as)) / ao))
	}
	return color.NRGBA{R: mix(src.R, dst.R), G: mix(src.G, dst.G), B: mix(src.B, dst.B), A: uint8(math.Round(ao * 255))}
}

func TestOverBlendMatchesTheFloatingPointFormula(t *testing.T) {
	alphas := []uint8{0, 1, 17, 64, 127, 128, 200, 254, 255}
	channels := []uint8{0, 1, 99, 128, 254, 255}
	for _, sa := range alphas {
		for _, da := range alphas {
			for _, sc := range channels {
				for _, dc := range channels {
					src := color.NRGBA{R: sc, G: 255 - sc, B: sc / 2, A: sa}
					dst := color.NRGBA{R: dc, G: dc / 3, B: 255 - dc, A: da}
					got := []byte{dst.R, dst.G, dst.B, dst.A}
					blendPixel(got, []byte{src.R, src.G, src.B, src.A})
					want := referenceOver(dst, src)
					if sa == 0 {
						want = dst
					}
					if da == 0 && sa != 0 {
						want = src
					}
					have := color.NRGBA{R: got[0], G: got[1], B: got[2], A: got[3]}
					if absDiff(have.R, want.R) > 1 || absDiff(have.G, want.G) > 1 || absDiff(have.B, want.B) > 1 || absDiff(have.A, want.A) > 1 {
						t.Fatalf("over %v on %v = %v, want about %v", src, dst, have, want)
					}
				}
			}
		}
	}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestSingleFrameAnimationIsAStillImage(t *testing.T) {
	src := testimg.Noise(9, 7, 3, true)
	data := animation{plays: 5, frames: []frameSpec{{png: rgbaPNG(src), num: 1, den: 10}}}.bytes(t)
	info, err := probe(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Animated || info.Frames != 1 || info.Format != codec.APNG {
		t.Fatalf("%+v", info)
	}
	img := mustDecodeAll(t, data)
	if len(img.Frames) != 1 || !testimg.Identical(img.First(), src) || img.Info.Animated {
		t.Fatalf("%d frames", len(img.Frames))
	}
}

func TestDefaultImageOutsideTheAnimationIsSkipped(t *testing.T) {
	a := animation{
		defaultPNG: rgbaPNG(solid(4, 4, blue)),
		pieces:     2,
		frames:     []frameSpec{canvasFrame(red), rect(green, 2, 2, 2, 2, 0, 0)},
	}
	data := a.bytes(t)
	info, err := probe(data)
	if err != nil || info.Frames != 2 || !info.Animated {
		t.Fatalf("%+v %v", info, err)
	}
	all := mustDecodeAll(t, data)
	if len(all.Frames) != 2 {
		t.Fatalf("%d frames", len(all.Frames))
	}
	requireArt(t, all.Frames[0].Pix, "rrrr", "rrrr", "rrrr", "rrrr")
	requireArt(t, all.Frames[1].Pix, "rrrr", "rrrr", "rrgg", "rrgg")
	first, err := decodeWith(data, codec.FirstFrame, codec.DefaultLimits())
	if err != nil || len(first.Frames) != 1 {
		t.Fatalf("%v", err)
	}
	requireArt(t, first.First(), "rrrr", "rrrr", "rrrr", "rrrr")
	if first.Info.Frames != 2 || !first.Info.Animated {
		t.Fatalf("first-frame decode lost the frame count: %+v", first.Info)
	}
}

func TestAnimationFramesMayBeSmallerThanTheCanvasWhenTheDefaultImageIsSeparate(t *testing.T) {
	a := animation{
		defaultPNG: rgbaPNG(solid(4, 4, blue)),
		frames:     []frameSpec{rect(red, 2, 2, 1, 1, 0, 0), rect(green, 1, 1, 0, 0, 0, 0)},
	}
	img := mustDecodeAll(t, a.bytes(t))
	requireArt(t, img.Frames[0].Pix, "....", ".rr.", ".rr.", "....")
	requireArt(t, img.Frames[1].Pix, "g...", ".rr.", ".rr.", "....")
}

func TestFirstFrameOnlyDecodesTheFirstFrame(t *testing.T) {
	data := animation{frames: []frameSpec{rect(red, 4, 4, 0, 0, 1, 0), rect(green, 2, 2, 0, 0, 0, 0), canvasFrame(blue)}}.chunks(t).corruptCRC("fdAT", 1).bytes()
	first, err := decodeWith(data, codec.FirstFrame, codec.DefaultLimits())
	if err != nil {
		t.Fatalf("a damaged later frame must not break the first-frame decode: %v", err)
	}
	if len(first.Frames) != 1 {
		t.Fatalf("%d frames", len(first.Frames))
	}
	requireArt(t, first.First(), "rrrr", "rrrr", "rrrr", "rrrr")
	if _, err := decodeAll(data); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("all frames: %v", err)
	}
	zero, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{})
	if err != nil || len(zero.Frames) != 1 {
		t.Fatalf("zero options: %v", err)
	}
}

func randomNRGBA(w, h int, seed int64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	rand.New(rand.NewSource(seed)).Read(img.Pix)
	return img
}

func closeTo(a, b color.NRGBA, tolerance int) bool {
	return absDiff(a.R, b.R) <= tolerance && absDiff(a.G, b.G) <= tolerance && absDiff(a.B, b.B) <= tolerance && absDiff(a.A, b.A) <= tolerance
}

func TestFramesMatchTheStandardDecoderForEveryColourType(t *testing.T) {
	palette := func(n int) color.Palette {
		p := make(color.Palette, n)
		for i := range p {
			p[i] = color.NRGBA{R: uint8(i * 37), G: uint8(255 - i*11), B: uint8(i * 7), A: uint8(255 - (i%4)*60)}
		}
		return p
	}
	build := func(kind string, seed int64) image.Image {
		w, h := 11, 6
		rng := rand.New(rand.NewSource(seed))
		switch kind {
		case "gray8":
			img := image.NewGray(image.Rect(0, 0, w, h))
			rng.Read(img.Pix)
			return img
		case "gray16":
			img := image.NewGray16(image.Rect(0, 0, w, h))
			rng.Read(img.Pix)
			return img
		case "rgb8":
			img := randomNRGBA(w, h, seed)
			for i := 3; i < len(img.Pix); i += 4 {
				img.Pix[i] = 255
			}
			return img
		case "rgba8":
			return randomNRGBA(w, h, seed)
		case "rgba16":
			img := image.NewNRGBA64(image.Rect(0, 0, w, h))
			rng.Read(img.Pix)
			return img
		}
		var depthPalette color.Palette
		switch kind {
		case "indexed1":
			depthPalette = palette(2)
		case "indexed2":
			depthPalette = palette(4)
		case "indexed4":
			depthPalette = palette(16)
		default:
			depthPalette = palette(200)
		}
		img := image.NewPaletted(image.Rect(0, 0, w, h), depthPalette)
		for i := range img.Pix {
			img.Pix[i] = uint8(rng.Intn(len(depthPalette)))
		}
		return img
	}
	kinds := []struct {
		name      string
		tolerance int
		alpha     bool
		depth     int
	}{
		{"gray8", 0, false, 8}, {"gray16", 1, false, 16}, {"rgb8", 0, false, 24}, {"rgba8", 0, true, 32},
		{"rgba16", 1, true, 64}, {"indexed1", 0, true, 1}, {"indexed2", 0, true, 2}, {"indexed4", 0, true, 4}, {"indexed8", 0, true, 8},
	}
	for _, k := range kinds {
		first, second := build(k.name, 1), build(k.name, 2)
		sources := []image.Image{first, second}
		files := [][]byte{encodeStd(t, first), encodeStd(t, second)}
		data := animation{frames: []frameSpec{{png: files[0], num: 1, den: 10}, {png: files[1], num: 1, den: 10}}, pieces: 2}.bytes(t)
		info, err := probe(data)
		if err != nil {
			t.Fatalf("%s: %v", k.name, err)
		}
		if info.BitDepth != k.depth || info.HasAlpha != k.alpha || info.Frames != 2 {
			t.Fatalf("%s: %+v", k.name, info)
		}
		img := mustDecodeAll(t, data)
		for i, file := range files {
			reference, err := stdpng.Decode(bytes.NewReader(file))
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < 6; y++ {
				for x := 0; x < 11; x++ {
					want := color.NRGBAModel.Convert(reference.At(x, y)).(color.NRGBA)
					if p, ok := sources[i].(*image.NRGBA64); ok {
						q := p.NRGBA64At(x, y)
						want = color.NRGBA{R: uint8(q.R >> 8), G: uint8(q.G >> 8), B: uint8(q.B >> 8), A: uint8(q.A >> 8)}
					}
					if have := at(img.Frames[i].Pix, x, y); !closeTo(have, want, k.tolerance) {
						t.Fatalf("%s frame %d (%d,%d): %v, want %v", k.name, i, x, y, have, want)
					}
				}
			}
		}
	}
}

func TestInterlacedFramesAreDecoded(t *testing.T) {
	first, second := randomNRGBA(13, 11, 5), randomNRGBA(13, 11, 6)
	data := animation{frames: []frameSpec{{png: interlacedRGBAPNG(first), num: 1, den: 10}, {png: interlacedRGBAPNG(second), num: 1, den: 10}}}.bytes(t)
	img := mustDecodeAll(t, data)
	if !testimg.Identical(img.Frames[0].Pix, first) || !testimg.Identical(img.Frames[1].Pix, second) {
		t.Fatal("interlaced frames differ")
	}
}

func TestAncillaryChunksAreSkippedAndTrailingBytesIgnored(t *testing.T) {
	list := animation{frames: []frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, 0, 0)}}.chunks(t)
	list = list.insertAt(list.index("IDAT", 0), rawChunk{kind: "gAMA", body: []byte{0, 1, 0, 0}}, rawChunk{kind: "iCCP", body: make([]byte, 200)})
	list = list.insertAt(list.index("fcTL", 1), rawChunk{kind: "tEXt", body: []byte("a\x00b")})
	list = list.insertAt(list.index("acTL", 0)+1, rawChunk{kind: "acTL", body: animationControl(9, 9)})
	data := append(list.bytes(), "trailing junk"...)
	img := mustDecodeAll(t, data)
	if len(img.Frames) != 2 || img.Loop != 0 {
		t.Fatalf("%d frames loop %d", len(img.Frames), img.Loop)
	}
	requireArt(t, img.Frames[1].Pix, "rrrr", "rggr", "rggr", "rrrr")
}

func TestHasAlphaFollowsWhatDecodingProduces(t *testing.T) {
	opaque := func(c color.NRGBA, w, h, x, y int, dispose byte) frameSpec {
		return frameSpec{png: encodeStd(t, solid(w, h, c)), x: x, y: y, num: 1, den: 10, dispose: dispose}
	}
	cases := []struct {
		name string
		a    animation
		want bool
	}{
		{"opaque frames", animation{frames: []frameSpec{opaque(red, 4, 4, 0, 0, 0), opaque(green, 2, 2, 1, 1, 0)}}, false},
		{"background disposal of the last frame is not visible", animation{frames: []frameSpec{opaque(red, 4, 4, 0, 0, 0), opaque(green, 2, 2, 1, 1, 1)}}, false},
		{"background disposal of an earlier frame", animation{frames: []frameSpec{opaque(red, 4, 4, 0, 0, 1), opaque(green, 2, 2, 1, 1, 0)}}, true},
		{"previous disposal keeps the canvas opaque", animation{frames: []frameSpec{opaque(red, 4, 4, 0, 0, 2), opaque(green, 2, 2, 1, 1, 0)}}, false},
		{"partial first frame leaves transparent margins", animation{defaultPNG: encodeStd(t, solid(4, 4, blue)), frames: []frameSpec{opaque(red, 2, 2, 1, 1, 0), opaque(green, 4, 4, 0, 0, 0)}}, true},
		{"full first frame after a default image", animation{defaultPNG: encodeStd(t, solid(4, 4, blue)), frames: []frameSpec{opaque(red, 4, 4, 0, 0, 0), opaque(green, 2, 2, 1, 1, 0)}}, false},
		{"alpha channel", animation{frames: []frameSpec{canvasFrame(red), canvasFrame(green)}}, true},
	}
	for _, c := range cases {
		info, err := probe(c.a.bytes(t))
		if err != nil || info.HasAlpha != c.want {
			t.Errorf("%s: HasAlpha %v (%v), want %v", c.name, info.HasAlpha, err, c.want)
		}
	}
	keyed := animation{frames: []frameSpec{opaque(red, 4, 4, 0, 0, 0), opaque(green, 4, 4, 0, 0, 0)}}.chunks(t)
	keyed = keyed.insertAt(keyed.index("acTL", 0), rawChunk{kind: "tRNS", body: []byte{0, 255, 0, 0, 0, 0}})
	if info, err := probe(keyed.bytes()); err != nil || !info.HasAlpha {
		t.Errorf("colour key: %+v %v", info, err)
	}
}

func TestOrientationFromExifChunk(t *testing.T) {
	for _, bigEndian := range []bool{false, true} {
		for v := uint16(1); v <= 8; v++ {
			data := animation{frames: []frameSpec{canvasFrame(red), canvasFrame(green)}, before: []rawChunk{{kind: "eXIf", body: testimg.OrientationTIFF(bigEndian, v)}}}.bytes(t)
			info, err := probe(data)
			if err != nil || info.Orientation != codec.Orientation(v) {
				t.Fatalf("v=%d: %+v %v", v, info, err)
			}
		}
	}
	data := animation{frames: []frameSpec{canvasFrame(red), canvasFrame(green)}, before: []rawChunk{{kind: "eXIf", body: []byte("garbage")}}}.bytes(t)
	if info, err := probe(data); err != nil || info.Orientation != codec.OrientNormal {
		t.Fatalf("%+v %v", info, err)
	}
}

func mutate(t *testing.T, name string, base chunkList, change func(chunkList) chunkList, probeFails bool) {
	t.Helper()
	data := change(base.clone()).bytes()
	if _, err := decodeAll(data); !errors.Is(err, codec.ErrCorrupt) {
		t.Errorf("%s: decode: %v", name, err)
	}
	_, err := probe(data)
	if probeFails && !errors.Is(err, codec.ErrCorrupt) {
		t.Errorf("%s: probe: %v", name, err)
	}
	if !probeFails && err != nil {
		t.Errorf("%s: probe should accept it: %v", name, err)
	}
}

func setBody(kind string, nth int, set func([]byte)) func(chunkList) chunkList {
	return func(c chunkList) chunkList {
		set(c[c.index(kind, nth)].body)
		return c
	}
}

func TestMalformedStructureIsCorrupt(t *testing.T) {
	base := animation{plays: 1, pieces: 2, frames: []frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, 0, 0), rect(blue, 2, 2, 2, 2, 0, 1)}}.chunks(t)
	huge := uint32(0xfffffff0)
	structural := []struct {
		name   string
		change func(chunkList) chunkList
	}{
		{"IHDR checksum", func(c chunkList) chunkList { return c.corruptCRC("IHDR", 0) }},
		{"acTL checksum", func(c chunkList) chunkList { return c.corruptCRC("acTL", 0) }},
		{"fcTL checksum", func(c chunkList) chunkList { return c.corruptCRC("fcTL", 1) }},
		{"acTL of 4 bytes", func(c chunkList) chunkList {
			return c.edit("acTL", 0, func(ch *rawChunk) { ch.body = ch.body[:4] })
		}},
		{"acTL without frames", setBody("acTL", 0, func(b []byte) { binary.BigEndian.PutUint32(b, 0) })},
		{"fewer frames declared than present", setBody("acTL", 0, func(b []byte) { binary.BigEndian.PutUint32(b, 2) })},
		{"more frames declared than present", setBody("acTL", 0, func(b []byte) { binary.BigEndian.PutUint32(b, 4) })},
		{"fcTL sequence gap", setBody("fcTL", 1, func(b []byte) { b[3] += 5 })},
		{"fcTL sequence repeated", setBody("fcTL", 2, func(b []byte) { b[3]-- })},
		{"fdAT sequence gap", setBody("fdAT", 1, func(b []byte) { b[3] += 9 })},
		{"fdAT sequence repeated", setBody("fdAT", 1, func(b []byte) { b[3]-- })},
		{"frame wider than the canvas", setBody("fcTL", 1, func(b []byte) { b[7] = 5 })},
		{"frame taller than the canvas", setBody("fcTL", 1, func(b []byte) { b[11] = 9 })},
		{"frame with no width", setBody("fcTL", 1, func(b []byte) { binary.BigEndian.PutUint32(b[4:], 0) })},
		{"frame with no height", setBody("fcTL", 1, func(b []byte) { binary.BigEndian.PutUint32(b[8:], 0) })},
		{"frame pushed past the right edge", setBody("fcTL", 1, func(b []byte) { b[15] = 3 })},
		{"frame pushed past the bottom edge", setBody("fcTL", 1, func(b []byte) { b[19] = 3 })},
		{"frame offset beyond 32 bits of sanity", setBody("fcTL", 1, func(b []byte) { binary.BigEndian.PutUint32(b[12:], 0xffffffff) })},
		{"frame size beyond 32 bits of sanity", setBody("fcTL", 1, func(b []byte) { binary.BigEndian.PutUint32(b[4:], 0xffffffff) })},
		{"unknown dispose operation", setBody("fcTL", 1, func(b []byte) { b[24] = 3 })},
		{"unknown blend operation", setBody("fcTL", 1, func(b []byte) { b[25] = 2 })},
		{"default frame smaller than the canvas", setBody("fcTL", 0, func(b []byte) { binary.BigEndian.PutUint32(b[4:], 3) })},
		{"default frame offset", setBody("fcTL", 0, func(b []byte) { b[15] = 1 })},
		{"fcTL of 25 bytes", func(c chunkList) chunkList {
			return c.edit("fcTL", 1, func(ch *rawChunk) { ch.body = ch.body[:25] })
		}},
		{"no animation control", func(c chunkList) chunkList { return c.removeAt(c.index("acTL", 0)) }},
		{"frame control before animation control", func(c chunkList) chunkList {
			control := c[c.index("acTL", 0)]
			c = c.removeAt(c.index("acTL", 0))
			return c.insertAt(c.index("fcTL", 0)+1, control)
		}},
		{"frame data outside a frame", func(c chunkList) chunkList {
			return c.insertAt(c.index("fcTL", 0), rawChunk{kind: "fdAT", body: []byte{0, 0, 0, 0, 1, 2, 3}})
		}},
		{"fdAT for the frame stored in IDAT", func(c chunkList) chunkList {
			return c.insertAt(c.index("IDAT", 1)+1, rawChunk{kind: "fdAT", body: []byte{0, 0, 0, 1, 1, 2, 3}})
		}},
		{"fdAT shorter than its sequence number", func(c chunkList) chunkList {
			return c.edit("fdAT", 0, func(ch *rawChunk) { ch.body = ch.body[:2] })
		}},
		{"IDAT chunks split by another chunk", func(c chunkList) chunkList {
			return c.insertAt(c.index("IDAT", 1), rawChunk{kind: "tEXt", body: []byte("a\x00b")})
		}},
		{"missing end chunk", func(c chunkList) chunkList { return c.removeAt(c.index("IEND", 0)) }},
		{"frame without data in the middle", func(c chunkList) chunkList {
			c = c.removeAt(c.index("fdAT", 0))
			return c.removeAt(c.index("fdAT", 0))
		}},
		{"last frame without data", func(c chunkList) chunkList {
			c = c.removeAt(c.index("fdAT", 2))
			return c.removeAt(c.index("fdAT", 2))
		}},
		{"frame whose data chunks are empty", func(c chunkList) chunkList {
			for i := range c {
				if c[i].kind == "fdAT" {
					c[i].body = c[i].body[:4]
				}
			}
			return c
		}},
		{"two frame controls in a row", func(c chunkList) chunkList {
			return c.insertAt(c.index("fcTL", 0)+1, c[c.index("fcTL", 1)])
		}},
		{"no image data at all", func(c chunkList) chunkList {
			return chunkList{c[0], c[c.index("acTL", 0)], c[c.index("IEND", 0)]}
		}},
		{"chunk longer than the file", func(c chunkList) chunkList {
			return c.edit("fcTL", 2, func(ch *rawChunk) { ch.badSize = &huge })
		}},
		{"image data before the animation control", func(c chunkList) chunkList {
			return chunkList{c[0], {kind: "IDAT", body: c[c.index("IDAT", 0)].body}, c[c.index("acTL", 0)], c[c.index("IEND", 0)]}
		}},
	}
	for _, c := range structural {
		mutate(t, c.name, base, c.change, true)
	}
}

func TestMalformedPixelDataIsCorrupt(t *testing.T) {
	base := animation{pieces: 2, frames: []frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, 0, 0), canvasFrame(blue)}}.chunks(t)
	cases := []struct {
		name   string
		change func(chunkList) chunkList
	}{
		{"IDAT checksum", func(c chunkList) chunkList { return c.corruptCRC("IDAT", 1) }},
		{"fdAT checksum", func(c chunkList) chunkList { return c.corruptCRC("fdAT", 2) }},
		{"truncated compressed stream", func(c chunkList) chunkList {
			return c.edit("fdAT", 0, func(ch *rawChunk) { ch.body = ch.body[:6] }).edit("fdAT", 1, func(ch *rawChunk) { ch.body = ch.body[:5] })
		}},
		{"random bytes as frame data", func(c chunkList) chunkList {
			noise := make([]byte, 40)
			rand.New(rand.NewSource(1)).Read(noise)
			return c.edit("fdAT", 0, func(ch *rawChunk) { ch.body = append(ch.body[:4], noise...) }).edit("fdAT", 1, func(ch *rawChunk) { ch.body = ch.body[:4] })
		}},
		{"frame data for a larger image", func(c chunkList) chunkList {
			return c.edit("fcTL", 1, func(ch *rawChunk) {
				binary.BigEndian.PutUint32(ch.body[4:], 3)
				binary.BigEndian.PutUint32(ch.body[8:], 3)
			})
		}},
	}
	for _, c := range cases {
		mutate(t, c.name, base, c.change, false)
	}
}

func TestIndexedFramesNeedTheirPalette(t *testing.T) {
	pal := color.Palette{color.NRGBA{R: 255, A: 255}, color.NRGBA{G: 255, A: 128}}
	a, b := image.NewPaletted(image.Rect(0, 0, 4, 4), pal), image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
	for i := range b.Pix {
		b.Pix[i] = 1
	}
	base := animation{frames: []frameSpec{{png: encodeStd(t, a), num: 1, den: 10}, {png: encodeStd(t, b), num: 1, den: 10}}}.chunks(t)
	if _, err := decodeAll(base.bytes()); err != nil {
		t.Fatal(err)
	}
	mutate(t, "palette checksum", base, func(c chunkList) chunkList { return c.corruptCRC("PLTE", 0) }, true)
	mutate(t, "palette of 4 bytes", base, func(c chunkList) chunkList {
		return c.edit("PLTE", 0, func(ch *rawChunk) { ch.body = ch.body[:4] })
	}, true)
	mutate(t, "empty palette", base, func(c chunkList) chunkList {
		return c.edit("PLTE", 0, func(ch *rawChunk) { ch.body = nil })
	}, true)
	mutate(t, "oversized palette", base, func(c chunkList) chunkList {
		return c.edit("PLTE", 0, func(ch *rawChunk) { ch.body = make([]byte, 771) })
	}, true)
	mutate(t, "transparency checksum", base, func(c chunkList) chunkList { return c.corruptCRC("tRNS", 0) }, true)
	mutate(t, "oversized transparency", base, func(c chunkList) chunkList {
		return c.edit("tRNS", 0, func(ch *rawChunk) { ch.body = make([]byte, 800) })
	}, true)
	mutate(t, "no palette", base, func(c chunkList) chunkList { return c.removeAt(c.index("PLTE", 0)) }, false)
	late := base.clone()
	late = late.insertAt(late.index("fcTL", 0), late[late.index("PLTE", 0)], late[late.index("tRNS", 0)])
	if _, err := decodeAll(late.bytes()); err != nil {
		t.Fatalf("a second palette after the first is ignored: %v", err)
	}
	moved := base.clone()
	paletteChunk := moved[moved.index("PLTE", 0)]
	moved = moved.removeAt(moved.index("PLTE", 0))
	moved = moved.insertAt(moved.index("IDAT", 0), paletteChunk)
	if _, err := decodeAll(moved.bytes()); err != nil {
		t.Fatalf("palette between acTL and fcTL: %v", err)
	}
	after := base.clone()
	paletteChunk = after[after.index("PLTE", 0)]
	after = after.removeAt(after.index("PLTE", 0))
	after = after.insertAt(after.index("IEND", 0), paletteChunk)
	if _, err := decodeAll(after.bytes()); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("a palette after the image data must not rescue the file: %v", err)
	}
}

func TestPlainPNGIsNotAnAnimation(t *testing.T) {
	plain := encodeStd(t, testimg.Noise(6, 6, 1, true))
	if _, err := probe(plain); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("probe: %v", err)
	}
	if _, err := decodeAll(plain); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("decode: %v", err)
	}
	if _, err := probe([]byte("GIF89a not a png at all, not even close to one")); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("garbage: %v", err)
	}
}

func headerOnly(width, height uint32, control []byte) []byte {
	return chunkList{
		{kind: "IHDR", body: ihdr(0, 0, 8, 6, 0)},
		{kind: "acTL", body: control},
		{kind: "IEND"},
	}.edit("IHDR", 0, func(ch *rawChunk) {
		binary.BigEndian.PutUint32(ch.body[0:], width)
		binary.BigEndian.PutUint32(ch.body[4:], height)
	}).bytes()
}

func TestBombsAreRejectedBeforeAnyAllocation(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"canvas of a billion pixels squared", headerOnly(1<<30, 1<<30, animationControl(1, 0)), codec.ErrTooLarge},
		{"canvas beyond the PNG limit", headerOnly(0xffffffff, 1, animationControl(1, 0)), codec.ErrCorrupt},
		{"empty canvas", headerOnly(0, 5, animationControl(1, 0)), codec.ErrCorrupt},
		{"four billion frames", headerOnly(4, 4, animationControl(0xffffffff, 0)), codec.ErrTooLarge},
		{"frame count just above the limit", headerOnly(4, 4, animationControl(10_001, 0)), codec.ErrTooLarge},
	}
	for _, c := range cases {
		testimg.Within(t, 10*time.Second, func() {
			if _, err := probe(c.data); !errors.Is(err, c.want) {
				t.Errorf("%s: probe: %v", c.name, err)
			}
			if _, err := decodeAll(c.data); !errors.Is(err, c.want) {
				t.Errorf("%s: decode: %v", c.name, err)
			}
		})
	}
}

func TestCompressionBombInsideAFrameIsCorrupt(t *testing.T) {
	bomb := deflate(make([]byte, 64<<20))
	data := chunkList{
		{kind: "IHDR", body: ihdr(4, 4, 8, 6, 0)},
		{kind: "acTL", body: animationControl(1, 0)},
		{kind: "fcTL", body: frameControl(0, frameSpec{num: 1, den: 10}, 4, 4)},
		{kind: "IDAT", body: bomb},
		{kind: "IEND"},
	}.bytes()
	testimg.Within(t, 20*time.Second, func() {
		if _, err := decodeAll(data); !errors.Is(err, codec.ErrCorrupt) {
			t.Errorf("%v", err)
		}
	})
}

func TestLimits(t *testing.T) {
	data := animation{frames: []frameSpec{canvasFrame(red), canvasFrame(green), canvasFrame(blue)}}.bytes(t)
	ctx := context.Background()
	reader := bytes.NewReader(data)
	lim := func(change func(*codec.Limits)) codec.Limits {
		l := codec.DefaultLimits()
		change(&l)
		return l
	}
	cases := []struct {
		name     string
		limits   codec.Limits
		probeErr error
		firstErr error
		allErr   error
	}{
		{"defaults", codec.DefaultLimits(), nil, nil, nil},
		{"file size", lim(func(l *codec.Limits) { l.MaxFileBytes = 10 }), codec.ErrTooLarge, codec.ErrTooLarge, codec.ErrTooLarge},
		{"canvas pixels", lim(func(l *codec.Limits) { l.MaxPixels = 15 }), codec.ErrTooLarge, codec.ErrTooLarge, codec.ErrTooLarge},
		{"frame count", lim(func(l *codec.Limits) { l.MaxFrames = 2 }), codec.ErrTooLarge, codec.ErrTooLarge, codec.ErrTooLarge},
		{"frame count at the limit", lim(func(l *codec.Limits) { l.MaxFrames = 3 }), nil, nil, nil},
		{"total frame pixels", lim(func(l *codec.Limits) { l.MaxFramePixels = 40 }), nil, nil, codec.ErrTooLarge},
		{"total frame pixels at the limit", lim(func(l *codec.Limits) { l.MaxFramePixels = 48 }), nil, nil, nil},
		{"one frame does not fit", lim(func(l *codec.Limits) { l.MaxFramePixels = 15 }), nil, codec.ErrTooLarge, codec.ErrTooLarge},
	}
	for _, c := range cases {
		_, err := Decoder{}.Probe(ctx, reader, int64(len(data)), c.limits)
		if !errors.Is(err, c.probeErr) {
			t.Errorf("%s: probe: %v", c.name, err)
		}
		_, err = Decoder{}.Decode(ctx, reader, int64(len(data)), codec.Options{Limits: c.limits, Frames: codec.FirstFrame})
		if !errors.Is(err, c.firstErr) {
			t.Errorf("%s: first frame: %v", c.name, err)
		}
		_, err = Decoder{}.Decode(ctx, reader, int64(len(data)), codec.Options{Limits: c.limits, Frames: codec.AllFrames})
		if !errors.Is(err, c.allErr) {
			t.Errorf("%s: all frames: %v", c.name, err)
		}
	}
}

type cancelingReader struct {
	r       *bytes.Reader
	after   int
	reads   int
	cancel  context.CancelFunc
	counter *int
}

func (c *cancelingReader) ReadAt(p []byte, off int64) (int, error) {
	c.reads++
	if c.counter != nil {
		*c.counter = c.reads
	}
	if c.after > 0 && c.reads >= c.after {
		c.cancel()
	}
	return c.r.ReadAt(p, off)
}

func TestCancellationAtEveryReadStopsTheDecode(t *testing.T) {
	data := animation{pieces: 3, frames: []frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, 0, 0), canvasFrame(blue), canvasFrame(red)}}.bytes(t)
	total := 0
	ctx, cancel := context.WithCancel(context.Background())
	counting := &cancelingReader{r: bytes.NewReader(data), cancel: cancel, counter: &total}
	if _, err := (Decoder{}).Decode(ctx, counting, int64(len(data)), codec.Options{Frames: codec.AllFrames}); err != nil {
		t.Fatal(err)
	}
	cancel()
	if total < 5 {
		t.Fatalf("only %d reads", total)
	}
	stopped := 0
	for after := 1; after <= total; after++ {
		ctx, cancel := context.WithCancel(context.Background())
		reader := &cancelingReader{r: bytes.NewReader(data), after: after, cancel: cancel}
		_, err := Decoder{}.Decode(ctx, reader, int64(len(data)), codec.Options{Frames: codec.AllFrames})
		cancel()
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel at read %d: %v", after, err)
			}
			stopped++
		}
	}
	if stopped == 0 {
		t.Fatal("cancellation never took effect")
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := probe2(ctx, data); !errors.Is(err, context.Canceled) {
		t.Fatalf("probe: %v", err)
	}
	if _, err := (Decoder{}).Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("decode: %v", err)
	}
}

func probe2(ctx context.Context, data []byte) (codec.Info, error) {
	return Decoder{}.Probe(ctx, bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
}

func TestEveryTruncatedPrefix(t *testing.T) {
	data := animation{pieces: 2, frames: []frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, 0, 1)}, before: []rawChunk{{kind: "eXIf", body: testimg.OrientationTIFF(false, 3)}}}.bytes(t)
	for n := 0; n < len(data); n++ {
		if _, err := probe(data[:n]); err == nil {
			t.Fatalf("prefix of %d/%d bytes probed without error", n, len(data))
		} else {
			testimg.RequireCodecError(t, err)
		}
		if _, err := decodeAll(data[:n]); err == nil {
			t.Fatalf("prefix of %d/%d bytes decoded without error", n, len(data))
		} else {
			testimg.RequireCodecError(t, err)
		}
		(Decoder{}).Sniff(data[:n])
	}
	if _, err := decodeAll(data); err != nil {
		t.Fatal(err)
	}
}

func withFlood(list chunkList, count int) []byte {
	at := list.index("IDAT", 0)
	head := list[:at].bytes()
	tail := list[at:].bytes()[len(signature):]
	flood := bytes.Repeat(chunk("tEXt", nil), count)
	return append(append(head, flood...), tail...)
}

func TestManyTinyChunksFinishQuickly(t *testing.T) {
	list := animation{frames: []frameSpec{canvasFrame(red), canvasFrame(green)}}.chunks(t)
	data := withFlood(list, 400_000)
	testimg.Within(t, 20*time.Second, func() {
		if _, err := probe(data); err != nil {
			t.Errorf("probe: %v", err)
		}
		img, err := decodeAll(data)
		if err != nil || len(img.Frames) != 2 {
			t.Errorf("decode: %v", err)
		}
	})
}

func TestChunkCountIsBounded(t *testing.T) {
	list := animation{frames: []frameSpec{canvasFrame(red), canvasFrame(green)}}.chunks(t)
	data := withFlood(list, maxChunks)
	testimg.Within(t, 60*time.Second, func() {
		if _, err := probe(data); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("probe: %v", err)
		}
	})
}

func TestCorruptedBytesNeverPanic(t *testing.T) {
	data := animation{pieces: 2, frames: []frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, 2, 1), rect(blue, 3, 3, 0, 0, 1, 0)}, before: []rawChunk{{kind: "eXIf", body: testimg.OrientationTIFF(true, 6)}}}.bytes(t)
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 600; i++ {
		mutated := append([]byte(nil), data...)
		for k := 0; k < 1+rng.Intn(4); k++ {
			mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
		}
		if _, err := decodeAll(mutated); err != nil {
			testimg.RequireCodecError(t, err)
		}
		if _, err := probe(mutated); err != nil {
			testimg.RequireCodecError(t, err)
		}
	}
}

func FuzzSniff(f *testing.F) {
	f.Add(animation{frames: []frameSpec{canvasFrame(red), canvasFrame(green)}}.bytes(f)[:80])
	f.Add(encodeStd(f, testimg.Noise(4, 4, 1, true))[:60])
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Fuzz(func(t *testing.T, head []byte) {
		Decoder{}.Sniff(head)
	})
}

func fuzzSeeds(f *testing.F) {
	two := []frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, 2, 1)}
	f.Add(animation{plays: 2, frames: two, pieces: 2}.bytes(f))
	f.Add(animation{frames: two, defaultPNG: rgbaPNG(solid(4, 4, blue))}.bytes(f))
	f.Add(animation{frames: []frameSpec{{png: interlacedRGBAPNG(randomNRGBA(9, 9, 1)), num: 1, den: 10}}}.bytes(f))
	f.Add(animation{frames: []frameSpec{{png: encodeStd(f, solid(5, 5, red)), num: 1, den: 1000}, {png: encodeStd(f, solid(5, 5, blue)), x: 0, y: 0, dispose: 1}}}.bytes(f))
	f.Add(headerOnly(1<<30, 1<<30, animationControl(1, 0)))
	f.Add(headerOnly(4, 4, animationControl(0xffffffff, 0)))
	f.Add(encodeStd(f, testimg.Noise(5, 4, 9, true)))
}

func FuzzProbe(f *testing.F) {
	fuzzSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), testimg.SmallLimits())
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
	})
}

func FuzzDecode(f *testing.F) {
	fuzzSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := testimg.SmallLimits()
		ctx := context.Background()
		Decoder{}.Sniff(data)
		img, err := Decoder{}.Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim, Frames: codec.AllFrames, Deep: len(data)&1 == 1})
		if err == nil && (img == nil || len(img.Frames) == 0 || img.Frames[0].Pix == nil && img.Frames[0].Deep == nil || len(img.Frames) != img.Info.Frames) {
			t.Fatal("success without pixels")
		}
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
	})
}
