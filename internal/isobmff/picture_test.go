package isobmff_test

import (
	"errors"
	"image"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/isobmff"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

var alphaURNs = []string{alphaURN, "urn:mpeg:hevc:2015:auxid:1"}

func describe(t *testing.T, f isotest.File) isobmff.Picture {
	t.Helper()
	file := open(t, f)
	picture, err := file.Describe(file.PrimaryItem(), alphaURNs)
	if err != nil {
		t.Fatal(err)
	}
	return picture
}

func describeErr(f isotest.File) error {
	data := f.Bytes()
	file, err := openBytes(data)
	if err != nil {
		return err
	}
	_, err = file.Describe(file.PrimaryItem(), alphaURNs)
	return err
}

func TestDescribeReadsSizeCodecAndBitDepth(t *testing.T) {
	hevc := func(chroma, depth byte) isotest.File {
		return isotest.File{Major: "heic", Compatible: []string{"mif1"}, Primary: 1, Items: []isotest.Item{
			{ID: 1, Type: "hvc1", Data: []byte(payload), Props: []isotest.Prop{isotest.Ispe(64, 48), isotest.HVCC(chroma, depth)}},
		}}
	}
	withProps := func(kind string, props ...isotest.Prop) isotest.File {
		return isotest.File{Major: "avif", Primary: 1, Items: []isotest.Item{{ID: 1, Type: kind, Data: []byte(payload), Props: append([]isotest.Prop{isotest.Ispe(10, 6)}, props...)}}}
	}
	tests := []struct {
		name       string
		file       isotest.File
		codec      isobmff.Codec
		depth      int
		mono       bool
		bpp        int
		width      int
		itemType   string
		hasPicture bool
	}{
		{"av1 8 bit", av1Still(33, 21), isobmff.CodecAV1, 8, false, 24, 33, "av01", true},
		{"av1 10 bit", withProps("av01", isotest.AV1C(true, false, false)), isobmff.CodecAV1, 10, false, 30, 10, "av01", true},
		{"av1 12 bit", withProps("av01", isotest.AV1C(true, true, false)), isobmff.CodecAV1, 12, false, 36, 10, "av01", true},
		{"av1 monochrome", withProps("av01", isotest.AV1C(false, false, true)), isobmff.CodecAV1, 8, true, 8, 10, "av01", true},
		{"hevc 8 bit", hevc(1, 8), isobmff.CodecHEVC, 8, false, 24, 64, "hvc1", true},
		{"hevc 10 bit", hevc(1, 10), isobmff.CodecHEVC, 10, false, 30, 64, "hvc1", true},
		{"hevc monochrome", hevc(0, 12), isobmff.CodecHEVC, 12, true, 12, 64, "hvc1", true},
		{"pixel information only", withProps("av01", isotest.Pixi(10, 10, 10)), isobmff.CodecAV1, 10, false, 30, 10, "av01", true},
		{"single channel pixel information", withProps("av01", isotest.Pixi(12)), isobmff.CodecAV1, 12, true, 12, 10, "av01", true},
		{"no configuration at all", withProps("av01"), isobmff.CodecAV1, 8, false, 24, 10, "av01", true},
		{"other item type", withProps("jpeg"), isobmff.CodecOther, 8, false, 24, 10, "jpeg", true},
		{"overlay item type", withProps("iovl"), isobmff.CodecOther, 8, false, 24, 10, "iovl", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := describe(t, tt.file)
			if p.Codec != tt.codec || p.BitDepth != tt.depth || p.Monochrome != tt.mono || p.BitsPerPixel() != tt.bpp || p.Width != tt.width || p.ItemType != tt.itemType {
				t.Fatalf("%+v bpp %d", p, p.BitsPerPixel())
			}
			if p.Alpha || p.Grid != nil {
				t.Fatalf("unexpected extras: %+v", p)
			}
		})
	}
}

