package jpegdec

import (
	"bytes"
	"context"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func corpus(t testing.TB) map[string][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "*.jpeg"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no corpus: %v", err)
	}
	files := map[string][]byte{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Base(p)] = data
	}
	return files
}

func sortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func referenceNRGBA(t testing.TB, data []byte) (*image.NRGBA, error) {
	t.Helper()
	img, err := stdjpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			c := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			out.SetNRGBA(x, y, c)
		}
	}
	return out, nil
}

func open(t testing.TB, data []byte, o Options) *Decoder {
	t.Helper()
	d, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)), o)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func decodeAt(t testing.TB, data []byte, shift int, o Options) *image.NRGBA {
	t.Helper()
	d := open(t, data, o)
	defer d.Close()
	img, err := d.Decode(context.Background(), shift)
	if err != nil {
		t.Fatal(err)
	}
	return img
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

func firstDifference(a, b *image.NRGBA) string {
	if a.Bounds() != b.Bounds() {
		return "bounds " + a.Bounds().String() + " vs " + b.Bounds().String()
	}
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			if a.NRGBAAt(x, y) != b.NRGBAAt(x, y) {
				return "pixel (" + itoa(x) + "," + itoa(y) + ") " + colorString(a.NRGBAAt(x, y)) + " vs " + colorString(b.NRGBAAt(x, y))
			}
		}
	}
	return "identical"
}

func itoa(v int) string {
	return string(appendInt(nil, v))
}

func appendInt(b []byte, v int) []byte {
	if v < 0 {
		b = append(b, '-')
		v = -v
	}
	if v >= 10 {
		b = appendInt(b, v/10)
	}
	return append(b, byte('0'+v%10))
}

func colorString(c color.NRGBA) string {
	return "[" + itoa(int(c.R)) + " " + itoa(int(c.G)) + " " + itoa(int(c.B)) + " " + itoa(int(c.A)) + "]"
}

func crop(img *image.NRGBA, r image.Rectangle) *image.NRGBA {
	r = r.Intersect(img.Bounds())
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		copy(out.Pix[y*out.Stride:y*out.Stride+r.Dx()*4], img.Pix[img.PixOffset(r.Min.X, r.Min.Y+y):][:r.Dx()*4])
	}
	return out
}

func boxReduce(img *image.NRGBA, scale int) *image.NRGBA {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	ow, oh := (w+scale-1)/scale, (h+scale-1)/scale
	out := image.NewNRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			var sum [3]int
			count := 0
			for yy := y * scale; yy < min((y+1)*scale, h); yy++ {
				for xx := x * scale; xx < min((x+1)*scale, w); xx++ {
					p := img.NRGBAAt(xx, yy)
					sum[0] += int(p.R)
					sum[1] += int(p.G)
					sum[2] += int(p.B)
					count++
				}
			}
			out.SetNRGBA(x, y, color.NRGBA{R: uint8((sum[0] + count/2) / count), G: uint8((sum[1] + count/2) / count), B: uint8((sum[2] + count/2) / count), A: 255})
		}
	}
	return out
}

func meanAbsoluteError(a, b *image.NRGBA) float64 {
	var total float64
	n := 0
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			pa, pb := a.NRGBAAt(x, y), b.NRGBAAt(x, y)
			total += math.Abs(float64(pa.R)-float64(pb.R)) + math.Abs(float64(pa.G)-float64(pb.G)) + math.Abs(float64(pa.B)-float64(pb.B))
			n += 3
		}
	}
	return total / float64(n)
}

func psnr(a, b *image.NRGBA) float64 {
	var total float64
	n := 0
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			pa, pb := a.NRGBAAt(x, y), b.NRGBAAt(x, y)
			for _, d := range []float64{float64(pa.R) - float64(pb.R), float64(pa.G) - float64(pb.G), float64(pa.B) - float64(pb.B)} {
				total += d * d
				n++
			}
		}
	}
	mse := total / float64(n)
	if mse == 0 {
		return 99
	}
	return 10 * math.Log10(255*255/mse)
}

func bytesReader(data []byte) *bytes.Reader { return bytes.NewReader(data) }

func rect(x0, y0, x1, y1 int) image.Rectangle { return image.Rect(x0, y0, x1, y1) }
