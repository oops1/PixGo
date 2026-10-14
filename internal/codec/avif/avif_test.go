package avif

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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	gavif "github.com/gen2brain/gav1d/avif"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
	"github.com/oops1/PixGo/internal/imaging"
	"github.com/oops1/PixGo/internal/isobmff"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
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

func encode(t testing.TB, img image.Image, opts gavif.EncodeOptions) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gavif.Encode(&buf, img, opts); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func smoothImage(width, height int, alpha bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			a := uint8(255)
			if alpha {
				a = uint8(40 + 200*x/width)
			}
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(40 + 160*x/width), G: uint8(60 + 140*y/height), B: uint8(200 - 100*x/width), A: a})
		}
	}
	return img
}

func meanAbsDelta(a, b *image.NRGBA, channels int) float64 {
	var total, count float64
	for i := 0; i < len(a.Pix); i += 4 {
		for c := 0; c < channels; c++ {
			d := float64(a.Pix[i+c]) - float64(b.Pix[i+c])
			if d < 0 {
				d = -d
			}
			total += d
			count++
		}
	}
	return total / count
}

func TestSniffRecognisesAVIFBrandsAndNothingElse(t *testing.T) {
	d := Decoder{}
	withItem := func(major string, compatible []string, kind string) []byte {
		return isotest.File{Major: major, Compatible: compatible, Primary: 1, Items: []isotest.Item{{ID: 1, Type: kind, Data: []byte{1}}}}.Bytes()
	}
	tests := []struct {
		name string
		head []byte
		want bool
	}{
		{"encoded file", readTestdata(t, "yuv420-8.avif")[:512], true},
		{"sequence", isotest.File{Major: "avis", Compatible: []string{"avif", "msf1"}}.Bytes(), true},
		{"generic major with avif", isotest.File{Major: "mif1", Compatible: []string{"miaf", "avif"}}.Bytes(), true},
		{"generic major with AV1 profile brand", isotest.File{Major: "mif1", Compatible: []string{"MA1A"}}.Bytes(), true},
		{"generic brands, av01 item", withItem("mif1", []string{"miaf"}, "av01"), true},
		{"generic brands, nothing to tell them apart", isotest.File{Major: "mif1", Compatible: []string{"miaf"}}.Bytes(), true},
		{"heic file", isotest.File{Major: "heic", Compatible: []string{"mif1", "heic"}}.Bytes(), false},
		{"generic brands, hvc1 item", withItem("mif1", []string{"miaf"}, "hvc1"), false},
		{"mp4 video", isotest.File{Major: "isom", Compatible: []string{"mp42"}}.Bytes(), false},
		{"jxl container", []byte("\x00\x00\x00\x0cJXL \x0d\x0a\x87\x0a\x00\x00\x00\x14ftypjxl \x00\x00\x00\x00jxl "), false},
		{"png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), false},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := d.Sniff(tt.head); got != tt.want {
				t.Fatalf("Sniff = %v", got)
			}
		})
	}
	if d.ID() != codec.AVIF || d.Name() != "AVIF" || len(d.Extensions()) != 1 || d.Extensions()[0] != ".avif" {
		t.Fatal("identity")
	}
}