func TestDescribeAddsAlphaBitsToThePixelSize(t *testing.T) {
	f := av1Still(16, 16)
	f.Items = append(f.Items, isotest.Item{ID: 2, Type: "av01", Data: []byte("alpha-plane"), Props: []isotest.Prop{isotest.Ispe(16, 16), isotest.AuxC(alphaURN), isotest.AV1C(false, false, true)}})
	f.Refs = []isotest.Ref{{Type: "auxl", From: 2, To: []uint32{1}}}
	p := describe(t, f)
	if !p.Alpha || p.BitsPerPixel() != 32 {
		t.Fatalf("%+v", p)
	}
}

func TestDescribeRecognisesOnlyTheListedAlphaTypes(t *testing.T) {
	build := func(urn string, link bool) isotest.File {
		f := av1Still(16, 16)
		f.Items = append(f.Items, isotest.Item{ID: 2, Type: "av01", Data: []byte("alpha-plane"), Props: []isotest.Prop{isotest.Ispe(16, 16), isotest.AuxC(urn)}})
		if link {
			f.Refs = []isotest.Ref{{Type: "auxl", From: 2, To: []uint32{1}}}
		}
		return f
	}
	tests := []struct {
		name string
		file isotest.File
		want bool
	}{
		{"mpeg alpha", build(alphaURN, true), true},
		{"hevc alpha", build("urn:mpeg:hevc:2015:auxid:1", true), true},
		{"depth map", build("urn:mpeg:mpegB:cicp:systems:auxiliary:depth", true), false},
		{"not linked", build(alphaURN, false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describe(t, tt.file).Alpha; got != tt.want {
				t.Fatalf("alpha = %v", got)
			}
		})
	}
	file := open(t, build(alphaURN, true))
	if file.AlphaItem(1, []string{"urn:mpeg:hevc:2015:auxid:1"}) != nil {
		t.Fatal("alpha found through an unlisted type")
	}
	if file.AlphaItem(7, alphaURNs) != nil {
		t.Fatal("alpha found for a missing item")
	}
}

