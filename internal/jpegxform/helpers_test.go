package jpegxform

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func corpusNames(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "*.jpeg"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no test corpus: %v", err)
	}
	var names []string
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}
	sort.Strings(names)
	return names
}

func supportedCorpus(t testing.TB) []string {
	t.Helper()
	var out []string
	for _, n := range corpusNames(t) {
		if !strings.Contains(n, "progressive") {
			out = append(out, n)
		}
	}
	return out
}

func readCorpus(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

var allOps = []Op{Identity, FlipH, FlipV, Rotate90, Rotate180, Rotate270, Transpose, Transverse}

func runTransform(data []byte, op Op, o Options) ([]byte, error) {
	var out bytes.Buffer
	err := Transform(&out, bytes.NewReader(data), int64(len(data)), op, o)
	return out.Bytes(), err
}

func mustTransform(t testing.TB, data []byte, op Op, o Options) []byte {
	t.Helper()
	out, err := runTransform(data, op, o)
	if err != nil {
		t.Fatalf("%v: %v", op, err)
	}
	return out
}

func stdDecode(t testing.TB, data []byte) image.Image {
	t.Helper()
	img, err := stdjpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the standard decoder: %v", err)
	}
	return img
}

func isStdUnsupported(err error) bool {
	var u stdjpeg.UnsupportedError
	return errors.As(err, &u)
}

func rgbaAt(img image.Image, x, y int) color.RGBA {
	return color.RGBAModel.Convert(img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y)).(color.RGBA)
}

func cropImage(img image.Image, w, h int) image.Image {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetRGBA(x, y, rgbaAt(img, x, y))
		}
	}
	_ = b
	return out
}

func orientImage(img image.Image, op Op) *image.RGBA {
	o := op.Orientation()
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	ow, oh := o.Size(w, h)
	out := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			sx, sy := o.ToSource(float64(x)+.5, float64(y)+.5, float64(w), float64(h))
			out.SetRGBA(x, y, rgbaAt(img, int(math.Floor(sx)), int(math.Floor(sy))))
		}
	}
	return out
}

type diffStats struct {
	max   int
	exact int
	total int
	sum   float64
}

func (d diffStats) mean() float64 { return d.sum / float64(max(d.total, 1)) }

func compareImages(t testing.TB, got, want image.Image) diffStats {
	t.Helper()
	gb, wb := got.Bounds(), want.Bounds()
	if gb.Dx() != wb.Dx() || gb.Dy() != wb.Dy() {
		t.Fatalf("size %dx%d, want %dx%d", gb.Dx(), gb.Dy(), wb.Dx(), wb.Dy())
	}
	var d diffStats
	for y := 0; y < wb.Dy(); y++ {
		for x := 0; x < wb.Dx(); x++ {
			a, b := rgbaAt(got, x, y), rgbaAt(want, x, y)
			m := max(absInt(int(a.R)-int(b.R)), absInt(int(a.G)-int(b.G)), absInt(int(a.B)-int(b.B)))
			d.max = max(d.max, m)
			d.sum += float64(m)
			d.total++
			if m == 0 {
				d.exact++
			}
		}
	}
	return d
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func parseCoefficients(t testing.TB, data []byte) (*header, []grid) {
	t.Helper()
	src := newByteSource(bytes.NewReader(data), int64(len(data)), 0)
	h, err := readHeader(src, true)
	if err != nil {
		t.Fatalf("header: %v", err)
	}
	grids, err := decodeScan(t.Context(), src, h)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return h, grids
}

func decodeStdErr(data []byte) (image.Image, error) {
	return stdjpeg.Decode(bytes.NewReader(data))
}

func parseHeaderOnly(data []byte) (*header, error) {
	return readHeader(newByteSource(bytes.NewReader(data), int64(len(data)), 0), true)
}
