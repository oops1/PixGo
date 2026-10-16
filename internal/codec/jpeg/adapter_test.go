package jpeg

import (
	"bytes"
	"context"
	"errors"
	"image"
	stdjpeg "image/jpeg"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func corpus(t testing.TB) (map[string][]byte, []string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "jpegdec", "testdata", "*.jpeg"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no corpus: %v", err)
	}
	files := map[string][]byte{}
	var names []string
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Base(p)] = data
		names = append(names, filepath.Base(p))
	}
	sort.Strings(names)
	return files, names
}

func samePixels(a, b *image.NRGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	w := a.Bounds().Dx() * 4
	for y := 0; y < a.Bounds().Dy(); y++ {
		if !bytes.Equal(a.Pix[y*a.Stride:y*a.Stride+w], b.Pix[y*b.Stride:y*b.Stride+w]) {
			return false
		}
	}
	return true
}

func crop(img *image.NRGBA, r image.Rectangle) *image.NRGBA {
	r = r.Intersect(img.Bounds())
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		copy(out.Pix[y*out.Stride:y*out.Stride+r.Dx()*4], img.Pix[img.PixOffset(r.Min.X, r.Min.Y+y):][:r.Dx()*4])
	}
	return out
}

func decodeWith(t testing.TB, data []byte, o codec.Options) (*codec.Image, error) {
	t.Helper()
	return Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), o)
}

func standardClassification(ctx context.Context, err error) error {
	isUnsupported := func(err error) bool {
		var unsupported stdjpeg.UnsupportedError
		return errors.As(err, &unsupported)
	}
	return guard.Classify(ctx, err, isUnsupported)
}