func TestDescribeRejectsPicturesWithoutUsableSizes(t *testing.T) {
	build := func(props ...isotest.Prop) isotest.File {
		return isotest.File{Major: "avif", Primary: 1, Items: []isotest.Item{{ID: 1, Type: "av01", Data: []byte(payload), Props: props}}}
	}
	tests := []struct {
		name string
		file isotest.File
	}{
		{"no ispe", build(isotest.AV1C(false, false, false))},
		{"zero width", build(isotest.Ispe(0, 8))},
		{"zero height", build(isotest.Ispe(8, 0))},
		{"width over the dimension limit", build(isotest.Ispe(isobmff.MaxDimension+1, 8))},
		{"height over the dimension limit", build(isotest.Ispe(8, isobmff.MaxDimension+1))},
		{"clean aperture larger than the picture", build(isotest.Ispe(8, 8), isotest.Clap(9, 1, 8, 1, 0, 1, 0, 1))},
		{"clean aperture with a zero denominator", build(isotest.Ispe(8, 8), isotest.Clap(4, 0, 4, 1, 0, 1, 0, 1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := describeErr(tt.file); !errors.Is(err, codec.ErrCorrupt) {
				t.Fatalf("error %v", err)
			}
		})
	}
}

func TestCropFollowsTheCleanApertureArithmetic(t *testing.T) {
	tests := []struct {
		name  string
		clap  [8]int32
		w, h  int
		want  image.Rectangle
		valid bool
	}{
		{"whole picture", [8]int32{10, 1, 8, 1, 0, 1, 0, 1}, 10, 8, image.Rect(0, 0, 10, 8), true},
		{"centred", [8]int32{4, 1, 4, 1, 0, 1, 0, 1}, 10, 8, image.Rect(3, 2, 7, 6), true},
		{"shifted right", [8]int32{4, 1, 4, 1, 2, 1, 0, 1}, 10, 8, image.Rect(5, 2, 9, 6), true},
		{"shifted by half pixels", [8]int32{4, 1, 4, 1, 1, 2, 1, 2}, 11, 9, image.Rect(4, 3, 8, 7), true},
		{"scaled fractions", [8]int32{8, 2, 6, 2, 0, 1, 0, 1}, 10, 9, image.Rect(3, 3, 7, 6), true},
		{"fractional width", [8]int32{9, 2, 4, 1, 0, 1, 0, 1}, 10, 8, image.Rectangle{}, false},
		{"fractional height", [8]int32{4, 1, 9, 2, 0, 1, 0, 1}, 10, 8, image.Rectangle{}, false},
		{"fractional origin", [8]int32{4, 1, 4, 1, 1, 2, 0, 1}, 10, 8, image.Rectangle{}, false},
		{"negative width", [8]int32{-4, 1, 4, 1, 0, 1, 0, 1}, 10, 8, image.Rectangle{}, false},
		{"zero height", [8]int32{4, 1, 0, 1, 0, 1, 0, 1}, 10, 8, image.Rectangle{}, false},
		{"negative denominator", [8]int32{4, -1, 4, 1, 0, 1, 0, 1}, 10, 8, image.Rectangle{}, false},
		{"zero offset denominator", [8]int32{4, 1, 4, 1, 0, 0, 0, 1}, 10, 8, image.Rectangle{}, false},
		{"origin left of the picture", [8]int32{4, 1, 4, 1, -9, 1, 0, 1}, 10, 8, image.Rectangle{}, false},
		{"origin above the picture", [8]int32{4, 1, 4, 1, 0, 1, -9, 1}, 10, 8, image.Rectangle{}, false},
		{"runs past the right edge", [8]int32{4, 1, 4, 1, 9, 1, 0, 1}, 10, 8, image.Rectangle{}, false},
		{"runs past the bottom edge", [8]int32{4, 1, 4, 1, 0, 1, 9, 1}, 10, 8, image.Rectangle{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var clap [8]uint32
			for i, v := range tt.clap {
				clap[i] = uint32(v)
			}
			rect, ok := isobmff.Transform{Clap: &clap}.Crop(tt.w, tt.h)
			if ok != tt.valid || (ok && rect != tt.want) {
				t.Fatalf("%v %v, want %v %v", rect, ok, tt.want, tt.valid)
			}
		})
	}
	if rect, ok := (isobmff.Transform{}).Crop(5, 4); !ok || rect != image.Rect(0, 0, 5, 4) {
		t.Fatalf("no clap: %v %v", rect, ok)
	}
}

func gridOf(grid [][]int, rot, mirrorAxis int) [][]int {
	for i := 0; i < rot; i++ {
		h, w := len(grid), len(grid[0])
		next := make([][]int, w)
		for y := range next {
			next[y] = make([]int, h)
			for x := range next[y] {
				next[y][x] = grid[x][w-1-y]
			}
		}
		grid = next
	}
	out := make([][]int, len(grid))
	for y := range grid {
		out[y] = append([]int(nil), grid[y]...)
	}
	switch mirrorAxis {
	case 0:
		for y := range out {
			out[y] = grid[len(grid)-1-y]
		}
	case 1:
		for y := range out {
			for x := range out[y] {
				out[y][x] = grid[y][len(grid[y])-1-x]
			}
		}
	}
	return out
}

func displayed(o codec.Orientation, src [][]int) [][]int {
	h, w := len(src), len(src[0])
	dw, dh := o.Size(w, h)
	out := make([][]int, dh)
	for v := range out {
		out[v] = make([]int, dw)
		for u := range out[v] {
			x, y := o.ToSource(float64(u)+0.5, float64(v)+0.5, float64(w), float64(h))
			out[v][u] = src[int(y)][int(x)]
		}
	}
	return out
}

func sameGrid(a, b [][]int) bool {
	if len(a) != len(b) {
		return false
	}
	for y := range a {
		if len(a[y]) != len(b[y]) {
			return false
		}
		for x := range a[y] {
			if a[y][x] != b[y][x] {
				return false
			}
		}
	}
	return true
}

func TestTransformOrientationMatchesRotateThenMirror(t *testing.T) {
	src := [][]int{{0, 1, 2}, {3, 4, 5}}
	for rotation := uint8(0); rotation < 4; rotation++ {
		for _, mirror := range []int{-1, 0, 1} {
			transform := isobmff.Transform{Rotation: rotation, Mirrored: mirror >= 0, Axis: uint8(max(mirror, 0))}
			want := gridOf(src, int(rotation), mirror)
			got := displayed(transform.Orientation(), src)
			if !sameGrid(got, want) {
				t.Errorf("rotation %d mirror %d: orientation %d shows %v, want %v", rotation, mirror, transform.Orientation(), got, want)
			}
		}
	}
}

func gridFile(rows, cols int, tileW, tileH, outW, outH uint32, wide bool) isotest.File {
	f := isotest.File{Major: "heic", Compatible: []string{"mif1"}, Primary: 100}
	var tiles []uint32
	for i := 0; i < rows*cols; i++ {
		id := uint32(i + 1)
		tiles = append(tiles, id)
		f.Items = append(f.Items, isotest.Item{ID: id, Type: "hvc1", Data: []byte(payload), Props: []isotest.Prop{isotest.Ispe(tileW, tileH), isotest.HVCC(1, 8)}})
	}
	f.Items = append(f.Items, isotest.Item{ID: 100, Type: "grid", Data: isotest.GridDescriptor(rows, cols, outW, outH, wide), Props: []isotest.Prop{isotest.Ispe(outW, outH)}})
	f.Refs = []isotest.Ref{{Type: "dimg", From: 100, To: tiles}}
	return f
}

func TestDescribeReadsGrids(t *testing.T) {
	for _, wide := range []bool{false, true} {
		p := describe(t, gridFile(2, 3, 512, 512, 1500, 1000, wide))
		if p.Grid == nil || p.Grid.Rows != 2 || p.Grid.Cols != 3 || p.Grid.Width != 1500 || p.Grid.Height != 1000 || len(p.Grid.Tiles) != 6 {
			t.Fatalf("%+v", p.Grid)
		}
		if p.Grid.TileWidth != 512 || p.Grid.TileHeight != 512 || p.Codec != isobmff.CodecHEVC || p.ItemType != "grid of hvc1" || p.Width != 1500 {
			t.Fatalf("%+v", p)
		}
		if p.BitDepth != 8 || p.Monochrome {
			t.Fatalf("depth %d", p.BitDepth)
		}
	}
}

func TestDescribeReportsAlphaThatLivesOnGridTiles(t *testing.T) {
	f := gridFile(1, 2, 64, 64, 128, 64, false)
	for id := uint32(1); id <= 2; id++ {
		f.Items = append(f.Items, isotest.Item{ID: 200 + id, Type: "hvc1", Data: []byte("a"), Props: []isotest.Prop{isotest.Ispe(64, 64), isotest.AuxC(alphaURN)}})
		f.Refs = append(f.Refs, isotest.Ref{Type: "auxl", From: 200 + id, To: []uint32{id}})
	}
	if !describe(t, f).Alpha {
		t.Fatal("tile alpha not found")
	}
	f.Refs = f.Refs[:2]
	if describe(t, f).Alpha {
		t.Fatal("alpha on only some tiles was accepted")
	}
	g := gridFile(1, 2, 64, 64, 128, 64, false)
	g.Items = append(g.Items, isotest.Item{ID: 300, Type: "grid", Data: isotest.GridDescriptor(1, 2, 128, 64, false), Props: []isotest.Prop{isotest.Ispe(128, 64), isotest.AuxC(alphaURN)}})
	g.Refs = append(g.Refs, isotest.Ref{Type: "auxl", From: 300, To: []uint32{100}})
	if !describe(t, g).Alpha {
		t.Fatal("grid alpha not found")
	}
}

func TestDescribeRejectsBrokenGrids(t *testing.T) {
	mutate := func(change func(f *isotest.File)) isotest.File {
		f := gridFile(2, 2, 64, 64, 128, 128, false)
		change(&f)
		return f
	}
	gridItem := func(f *isotest.File) *isotest.Item { return &f.Items[len(f.Items)-1] }
	tests := []struct {
		name string
		file isotest.File
	}{
		{"missing tile reference", mutate(func(f *isotest.File) { f.Refs[0].To = f.Refs[0].To[:3] })},
		{"tile reference to nothing", mutate(func(f *isotest.File) { f.Refs[0].To[2] = 999 })},
		{"tile without size", mutate(func(f *isotest.File) { f.Items[1].Props = f.Items[1].Props[1:] })},
		{"tile of another size", mutate(func(f *isotest.File) { f.Items[2].Props[0] = isotest.Ispe(32, 64) })},
		{"tile of another kind", mutate(func(f *isotest.File) { f.Items[3].Type = "av01" })},
		{"tiles too small", mutate(func(f *isotest.File) {
			*gridItem(f) = isotest.Item{ID: 100, Type: "grid", Data: isotest.GridDescriptor(2, 2, 129, 128, false), Props: []isotest.Prop{isotest.Ispe(128, 128)}}
		})},
		{"descriptor too short", mutate(func(f *isotest.File) { gridItem(f).Data = []byte{0, 0, 1, 1} })},
		{"zero output size", mutate(func(f *isotest.File) { gridItem(f).Data = isotest.GridDescriptor(2, 2, 0, 128, false) })},
		{"output size over the limit", mutate(func(f *isotest.File) {
			gridItem(f).Data = isotest.GridDescriptor(2, 2, isobmff.MaxDimension+1, 128, true)
		})},
		{"picture larger than the grid", mutate(func(f *isotest.File) { gridItem(f).Props = []isotest.Prop{isotest.Ispe(200, 128)} })},
		{"tile with an unusable size", mutate(func(f *isotest.File) { f.Items[0].Props[0] = isotest.Ispe(0, 64) })},
		{"grid without data", mutate(func(f *isotest.File) { gridItem(f).NoData = true })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := describeErr(tt.file); !errors.Is(err, codec.ErrCorrupt) {
				t.Fatalf("error %v", err)
			}
		})
	}
}

