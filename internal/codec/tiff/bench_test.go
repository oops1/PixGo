package tiff

import (
	"bytes"
	"context"
	"image"
	"sync"
	"testing"

	xtiff "golang.org/x/image/tiff"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
)

const (
	benchWidth  = 6000
	benchHeight = 4000
)

var (
	benchOnce  sync.Once
	benchFiles map[string][]byte
)

func benchData(name string) []byte {
	benchOnce.Do(func() {
		benchFiles = map[string][]byte{}
		strips := kinds[12].fixture(benchWidth, 64, 71)
		strips.claimH = benchHeight
		strips.rowsPerStrip = 64
		strips.alias = true
		for label, compression := range map[string]int{"strips-none": 1, "strips-deflate": 8, "strips-lzw": 5, "strips-packbits": 32773} {
			f := strips
			f.compression = compression
			benchFiles[label] = f.bytes()
		}
		tiles := kinds[12].fixture(256, 256, 72)
		tiles.claimW, tiles.claimH = benchWidth, benchHeight
		tiles.tileW, tiles.tileH = 256, 256
		tiles.alias = true
		for label, compression := range map[string]int{"tiles-none": 1, "tiles-deflate": 8, "tiles-lzw": 5} {
			f := tiles
			f.compression = compression
			benchFiles[label] = f.bytes()
		}
		wide := strips
		wide.compression = 8
		wide.predictor = 2
		benchFiles["strips-deflate-predictor"] = wide.bytes()
	})
	return benchFiles[name]
}

func benchDecode(b *testing.B, name string, scale int) {
	data := benchData(name)
	b.SetBytes(benchWidth * benchHeight * 4)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits(), Scale: scale})
		if err != nil {
			b.Fatal(err)
		}
		if img.First() == nil {
			b.Fatal("no pixels")
		}
	}
}

func BenchmarkDecode24MpxStripsNone(b *testing.B)          { benchDecode(b, "strips-none", 1) }
func BenchmarkDecode24MpxStripsDeflate(b *testing.B)       { benchDecode(b, "strips-deflate", 1) }
func BenchmarkDecode24MpxStripsLZW(b *testing.B)           { benchDecode(b, "strips-lzw", 1) }
func BenchmarkDecode24MpxStripsPackBits(b *testing.B)      { benchDecode(b, "strips-packbits", 1) }
func BenchmarkDecode24MpxTilesDeflate(b *testing.B)        { benchDecode(b, "tiles-deflate", 1) }
func BenchmarkDecode24MpxTilesNone(b *testing.B)           { benchDecode(b, "tiles-none", 1) }
func BenchmarkDecode24MpxStripsDeflateScale2(b *testing.B) { benchDecode(b, "strips-deflate", 2) }
func BenchmarkDecode24MpxStripsDeflateScale8(b *testing.B) { benchDecode(b, "strips-deflate", 8) }
func BenchmarkDecode24MpxTilesDeflateScale8(b *testing.B)  { benchDecode(b, "tiles-deflate", 8) }

func benchXImage(b *testing.B, name string) {
	data := benchData(name)
	b.SetBytes(benchWidth * benchHeight * 4)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decoded, err := xtiff.Decode(bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		}
		convert.NRGBA(decoded)
	}
}

func BenchmarkXImage24MpxStripsNone(b *testing.B)    { benchXImage(b, "strips-none") }
func BenchmarkXImage24MpxStripsDeflate(b *testing.B) { benchXImage(b, "strips-deflate") }
func BenchmarkXImage24MpxTilesDeflate(b *testing.B)  { benchXImage(b, "tiles-deflate") }

func benchOverview(b *testing.B, name string, shift int) {
	data := benchData(name)
	rd, err := Decoder{CacheBytes: -1}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		b.Fatal(err)
	}
	defer rd.Close()
	b.SetBytes(benchWidth * benchHeight * 4)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := rd.Overview(context.Background(), shift); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOverview3TilesDeflate(b *testing.B)  { benchOverview(b, "tiles-deflate", 3) }
func BenchmarkOverview3TilesNone(b *testing.B)     { benchOverview(b, "tiles-none", 3) }
func BenchmarkOverview3StripsDeflate(b *testing.B) { benchOverview(b, "strips-deflate", 3) }
func BenchmarkOverview3StripsNone(b *testing.B)    { benchOverview(b, "strips-none", 3) }
func BenchmarkOverview3StripsLZW(b *testing.B)     { benchOverview(b, "strips-lzw", 3) }

func benchRegion(b *testing.B, name string, shift int, cacheBytes int64, rect image.Rectangle) {
	data := benchData(name)
	rd, err := Decoder{CacheBytes: cacheBytes}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		b.Fatal(err)
	}
	defer rd.Close()
	if _, err := rd.Region(context.Background(), shift, rect); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(rect.Dx() * rect.Dy() * 4))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := rd.Region(context.Background(), shift, rect); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRegion256TilesDeflateCold(b *testing.B) {
	benchRegion(b, "tiles-deflate", 0, -1, image.Rect(1000, 1000, 1256, 1256))
}

func BenchmarkRegion256TilesDeflateWarm(b *testing.B) {
	benchRegion(b, "tiles-deflate", 0, 64<<20, image.Rect(1000, 1000, 1256, 1256))
}

func BenchmarkRegion256Shift2TilesDeflateCold(b *testing.B) {
	benchRegion(b, "tiles-deflate", 2, -1, image.Rect(100, 100, 356, 356))
}

func BenchmarkRegion256Shift2TilesDeflateWarm(b *testing.B) {
	benchRegion(b, "tiles-deflate", 2, 64<<20, image.Rect(100, 100, 356, 356))
}

func BenchmarkRegion256StripsDeflateCold(b *testing.B) {
	benchRegion(b, "strips-deflate", 0, -1, image.Rect(1000, 1000, 1256, 1256))
}

func BenchmarkRegion256StripsDeflateWarm(b *testing.B) {
	benchRegion(b, "strips-deflate", 0, 64<<20, image.Rect(1000, 1000, 1256, 1256))
}

func BenchmarkRegion256Shift1StripsDeflateWarm(b *testing.B) {
	benchRegion(b, "strips-deflate", 1, 64<<20, image.Rect(500, 500, 756, 756))
}
