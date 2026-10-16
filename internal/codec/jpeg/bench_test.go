package jpeg

import (
	"bytes"
	"context"
	"fmt"
	"image"
	stdjpeg "image/jpeg"
	"math"
	"sync"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
)

var (
	megapixelOnce sync.Once
	megapixelData []byte
)

func twentyFourMegapixels(b testing.TB) []byte {
	megapixelOnce.Do(func() {
		const width, height = 6000, 4000
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		state := uint64(0x9e3779b97f4a7c15)
		for y := 0; y < height; y++ {
			row := img.Pix[y*img.Stride : y*img.Stride+width*4]
			fy := float64(y) / height
			for x := 0; x < width; x++ {
				fx := float64(x) / width
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
		var buf bytes.Buffer
		if err := stdjpeg.Encode(&buf, img, &stdjpeg.Options{Quality: 90}); err != nil {
			b.Fatal(err)
		}
		megapixelData = buf.Bytes()
	})
	return megapixelData
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

func BenchmarkStandardLibraryPlusConversion24Mpx(b *testing.B) {
	data := twentyFourMegapixels(b)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		img, err := stdjpeg.Decode(bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		}
		if convert.NRGBA(img).Bounds().Dx() != 6000 {
			b.Fatal("size")
		}
	}
}

func BenchmarkAdapterDecode24Mpx(b *testing.B) {
	data := twentyFourMegapixels(b)
	for _, workers := range []int{1, 0} {
		for _, scale := range []int{1, 2, 4, 8} {
			b.Run(fmt.Sprintf("workers=%d/scale=%d", workers, scale), func(b *testing.B) {
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					img, err := Decoder{Workers: workers}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Scale: scale})
					if err != nil || img.Scale != scale {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkTileRegion256(b *testing.B) {
	data := twentyFourMegapixels(b)
	tr, err := Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Limits{})
	if err != nil {
		b.Fatal(err)
	}
	defer tr.Close()
	if _, err := tr.Overview(context.Background(), 3); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x, y := (i*977)%5600, (i*613)%3600
		img, err := tr.Region(context.Background(), 0, image.Rect(x, y, x+256, y+256))
		if err != nil || img.Bounds().Dx() != 256 {
			b.Fatal(err)
		}
	}
}

func BenchmarkTileOverview3(b *testing.B) {
	data := twentyFourMegapixels(b)
	tr, err := Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Limits{})
	if err != nil {
		b.Fatal(err)
	}
	defer tr.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := tr.Overview(context.Background(), 3); err != nil {
			b.Fatal(err)
		}
	}
}
