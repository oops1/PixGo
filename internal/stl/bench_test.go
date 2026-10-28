package stl

import (
	"bytes"
	"context"
	"testing"
)

const benchTriangles = 1_000_000

var benchData = map[string][]byte{}

func benchFile(kind string) []byte {
	if d, ok := benchData[kind]; ok {
		return d
	}
	ts := bigTriangles(benchTriangles, 42)
	var d []byte
	if kind == "binary" {
		d = binarySTL("bench", ts, nil, 0)
	} else {
		d = asciiSTL("bench", [][]tri{ts}, asciiStyle{exp: true})
	}
	benchData[kind] = d
	return d
}

func benchRead(b *testing.B, kind string) {
	data := benchFile(kind)
	r := bytes.NewReader(data)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := Read(context.Background(), r, int64(len(data)), Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func benchProbe(b *testing.B, kind string) {
	data := benchFile(kind)
	r := bytes.NewReader(data)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := Probe(context.Background(), r, int64(len(data))); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadBinary1M(b *testing.B)  { benchRead(b, "binary") }
func BenchmarkProbeBinary1M(b *testing.B) { benchProbe(b, "binary") }
func BenchmarkReadASCII1M(b *testing.B)   { benchRead(b, "ascii") }
func BenchmarkProbeASCII1M(b *testing.B)  { benchProbe(b, "ascii") }