func TestEncodedPicturesRoundTripAtEverySize(t *testing.T) {
	sizes := [][2]int{{1, 1}, {2, 2}, {7, 5}, {33, 21}, {64, 64}, {161, 121}}
	for _, size := range sizes {
		for _, alpha := range []bool{false, true} {
			src := smoothImage(size[0], size[1], alpha)
			data := encode(t, src, gavif.EncodeOptions{Quality: 95, Speed: 10})
			info, err := probe(data, codec.DefaultLimits())
			if err != nil {
				t.Fatalf("%v alpha=%v: %v", size, alpha, err)
			}
			wantDepth := 24
			if alpha {
				wantDepth = 32
			}
			if info.Format != codec.AVIF || info.FormatName != "AVIF" || info.Width != size[0] || info.Height != size[1] || info.Frames != 1 || info.Animated || info.HasAlpha != alpha || info.Orientation != codec.OrientNormal || info.BitDepth != wantDepth {
				t.Fatalf("%v alpha=%v: %+v", size, alpha, info)
			}
			img, err := decodeAll(data)
			if err != nil {
				t.Fatalf("%v alpha=%v: %v", size, alpha, err)
			}
			got := img.First()
			if got.Bounds() != src.Bounds() || len(img.Frames) != 1 || img.Info != info || img.Scale != 1 {
				t.Fatalf("%v: %v %+v", size, got.Bounds(), img.Info)
			}
			if size[0] >= 33 {
				if delta := meanAbsDelta(got, src, 3); delta > 4 {
					t.Fatalf("%v alpha=%v: colour error %.2f", size, alpha, delta)
				}
				if alpha {
					if delta := meanAbsDelta(got, src, 4) - meanAbsDelta(got, src, 3)*0.75; delta > 5 {
						t.Fatalf("%v: alpha error %.2f", size, delta)
					}
				}
			}
		}
	}
}

type corpusCase struct {
	name        string
	width       int
	height      int
	alpha       bool
	bitDepth    int
	orientation codec.Orientation
	frames      int
}

var corpus = []corpusCase{
	{"yuv420-8.avif", 160, 120, false, 24, codec.OrientNormal, 1},
	{"yuv420-10.avif", 160, 120, false, 30, codec.OrientNormal, 1},
	{"yuv420-odd.avif", 161, 121, false, 24, codec.OrientNormal, 1},
	{"yuv422-8.avif", 160, 120, false, 24, codec.OrientNormal, 1},
	{"yuv422-12.avif", 160, 120, false, 36, codec.OrientNormal, 1},
	{"yuv444-8.avif", 160, 120, false, 24, codec.OrientNormal, 1},
	{"yuv444-10.avif", 160, 120, false, 30, codec.OrientNormal, 1},
	{"mono-8.avif", 160, 120, false, 8, codec.OrientNormal, 1},
	{"mono-12.avif", 160, 120, false, 12, codec.OrientNormal, 1},
	{"alpha-420.avif", 160, 120, true, 32, codec.OrientNormal, 1},
	{"alpha-444.avif", 160, 120, true, 32, codec.OrientNormal, 1},
	{"bt709-full.avif", 160, 120, true, 32, codec.OrientNormal, 1},
	{"bt2020-422.avif", 160, 120, true, 32, codec.OrientNormal, 1},
	{"unspec-420.avif", 160, 120, true, 32, codec.OrientNormal, 1},
	{"identity-444.avif", 160, 120, true, 32, codec.OrientNormal, 1},
	{"grid-2x2.avif", 320, 240, false, 24, codec.OrientNormal, 1},
	{"irot1.avif", 160, 120, true, 32, codec.OrientRotate270, 1},
	{"irot2.avif", 160, 120, true, 32, codec.OrientRotate180, 1},
	{"imir0.avif", 160, 120, false, 24, codec.OrientFlipV, 1},
	{"irot3-imir1.avif", 160, 120, false, 24, codec.OrientTranspose, 1},
	{"clap-10.avif", 96, 64, false, 30, codec.OrientNormal, 1},
	{"clap-crop.avif", 96, 64, false, 24, codec.OrientNormal, 1},
	{"clap-irot.avif", 80, 64, true, 32, codec.OrientRotate270, 1},
	{"anim.avif", 96, 64, false, 24, codec.OrientNormal, 5},
}

