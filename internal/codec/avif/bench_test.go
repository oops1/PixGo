package avif

import (
	"bytes"
	"context"
	"sync"
	"testing"

	gavif "github.com/gen2brain/gav1d/avif"

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
)

func benchFile(b *testing.B) []byte {
	b.Helper()
	return benchFiles(b)[0].data
}

func benchFiles(b *testing.B) []benchSample {
	b.Helper()
	benchOnce.Do(func() {
		benchSamples = []benchSample{
			{"photo", encode(b, testimg.Photo(benchWidth, benchHeight), gavif.EncodeOptions{Quality: 60, Speed: 6})},
			{"noise", encode(b, testimg.Noise(benchWidth, benchHeight, 1, false), gavif.EncodeOptions{Quality: 60, Speed: 10})},
		}
	})
	return benchSamples
}

func reportRates(b *testing.B, fileBytes int) {
	perOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N) / 1e6
	b.ReportMetric(perOp/(float64(benchWidth*benchHeight)/1e6), "ms/Mpx")
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
			reportRates(b, len(sample.data))
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

func BenchmarkDecodeAnimation(b *testing.B) {
	data := readTestdata(b, "anim.avif")
	options := codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), options); err != nil {
			b.Fatal(err)
		}
	}
}