func TestCorpusMatchesTheStandardLibraryPlusConversion(t *testing.T) {
	files, names := corpus(t)
	for _, name := range names {
		data := files[name]
		std, stdErr := stdjpeg.Decode(bytes.NewReader(data))
		img, err := decodeWith(t, data, codec.Options{Limits: codec.DefaultLimits()})
		if stdErr != nil {
			if err == nil {
				t.Errorf("%s: decoded a file the standard library rejects", name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !samePixels(img.First(), convert.NRGBA(std)) {
			t.Errorf("%s: differs from the standard library plus conversion", name)
		}
		if img.Scale != 1 || img.Info.Width != 150 || img.Info.Height != 103 {
			t.Errorf("%s: %+v scale %d", name, img.Info, img.Scale)
		}
	}
}

func TestErrorClassesMatchTheStandardLibraryPath(t *testing.T) {
	files, names := corpus(t)
	cases := map[string][]byte{}
	for _, name := range names {
		data := files[name]
		cases[name+"/arithmetic"] = bytes.Replace(data, []byte{0xff, 0xc0}, []byte{0xff, 0xc9}, 1)
		cases[name+"/precision"] = bytes.Replace(data, []byte{0xff, 0xc0, 0x00, 0x11, 0x08}, []byte{0xff, 0xc0, 0x00, 0x11, 0x0c}, 1)
		cases[name+"/cut"] = data[:len(data)*2/3]
		cases[name+"/no-eoi"] = data[:len(data)-2]
	}
	for name, data := range cases {
		_, stdErr := stdjpeg.Decode(bytes.NewReader(data))
		_, err := decodeWith(t, data, codec.Options{})
		if (err == nil) != (stdErr == nil) {
			t.Errorf("%s: standard %v, mine %v", name, stdErr, err)
			continue
		}
		if err == nil {
			continue
		}
		want := standardClassification(context.Background(), stdErr)
		for _, class := range []error{codec.ErrUnsupported, codec.ErrCorrupt, codec.ErrTooLarge} {
			if errors.Is(want, class) != errors.Is(err, class) {
				t.Errorf("%s: standard %v, mine %v", name, want, err)
			}
		}
	}
}

func photo(t testing.TB, w, h int) []byte {
	t.Helper()
	return encodeStd(t, rgbaOf(testimg.Photo(w, h)), 88)
}

func TestScaleReducesNativelyAndReportsTheDivisor(t *testing.T) {
	data := photo(t, 321, 245)
	for _, tc := range []struct{ requested, actual int }{{0, 1}, {1, 1}, {-3, 1}, {2, 2}, {3, 2}, {4, 4}, {5, 4}, {7, 4}, {8, 8}, {9, 8}, {64, 8}} {
		img, err := decodeWith(t, data, codec.Options{Scale: tc.requested})
		if err != nil {
			t.Fatalf("scale %d: %v", tc.requested, err)
		}
		shift := map[int]int{1: 0, 2: 1, 4: 2, 8: 3}[tc.actual]
		w, h := codec.LevelSize(321, 245, shift)
		if img.Scale != tc.actual || img.First().Bounds() != image.Rect(0, 0, w, h) {
			t.Fatalf("requested %d: scale %d bounds %v, want %d and %dx%d", tc.requested, img.Scale, img.First().Bounds(), tc.actual, w, h)
		}
		if img.Info.Width != 321 || img.Info.Height != 245 {
			t.Fatalf("info describes the file, got %+v", img.Info)
		}
	}
}

func TestPixelLimitAppliesToTheScaledOutput(t *testing.T) {
	data := photo(t, 400, 300)
	lim := codec.DefaultLimits()
	lim.MaxPixels = 40_000
	info, err := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
	if !errors.Is(err, codec.ErrTooLarge) || info.Width != 400 || info.Height != 300 || info.Format != codec.JPEG {
		t.Fatalf("probe: %+v %v", info, err)
	}
	if _, err := decodeWith(t, data, codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("full decode: %v", err)
	}
	half, err := decodeWith(t, data, codec.Options{Limits: lim, Scale: 2})
	if err != nil || half.Scale != 2 || half.First().Bounds() != image.Rect(0, 0, 200, 150) {
		t.Fatalf("scale 2: %v %+v", err, half)
	}
	lim.MaxPixels = 20_000
	if _, err := decodeWith(t, data, codec.Options{Limits: lim, Scale: 2}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("scale 2 (%d pixels): %v", 200*150, err)
	}
	img, err := decodeWith(t, data, codec.Options{Limits: lim, Scale: 4})
	if err != nil || img.Scale != 4 || img.First().Bounds() != image.Rect(0, 0, 100, 75) {
		t.Fatalf("scale 4: %v %+v", err, img)
	}
	region := image.Rect(10, 10, 210, 190)
	if _, err := decodeWith(t, data, codec.Options{Limits: lim, Region: &region}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("region of %d pixels: %v", region.Dx()*region.Dy(), err)
	}
	small := image.Rect(10, 10, 110, 110)
	lim.MaxPixels = 40_000
	piece, err := decodeWith(t, data, codec.Options{Limits: lim, Region: &small})
	if err != nil || piece.First().Bounds() != image.Rect(0, 0, 100, 100) {
		t.Fatalf("region: %v %+v", err, piece)
	}
	lim.MaxTiledPixels = 100_000
	info, err = Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
	if !errors.Is(err, codec.ErrTooLarge) || info != (codec.Info{}) {
		t.Fatalf("beyond the tiled limit: %+v %v", info, err)
	}
	if _, err := decodeWith(t, data, codec.Options{Limits: lim, Scale: 8}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("scale 8 beyond the tiled limit: %v", err)
	}
}

func TestProbeKeepsTheOrientationOfOversizedPictures(t *testing.T) {
	data := withExifOrientation(photo(t, 200, 150), false, 6)
	lim := codec.DefaultLimits()
	lim.MaxPixels = 1000
	info, err := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
	if !errors.Is(err, codec.ErrTooLarge) || info.Orientation != codec.OrientRotate90 {
		t.Fatalf("%+v %v", info, err)
	}
	region := image.Rect(0, 0, 20, 20)
	img, err := decodeWith(t, data, codec.Options{Limits: lim, Region: &region})
	if err != nil || img.Info.Orientation != info.Orientation {
		t.Fatalf("%v %+v", err, img)
	}
}

func TestDecodeRegionIsACropOfTheSameLevel(t *testing.T) {
	files, names := corpus(t)
	regions := []image.Rectangle{image.Rect(0, 0, 150, 103), image.Rect(3, 5, 61, 47), image.Rect(100, 60, 400, 400), image.Rect(-9, -9, 30, 20), image.Rect(70, 40, 71, 41)}
	for _, name := range names {
		for _, scale := range []int{1, 2, 4, 8} {
			full, err := decodeWith(t, files[name], codec.Options{Scale: scale})
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			for _, r := range regions {
				r := r
				piece, err := decodeWith(t, files[name], codec.Options{Scale: scale, Region: &r})
				if err != nil {
					if r.Intersect(full.First().Bounds()).Empty() {
						continue
					}
					t.Fatalf("%s scale %d region %v: %v", name, scale, r, err)
				}
				if piece.Scale != scale || !samePixels(piece.First(), crop(full.First(), r)) {
					t.Fatalf("%s scale %d region %v differs", name, scale, r)
				}
			}
		}
	}
}

func TestRegionOutsideThePictureIsCorrupt(t *testing.T) {
	data := photo(t, 64, 48)
	r := image.Rect(500, 500, 600, 600)
	if _, err := decodeWith(t, data, codec.Options{Region: &r}); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
}

func openTiled(t testing.TB, data []byte, lim codec.Limits) codec.TileReader {
	t.Helper()
	tr, err := Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestTiledDecoderInterfaceIsImplemented(t *testing.T) {
	var _ codec.TiledDecoder = Decoder{}
	var d codec.Decoder = Decoder{}
	if _, ok := d.(codec.TiledDecoder); !ok {
		t.Fatal("registered decoder does not implement codec.TiledDecoder")
	}
}

func TestTileReaderRegionsEqualOverviewCrops(t *testing.T) {
	files, names := corpus(t)
	for _, name := range names {
		tr := openTiled(t, files[name], codec.Limits{})
		info := tr.Info()
		if info.Format != codec.JPEG || info.Width != 150 || info.Height != 103 {
			t.Fatalf("%s: %+v", name, info)
		}
		for shift := 0; shift <= 5; shift++ {
			overview, err := tr.Overview(context.Background(), shift)
			if err != nil {
				t.Fatalf("%s overview %d: %v", name, shift, err)
			}
			w, h := codec.LevelSize(150, 103, shift)
			if overview.Bounds() != image.Rect(0, 0, w, h) {
				t.Fatalf("%s overview %d: %v", name, shift, overview.Bounds())
			}
			if shift < tr.MinShift() {
				continue
			}
			for _, r := range []image.Rectangle{image.Rect(0, 0, w, h), image.Rect(1, 1, w, h), image.Rect(w/3, h/4, w*2/3+1, h*3/4+1), image.Rect(w-1, h-1, w+5, h+5), image.Rect(0, 0, 1, 1)} {
				got, err := tr.Region(context.Background(), shift, r)
				if err != nil {
					t.Fatalf("%s region %v at %d: %v", name, r, shift, err)
				}
				if !samePixels(got, crop(overview, r)) {
					t.Fatalf("%s region %v at level %d differs from the overview", name, r, shift)
				}
			}
		}
		if err := tr.Close(); err != nil {
			t.Fatal(err)
		}
		if err := tr.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTileReaderMinShift(t *testing.T) {
	files, names := corpus(t)
	for _, name := range names {
		tr := openTiled(t, files[name], codec.Limits{})
		progressive := strings.Contains(name, "progressive") || strings.Contains(name, "separate")
		if progressive {
			if tr.MinShift() != 3 {
				t.Fatalf("%s: MinShift %d before the overview", name, tr.MinShift())
			}
			if _, err := tr.Overview(context.Background(), 2); err != nil {
				t.Fatal(err)
			}
			if tr.MinShift() != 2 {
				t.Fatalf("%s: MinShift %d after a level-2 overview", name, tr.MinShift())
			}
			if _, err := tr.Region(context.Background(), 1, image.Rect(0, 0, 10, 10)); !errors.Is(err, codec.ErrUnsupported) {
				t.Fatalf("%s: finer region: %v", name, err)
			}
		} else if tr.MinShift() != 0 {
			t.Fatalf("%s: MinShift %d", name, tr.MinShift())
		}
		tr.Close()
	}
}

func TestOverviewBeyondLevelThreeIsABoxReduction(t *testing.T) {
	data := photo(t, 997, 611)
	tr := openTiled(t, data, codec.Limits{})
	defer tr.Close()
	three, err := tr.Overview(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	five, err := tr.Overview(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if want := halve(halve(three)); !samePixels(five, want) {
		t.Fatal("level 5 is not two halvings of level 3")
	}
	w, h := codec.LevelSize(997, 611, 5)
	if five.Bounds() != image.Rect(0, 0, w, h) {
		t.Fatalf("level 5 bounds %v", five.Bounds())
	}
	if _, err := tr.Overview(context.Background(), -1); !errors.Is(err, codec.ErrUnsupported) {
		t.Fatalf("negative level: %v", err)
	}
	if _, err := tr.Region(context.Background(), -1, image.Rect(0, 0, 5, 5)); !errors.Is(err, codec.ErrUnsupported) {
		t.Fatalf("negative level region: %v", err)
	}
}

func TestHalvingKeepsEdgesAndRounding(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 3))
	for i := range src.Pix {
		src.Pix[i] = uint8(i * 7)
	}
	for i := 3; i < len(src.Pix); i += 4 {
		src.Pix[i] = 0xff
	}
	out := halve(src)
	if out.Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatal(out.Bounds())
	}
	px := func(x, y int) [3]int {
		o := src.PixOffset(x, y)
		return [3]int{int(src.Pix[o]), int(src.Pix[o+1]), int(src.Pix[o+2])}
	}
	for c := 0; c < 3; c++ {
		if want := (px(0, 0)[c] + px(1, 0)[c] + px(0, 1)[c] + px(1, 1)[c] + 2) >> 2; int(out.Pix[c]) != want {
			t.Fatalf("interior channel %d: %d, want %d", c, out.Pix[c], want)
		}
		if want := (px(2, 2)[c]*4 + 2) >> 2; int(out.Pix[out.PixOffset(1, 1)+c]) != want {
			t.Fatalf("corner channel %d: %d, want %d", c, out.Pix[out.PixOffset(1, 1)+c], want)
		}
	}
}

func TestTileReaderRespectsTheFileLimitButNotThePixelLimit(t *testing.T) {
	data := photo(t, 300, 200)
	lim := codec.DefaultLimits()
	lim.MaxPixels = 1000
	tr := openTiled(t, data, lim)
	if tr.Info().Width != 300 {
		t.Fatal(tr.Info())
	}
	tr.Close()
	lim = codec.DefaultLimits()
	lim.MaxFileBytes = int64(len(data)) - 1
	if _, err := (Decoder{}).OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), lim); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("file limit: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxTiledPixels = 50_000
	if _, err := (Decoder{}).OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), lim); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("tiled limit: %v", err)
	}
	if _, err := (Decoder{}).OpenTiled(context.Background(), bytes.NewReader([]byte("nonsense")), 8, codec.Limits{}); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("garbage: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Decoder{}).OpenTiled(ctx, bytes.NewReader(data), int64(len(data)), codec.Limits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

func TestTileReaderDoesNotTouchPixelsWhenOpening(t *testing.T) {
	data := photo(t, 800, 600)
	counter := &countingReader{data: data}
	tr, err := Decoder{}.OpenTiled(context.Background(), counter, int64(len(data)), codec.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if counter.bytes.Load() > 80<<10 {
		t.Fatalf("opening read %d of %d bytes", counter.bytes.Load(), len(data))
	}
}

func TestTileReaderRegionsReadOnlyWhatTheyNeed(t *testing.T) {
	data := photo(t, 1600, 1200)
	counter := &countingReader{data: data}
	tr, err := Decoder{}.OpenTiled(context.Background(), counter, int64(len(data)), codec.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if _, err := tr.Overview(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	counter.bytes.Store(0)
	if _, err := tr.Region(context.Background(), 0, image.Rect(700, 500, 956, 756)); err != nil {
		t.Fatal(err)
	}
	if read := counter.bytes.Load(); read > int64(len(data))/3 {
		t.Fatalf("a 256x256 region read %d of %d bytes", read, len(data))
	}
}

func TestConcurrentTileRegions(t *testing.T) {
	data := photo(t, 900, 700)
	tr := openTiled(t, data, codec.Limits{})
	defer tr.Close()
	overview, err := tr.Overview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan string, 8)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				x, y := (g*97+i*131)%700, (g*53+i*71)%500
				r := image.Rect(x, y, x+200, y+200)
				got, err := tr.Region(context.Background(), 0, r)
				if err != nil || !samePixels(got, crop(overview, r)) {
					failures <- r.String()
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(failures)
	for f := range failures {
		t.Fatalf("region %s differs", f)
	}
}

func TestWorkersFieldDoesNotChangeTheResult(t *testing.T) {
	data := photo(t, 640, 480)
	want, err := Decoder{Workers: 1}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, workers := range []int{0, 2, 7} {
		got, err := Decoder{Workers: workers}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{})
		if err != nil || !samePixels(got.First(), want.First()) {
			t.Fatalf("workers %d: %v", workers, err)
		}
	}
}

func TestOpaqueAlphaEverywhere(t *testing.T) {
	data := photo(t, 123, 77)
	for _, scale := range []int{1, 2, 4, 8} {
		img, err := decodeWith(t, data, codec.Options{Scale: scale})
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < img.First().Bounds().Dy(); y++ {
			for x := 0; x < img.First().Bounds().Dx(); x++ {
				if img.First().NRGBAAt(x, y).A != 0xff {
					t.Fatalf("scale %d: transparent pixel at %d,%d", scale, x, y)
				}
			}
		}
	}
}

func TestReducedPhotoStaysCloseToTheSource(t *testing.T) {
	src := testimg.Photo(512, 384)
	data := encodeStd(t, rgbaOf(src), 92)
	for _, scale := range []int{2, 4, 8} {
		img, err := decodeWith(t, data, codec.Options{Scale: scale})
		if err != nil {
			t.Fatal(err)
		}
		reduced := img.First()
		var total, count int
		for y := 0; y < reduced.Bounds().Dy(); y++ {
			for x := 0; x < reduced.Bounds().Dx(); x++ {
				var sum [3]int
				for yy := 0; yy < scale; yy++ {
					for xx := 0; xx < scale; xx++ {
						c := src.NRGBAAt(x*scale+xx, y*scale+yy)
						sum[0] += int(c.R)
						sum[1] += int(c.G)
						sum[2] += int(c.B)
					}
				}
				got := reduced.NRGBAAt(x, y)
				for i, v := range [3]int{int(got.R), int(got.G), int(got.B)} {
					d := v - sum[i]/(scale*scale)
					if d < 0 {
						d = -d
					}
					total += d
					count++
				}
			}
		}
		if mean := float64(total) / float64(count); mean > 3 {
			t.Fatalf("scale %d: mean error %.2f against the source average", scale, mean)
		}
	}
}

type countingReader struct {
	data  []byte
	bytes atomic.Int64
}

func (c *countingReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(c.data)) {
		return 0, io.EOF
	}
	n := copy(p, c.data[off:])
	c.bytes.Add(int64(n))
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
