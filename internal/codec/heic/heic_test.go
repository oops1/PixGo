package heic

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

	gheic "github.com/gen2brain/h265/heic"

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

func encode(t testing.TB, img image.Image, opts gheic.EncodeOptions) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gheic.Encode(&buf, img, opts); err != nil {
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

func TestSniffRecognisesHEICBrandsAndNothingElse(t *testing.T) {
	d := Decoder{}
	withItem := func(major string, compatible []string, kind string) []byte {
		return isotest.File{Major: major, Compatible: compatible, Primary: 1, Items: []isotest.Item{{ID: 1, Type: kind, Data: []byte{1}}}}.Bytes()
	}
	tests := []struct {
		name string
		head []byte
		want bool
	}{
		{"encoded file", readTestdata(t, "basic.heic")[:512], true},
		{"heix brand", readTestdata(t, "chroma422.heic")[:512], true},
		{"hevc sequence", isotest.File{Major: "hevc", Compatible: []string{"msf1"}}.Bytes(), true},
		{"generic major with heic compatible", isotest.File{Major: "mif1", Compatible: []string{"heic", "miaf"}}.Bytes(), true},
		{"generic brands, hvc1 item", withItem("mif1", []string{"miaf"}, "hvc1"), true},
		{"generic brands, nothing to tell them apart", isotest.File{Major: "mif1", Compatible: []string{"miaf"}}.Bytes(), true},
		{"avif file", isotest.File{Major: "avif", Compatible: []string{"mif1", "miaf"}}.Bytes(), false},
		{"generic major with avif compatible", isotest.File{Major: "mif1", Compatible: []string{"avif", "miaf"}}.Bytes(), false},
		{"generic brands, av01 item", withItem("mif1", []string{"miaf"}, "av01"), false},
		{"mp4 video", isotest.File{Major: "isom", Compatible: []string{"mp42"}}.Bytes(), false},
		{"quicktime", isotest.File{Major: "qt  "}.Bytes(), false},
		{"jpeg", []byte{0xff, 0xd8, 0xff, 0xe0}, false},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := d.Sniff(tt.head); got != tt.want {
				t.Fatalf("Sniff = %v", got)
			}
		})
	}
	extensions := d.Extensions()
	if d.ID() != codec.HEIC || d.Name() != "HEIC" || len(extensions) != 3 || extensions[0] != ".heic" || extensions[1] != ".heif" || extensions[2] != ".hif" {
		t.Fatal("identity")
	}
}

func TestEncodedPicturesRoundTripAtEverySize(t *testing.T) {
	sizes := [][2]int{{8, 8}, {7, 5}, {33, 21}, {64, 64}, {161, 121}}
	for _, size := range sizes {
		for _, alpha := range []bool{false, true} {
			src := smoothImage(size[0], size[1], alpha)
			data := encode(t, src, gheic.EncodeOptions{Quality: 95})
			info, err := probe(data, codec.DefaultLimits())
			if err != nil {
				t.Fatalf("%v alpha=%v: %v", size, alpha, err)
			}
			wantDepth := 24
			if alpha {
				wantDepth = 32
			}
			if info.Format != codec.HEIC || info.FormatName != "HEIC" || info.Width != size[0] || info.Height != size[1] || info.Frames != 1 || info.Animated || info.HasAlpha != alpha || info.Orientation != codec.OrientNormal || info.BitDepth != wantDepth {
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
			}
		}
	}
}

