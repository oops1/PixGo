package thumbs

import (
	"fmt"
	"image/color"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func displayedQuadrant(o codec.Orientation, qx, qy, w, h int) color.NRGBA {
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	x := int((float64(qx)*0.5 + 0.25) * float64(dw))
	y := int((float64(qy)*0.5 + 0.25) * float64(dh))
	col, row := exifStoredFor(o, x, y, w, h)
	switch {
	case col < w/2 && row < h/2:
		return colRed
	case col >= w/2 && row < h/2:
		return colGreen
	case col < w/2:
		return colBlue
	}
	return colWhite
}

func TestJPEGOrientationsProduceUprightThumbnails(t *testing.T) {
	const w, h, side = 160, 100, 80
	for _, big := range []bool{false, true} {
		for o := uint16(1); o <= 8; o++ {
			t.Run(fmt.Sprintf("orientation %d big-endian %v", o, big), func(t *testing.T) {
				reg, cj := jpegRegistry()
				hn := newHarness(t, reg, Options{})
				data := injectEXIF(jpegBytes(t, quadrantImage(w, h), 95), tiffWithOrientation(big, o, nil))
				path := hn.write("o.jpg", data)
				th := hn.get(path, side)
				if th.Err != nil {
					t.Fatal(th.Err)
				}
				wantW, wantH := 80, 50
				if o >= 5 {
					wantW, wantH = 50, 80
				}
				if th.Img.Bounds().Dx() != wantW || th.Img.Bounds().Dy() != wantH {
					t.Fatalf("size %v, want %dx%d", th.Img.Bounds(), wantW, wantH)
				}
				if th.SrcW != w || th.SrcH != h {
					t.Fatalf("source size %dx%d, want %dx%d", th.SrcW, th.SrcH, w, h)
				}
				if th.Orientation != codec.Orientation(o) {
					t.Fatalf("orientation %d, want %d", th.Orientation, o)
				}
				if cj.decodes.Load() != 1 {
					t.Fatalf("decodes = %d", cj.decodes.Load())
				}
				for qy := 0; qy < 2; qy++ {
					for qx := 0; qx < 2; qx++ {
						want := displayedQuadrant(codec.Orientation(o), qx, qy, w, h)
						px := int((float64(qx)*0.5 + 0.25) * float64(wantW))
						py := int((float64(qy)*0.5 + 0.25) * float64(wantH))
						if got := pixelAt(th.Img, px, py); !colorNear(got, want, 24) {
							t.Errorf("quadrant (%d,%d) at (%d,%d) = %v, want about %v", qx, qy, px, py, got, want)
						}
					}
				}
			})
		}
	}
}

func TestJPEGWithoutEXIFIsLeftAlone(t *testing.T) {
	reg, _ := jpegRegistry()
	hn := newHarness(t, reg, Options{})
	path := hn.write("plain.jpg", jpegBytes(t, quadrantImage(160, 100), 95))
	th := hn.get(path, 80)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if th.Img.Bounds().Dx() != 80 || th.Img.Bounds().Dy() != 50 || th.Orientation != codec.OrientNormal {
		t.Fatalf("got %v orientation %d", th.Img.Bounds(), th.Orientation)
	}
	if got := pixelAt(th.Img, 10, 10); !colorNear(got, colRed, 24) {
		t.Fatalf("top-left = %v", got)
	}
}

func exifThumbFixture(t *testing.T, mainW, mainH, thW, thH int, orientation uint16) []byte {
	t.Helper()
	main := solid(mainW, mainH, colBlue)
	thumb := jpegBytes(t, solid(thW, thH, colRed), 90)
	return injectEXIF(jpegBytes(t, main, 90), tiffWithOrientation(false, orientation, thumb))
}

func TestEXIFThumbnailIsUsedWhenLargeEnough(t *testing.T) {
	reg, cj := jpegRegistry()
	hn := newHarness(t, reg, Options{})
	path := hn.write("t.jpg", exifThumbFixture(t, 600, 400, 160, 107, 1))
	th := hn.get(path, 128)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if cj.decodes.Load() != 0 {
		t.Fatalf("main image was decoded %d times, the EXIF thumbnail should have been enough", cj.decodes.Load())
	}
	if cj.probes.Load() != 1 {
		t.Fatalf("probes = %d", cj.probes.Load())
	}
	if th.Img.Bounds().Dx() != 128 || th.Img.Bounds().Dy() != 85 {
		t.Fatalf("size %v, want 128x85", th.Img.Bounds())
	}
	if th.SrcW != 600 || th.SrcH != 400 {
		t.Fatalf("source size %dx%d", th.SrcW, th.SrcH)
	}
	if got := pixelAt(th.Img, 64, 42); !colorNear(got, colRed, 12) {
		t.Fatalf("centre = %v, want the red EXIF thumbnail", got)
	}
}

func TestEXIFThumbnailKeepsItsSizeBelowTheRequest(t *testing.T) {
	reg, cj := jpegRegistry()
	hn := newHarness(t, reg, Options{})
	path := hn.write("t.jpg", exifThumbFixture(t, 600, 400, 160, 107, 1))
	th := hn.get(path, 192)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if cj.decodes.Load() != 0 {
		t.Fatal("160 px thumbnail is at least 80% of 192 and must be used")
	}
	if th.Img.Bounds().Dx() != 160 || th.Img.Bounds().Dy() != 107 {
		t.Fatalf("size %v, want the unscaled 160x107", th.Img.Bounds())
	}
}

func TestEXIFThumbnailRejectedWhenTooSmall(t *testing.T) {
	reg, cj := jpegRegistry()
	hn := newHarness(t, reg, Options{})
	path := hn.write("t.jpg", exifThumbFixture(t, 600, 400, 160, 107, 1))
	th := hn.get(path, 256)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if cj.decodes.Load() != 1 {
		t.Fatalf("decodes = %d, want the main image to be decoded", cj.decodes.Load())
	}
	if th.Img.Bounds().Dx() != 256 || th.Img.Bounds().Dy() != 171 {
		t.Fatalf("size %v, want 256x171", th.Img.Bounds())
	}
	if got := pixelAt(th.Img, 128, 85); !colorNear(got, colBlue, 12) {
		t.Fatalf("centre = %v, want the blue main image", got)
	}
}

func TestEXIFThumbnailRejectedOnAspectMismatch(t *testing.T) {
	cases := []struct {
		name   string
		tw, th int
	}{
		{"square thumbnail", 160, 160},
		{"rotated thumbnail", 107, 160},
		{"slightly too wide", 164, 107},
		{"slightly too tall", 160, 103},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg, cj := jpegRegistry()
			hn := newHarness(t, reg, Options{})
			path := hn.write("t.jpg", exifThumbFixture(t, 600, 400, tc.tw, tc.th, 1))
			th := hn.get(path, 128)
			if th.Err != nil {
				t.Fatal(th.Err)
			}
			if cj.decodes.Load() != 1 {
				t.Fatalf("decodes = %d, the mismatching thumbnail must be ignored", cj.decodes.Load())
			}
			if got := pixelAt(th.Img, 64, 42); !colorNear(got, colBlue, 12) {
				t.Fatalf("centre = %v, want the main image", got)
			}
		})
	}
}