func TestCorpusIsProbedHonestly(t *testing.T) {
	for _, tt := range corpus {
		t.Run(tt.name, func(t *testing.T) {
			info, err := probe(readTestdata(t, tt.name), codec.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			want := codec.Info{Format: codec.AVIF, FormatName: "AVIF", Width: tt.width, Height: tt.height, Frames: tt.frames, Animated: tt.frames > 1, HasAlpha: tt.alpha, Orientation: tt.orientation, BitDepth: tt.bitDepth}
			if info != want {
				t.Fatalf("%+v, want %+v", info, want)
			}
		})
	}
}

func eightBit(img image.Image) *image.NRGBA {
	switch m := img.(type) {
	case *image.NRGBA64:
		out := image.NewNRGBA(m.Bounds())
		for y := 0; y < m.Rect.Dy(); y++ {
			for x := 0; x < m.Rect.Dx(); x++ {
				c := m.NRGBA64At(x, y)
				out.SetNRGBA(x, y, color.NRGBA{R: uint8(c.R >> 8), G: uint8(c.G >> 8), B: uint8(c.B >> 8), A: uint8(c.A >> 8)})
			}
		}
		return out
	case *image.NRGBA:
		return m
	}
	return convert.NRGBA(img)
}

func TestCorpusDecodesLikeTheLibraryAfterTheOrientationIsApplied(t *testing.T) {
	for _, tt := range corpus {
		t.Run(tt.name, func(t *testing.T) {
			data := readTestdata(t, tt.name)
			got, err := decodeAll(data)
			if err != nil {
				t.Fatal(err)
			}
			if got.Info.Width != tt.width || got.Info.Height != tt.height || got.First().Bounds().Dx() != tt.width || got.First().Bounds().Dy() != tt.height {
				t.Fatalf("decoded %v, info %+v", got.First().Bounds(), got.Info)
			}
			if len(got.Frames) != tt.frames || got.Info.Frames != tt.frames {
				t.Fatalf("%d frames", len(got.Frames))
			}
			reference, err := gavif.DecodeAll(bytes.NewReader(data), gavif.Options{AutoRotate: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(reference.Image) != len(got.Frames) {
				t.Fatalf("reference has %d frames", len(reference.Image))
			}
			for i := range got.Frames {
				want := eightBit(reference.Image[i])
				shown := imaging.Orient(got.Frames[i].Pix, got.Info.Orientation)
				if shown.Bounds() != want.Bounds() || testimg.MaxDelta(shown, want) != 0 {
					t.Fatalf("frame %d: %v vs %v, delta %d", i, shown.Bounds(), want.Bounds(), testimg.MaxDelta(shown, want))
				}
			}
		})
	}
}

func TestDecodedAlphaIsCarriedThrough(t *testing.T) {
	img, err := decodeAll(readTestdata(t, "alpha-420.avif"))
	if err != nil {
		t.Fatal(err)
	}
	lowest, highest := uint8(255), uint8(0)
	for i := 3; i < len(img.First().Pix); i += 4 {
		lowest, highest = min(lowest, img.First().Pix[i]), max(highest, img.First().Pix[i])
	}
	if lowest == highest {
		t.Fatalf("alpha is flat at %d", lowest)
	}
	opaque, err := decodeAll(readTestdata(t, "yuv420-8.avif"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 3; i < len(opaque.First().Pix); i += 4 {
		if opaque.First().Pix[i] != 255 {
			t.Fatal("opaque picture decoded with transparency")
		}
	}
}

func TestHighBitDepthIsReducedToEightBits(t *testing.T) {
	for _, name := range []string{"yuv420-10.avif", "yuv444-10.avif", "yuv422-12.avif", "mono-12.avif"} {
		data := readTestdata(t, name)
		reference, err := gavif.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		wide, ok := reference.(*image.NRGBA64)
		if !ok {
			t.Fatalf("%s: reference is %T", name, reference)
		}
		img, err := decodeAll(data)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < wide.Rect.Dy(); y += 7 {
			for x := 0; x < wide.Rect.Dx(); x += 5 {
				c := wide.NRGBA64At(x, y)
				if got := img.First().NRGBAAt(x, y); got.R != uint8(c.R>>8) || got.G != uint8(c.G>>8) || got.B != uint8(c.B>>8) {
					t.Fatalf("%s (%d,%d): %v from %v", name, x, y, got, c)
				}
			}
		}
	}
}

func TestAnimationCarriesDelaysAndLoop(t *testing.T) {
	data := readTestdata(t, "anim.avif")
	all, err := decodeAll(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Frames) != 5 || !all.Info.Animated || all.Info.Frames != 5 || all.Loop != 0 {
		t.Fatalf("%d frames, loop %d, %+v", len(all.Frames), all.Loop, all.Info)
	}
	for i, frame := range all.Frames {
		if frame.Delay != 100*time.Millisecond {
			t.Fatalf("frame %d lasts %v", i, frame.Delay)
		}
	}
	first, err := decode(data, codec.Options{Limits: codec.DefaultLimits(), Frames: codec.FirstFrame})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Frames) != 1 || first.Info.Frames != 5 || !first.Info.Animated {
		t.Fatalf("first frame only: %d frames, %+v", len(first.Frames), first.Info)
	}
	if first.First().Bounds() != all.First().Bounds() {
		t.Fatal("first frame differs in size")
	}
}

func TestFrameLimitsApplyToAnimations(t *testing.T) {
	data := readTestdata(t, "anim.avif")
	lim := codec.DefaultLimits()
	lim.MaxFrames = 3
	if _, err := decode(data, codec.Options{Limits: lim, Frames: codec.AllFrames}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("frame count: %v", err)
	}
	if _, err := decode(data, codec.Options{Limits: lim, Frames: codec.FirstFrame}); err != nil {
		t.Fatalf("first frame must not need the frame budget: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxFramePixels = 96 * 64 * 4
	if _, err := decode(data, codec.Options{Limits: lim, Frames: codec.AllFrames}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("frame pixels: %v", err)
	}
}

func TestLimitsAreCheckedBeforeDecoding(t *testing.T) {
	data := readTestdata(t, "yuv420-8.avif")
	tests := []struct {
		name string
		lim  codec.Limits
	}{
		{"pixels", codec.Limits{MaxPixels: 160*120 - 1}},
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
	if _, err := probe(data, codec.Limits{MaxPixels: 160 * 120}); err != nil {
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

func av1File(width, height uint32, extra ...isotest.Prop) isotest.File {
	props := append([]isotest.Prop{isotest.Ispe(width, height), isotest.AV1C(false, false, false)}, extra...)
	return isotest.File{Major: "avif", Compatible: []string{"avif", "mif1", "miaf"}, Primary: 1, Items: []isotest.Item{{ID: 1, Type: "av01", Data: []byte("not really AV1"), Props: props}}}
}

func TestDecompressionBombsAreRefusedWithoutAllocatingTheirSize(t *testing.T) {
	bomb := av1File(100000, 100000).Bytes()
	gridTiles := isotest.File{Major: "avif", Compatible: []string{"mif1"}, Primary: 1000}
	var tiles []uint32
	for i := 1; i <= 64; i++ {
		tiles = append(tiles, uint32(i))
		gridTiles.Items = append(gridTiles.Items, isotest.Item{ID: uint32(i), Type: "av01", Data: []byte("tile"), Props: []isotest.Prop{isotest.Ispe(8000, 8000), isotest.AV1C(false, false, false)}})
	}
	gridTiles.Items = append(gridTiles.Items, isotest.Item{ID: 1000, Type: "grid", Data: isotest.GridDescriptor(8, 8, 64000, 64000, true), Props: []isotest.Prop{isotest.Ispe(1000, 1000)}})
	gridTiles.Refs = []isotest.Ref{{Type: "dimg", From: 1000, To: tiles}}
	hugeGrid := gridTiles
	hugeGrid.Items = append([]isotest.Item{}, gridTiles.Items...)
	for i := 0; i < 64; i++ {
		hugeGrid.Items[i].Props = []isotest.Prop{isotest.Ispe(100000, 100000), isotest.AV1C(false, false, false)}
	}
	hugeGrid.Items[64] = isotest.Item{ID: 1000, Type: "grid", Data: isotest.GridDescriptor(8, 8, 800000, 800000, true), Props: []isotest.Prop{isotest.Ispe(800000, 800000)}}
	for name, data := range map[string][]byte{"ispe": bomb, "grid tiles": gridTiles.Bytes(), "grid output": hugeGrid.Bytes()} {
		for _, run := range []func() error{
			func() error { _, err := probe(data, codec.DefaultLimits()); return err },
			func() error { _, err := decode(data, codec.Options{}); return err },
		} {
			var err error
			spent := allocated(func() { err = run() })
			if !errors.Is(err, codec.ErrTooLarge) {
				t.Fatalf("%s: %v", name, err)
			}
			if spent > 4<<20 {
				t.Fatalf("%s: refusing allocated %d bytes", name, spent)
			}
		}
	}
}

func TestEveryTruncationOfAStillFailsCleanly(t *testing.T) {
	for _, name := range []string{"yuv420-8.avif", "alpha-420.avif", "grid-2x2.avif", "irot3-imir1.avif"} {
		data := readTestdata(t, name)
		file, err := isobmff.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		var needed uint64
		for _, item := range file.Items {
			for _, extent := range item.Extents {
				needed = max(needed, item.BaseOffset+extent.Offset+extent.Length)
			}
		}
		for cut := 0; cut < len(data); cut++ {
			part := data[:cut]
			_, probeErr := probe(part, codec.DefaultLimits())
			_, decodeErr := decodeAll(part)
			for _, err := range []error{probeErr, decodeErr} {
				if err != nil && !errors.Is(err, codec.ErrCorrupt) && !errors.Is(err, codec.ErrUnsupported) && !errors.Is(err, codec.ErrTooLarge) {
					t.Fatalf("%s cut %d: unclassified error %v", name, cut, err)
				}
			}
			if uint64(cut) < needed && decodeErr == nil {
				t.Fatalf("%s cut at %d of %d decoded", name, cut, len(data))
			}
		}
	}
}

func TestTruncatedAnimationStillShowsItsFirstFrame(t *testing.T) {
	data := readTestdata(t, "anim.avif")
	for cut := 0; cut < len(data); cut++ {
		img, err := decode(data[:cut], codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames})
		if err == nil {
			if len(img.Frames) == 0 {
				t.Fatalf("cut %d: no frames", cut)
			}
			continue
		}
		testimg.RequireCodecError(t, err)
	}
}

func TestDamagedFilesNeverPanic(t *testing.T) {
	random := rand.New(rand.NewSource(7))
	for _, name := range []string{"yuv420-8.avif", "alpha-444.avif", "grid-2x2.avif", "clap-irot.avif", "anim.avif", "mono-12.avif"} {
		data := readTestdata(t, name)
		positions := make([]int, 0, 400)
		for i := 0; i < len(data) && i < 260; i++ {
			positions = append(positions, i)
		}
		for i := 0; i < 100; i++ {
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
	head := isotest.File{Major: "avif", Compatible: []string{"avif", "mif1"}, NoMeta: true}.Bytes()
	random := rand.New(rand.NewSource(3))
	for _, length := range []int{0, 1, 7, 8, 40, 500, 5000} {
		junk := make([]byte, length)
		random.Read(junk)
		data := append(append([]byte{}, head...), junk...)
		if _, err := probe(data, codec.DefaultLimits()); err != nil {
			testimg.RequireCodecError(t, err)
		}
		if _, err := decode(data, codec.Options{}); err == nil {
			t.Fatalf("%d bytes of junk decoded", length)
		} else {
			testimg.RequireCodecError(t, err)
		}
	}
}

func TestFilesOfOtherFormatsAreRefusedWithAReason(t *testing.T) {
	heic, err := os.ReadFile(filepath.Join("..", "heic", "testdata", "basic.heic"))
	if err != nil {
		t.Fatal(err)
	}
	hevcItem := isotest.File{Major: "avif", Compatible: []string{"avif", "mif1"}, Primary: 1, Items: []isotest.Item{{ID: 1, Type: "hvc1", Data: []byte("x"), Props: []isotest.Prop{isotest.Ispe(8, 8), isotest.HVCC(1, 8)}}}}.Bytes()
	jpegItem := isotest.File{Major: "avif", Compatible: []string{"avif"}, Primary: 1, Items: []isotest.Item{{ID: 1, Type: "jpeg", Data: []byte("x"), Props: []isotest.Prop{isotest.Ispe(8, 8)}}}}.Bytes()
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"HEIC file", heic, codec.ErrUnsupported},
		{"HEIC brand without a generic brand", isotest.File{Major: "heic", Compatible: []string{"heic"}}.Bytes(), codec.ErrUnsupported},
		{"HEVC item in an AVIF container", hevcItem, codec.ErrUnsupported},
		{"JPEG item", jpegItem, codec.ErrCorrupt},
		{"MP4 brand", isotest.File{Major: "isom", Compatible: []string{"mp42"}}.Bytes(), codec.ErrCorrupt},
		{"no meta box", isotest.File{Major: "avif", NoMeta: true}.Bytes(), codec.ErrCorrupt},
		{"sequence without a still image", isotest.File{Major: "avis", Compatible: []string{"avif", "msf1"}, NoMeta: true, Tracks: []isotest.Track{{Handler: "pict", Samples: 4, Width: 8, Height: 8}}}.Bytes(), codec.ErrUnsupported},
		{"minimized box", isotest.File{Major: "avif", NoMeta: true, Extra: [][]byte{isotest.Box("mini", make([]byte, 8))}}.Bytes(), codec.ErrCorrupt},
		{"essential property of an unknown kind", av1File(8, 8, isotest.Unknown("rloc", true)).Bytes(), codec.ErrUnsupported},
		{"clean aperture outside the picture", av1File(8, 8, isotest.Clap(20, 1, 8, 1, 0, 1, 0, 1)).Bytes(), codec.ErrCorrupt},
		{"picture without a size", isotest.File{Major: "avif", Primary: 1, Items: []isotest.Item{{ID: 1, Type: "av01", Data: []byte("x"), Props: []isotest.Prop{isotest.AV1C(false, false, false)}}}}.Bytes(), codec.ErrCorrupt},
		{"nothing to show", isotest.File{Major: "avif", Compatible: []string{"avif"}, Primary: 5}.Bytes(), codec.ErrCorrupt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, probeErr := probe(tt.data, codec.DefaultLimits())
			_, decodeErr := decode(tt.data, codec.Options{})
			for _, err := range []error{probeErr, decodeErr} {
				if !errors.Is(err, tt.want) {
					t.Fatalf("error %v, want %v", err, tt.want)
				}
			}
		})
	}
}

func TestPrimaryItemFallsBackToTheFirstImageItem(t *testing.T) {
	data := readTestdata(t, "yuv420-8.avif")
	rebuilt, err := isotest.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, primary := range []uint32{0, 9} {
		rebuilt.Primary = primary
		img, err := decode(rebuilt.Bytes(), codec.Options{})
		if err != nil || img.Info.Width != 160 || img.Info.Height != 120 {
			t.Fatalf("primary %d: %v", primary, err)
		}
		reference, err := decode(data, codec.Options{})
		if err != nil || !testimg.Identical(img.First(), reference.First()) {
			t.Fatalf("primary %d: rebuilt file decodes differently: %v", primary, err)
		}
	}
}

func TestContainerPropertiesDriveOrientationAndCropping(t *testing.T) {
	data := readTestdata(t, "yuv420-8.avif")
	plain, err := decode(data, codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := isotest.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for rotation := byte(0); rotation < 4; rotation++ {
		for _, mirror := range []int{-1, 0, 1} {
			file := rebuilt
			file.Items = append([]isotest.Item(nil), rebuilt.Items...)
			file.Items[0].Props = append(append([]isotest.Prop(nil), rebuilt.Items[0].Props...), isotest.Clap(100, 1, 80, 1, 0, 1, 0, 1), isotest.Irot(rotation))
			if mirror >= 0 {
				file.Items[0].Props = append(file.Items[0].Props, isotest.Imir(byte(mirror)))
			}
			built := file.Bytes()
			got, err := decode(built, codec.Options{})
			if err != nil {
				t.Fatalf("rotation %d mirror %d: %v", rotation, mirror, err)
			}
			if got.Info.Width != 100 || got.Info.Height != 80 || got.First().Bounds().Dx() != 100 {
				t.Fatalf("stored size %dx%d", got.Info.Width, got.Info.Height)
			}
			reference, err := gavif.Decode(bytes.NewReader(built), gavif.Options{AutoRotate: true})
			if err != nil {
				t.Fatal(err)
			}
			shown := imaging.Orient(got.First(), got.Info.Orientation)
			if want := eightBit(reference); shown.Bounds() != want.Bounds() || testimg.MaxDelta(shown, want) != 0 {
				t.Fatalf("rotation %d mirror %d: differs from the library, delta %d", rotation, mirror, testimg.MaxDelta(shown, want))
			}
			if got.Info.Orientation.Transposes() != (rotation%2 == 1) {
				t.Fatalf("rotation %d: orientation %d", rotation, got.Info.Orientation)
			}
		}
	}
	if plain.Info.Orientation != codec.OrientNormal {
		t.Fatal("plain file reports an orientation")
	}
}

func TestExifOrientationInsideTheFileIsNotAppliedTwice(t *testing.T) {
	data := readTestdata(t, "irot1.avif")
	withoutExif, err := decode(data, codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := isotest.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	exif := append([]byte{0, 0, 0, 0}, testimg.OrientationTIFF(false, 6)...)
	rebuilt.Items = append(rebuilt.Items, isotest.Item{ID: 50, Type: "Exif", Data: exif})
	rebuilt.Refs = append(rebuilt.Refs, isotest.Ref{Type: "cdsc", From: 50, To: []uint32{rebuilt.Primary}})
	withExif, err := decode(rebuilt.Bytes(), codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if withExif.Info.Orientation != withoutExif.Info.Orientation || withExif.Info.Orientation != codec.OrientRotate270 {
		t.Fatalf("orientation %d, want the container rotation %d", withExif.Info.Orientation, withoutExif.Info.Orientation)
	}
	if !testimg.Identical(withExif.First(), withoutExif.First()) {
		t.Fatal("pixels changed")
	}
}

func TestCanceledContextStopsEveryStage(t *testing.T) {
	data := readTestdata(t, "grid-2x2.avif")
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
	data := readTestdata(t, "grid-2x2.avif")
	for after := 1; after < 12; after++ {
		ctx, cancel := context.WithCancel(context.Background())
		reader := &cancelingReader{data: data, cancel: cancel, after: after}
		_, err := (Decoder{}).Decode(ctx, reader, int64(len(data)), codec.Options{})
		cancel()
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel at read %d: %v", after, err)
		}
	}
}

func TestDecodersAreSafeToShareBetweenGoroutines(t *testing.T) {
	data := readTestdata(t, "irot3-imir1.avif")
	want, err := decode(data, codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := decode(data, codec.Options{})
			if err != nil || !testimg.Identical(got.First(), want.First()) {
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

func TestReadersThatDeliverEverythingAtTheEndStillWork(t *testing.T) {
	data := readTestdata(t, "yuv420-8.avif")
	want, err := decode(data, codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{len(data), len(data) + 100} {
		padded := append(append([]byte{}, data...), make([]byte, size-len(data))...)
		got, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(padded), int64(len(data)), codec.Options{})
		if err != nil || !testimg.Identical(got.First(), want.First()) {
			t.Fatalf("size %d: %v", size, err)
		}
	}
}

func TestProbeAgreesWithTheLibraryConfiguration(t *testing.T) {
	for _, tt := range corpus {
		if strings.HasPrefix(tt.name, "clap-") {
			continue
		}
		data := readTestdata(t, tt.name)
		config, err := gavif.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		info, err := probe(data, codec.DefaultLimits())
		if err != nil || info.Width != config.Width || info.Height != config.Height {
			t.Fatalf("%s: probe %+v, library %dx%d", tt.name, info, config.Width, config.Height)
		}
		wide := slices.Contains([]int{10, 12, 30, 36, 40, 48}, info.BitDepth)
		if (config.ColorModel == color.NRGBA64Model) != wide {
			t.Fatalf("%s: library colour model %v, depth %d", tt.name, config.ColorModel, info.BitDepth)
		}
	}
}
