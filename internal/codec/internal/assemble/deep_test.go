package assemble

import (
	"context"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func wide(width, height int) *image.NRGBA64 {
	img := image.NewNRGBA64(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA64(x, y, color.NRGBA64{R: uint16(x*1000 + 1), G: uint16(y*1000 + 2), B: uint16(x + y), A: 0xffff})
		}
	}
	return img
}

func TestColorMapsCodePointsAndProfiles(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 30)
	got := Color(profile, 9, 16, 9, true)
	if string(got.ICC) != string(profile) || got.CICP == nil || *got.CICP != (codec.CICP{Primaries: 9, Transfer: 16, Matrix: 9, FullRange: true}) {
		t.Fatalf("%+v", got)
	}
	if got := Color(nil, 2, 2, 6, false); !got.Empty() {
		t.Fatalf("unspecified description reported: %+v", got.CICP)
	}
	if got := Color(nil, 1, 2, 1, false); got.CICP == nil || got.CICP.Primaries != 1 {
		t.Fatal("half specified description dropped")
	}
	if got := Color(nil, 300, 13, 0, false); got.CICP != nil {
		t.Fatal("code point outside a byte accepted")
	}
	if got := Color([]byte("junk"), 2, 2, 2, false); !got.Empty() {
		t.Fatal("garbage profile accepted")
	}
}

func TestWantsDeepFollowsTheSourceDepthOrTheTransfer(t *testing.T) {
	eight := Source{Images: []image.Image{labelled(4, 4)}}
	if WantsDeep(eight, codec.ColorInfo{}) {
		t.Fatal("8 bit sRGB")
	}
	if !WantsDeep(eight, codec.ColorInfo{CICP: &codec.CICP{Transfer: 16}}) || !WantsDeep(eight, codec.ColorInfo{CICP: &codec.CICP{Transfer: 18}}) {
		t.Fatal("8 bit HDR")
	}
	if WantsDeep(eight, codec.ColorInfo{CICP: &codec.CICP{Transfer: 13}}) {
		t.Fatal("8 bit sRGB CICP")
	}
	if !WantsDeep(Source{Images: []image.Image{wide(4, 4)}}, codec.ColorInfo{}) || !WantsDeep(Source{Images: []image.Image{image.NewGray16(image.Rect(0, 0, 2, 2))}}, codec.ColorInfo{}) {
		t.Fatal("16 bit source")
	}
	if WantsDeep(Source{}, codec.ColorInfo{}) {
		t.Fatal("empty source")
	}
}

func TestDeepFramesCropAndCheckSizes(t *testing.T) {
	src := Source{Images: []image.Image{wide(10, 8), wide(10, 8)}, Delays: []float64{0.04, 0.05}}
	crop := func(w, h int) (image.Rectangle, bool) { return image.Rect(2, 1, w-1, h-2), true }
	frames, err := DeepFrames(context.Background(), "test", src, crop)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 || frames[0].Pix != nil || frames[0].Deep.Rect != image.Rect(0, 0, 7, 5) || frames[1].Delay != 50*time.Millisecond {
		t.Fatalf("%v %v", frames[0].Deep.Rect, frames[1].Delay)
	}
	want := wide(10, 8).NRGBA64At(2, 1)
	if frames[0].Deep.NRGBA64At(0, 0) != want {
		t.Fatal("crop origin")
	}
	if _, err := DeepFrames(context.Background(), "test", Source{Images: []image.Image{wide(10, 8)}}, func(w, h int) (image.Rectangle, bool) { return image.Rectangle{}, false }); err == nil {
		t.Fatal("bad aperture accepted")
	}
	if _, err := DeepFrames(context.Background(), "test", Source{Images: []image.Image{wide(10, 8), wide(9, 8)}}, nil); err == nil {
		t.Fatal("mismatched frames accepted")
	}
	if _, err := DeepFrames(context.Background(), "test", Source{}, nil); err == nil {
		t.Fatal("no frames accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DeepFrames(ctx, "test", src, nil); err == nil {
		t.Fatal("cancelled context ignored")
	}
}

func TestDeepFramesWidenEightBitFrames(t *testing.T) {
	frames, err := DeepFrames(context.Background(), "test", Source{Images: []image.Image{labelled(5, 3)}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := frames[0].Deep.NRGBA64At(4, 2); got != (color.NRGBA64{R: 4 * 257, G: 2 * 257, B: 6 * 257, A: 0xffff}) {
		t.Fatalf("%v", got)
	}
}

func TestCropDeepReturnsTheSameImageForTheFullRectangle(t *testing.T) {
	img := wide(6, 6)
	if CropDeep(img, img.Bounds()) != img {
		t.Fatal("copied")
	}
}
