package heic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	gheic "github.com/gen2brain/h265/heic"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

func fuzzSeeds(f *testing.F) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.heic"))
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
	f.Add(encode(f, smoothImage(33, 21, true), gheic.EncodeOptions{}))
	f.Add(isotest.File{Major: "heic", Compatible: []string{"mif1", "heic"}, Primary: 1, Items: []isotest.Item{hevcItem(1, 100000, 100000)}}.Bytes())
	f.Add(isotest.File{Major: "mif1", Compatible: []string{"miaf"}, NoMeta: true}.Bytes())
	f.Add(phoneLikeFile(f, 1, 0, 6))
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
		if len(img.Frames) == 0 || img.Info.Frames != len(img.Frames) && img.Info.Frames > 1 {
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
		if info.Width <= 0 || info.Height <= 0 || info.Frames < 1 || !info.Orientation.Valid() || info.Format != codec.HEIC {
			t.Fatalf("%+v", info)
		}
		head := data
		if len(head) > codec.SniffLen {
			head = head[:codec.SniffLen]
		}
		(Decoder{}).Sniff(head)
	})
}
