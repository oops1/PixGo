package testimg

import (
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

func xorshift(state *uint64) uint64 {
	x := *state
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	*state = x
	return x
}

func Photo(width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	state := uint64(0x9e3779b97f4a7c15)
	for y := 0; y < height; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+width*4]
		fy := float64(y) / float64(height)
		for x := 0; x < width; x++ {
			fx := float64(x) / float64(width)
			wave := math.Sin(fx*37+fy*11) * 18
			noise := float64(int(xorshift(&state)>>60)) - 7.5
			o := x * 4
			row[o] = clamp(40 + 170*fx + wave + noise)
			row[o+1] = clamp(60 + 150*fy + wave*0.6 + noise*1.3)
			row[o+2] = clamp(200 - 120*fx*fy + wave*0.3 + noise*0.8)
			row[o+3] = 0xff
		}
	}
	return img
}

func clamp(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func Noise(width, height int, seed uint64, withAlpha bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	state := seed*0x2545f4914f6cdd1d + 0x9e3779b97f4a7c15
	for i := 0; i < len(img.Pix); i += 4 {
		v := xorshift(&state)
		img.Pix[i] = uint8(v)
		img.Pix[i+1] = uint8(v >> 8)
		img.Pix[i+2] = uint8(v >> 16)
		if withAlpha {
			img.Pix[i+3] = uint8(v >> 24)
		} else {
			img.Pix[i+3] = 0xff
		}
	}
	return img
}

func Identical(a, b *image.NRGBA) bool {
	if a == nil || b == nil || a.Bounds() != b.Bounds() {
		return false
	}
	bounds := a.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		ra := a.Pix[a.PixOffset(bounds.Min.X, y):][:bounds.Dx()*4]
		rb := b.Pix[b.PixOffset(bounds.Min.X, y):][:bounds.Dx()*4]
		for i := range ra {
			if ra[i] != rb[i] {
				return false
			}
		}
	}
	return true
}

func MaxDelta(a, b *image.NRGBA) int {
	if a.Bounds() != b.Bounds() {
		return math.MaxInt
	}
	worst := 0
	bounds := a.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		ra := a.Pix[a.PixOffset(bounds.Min.X, y):][:bounds.Dx()*4]
		rb := b.Pix[b.PixOffset(bounds.Min.X, y):][:bounds.Dx()*4]
		for i := range ra {
			d := int(ra[i]) - int(rb[i])
			if d < 0 {
				d = -d
			}
			if d > worst {
				worst = d
			}
		}
	}
	return worst
}

func Solid(width, height int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func SmallLimits() codec.Limits {
	return codec.Limits{
		MaxFileBytes:   16 << 20,
		MaxPixels:      4 << 20,
		MaxFrames:      256,
		MaxFramePixels: 16 << 20,
	}
}

func OrientationTIFF(bigEndian bool, value uint16) []byte {
	var order binary.AppendByteOrder = binary.LittleEndian
	out := []byte{'I', 'I'}
	if bigEndian {
		order = binary.BigEndian
		out = []byte{'M', 'M'}
	}
	out = order.AppendUint16(out, 42)
	out = order.AppendUint32(out, 8)
	out = order.AppendUint16(out, 1)
	out = order.AppendUint16(out, 0x0112)
	out = order.AppendUint16(out, 3)
	out = order.AppendUint32(out, 1)
	out = order.AppendUint16(out, value)
	out = order.AppendUint16(out, 0)
	out = order.AppendUint32(out, 0)
	return out
}

func RequireCodecError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, codec.ErrCorrupt) || errors.Is(err, codec.ErrUnsupported) || errors.Is(err, codec.ErrTooLarge) {
		return
	}
	t.Fatalf("error does not wrap a codec sentinel: %v", err)
}

func Within(t *testing.T, limit time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("did not finish within %v", limit)
	}
}
