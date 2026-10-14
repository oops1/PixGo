package raw

import (
	"bytes"
	"context"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func fuzzSeeds(f *testing.F) {
	p := makePreviews(f)
	for _, s := range allSamples(f) {
		f.Add(s.data)
	}
	f.Add(carvedFile(p, 200))
	f.Add(lastPreviewFile(p))
	f.Add(rafWithOffsets(0xffffffff, 0xffffffff))
	f.Add(patchRAF(rafFile(p.large), 160, 0))
	f.Add(withBytes(orfFile(p, "new"), 2, 'R', 'S'))
	f.Add(bytes.Repeat([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 2}, 40))
	f.Add([]byte{'I', 'I', 42, 0, 8, 0, 0, 0, 0xff, 0xff})
	func() {
		t := standardTIFF(little)
		prev := t.later(losslessJPEG(100, 100))
		ifd0, _ := t.ifd(join(stripTags(prev, compressionJPEG), []entry{dngVersion()})...)
		t.setFirst(ifd0)
		f.Add(t.data())
	}()
}

func FuzzSniff(f *testing.F) {
	for _, s := range allSamples(f) {
		f.Add(head(s.data))
	}
	f.Add([]byte("FUJIFILMCCD-RAW "))
	f.Add([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'c', 'r', 'x', ' '})
	f.Fuzz(func(t *testing.T, data []byte) {
		Decoder{}.Sniff(data)
	})
}

func FuzzProbe(f *testing.F) {
	fuzzSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), testimg.SmallLimits())
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
	})
}

func FuzzDecode(f *testing.F) {
	fuzzSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		Decoder{}.Sniff(head(data))
		img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: testimg.SmallLimits()})
		if err == nil && (img == nil || img.First() == nil || !img.Info.Preview || img.Info.Format != codec.RAW) {
			t.Fatal("success without a preview image")
		}
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
	})
}
