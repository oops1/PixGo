package assemble

import (
	"context"
	"errors"
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

func labelled(width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	return img
}

func TestDelayClampsAndConverts(t *testing.T) {
	tests := []struct {
		seconds float64
		want    time.Duration
	}{
		{0, 0},
		{-1, 0},
		{math.NaN(), 0},
		{0.04, 40 * time.Millisecond},
		{2.5, 2500 * time.Millisecond},
		{1e12, time.Hour},
		{math.Inf(1), time.Hour},
	}
	for _, tt := range tests {
		if got := Delay(tt.seconds); got != tt.want {
			t.Errorf("Delay(%v) = %v, want %v", tt.seconds, got, tt.want)
		}
	}
}

func TestCropCopiesTheRectangle(t *testing.T) {
	src := labelled(10, 8)
	if Crop(src, src.Bounds()) != src {
		t.Fatal("cropping to the full rectangle must not copy")
	}
	got := Crop(src, image.Rect(3, 2, 7, 6))
	if got.Bounds() != image.Rect(0, 0, 4, 4) {
		t.Fatalf("%v", got.Bounds())
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if got.NRGBAAt(x, y) != src.NRGBAAt(x+3, y+2) {
				t.Fatalf("(%d,%d)", x, y)
			}
		}
	}
	got.Pix[0] = 99
	if src.Pix[src.PixOffset(3, 2)] == 99 {
		t.Fatal("the crop shares memory with its source")
	}
}

func TestFramesConvertsCropsAndTimesEveryImage(t *testing.T) {
	grey := image.NewGray(image.Rect(0, 0, 6, 4))
	grey.Pix[0] = 200
	wide := image.NewNRGBA64(image.Rect(0, 0, 6, 4))
	wide.SetNRGBA64(0, 0, color.NRGBA64{R: 0xabcd, G: 0x1234, B: 0xffff, A: 0xffff})
	src := Source{Images: []image.Image{labelled(6, 4), grey, wide}, Delays: []float64{0.1, 0.25}}
	centre := func(width, height int) (image.Rectangle, bool) {
		return image.Rect(1, 1, width-1, height-1), true
	}
	frames, err := Frames(context.Background(), "demo", src, centre)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 || frames[0].Delay != 100*time.Millisecond || frames[1].Delay != 250*time.Millisecond || frames[2].Delay != 0 {
		t.Fatalf("%+v", frames)
	}
	for _, frame := range frames {
		if frame.Pix.Bounds() != image.Rect(0, 0, 4, 2) {
			t.Fatalf("%v", frame.Pix.Bounds())
		}
	}
	plain, err := Frames(context.Background(), "demo", Source{Images: []image.Image{wide}}, nil)
	if err != nil || plain[0].Pix.NRGBAAt(0, 0) != (color.NRGBA{R: 0xab, G: 0x12, B: 0xff, A: 0xff}) {
		t.Fatalf("%v %v", err, plain[0].Pix.NRGBAAt(0, 0))
	}
}

func TestFramesRefusesWhatDoesNotAddUp(t *testing.T) {
	refuse := func(int, int) (image.Rectangle, bool) { return image.Rectangle{}, false }
	if _, err := Frames(context.Background(), "demo", Source{}, nil); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("no frames: %v", err)
	}
	if _, err := Frames(context.Background(), "demo", Source{Images: []image.Image{labelled(4, 4)}}, refuse); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("bad crop: %v", err)
	}
	if _, err := Frames(context.Background(), "demo", Source{Images: []image.Image{labelled(4, 4), labelled(5, 4)}}, nil); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("frames of different sizes: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Frames(ctx, "demo", Source{Images: []image.Image{labelled(4, 4)}}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}
