package jpegdec

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"math"
	"sync"
	"testing"
)

func syntheticPhoto(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	state := uint64(0x9e3779b97f4a7c15)
	for y := 0; y < height; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+width*4]
		fy := float64(y) / float64(height)
		for x := 0; x < width; x++ {
			fx := float64(x) / float64(width)
			wave := math.Sin(fx*37+fy*11)*18 + math.Sin(fx*311+fy*173)*6
			state ^= state << 13
			state ^= state >> 7
			state ^= state << 17
			noise := float64(int(state>>60)) - 7.5
			o := x * 4
			row[o] = clampByte(40 + 170*fx + wave + noise)
			row[o+1] = clampByte(60 + 150*fy + wave*0.6 + noise*1.3)
			row[o+2] = clampByte(200 - 120*fx*fy + wave*0.3 + noise*0.8)
			row[o+3] = 0xff
		}
	}
	return img
}

func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

var (
	bigOnce sync.Once
	bigData []byte
)

func bigJPEG(b testing.TB) []byte {
	bigOnce.Do(func() {
		var buf bytes.Buffer
		if err := stdjpeg.Encode(&buf, syntheticPhoto(6000, 4000), &stdjpeg.Options{Quality: 90}); err != nil {
			b.Fatal(err)
		}
		bigData = buf.Bytes()
	})
	return bigData
}

func BenchmarkStandardLibrary24Mpx(b *testing.B) {
	data := bigJPEG(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		img, err := stdjpeg.Decode(bytes.NewReader(data))
		if err != nil || img.Bounds().Dx() != 6000 {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode24Mpx(b *testing.B) {
	data := bigJPEG(b)
	for _, workers := range []int{1, 2, 4, 8} {
		for shift := 0; shift <= MaxShift; shift++ {
			b.Run(fmt.Sprintf("workers=%d/scale=%d", workers, 1<<shift), func(b *testing.B) {
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					d, err := Open(b.Context(), bytes.NewReader(data), int64(len(data)), Options{Workers: workers})
					if err != nil {
						b.Fatal(err)
					}
					img, err := d.Decode(b.Context(), shift)
					if err != nil || img.Bounds().Dx() != (6000+(1<<shift)-1)>>shift {
						b.Fatal(err)
					}
					d.Close()
				}
			})
		}
	}
}

var _ = color.NRGBA{}

var (
	mediumOnce sync.Once
	mediumData []byte
)

func mediumJPEG(b testing.TB) []byte {
	mediumOnce.Do(func() {
		var buf bytes.Buffer
		if err := stdjpeg.Encode(&buf, syntheticPhoto(2400, 1600), &stdjpeg.Options{Quality: 90}); err != nil {
			b.Fatal(err)
		}
		mediumData = buf.Bytes()
	})
	return mediumData
}
