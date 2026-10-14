package jxl

import (
	"bytes"
	"context"
	"runtime"
	"sync"
	"testing"

	gjxl "github.com/gen2brain/jxl"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

const benchWidth, benchHeight = 1280, 800

var (
	benchOnce    sync.Once
	benchLossy   []byte
	benchLossles []byte
	largeOnce    sync.Once
	largeData    []byte
)

func benchFiles(b *testing.B) ([]byte, []byte) {
	b.Helper()
	benchOnce.Do(func() {
		benchLossy = encode(b, testimg.Photo(benchWidth, benchHeight), gjxl.EncodeOptions{Quality: 90})
		benchLossles = encode(b, testimg.Photo(benchWidth, benchHeight), gjxl.EncodeOptions{Lossless: true, Effort: 3})
	})
	return benchLossy, benchLossles
}

var (
	noiseOnce sync.Once
	noiseData []byte
)

func noiseFile(b *testing.B) []byte {
	b.Helper()
	noiseOnce.Do(func() {
		noiseData = encode(b, testimg.Noise(benchWidth, benchHeight, 1, false), gjxl.EncodeOptions{Quality: 90})
	})
	return noiseData
}

func largeFile(b *testing.B) []byte {
	b.Helper()
	largeOnce.Do(func() {
		largeData = encode(b, testimg.Photo(4000, 3000), gjxl.EncodeOptions{Quality: 90})
	})
	return largeData
}

func reportRates(b *testing.B, fileBytes, pixels int) {
	perOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N) / 1e6
	b.ReportMetric(perOp/(float64(pixels)/1e6), "ms/Mpx")
	b.ReportMetric(perOp/(float64(fileBytes)/(1<<20)), "ms/MiB")
}

func BenchmarkDecode(b *testing.B) {
	lossy, lossless := benchFiles(b)
	for _, sample := range []struct {
		name string
		data []byte
	}{{"lossy", lossy}, {"lossless", lossless}, {"noise", noiseFile(b)}} {
		data := sample.data
		b.Run(sample.name, func(b *testing.B) {
			options := codec.Options{Limits: codec.DefaultLimits()}
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), options); err != nil {
					b.Fatal(err)
				}
			}
			reportRates(b, len(data), benchWidth*benchHeight)
		})
	}
}

func BenchmarkDecodeLarge(b *testing.B) {
	data := largeFile(b)
	options := codec.Options{Limits: codec.DefaultLimits()}
	for name, threads := range map[string]int{"serial": 0, "parallel": runtime.GOMAXPROCS(0)} {
		d := Decoder{Threads: threads}
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := d.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), options); err != nil {
					b.Fatal(err)
				}
			}
			reportRates(b, len(data), 4000*3000)
		})
	}
}

func BenchmarkProbe(b *testing.B) {
	lossy, _ := benchFiles(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(lossy), int64(len(lossy)), codec.DefaultLimits()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProbeAnimation(b *testing.B) {
	clips := make([]clipSpec, 200)
	for i := range clips {
		clips[i].duration = 1
	}
	data := buildAnimation(b, animationSpec{width: 64, height: 64, clips: clips})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits()); err != nil {
			b.Fatal(err)
		}
	}
}