func TestChromaSamplingsAndBitDepthsAreProbedAndDecoded(t *testing.T) {
	src := smoothImage(48, 40, false)
	chromas := map[string]gheic.ChromaFormat{"4:2:0": gheic.Chroma420, "4:2:2": gheic.Chroma422, "4:4:4": gheic.Chroma444, "gray": gheic.ChromaGray}
	for name, chroma := range chromas {
		for _, depth := range []int{8, 10, 12} {
			data := encode(t, src, gheic.EncodeOptions{Quality: 90, Chroma: chroma, BitDepth: depth})
			info, err := probe(data, codec.DefaultLimits())
			if err != nil {
				t.Fatalf("%s %d bit: %v", name, depth, err)
			}
			channels := 3
			if chroma == gheic.ChromaGray {
				channels = 1
			}
			if info.BitDepth != channels*depth || info.Width != 48 || info.Height != 40 {
				t.Fatalf("%s %d bit: %+v", name, depth, info)
			}
			img, err := decodeAll(data)
			if err != nil {
				t.Fatalf("%s %d bit: %v", name, depth, err)
			}
			if chroma != gheic.ChromaGray {
				if delta := meanAbsDelta(img.First(), src, 3); delta > 4 {
					t.Fatalf("%s %d bit: colour error %.2f", name, depth, delta)
				}
			}
		}
	}
}

func TestLosslessEncodingIsReproduced(t *testing.T) {
	src := smoothImage(40, 30, false)
	data := encode(t, src, gheic.EncodeOptions{Lossless: true, Chroma: gheic.Chroma444})
	img, err := decodeAll(data)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := gheic.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if !testimg.Identical(img.First(), eightBit(reference)) {
		t.Fatal("differs from the library")
	}
}

type corpusCase struct {
	name     string
	width    int
	height   int
	alpha    bool
	bitDepth int
}

var corpus = []corpusCase{
	{"basic.heic", 320, 240, false, 24},
	{"alpha.heic", 64, 64, true, 32},
	{"chroma422.heic", 320, 240, false, 24},
	{"lossless.heic", 320, 240, false, 24},
}

