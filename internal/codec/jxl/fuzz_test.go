package jxl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func fuzzSeeds(f *testing.F) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.jxl"))
	if err != nil || len(files) == 0 {
		f.Fatal("no seeds")
	}
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add(encodeLossless(f, 17, 9, 41))
	f.Add(buildAnimation(f, animationSpec{width: 16, height: 16, loops: 2, clips: []clipSpec{{duration: 1}, {hidden: true}, {duration: 2}}}))
	f.Add(buildAnimation(f, animationSpec{width: 16, height: 12, orientation: 6, preview: true, clips: []clipSpec{{duration: 1}, {duration: 2}}}))
	f.Add(buildStill(f, 24, 10, 8))
	f.Add(contained(encodeLossless(f, 16, 16, 42), box("Exif", []byte("exif"))))
	f.Add(boxed(append([][]byte{containerSignature, ftypBox()}, partial(encodeLossless(f, 16, 16, 43), 3)...)...))
	bomb := baseHeader(1<<30, 1<<30)
	f.Add(streamOf(bomb, frameSpec{allDefault: true, entries: []uint32{1}}.write(bomb, 1<<30, 1<<30)))
}

func requireClassified(t *testing.T, err error) {
	t.Helper()
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		testimg.RequireCodecError(t, err)
	}
}

func FuzzDecode(f *testing.F) {
	fuzzSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		img, err := decode(data, codec.Options{Limits: testimg.SmallLimits(), Frames: codec.AllFrames})
		requireClassified(t, err)
		if err != nil {
			return
		}
		if len(img.Frames) == 0 || img.Info.Frames != len(img.Frames) {
			t.Fatalf("%d frames, info %+v", len(img.Frames), img.Info)
		}
		for _, frame := range img.Frames {
			if frame.Pix == nil || frame.Pix.Bounds() != img.First().Bounds() || frame.Pix.Bounds().Dx() != img.Info.Width || frame.Pix.Bounds().Dy() != img.Info.Height {
				t.Fatalf("frame %v, info %+v", frame.Pix, img.Info)
			}
		}
	})
}

func FuzzProbe(f *testing.F) {
	fuzzSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		info, err := probe(data, codec.DefaultLimits())
		requireClassified(t, err)
		if err != nil {
			return
		}
		if info.Width <= 0 || info.Height <= 0 || info.Frames < 1 || info.Orientation != codec.OrientNormal || info.Format != codec.JXL {
			t.Fatalf("%+v", info)
		}
		head := data
		if len(head) > codec.SniffLen {
			head = head[:codec.SniffLen]
		}
		if !(Decoder{}).Sniff(head) {
			t.Fatalf("a probed file is not sniffed: %q", head[:min(len(head), 16)])
		}
	})
}