func TestPictureCheckAppliesPixelAndTileBudgets(t *testing.T) {
	lim := codec.Limits{MaxFileBytes: 1 << 30, MaxPixels: 1 << 20, MaxFrames: 10, MaxFramePixels: 1 << 21}
	tests := []struct {
		name string
		file isotest.File
		want error
	}{
		{"fits", av1Still(1000, 1000), nil},
		{"too many pixels", av1Still(2000, 2000), codec.ErrTooLarge},
		{"grid fits", gridFile(2, 2, 512, 512, 1000, 1000, false), nil},
		{"grid output too large", gridFile(2, 2, 2000, 2000, 4000, 4000, true), codec.ErrTooLarge},
		{"grid tiles over the frame budget", gridFile(8, 8, 1024, 1024, 1024, 1024, false), codec.ErrTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := describe(t, tt.file)
			err := p.Check(lim)
			if !errors.Is(err, tt.want) || (tt.want == nil && err != nil) {
				t.Fatalf("error %v, want %v", err, tt.want)
			}
		})
	}
}

func TestDescribeReadsTransformsFromTheItem(t *testing.T) {
	p := describe(t, av1Still(10, 6, isotest.Clap(6, 1, 4, 1, 0, 1, 0, 1), isotest.Irot(3), isotest.Imir(1)))
	if p.Transform.Clap == nil || p.Transform.Rotation != 3 || !p.Transform.Mirrored || p.Transform.Axis != 1 {
		t.Fatalf("%+v", p.Transform)
	}
	width, height, ok := p.StoredSize()
	if !ok || width != 6 || height != 4 {
		t.Fatalf("stored %dx%d %v", width, height, ok)
	}
	rect, ok := p.Cropped()
	if !ok || rect != image.Rect(2, 1, 8, 5) {
		t.Fatalf("cropped %v %v", rect, ok)
	}
}

func TestFrameLimitsLeaveRoomForWideSamples(t *testing.T) {
	lim := codec.Limits{MaxFileBytes: 1, MaxPixels: 2, MaxFrames: 3, MaxFramePixels: 3000}
	narrow := describe(t, av1Still(8, 8))
	wideFile := av1Still(8, 8)
	wideFile.Items[0].Props = []isotest.Prop{isotest.Ispe(8, 8), isotest.AV1C(true, false, false)}
	wide := describe(t, wideFile)
	if got := narrow.FrameLimits(lim); got != lim {
		t.Fatalf("8 bit: %+v", got)
	}
	if got := wide.FrameLimits(lim); got.MaxFramePixels != 1000 || got.MaxFrames != 3 || got.MaxPixels != 2 {
		t.Fatalf("10 bit: %+v", got)
	}
}
