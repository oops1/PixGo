package heic

import (
	"bytes"
	"context"
	"runtime"
	"sync"
	"testing"

	gheic "github.com/gen2brain/h265/heic"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

const benchWidth, benchHeight = 1280, 800

type benchSample struct {
	name string
	data []byte
}

var (
	benchOnce    sync.Once
	benchSamples []benchSample
	tiledOnce    sync.Once
	tiledData    []byte
)

func benchFile(b *testing.B) []byte {
	b.Helper()
	return benchFiles(b)[0].data
}

func benchFiles(b *testing.B) []benchSample {
	b.Helper()
	benchOnce.Do(func() {
		benchSamples = []benchSample{
			{"photo", encode(b, testimg.Photo(benchWidth, benchHeight), gheic.EncodeOptions{Quality: 60})},
			{"noise", encode(b, testimg.Noise(benchWidth, benchHeight, 1, false), gheic.EncodeOptions{Quality: 60})},
		}
	})
	return benchSamples
}

func tiledFile(b *testing.B) []byte {
	b.Helper()
	tiledOnce.Do(func() {
		tiledData = encode(b, testimg.Photo(4000, 3000), gheic.EncodeOptions{Quality: 60, Tile: 512})
	})
	return tiledData
}

func reportRates(b *testing.B, fileBytes, pixels int) {
	perOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N) / 1e6
	b.ReportMetric(perOp/(float64(pixels)/1e6), "ms/Mpx")
	b.ReportMetric(perOp/(float64(fileBytes)/(1<<20)), "ms/MiB")
}

func BenchmarkDecode(b *testing.B) {
	for _, sample := range benchFiles(b) {
		b.Run(sample.name, func(b *testing.B) {
			options := codec.Options{Limits: codec.DefaultLimits()}
			b.SetBytes(int64(len(sample.data)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(sample.data), int64(len(sample.data)), options); err != nil {
					b.Fatal(err)
				}
			}
			reportRates(b, len(sample.data), benchWidth*benchHeight)
		})
	}
}

func BenchmarkDecodeTiled(b *testing.B) {
	data := tiledFile(b)
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
	data := benchFile(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits()); err != nil {
			b.Fatal(err)
		}
	}
}