func TestEXIFThumbnailAspectWithinTolerance(t *testing.T) {
	reg, cj := jpegRegistry()
	hn := newHarness(t, reg, Options{})
	path := hn.write("t.jpg", exifThumbFixture(t, 600, 400, 160, 106, 1))
	th := hn.get(path, 128)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if cj.decodes.Load() != 0 {
		t.Fatal("160x106 is within 2% of 3:2 and should be used")
	}
}

func TestEXIFThumbnailIsOrientedLikeTheImage(t *testing.T) {
	reg, cj := jpegRegistry()
	hn := newHarness(t, reg, Options{})
	main := solid(600, 400, colBlue)
	thumb := jpegBytes(t, quadrantImage(160, 106), 95)
	data := injectEXIF(jpegBytes(t, main, 90), tiffWithOrientation(false, 6, thumb))
	path := hn.write("t.jpg", data)
	th := hn.get(path, 128)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if cj.decodes.Load() != 0 {
		t.Fatal("thumbnail must be used")
	}
	if th.Img.Bounds().Dx() != 85 || th.Img.Bounds().Dy() != 128 {
		t.Fatalf("size %v, want 85x128 after the 90 degree rotation", th.Img.Bounds())
	}
	want := displayedQuadrant(6, 0, 0, 160, 106)
	if got := pixelAt(th.Img, 20, 30); !colorNear(got, want, 24) {
		t.Fatalf("top-left = %v, want about %v", got, want)
	}
}

func TestEXIFThumbnailNeverUsedForOtherFormats(t *testing.T) {
	reg, cj := jpegRegistry()
	cj.idOver = codec.PNG
	hn := newHarness(t, reg, Options{})
	path := hn.write("t.jpg", exifThumbFixture(t, 600, 400, 160, 107, 1))
	th := hn.get(path, 128)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if cj.decodes.Load() != 1 {
		t.Fatalf("decodes = %d, only JPEG may use the embedded thumbnail", cj.decodes.Load())
	}
}

func TestEXIFThumbnailWithCorruptDataFallsBack(t *testing.T) {
	reg, cj := jpegRegistry()
	hn := newHarness(t, reg, Options{})
	thumb := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x04, 0x00, 0x00, 0xff, 0xd9, 1, 2, 3, 4, 5, 6}
	data := injectEXIF(jpegBytes(t, solid(600, 400, colBlue), 90), tiffWithOrientation(false, 1, thumb))
	path := hn.write("t.jpg", data)
	th := hn.get(path, 128)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if cj.decodes.Load() != 1 {
		t.Fatalf("decodes = %d, a broken thumbnail must fall back to the main image", cj.decodes.Load())
	}
}

func TestDecodeReceivesScaleHint(t *testing.T) {
	reg, cj := jpegRegistry()
	hn := newHarness(t, reg, Options{})
	path := hn.write("big.jpg", jpegBytes(t, quadrantImage(1024, 512), 90))
	th := hn.get(path, 128)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	o := cj.opts.Load()
	if o == nil || o.Scale != 8 {
		t.Fatalf("options %+v, want Scale 8", o)
	}
	if th.Img.Bounds().Dx() != 128 || th.Img.Bounds().Dy() != 64 {
		t.Fatalf("size %v", th.Img.Bounds())
	}
}
