package thumbs

import (
	"bytes"
	"fmt"
	"image"
	stdjpeg "image/jpeg"
	stdpng "image/png"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/jpeg"
	"github.com/oops1/PixGo/internal/codec/png"
)

var (
	benchOnce sync.Once
	benchDir  string
	benchBig  string
	benchPNGs []string
)

func TestMain(m *testing.M) {
	code := m.Run()
	if benchDir != "" {
		os.RemoveAll(benchDir)
	}
	os.Exit(code)
}

func photo(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	state := uint64(0x9e3779b97f4a7c15)
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+w*4]
		fy := float64(y) / float64(h)
		for x := 0; x < w; x++ {
			fx := float64(x) / float64(w)
			state ^= state << 13
			state ^= state >> 7
			state ^= state << 17
			noise := float64(int(state>>60)) - 7.5
			wave := math.Sin(fx*37+fy*11) * 18
			o := x * 4
			row[o] = clamp8(40 + 170*fx + wave + noise)
			row[o+1] = clamp8(60 + 150*fy + wave*0.6 + noise*1.3)
			row[o+2] = clamp8(200 - 120*fx*fy + wave*0.3 + noise*0.8)
			row[o+3] = 255
		}
	}
	return img
}

func clamp8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func benchFixtures(b *testing.B) {
	b.Helper()
	benchOnce.Do(func() {
		dir, err := os.MkdirTemp("", "thumbs-bench-")
		if err != nil {
			b.Fatal(err)
		}
		benchDir = dir
		var buf bytes.Buffer
		if err := stdjpeg.Encode(&buf, photo(6000, 4000), &stdjpeg.Options{Quality: 85}); err != nil {
			b.Fatal(err)
		}
		benchBig = filepath.Join(dir, "big.jpg")
		if err := os.WriteFile(benchBig, buf.Bytes(), 0o600); err != nil {
			b.Fatal(err)
		}
		small := photo(320, 240)
		for i := 0; i < 500; i++ {
			for j := 0; j < len(small.Pix); j += 4 {
				small.Pix[j] += uint8(i)
			}
			var pb bytes.Buffer
			if err := stdpng.Encode(&pb, small); err != nil {
				b.Fatal(err)
			}
			p := filepath.Join(dir, fmt.Sprintf("small%03d.png", i))
			if err := os.WriteFile(p, pb.Bytes(), 0o600); err != nil {
				b.Fatal(err)
			}
			benchPNGs = append(benchPNGs, p)
		}
	})
}

func benchRegistry() *codec.Registry {
	reg := codec.NewRegistry()
	reg.RegisterDecoder(jpeg.Decoder{})
	reg.RegisterDecoder(png.Decoder{})
	return reg
}

func runUntilReady(b *testing.B, s *Service, ui *uiQueue, paths []string, side int) {
	b.Helper()
	remaining := len(paths)
	done := make(chan struct{}, len(paths))
	unsub := s.Subscribe(func(string) { done <- struct{}{} })
	defer unsub()
	for _, p := range paths {
		if _, ok := s.Request(p, side, false); ok {
			remaining--
		}
	}
	deadline := time.After(5 * time.Minute)
	for remaining > 0 {
		ui.drain()
		select {
		case <-done:
			remaining--
		case <-time.After(time.Millisecond):
		case <-deadline:
			b.Fatal("timeout")
		}
	}
	ui.drain()
}

func BenchmarkThumb6000x4000JPEG(b *testing.B) {
	benchFixtures(b)
	reg := benchRegistry()
	for _, side := range []int{128, 256} {
		b.Run(fmt.Sprintf("side%d", side), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ui := &uiQueue{}
				s := New(reg, ui.post, Options{Workers: 1})
				runUntilReady(b, s, ui, []string{benchBig}, side)
				th, ok := s.Request(benchBig, side, false)
				if !ok || th.Err != nil || th.Img.Bounds().Dx() != side {
					b.Fatalf("bad result: ok=%v %+v", ok, th)
				}
				s.Close()
			}
		})
	}
}

func BenchmarkResizeArea(b *testing.B) {
	cases := []struct{ sw, sh, dw, dh int }{
		{6000, 4000, 256, 171},
		{750, 500, 256, 171},
		{1920, 1080, 256, 144},
		{256, 256, 128, 128},
	}
	for _, c := range cases {
		src := photo(c.sw, c.sh)
		b.Run(fmt.Sprintf("%dx%dto%dx%d", c.sw, c.sh, c.dw, c.dh), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(c.sw * c.sh * 4))
			for i := 0; i < b.N; i++ {
				resizeArea(src, c.dw, c.dh)
			}
		})
	}
}

func BenchmarkOrient256(b *testing.B) {
	src := image.NewRGBA(image.Rect(0, 0, 256, 171))
	for o := codec.OrientFlipH; o <= codec.OrientRotate270; o++ {
		b.Run(fmt.Sprintf("orientation%d", o), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				orient(src, o)
			}
		})
	}
}

func Benchmark500SmallPNGs(b *testing.B) {
	benchFixtures(b)
	reg := benchRegistry()
	b.Run("memory only", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ui := &uiQueue{}
			s := New(reg, ui.post, Options{Workers: 4})
			runUntilReady(b, s, ui, benchPNGs, 96)
			s.Close()
		}
	})
	b.Run("cold disk", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ui := &uiQueue{}
			s := New(reg, ui.post, Options{Workers: 4, DiskDir: b.TempDir()})
			runUntilReady(b, s, ui, benchPNGs, 96)
			s.Close()
		}
	})
	b.Run("warm disk", func(b *testing.B) {
		dir := b.TempDir()
		ui := &uiQueue{}
		s := New(reg, ui.post, Options{Workers: 4, DiskDir: dir})
		runUntilReady(b, s, ui, benchPNGs, 96)
		s.Close()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ui := &uiQueue{}
			s := New(reg, ui.post, Options{Workers: 4, DiskDir: dir})
			runUntilReady(b, s, ui, benchPNGs, 96)
			s.Close()
		}
	})
}

func BenchmarkRequestHit(b *testing.B) {
	reg, _ := fakeRegistry()
	dir := b.TempDir()
	p := filepath.Join(dir, "h.fake")
	if err := os.WriteFile(p, []byte("FAKE h"), 0o600); err != nil {
		b.Fatal(err)
	}
	ui := &uiQueue{}
	s := New(reg, ui.post, Options{Workers: 1})
	defer s.Close()
	s.Request(p, 128, true)
	deadline := time.Now().Add(10 * time.Second)
	for {
		ui.drain()
		if _, ok := s.Request(p, 128, true); ok {
			break
		}
		if time.Now().After(deadline) {
			b.Fatal("never ready")
		}
		time.Sleep(time.Millisecond)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := s.Request(p, 128, true); !ok {
			b.Fatal("miss")
		}
	}
}