func TestCorpusIsProbedAndDecodedLikeTheLibrary(t *testing.T) {
	for _, tt := range corpus {
		t.Run(tt.name, func(t *testing.T) {
			data := readTestdata(t, tt.name)
			info, err := probe(data, codec.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			want := codec.Info{Format: codec.HEIC, FormatName: "HEIC", Width: tt.width, Height: tt.height, Frames: 1, HasAlpha: tt.alpha, Orientation: codec.OrientNormal, BitDepth: tt.bitDepth}
			if info != want {
				t.Fatalf("%+v, want %+v", info, want)
			}
			got, err := decodeAll(data)
			if err != nil {
				t.Fatal(err)
			}
			reference, err := gheic.Decode(bytes.NewReader(data), gheic.Options{AutoRotate: true})
			if err != nil {
				t.Fatal(err)
			}
			if !testimg.Identical(got.First(), eightBit(reference)) {
				t.Fatal("differs from the library")
			}
		})
	}
}

func TestAlphaItemsAreCarriedThrough(t *testing.T) {
	img, err := decodeAll(readTestdata(t, "alpha.heic"))
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
}

func TestGridsOfTilesAreStitchedAndMeasured(t *testing.T) {
	src := smoothImage(150, 100, false)
	data := encode(t, src, gheic.EncodeOptions{Quality: 92, Tile: 64})
	file, err := isobmff.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if item := file.PrimaryItem(); item == nil || item.Type != "grid" {
		t.Fatalf("primary %+v", item)
	}
	info, err := probe(data, codec.DefaultLimits())
	if err != nil || info.Width != 150 || info.Height != 100 || info.BitDepth != 24 {
		t.Fatalf("%+v %v", info, err)
	}
	img, err := decodeAll(data)
	if err != nil {
		t.Fatal(err)
	}
	if img.First().Bounds() != src.Bounds() {
		t.Fatalf("%v", img.First().Bounds())
	}
	if delta := meanAbsDelta(img.First(), src, 3); delta > 4 {
		t.Fatalf("colour error %.2f", delta)
	}
	reference, err := gheic.Decode(bytes.NewReader(data))
	if err != nil || !testimg.Identical(img.First(), eightBit(reference)) {
		t.Fatalf("differs from the library: %v", err)
	}
}

func phoneLikeFile(t testing.TB, rotation byte, mirror int, exifOrientation uint16) []byte {
	t.Helper()
	src := smoothImage(150, 100, false)
	data := encode(t, src, gheic.EncodeOptions{Quality: 92, Tile: 64})
	file, err := isotest.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	grid := &file.Items[len(file.Items)-1]
	for i := range file.Items {
		if file.Items[i].Type == "grid" {
			grid = &file.Items[i]
		}
	}
	grid.Props = append(grid.Props, isotest.Irot(rotation))
	if mirror >= 0 {
		grid.Props = append(grid.Props, isotest.Imir(byte(mirror)))
	}
	if exifOrientation != 0 {
		exif := append([]byte{0, 0, 0, 0}, testimg.OrientationTIFF(true, exifOrientation)...)
		file.Items = append(file.Items, isotest.Item{ID: 900, Type: "Exif", Data: exif})
		file.Refs = append(file.Refs, isotest.Ref{Type: "cdsc", From: 900, To: []uint32{file.Primary}})
	}
	return file.Bytes()
}

func TestRotatedGridsLikeThoseOfPhonesAreOrientedOnce(t *testing.T) {
	for rotation := byte(0); rotation < 4; rotation++ {
		for _, mirror := range []int{-1, 0, 1} {
			data := phoneLikeFile(t, rotation, mirror, 6)
			got, err := decodeAll(data)
			if err != nil {
				t.Fatalf("rotation %d mirror %d: %v", rotation, mirror, err)
			}
			info, err := probe(data, codec.DefaultLimits())
			if err != nil || info != got.Info {
				t.Fatalf("probe %+v vs decode %+v: %v", info, got.Info, err)
			}
			if got.Info.Width != 150 || got.Info.Height != 100 || got.First().Bounds().Dx() != 150 {
				t.Fatalf("stored size %dx%d", got.Info.Width, got.Info.Height)
			}
			reference, err := gheic.Decode(bytes.NewReader(data), gheic.Options{AutoRotate: true})
			if err != nil {
				t.Fatal(err)
			}
			shown := imaging.Orient(got.First(), got.Info.Orientation)
			if want := eightBit(reference); shown.Bounds() != want.Bounds() || testimg.MaxDelta(shown, want) != 0 {
				t.Fatalf("rotation %d mirror %d: differs from the library, delta %d", rotation, mirror, testimg.MaxDelta(shown, want))
			}
			wantTransposed := rotation%2 == 1
			if got.Info.Orientation.Transposes() != wantTransposed {
				t.Fatalf("rotation %d: orientation %d", rotation, got.Info.Orientation)
			}
		}
	}
}

func TestExifOrientationIsIgnoredBecauseTheContainerRotates(t *testing.T) {
	without := phoneLikeFile(t, 1, -1, 0)
	with := phoneLikeFile(t, 1, -1, 8)
	a, err := decode(without, codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := decode(with, codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if a.Info != b.Info || a.Info.Orientation != codec.OrientRotate270 || !testimg.Identical(a.First(), b.First()) {
		t.Fatalf("%+v vs %+v", a.Info, b.Info)
	}
	plain := encode(t, smoothImage(40, 30, false), gheic.EncodeOptions{Exif: testimg.OrientationTIFF(false, 6)})
	info, err := probe(plain, codec.DefaultLimits())
	if err != nil || info.Orientation != codec.OrientNormal {
		t.Fatalf("an Exif tag without a container rotation must not rotate: %+v %v", info, err)
	}
}

func TestOddSizesCarryACleanApertureThatIsHonoured(t *testing.T) {
	for _, size := range [][2]int{{33, 21}, {35, 23}, {41, 19}} {
		data := encode(t, smoothImage(size[0], size[1], false), gheic.EncodeOptions{Quality: 90})
		img, err := decodeAll(data)
		if err != nil || img.First().Bounds().Dx() != size[0] || img.First().Bounds().Dy() != size[1] {
			t.Fatalf("%v: %v", size, err)
		}
	}
	data := encode(t, smoothImage(40, 30, false), gheic.EncodeOptions{Quality: 90})
	file, err := isotest.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	file.Items[0].Props = append(file.Items[0].Props, isotest.Clap(30, 1, 20, 1, 0, 1, 0, 1))
	built := file.Bytes()
	img, err := decode(built, codec.Options{})
	if err != nil || img.First().Bounds() != image.Rect(0, 0, 30, 20) || img.Info.Width != 30 || img.Info.Height != 20 {
		t.Fatalf("%v", err)
	}
	reference, err := gheic.Decode(bytes.NewReader(built), gheic.Options{AutoRotate: true})
	if err != nil || !testimg.Identical(img.First(), eightBit(reference)) {
		t.Fatalf("differs from the library: %v", err)
	}
}

func TestSequenceBoxesMakeAnAnimatedProbe(t *testing.T) {
	data := encode(t, smoothImage(24, 24, false), gheic.EncodeOptions{})
	file, err := isotest.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	file.Major = "heic"
	file.Tracks = []isotest.Track{{Handler: "pict", Samples: 6, Width: 24, Height: 24}}
	built := file.Bytes()
	info, err := probe(built, codec.DefaultLimits())
	if err != nil || info.Frames != 6 || !info.Animated {
		t.Fatalf("%+v %v", info, err)
	}
	lim := codec.DefaultLimits()
	lim.MaxFrames = 3
	if _, err := decode(built, codec.Options{Limits: lim, Frames: codec.AllFrames}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("frame budget: %v", err)
	}
	first, err := decode(built, codec.Options{Limits: lim, Frames: codec.FirstFrame})
	if err != nil || len(first.Frames) != 1 || first.Info.Frames != 6 || !first.Info.Animated {
		t.Fatalf("first frame: %v", err)
	}
	all, err := decodeAll(built)
	if err != nil || len(all.Frames) != 1 || all.Info.Animated {
		t.Fatalf("a track the library cannot decode must fall back to the still: %v", err)
	}
}

func TestLimitsAreCheckedBeforeDecoding(t *testing.T) {
	data := readTestdata(t, "basic.heic")
	tests := []struct {
		name string
		lim  codec.Limits
	}{
		{"pixels", codec.Limits{MaxPixels: 320*240 - 1}},
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
	if _, err := probe(data, codec.Limits{MaxPixels: 320 * 240}); err != nil {
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

func hevcItem(id uint32, width, height uint32) isotest.Item {
	return isotest.Item{ID: id, Type: "hvc1", Data: []byte("not really HEVC"), Props: []isotest.Prop{isotest.Ispe(width, height), isotest.HVCC(1, 8)}}
}

func TestDecompressionBombsAreRefusedWithoutAllocatingTheirSize(t *testing.T) {
	still := isotest.File{Major: "heic", Compatible: []string{"mif1", "heic"}, Primary: 1, Items: []isotest.Item{hevcItem(1, 100000, 100000)}}
	grid := isotest.File{Major: "heic", Compatible: []string{"mif1"}, Primary: 1000}
	var tiles []uint32
	for i := 1; i <= 64; i++ {
		tiles = append(tiles, uint32(i))
		grid.Items = append(grid.Items, hevcItem(uint32(i), 8000, 8000))
	}
	grid.Items = append(grid.Items, isotest.Item{ID: 1000, Type: "grid", Data: isotest.GridDescriptor(8, 8, 64000, 64000, true), Props: []isotest.Prop{isotest.Ispe(1000, 1000)}})
	grid.Refs = []isotest.Ref{{Type: "dimg", From: 1000, To: tiles}}
	for name, data := range map[string][]byte{"ispe": still.Bytes(), "grid": grid.Bytes()} {
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

func TestEveryTruncationFailsCleanly(t *testing.T) {
	phone := phoneLikeFile(t, 1, 1, 6)
	files := map[string][]byte{
		"alpha.heic":  readTestdata(t, "alpha.heic"),
		"basic.heic":  readTestdata(t, "basic.heic"),
		"phone-like":  phone,
		"tiny encode": encode(t, smoothImage(16, 16, true), gheic.EncodeOptions{}),
	}
	for name, data := range files {
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
		step := max(1, len(data)/1500)
		for cut := 0; cut < len(data); cut += step {
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

func TestDamagedFilesNeverPanic(t *testing.T) {
	random := rand.New(rand.NewSource(11))
	files := [][]byte{readTestdata(t, "alpha.heic"), readTestdata(t, "chroma422.heic"), phoneLikeFile(t, 3, 0, 6)}
	for _, data := range files {
		positions := make([]int, 0, 450)
		for i := 0; i < len(data) && i < 300; i++ {
			positions = append(positions, i)
		}
		for i := 0; i < 120; i++ {
			positions = append(positions, random.Intn(len(data)))
		}
		for _, position := range positions {
			for _, bit := range []uint{0, 4, 7} {
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
	head := isotest.File{Major: "heic", Compatible: []string{"mif1", "heic"}, NoMeta: true}.Bytes()
	random := rand.New(rand.NewSource(5))
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
	avif, err := os.ReadFile(filepath.Join("..", "avif", "testdata", "yuv420-8.avif"))
	if err != nil {
		t.Fatal(err)
	}
	av1Item := isotest.File{Major: "heic", Compatible: []string{"heic", "mif1"}, Primary: 1, Items: []isotest.Item{{ID: 1, Type: "av01", Data: []byte("x"), Props: []isotest.Prop{isotest.Ispe(8, 8), isotest.AV1C(false, false, false)}}}}.Bytes()
	overlay := isotest.File{Major: "heic", Compatible: []string{"heic"}, Primary: 1, Items: []isotest.Item{{ID: 1, Type: "iovl", Data: []byte("x"), Props: []isotest.Prop{isotest.Ispe(8, 8)}}}}.Bytes()
	unknownEssential := isotest.File{Major: "heic", Compatible: []string{"heic"}, Primary: 1, Items: []isotest.Item{{ID: 1, Type: "hvc1", Data: []byte("x"), Props: []isotest.Prop{isotest.Ispe(8, 8), isotest.HVCC(1, 8), isotest.Unknown("rloc", true)}}}}.Bytes()
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"AVIF file", avif, codec.ErrUnsupported},
		{"AV1 item in a HEIC container", av1Item, codec.ErrUnsupported},
		{"overlay item", overlay, codec.ErrUnsupported},
		{"essential property of an unknown kind", unknownEssential, codec.ErrUnsupported},
		{"sequence without a still image", isotest.File{Major: "hevc", Compatible: []string{"msf1"}, NoMeta: true, Tracks: []isotest.Track{{Handler: "pict", Samples: 3, Width: 8, Height: 8}}}.Bytes(), codec.ErrUnsupported},
		{"minimized box", isotest.File{Major: "heic", NoMeta: true, Extra: [][]byte{isotest.Box("mini", make([]byte, 8))}}.Bytes(), codec.ErrUnsupported},
		{"no primary item", isotest.File{Major: "heic", Compatible: []string{"heic"}, Primary: 5, Items: []isotest.Item{hevcItem(1, 8, 8)}}.Bytes(), codec.ErrCorrupt},
		{"no meta box", isotest.File{Major: "heic", NoMeta: true}.Bytes(), codec.ErrCorrupt},
		{"picture without a size", isotest.File{Major: "heic", Primary: 1, Items: []isotest.Item{{ID: 1, Type: "hvc1", Data: []byte("x"), Props: []isotest.Prop{isotest.HVCC(1, 8)}}}}.Bytes(), codec.ErrCorrupt},
		{"clean aperture outside the picture", isotest.File{Major: "heic", Primary: 1, Items: []isotest.Item{{ID: 1, Type: "hvc1", Data: []byte("x"), Props: []isotest.Prop{isotest.Ispe(8, 8), isotest.HVCC(1, 8), isotest.Clap(20, 1, 8, 1, 0, 1, 0, 1)}}}}.Bytes(), codec.ErrCorrupt},
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

func TestCanceledContextStopsEveryStage(t *testing.T) {
	data := readTestdata(t, "basic.heic")
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
	data := readTestdata(t, "basic.heic")
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
	data := phoneLikeFile(t, 3, 1, 6)
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

func TestParallelDecodingGivesTheSamePicture(t *testing.T) {
	data := phoneLikeFile(t, 1, 0, 6)
	serial, err := decode(data, codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, threads := range []int{-3, 0, 1, 2, 8} {
		got, err := (Decoder{Threads: threads}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{})
		if err != nil || got.Info != serial.Info || !testimg.Identical(got.First(), serial.First()) {
			t.Fatalf("%d threads: %v", threads, err)
		}
	}
}

func TestPhoneLikeFileWithThumbnailDepthMapAndMetadata(t *testing.T) {
	plain := phoneLikeFile(t, 1, -1, 0)
	file, err := isotest.Parse(plain)
	if err != nil {
		t.Fatal(err)
	}
	small, err := isotest.Parse(encode(t, smoothImage(20, 14, false), gheic.EncodeOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	thumbnail := small.Items[0]
	thumbnail.ID = 800
	depth := small.Items[0]
	depth.ID = 801
	depth.Props = append(append([]isotest.Prop(nil), depth.Props...), isotest.AuxC("urn:com:apple:photo:2018:aux:depthmap"))
	file.Items = append(file.Items,
		thumbnail,
		depth,
		isotest.Item{ID: 802, Type: "Exif", Data: append([]byte{0, 0, 0, 0}, testimg.OrientationTIFF(true, 6)...)},
		isotest.Item{ID: 803, Type: "mime", ContentType: "application/rdf+xml", Data: []byte("<x:xmpmeta/>")},
	)
	file.Refs = append(file.Refs,
		isotest.Ref{Type: "thmb", From: 800, To: []uint32{file.Primary}},
		isotest.Ref{Type: "auxl", From: 801, To: []uint32{file.Primary}},
		isotest.Ref{Type: "cdsc", From: 802, To: []uint32{file.Primary}},
		isotest.Ref{Type: "cdsc", From: 803, To: []uint32{file.Primary}},
	)
	busy := file.Bytes()
	want, err := decode(plain, codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := decode(busy, codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Info != want.Info || got.Info.HasAlpha || got.Info.Width != 150 || got.Info.Height != 100 || got.Info.Orientation != codec.OrientRotate270 {
		t.Fatalf("%+v vs %+v", got.Info, want.Info)
	}
	if !testimg.Identical(got.First(), want.First()) {
		t.Fatal("the extra items changed the picture")
	}
}

func TestProbeAgreesWithTheLibraryConfiguration(t *testing.T) {
	files := map[string][]byte{
		"alpha.heic":     readTestdata(t, "alpha.heic"),
		"basic.heic":     readTestdata(t, "basic.heic"),
		"chroma422.heic": readTestdata(t, "chroma422.heic"),
		"grid":           encode(t, smoothImage(150, 100, false), gheic.EncodeOptions{Tile: 64}),
		"10 bit":         encode(t, smoothImage(40, 30, false), gheic.EncodeOptions{BitDepth: 10}),
		"12 bit":         encode(t, smoothImage(40, 30, false), gheic.EncodeOptions{BitDepth: 12}),
	}
	for name, data := range files {
		config, err := gheic.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		info, err := probe(data, codec.DefaultLimits())
		if err != nil || info.Width != config.Width || info.Height != config.Height {
			t.Fatalf("%s: probe %+v, library %dx%d", name, info, config.Width, config.Height)
		}
		wide := info.BitDepth == 30 || info.BitDepth == 36
		if (config.ColorModel == color.NRGBA64Model) != wide {
			t.Fatalf("%s: library colour model %v, depth %d", name, config.ColorModel, info.BitDepth)
		}
	}
}
