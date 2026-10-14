package isobmff_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/isobmff"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

func fuzzSeeds() [][]byte {
	grid := gridFile(2, 2, 64, 64, 120, 100, false)
	animated := av1Still(32, 32, isotest.Clap(16, 1, 16, 1, 0, 1, 0, 1), isotest.Irot(1))
	animated.Tracks = []isotest.Track{{Handler: "pict", Samples: 4, Width: 32, Height: 32}}
	alpha := av1Still(16, 16)
	alpha.Items = append(alpha.Items, isotest.Item{ID: 2, Type: "av01", Data: []byte("alpha"), Props: []isotest.Prop{isotest.Ispe(16, 16), isotest.AuxC(alphaURN)}})
	alpha.Refs = []isotest.Ref{{Type: "auxl", From: 2, To: []uint32{1}}}
	return [][]byte{av1Still(8, 8).Bytes(), grid.Bytes(), animated.Bytes(), alpha.Bytes(), isotest.File{Major: "mif1"}.Bytes()}
}

func FuzzOpen(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		head := data
		if len(head) > codec.SniffLen {
			head = head[:codec.SniffLen]
		}
		isobmff.Classify(head)
		isobmff.ReadBrands(head)
		file, err := isobmff.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return
		}
		file.Sequence()
		for _, id := range file.Order {
			item := file.Items[id]
			file.ReadItem(item, 4096)
			if picture, err := file.Describe(item, alphaURNs); err == nil {
				picture.Check(codec.DefaultLimits())
				picture.Transform.Orientation()
				picture.StoredSize()
			}
		}
	})
}
