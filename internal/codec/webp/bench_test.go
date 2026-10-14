package webp

import (
	"bytes"
	"context"
	"image"
	"sync"
	"testing"

	vpx "github.com/gen2brain/vpx/webp"
	xwebp "golang.org/x/image/webp"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

const (
	benchWidth  = 6000
	benchHeight = 4000
)

var (
	benchOnce     sync.Once
	benchLossy    []byte
	benchLossless []byte
)

func benchSamples(b *testing.B) {
	b.Helper()
	benchOnce.Do(func() {
		source := testimg.Photo(benchWidth, benchHeight)
		benchLossy = vpxEncode(b, source, vpx.EncodeOptions{Quality: 80, Method: 2})
		benchLossless = vpxEncode(b, source, vpx.EncodeOptions{Lossless: true, Method: 1})
	})
	b.ResetTimer()
}

func BenchmarkDecode24MP(b *testing.B) {
	benchSamples(b)
	for name, data := range map[string][]byte{"lossy": benchLossy, "lossless": benchLossless} {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(benchWidth * benchHeight * 4)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
				if err != nil || img.First().Rect.Dx() != benchWidth {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDecode24MPStandardLibrary(b *testing.B) {
	benchSamples(b)
	for name, data := range map[string][]byte{"lossy": benchLossy, "lossless": benchLossless} {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(benchWidth * benchHeight * 4)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				decoded, err := xwebp.Decode(bytes.NewReader(data))
				if err != nil {
					b.Fatal(err)
				}
				if pix := convert.NRGBA(decoded); pix.Rect.Dx() != benchWidth {
					b.Fatal("size")
				}
			}
		})
	}
}

func BenchmarkProbe24MP(b *testing.B) {
	benchSamples(b)
	for name, data := range map[string][]byte{"lossy": benchLossy, "lossless": benchLossless} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDecodeAnimation(b *testing.B) {
	var frames []*image.NRGBA
	delays := make([]int, 60)
	for i := range delays {
		frames = append(frames, numbered(320, 240, i%8))
		delays[i] = 40
	}
	data := encodeAnimation(b, frames, delays, 0)
	for name, selection := range map[string]codec.FrameSel{"first frame": codec.FirstFrame, "all frames": codec.AllFrames} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits(), Frames: selection}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkEncode(b *testing.B) {
	source := testimg.Photo(2000, 1500)
	for name, o := range map[string]codec.EncodeOptions{"lossy 80": {Quality: 80}, "lossless": {Lossless: true}} {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(2000 * 1500 * 4)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := (Encoder{}).Encode(&bytes.Buffer{}, source, o); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
