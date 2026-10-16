package jpegxform

import (
	"bytes"
	"image"
	stdjpeg "image/jpeg"
	"io"
	"math/rand"
	"sync"
	"testing"
)

var (
	benchOnce sync.Once
	benchData []byte
	benchW    = 6000
	benchH    = 4000
)

func benchFile(b *testing.B) []byte {
	b.Helper()
	benchOnce.Do(func() {
		rng := rand.New(rand.NewSource(11))
		img := image.NewYCbCr(image.Rect(0, 0, benchW, benchH), image.YCbCrSubsampleRatio420)
		for y := 0; y < benchH; y++ {
			for x := 0; x < benchW; x++ {
				img.Y[y*img.YStride+x] = uint8((x*3+y*2)/8 + rng.Intn(24))
			}
		}
		for y := 0; y < benchH/2; y++ {
			for x := 0; x < benchW/2; x++ {
				img.Cb[y*img.CStride+x] = uint8(96 + (x+y)/64 + rng.Intn(6))
				img.Cr[y*img.CStride+x] = uint8(160 - (x+y)/64 + rng.Intn(6))
			}
		}
		var buf bytes.Buffer
		if err := stdjpeg.Encode(&buf, img, &stdjpeg.Options{Quality: 90}); err != nil {
			panic(err)
		}
		benchData = buf.Bytes()
	})
	return benchData
}

func benchTransform(b *testing.B, op Op) {
	data := benchFile(b)
	b.Logf("source file: %d bytes, %dx%d", len(data), benchW, benchH)
	r := bytes.NewReader(data)
	b.SetBytes(int64(benchW) * int64(benchH))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Transform(io.Discard, r, int64(len(data)), op, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransform24MpxIdentity(b *testing.B)  { benchTransform(b, Identity) }
func BenchmarkTransform24MpxRotate90(b *testing.B)  { benchTransform(b, Rotate90) }
func BenchmarkTransform24MpxRotate180(b *testing.B) { benchTransform(b, Rotate180) }

func BenchmarkDecodeScan24Mpx(b *testing.B) {
	data := benchFile(b)
	r := bytes.NewReader(data)
	b.SetBytes(int64(benchW) * int64(benchH))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src := newByteSource(r, int64(len(data)), 0)
		h, err := readHeader(src, true)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := decodeScan(b.Context(), src, h); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStandardDecode24Mpx(b *testing.B) {
	data := benchFile(b)
	b.SetBytes(int64(benchW) * int64(benchH))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := stdjpeg.Decode(bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}
