package webp

import (
	"bytes"
	"context"
	"image"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func fuzzLimits() codec.Limits {
	return codec.Limits{
		MaxFileBytes:   4 << 20,
		MaxPixels:      1 << 20,
		MaxFrames:      64,
		MaxFramePixels: 4 << 20,
	}
}

func addSeeds(f *testing.F) {
	for _, name := range realFiles {
		f.Add(readTestdata(f, name))
	}
	f.Add(riff(vp8(10, 10)))
	f.Add(riff(vp8l(7, 7, true)))
	f.Add(riff(vp8x(flagAnimation|flagAlpha, 40, 30), animChunk(2), frameChunk(0, 0, 40, 30, 40, 0), exifChunk(testimg.OrientationTIFF(false, 6), true)))
	f.Add(riff(vp8x(0x08, 64, 32), vp8(64, 32), exifChunk(testimg.OrientationTIFF(true, 3), false)))
	f.Add(riff(vp8x(0x2c, 20, 10), chunk("ICCP", []byte{1, 2, 3}), vp8(20, 10), chunk("XMP ", []byte("<x/>"))))
	f.Add(vpxEncode(f, withAlpha(smooth(20, 14)), optionsLossless()))
	f.Add(vpxEncode(f, withAlpha(smooth(23, 17)), vpxLossy(60)))
	f.Add(vpxEncode(f, smooth(33, 9), vpxLossy(30)))
	f.Add(buildAnimation(f, 24, 16, 0, compositingSpecs(false)))
	f.Add(buildAnimation(f, 24, 16, 3, compositingSpecs(true)))
	f.Add(encodeAnimation(f, []*image.NRGBA{numbered(11, 9, 0), numbered(11, 9, 1), numbered(11, 9, 2)}, []int{10, 20, 30}, 1))
}

func FuzzDecode(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := fuzzLimits()
		ctx := context.Background()
		Decoder{}.Sniff(data)
		walkChunks(ctx, bytes.NewReader(data), int64(len(data)))
		for _, frames := range []codec.FrameSel{codec.FirstFrame, codec.AllFrames} {
			img, err := Decoder{}.Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim, Frames: frames})
			if err != nil {
				testimg.RequireCodecError(t, err)
				continue
			}
			if img == nil || len(img.Frames) == 0 || img.Info.Frames < len(img.Frames) && frames == codec.AllFrames {
				t.Fatalf("success without frames: %+v", img)
			}
			if frames == codec.FirstFrame && len(img.Frames) != 1 {
				t.Fatalf("%d frames for the first frame", len(img.Frames))
			}
			for i, frame := range img.Frames {
				if frame.Pix == nil {
					t.Fatalf("frame %d has no pixels", i)
				}
				bounds := frame.Pix.Bounds()
				if bounds.Min.X != 0 || bounds.Min.Y != 0 || bounds.Dx() != img.Info.Width || bounds.Dy() != img.Info.Height {
					t.Fatalf("frame %d: %v for %dx%d", i, bounds, img.Info.Width, img.Info.Height)
				}
				if frame.Delay < 0 {
					t.Fatalf("frame %d: delay %v", i, frame.Delay)
				}
			}
			if int64(img.Info.Width)*int64(img.Info.Height) > lim.MaxPixels || img.Scale != 1 || img.Info.Format != codec.WebP {
				t.Fatalf("%+v", img.Info)
			}
		}
	})
}

func FuzzProbe(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := fuzzLimits()
		info, err := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
		if err != nil {
			testimg.RequireCodecError(t, err)
			return
		}
		if info.Width < 1 || info.Height < 1 || int64(info.Width)*int64(info.Height) > lim.MaxPixels || info.Frames < 1 || info.Format != codec.WebP {
			t.Fatalf("%+v", info)
		}
		if info.Animated && info.Frames < 2 || !info.Orientation.Valid() || info.BitDepth != 24 && info.BitDepth != 32 || info.HasAlpha != (info.BitDepth == 32) {
			t.Fatalf("%+v", info)
		}
		if !(Decoder{}).Sniff(data) {
			t.Fatalf("probed a file that is not sniffed as WebP: %+v", info)
		}
	})
}
