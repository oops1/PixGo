package jpegdec

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"runtime"
	"testing"
	"time"
)

const (
	fuzzPixelLimit  = 1 << 17
	fuzzAllocBudget = 64 << 20
	fuzzTimeLimit   = 20 * time.Second
)

func fuzzSeeds(f *testing.F) {
	files := corpus(f)
	for _, name := range sortedNames(files) {
		f.Add(files[name], uint8(0), uint16(0), uint16(0), uint16(40), uint16(30))
	}
	synthetic := syntheticFiles()
	for _, name := range []string{"420-restart-65x31", "444-restart1-17x33", "gray-150x103", "ycck-2x2", "separate-420", "411-65x31", "flex-cb-differs"} {
		f.Add(synthetic[name], uint8(1), uint16(5), uint16(3), uint16(20), uint16(17))
	}
	f.Add([]byte{0xff, 0xd8, 0xff, 0xc0, 0x00, 0x11, 0x08, 0xff, 0xff, 0xff, 0xff, 0x03, 0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01, 0xff, 0xda}, uint8(3), uint16(0), uint16(0), uint16(1), uint16(1))
}

func typedError(err error) bool {
	var format FormatError
	var unsupported UnsupportedError
	return errors.As(err, &format) || errors.As(err, &unsupported) || errors.Is(err, ErrTooLarge) ||
		errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
}

func withinTime(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(fuzzTimeLimit):
		t.Fatalf("no result within %v", fuzzTimeLimit)
	}
}

func FuzzDecode(f *testing.F) {
	fuzzSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte, selector uint8, _, _, _, _ uint16) {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		withinTime(t, func() {
			ctx := context.Background()
			if h, err := ReadHeader(ctx, bytes.NewReader(data), int64(len(data))); err != nil && !typedError(err) {
				t.Fatalf("header: untyped error %v", err)
			} else if err == nil && (h.Width < 0 || h.Height < 0) {
				t.Fatalf("header %+v", h)
			}
			d, err := Open(ctx, bytes.NewReader(data), int64(len(data)), Options{MaxPixels: fuzzPixelLimit, Workers: 1 + int(selector>>4)%3})
			if err != nil {
				if !typedError(err) {
					t.Fatalf("open: untyped error %v", err)
				}
				return
			}
			defer d.Close()
			for _, shift := range []int{0, MaxShift, int(selector) % (MaxShift + 1)} {
				img, err := d.Decode(ctx, shift)
				if err != nil {
					if !typedError(err) {
						t.Fatalf("decode shift %d: untyped error %v", shift, err)
					}
					continue
				}
				h := d.Header()
				scale := 1 << uint(shift)
				if img.Bounds() != image.Rect(0, 0, (h.Width+scale-1)/scale, (h.Height+scale-1)/scale) {
					t.Fatalf("shift %d: bounds %v for %dx%d", shift, img.Bounds(), h.Width, h.Height)
				}
			}
		})
		runtime.ReadMemStats(&after)
		if grown := after.TotalAlloc - before.TotalAlloc; grown > fuzzAllocBudget {
			t.Fatalf("allocated %d bytes for a %d byte input", grown, len(data))
		}
	})
}

func FuzzRegion(f *testing.F) {
	fuzzSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte, selector uint8, x, y, w, h uint16) {
		withinTime(t, func() {
			ctx := context.Background()
			d, err := Open(ctx, bytes.NewReader(data), int64(len(data)), Options{MaxPixels: fuzzPixelLimit, CheckpointEvery: 1 + int(selector>>4)%5, Workers: 1 + int(selector>>2)%2})
			if err != nil {
				return
			}
			defer d.Close()
			shift := int(selector) % (MaxShift + 1)
			full, err := d.Decode(ctx, shift)
			if err != nil {
				return
			}
			r := image.Rect(int(x)%(full.Bounds().Dx()+4)-2, int(y)%(full.Bounds().Dy()+4)-2, 0, 0)
			r.Max = image.Pt(r.Min.X+int(w)%80+1, r.Min.Y+int(h)%80+1)
			got, err := d.Region(ctx, shift, r)
			if err != nil {
				if shift < d.MinShift() {
					return
				}
				t.Fatalf("region %v at shift %d: %v", r, shift, err)
			}
			if want := crop(full, r); !samePixels(got, want) {
				t.Fatalf("region %v at shift %d differs: %s", r, shift, firstDifference(got, want))
			}
		})
	})
}
