package jpegdec

import (
	"bytes"
	"image"
	stdjpeg "image/jpeg"
	"math"
	"strings"
	"testing"
)

func TestReducedLevelsHaveCeilingSize(t *testing.T) {
	files := syntheticFiles()
	for _, name := range sortedNames(files) {
		if !strings.HasPrefix(name, "420-") && !strings.HasPrefix(name, "gray-") && !strings.HasPrefix(name, "444-") {
			continue
		}
		d := open(t, files[name], Options{})
		h := d.Header()
		for shift := 0; shift <= MaxShift; shift++ {
			img, err := d.Decode(t.Context(), shift)
			if err != nil {
				t.Fatalf("%s shift %d: %v", name, shift, err)
			}
			scale := 1 << shift
			wantW, wantH := (h.Width+scale-1)/scale, (h.Height+scale-1)/scale
			if img.Bounds().Dx() != wantW || img.Bounds().Dy() != wantH {
				t.Fatalf("%s shift %d: got %v, want %dx%d", name, shift, img.Bounds(), wantW, wantH)
			}
		}
		d.Close()
	}
}

type quality struct {
	mae  float64
	psnr float64
}

func measureQuality(t *testing.T, data []byte, shift int) quality {
	t.Helper()
	full := decodeAt(t, data, 0, Options{})
	reduced := decodeAt(t, data, shift, Options{})
	box := boxReduce(full, 1<<shift)
	if box.Bounds() != reduced.Bounds() {
		t.Fatalf("bounds %v vs %v", reduced.Bounds(), box.Bounds())
	}
	return quality{mae: meanAbsoluteError(reduced, box), psnr: psnr(reduced, box)}
}

func smoothPhoto(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			fx, fy := float64(x)/float64(width), float64(y)/float64(height)
			o := img.PixOffset(x, y)
			img.Pix[o] = clampByte(60 + 150*fx + 25*math.Sin(fx*9+fy*4) + 30*math.Exp(-((fx-0.3)*(fx-0.3)+(fy-0.4)*(fy-0.4))*40))
			img.Pix[o+1] = clampByte(90 + 110*fy + 20*math.Sin(fx*5-fy*7))
			img.Pix[o+2] = clampByte(200 - 100*fx*fy + 15*math.Cos(fx*11) + 25*math.Exp(-((fx-0.7)*(fx-0.7)+(fy-0.6)*(fy-0.6))*60))
			img.Pix[o+3] = 0xff
		}
	}
	return img
}

func encodeStandard(t testing.TB, img image.Image, quality int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := stdjpeg.Encode(&buf, img, &stdjpeg.Options{Quality: quality}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReducedDecodeOfSmoothPhotographsStaysCloseToABoxFilter(t *testing.T) {
	for _, size := range [][2]int{{640, 480}, {641, 479}} {
		data := encodeStandard(t, smoothPhoto(size[0], size[1]), 92)
		for shift := 1; shift <= MaxShift; shift++ {
			q := measureQuality(t, data, shift)
			t.Logf("%dx%d scale %d: MAE %.3f PSNR %.2f dB", size[0], size[1], 1<<shift, q.mae, q.psnr)
			if q.mae > 1 || q.psnr < 40 {
				t.Errorf("%dx%d scale %d: MAE %.3f PSNR %.2f dB", size[0], size[1], 1<<shift, q.mae, q.psnr)
			}
		}
	}
}

func TestReducedDecodeOfDetailedFramesStaysUsable(t *testing.T) {
	files := corpus(t)
	photographs := []string{
		"video-001.jpeg",
		"video-001.q50.420.jpeg",
		"video-001.q50.422.jpeg",
		"video-001.q50.440.jpeg",
		"video-001.q50.411.jpeg",
		"video-001.q50.410.jpeg",
		"video-001.progressive.jpeg",
		"video-001.q50.420.progressive.jpeg",
		"video-005.gray.q50.jpeg",
		"video-001.rgb.jpeg",
		"video-001.cmyk.jpeg",
	}
	for _, name := range photographs {
		for shift := 1; shift <= MaxShift; shift++ {
			q := measureQuality(t, files[name], shift)
			t.Logf("%-40s scale %d: MAE %.3f PSNR %.2f dB", name, 1<<shift, q.mae, q.psnr)
			if q.mae > 4 || q.psnr < 29 {
				t.Errorf("%s scale %d: MAE %.3f PSNR %.2f dB", name, 1<<shift, q.mae, q.psnr)
			}
		}
	}
}
