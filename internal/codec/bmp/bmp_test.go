package bmp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"math/rand"
	"testing"
	"time"

	xbmp "golang.org/x/image/bmp"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

type spec struct {
	dibSize     int
	width       int
	height      int
	bpp         int
	compression uint32
	colorsUsed  uint32
	masks       [4]uint32
	palette     []color.RGBA
	gap         []byte
	pixels      []byte
}

func build(s spec) []byte {
	if s.dibSize == 0 {
		s.dibSize = 40
	}
	le := binary.LittleEndian
	var dib []byte
	dib = le.AppendUint32(dib, uint32(s.dibSize))
	if s.dibSize == 12 {
		dib = le.AppendUint16(dib, uint16(s.width))
		dib = le.AppendUint16(dib, uint16(s.height))
		dib = le.AppendUint16(dib, 1)
		dib = le.AppendUint16(dib, uint16(s.bpp))
	} else {
		dib = le.AppendUint32(dib, uint32(int32(s.width)))
		dib = le.AppendUint32(dib, uint32(int32(s.height)))
		dib = le.AppendUint16(dib, 1)
		dib = le.AppendUint16(dib, uint16(s.bpp))
		dib = le.AppendUint32(dib, s.compression)
		dib = le.AppendUint32(dib, uint32(len(s.pixels)))
		dib = le.AppendUint32(dib, 2835)
		dib = le.AppendUint32(dib, 2835)
		dib = le.AppendUint32(dib, s.colorsUsed)
		dib = le.AppendUint32(dib, 0)
		if s.dibSize >= 52 {
			for i := 0; i < 3; i++ {
				dib = le.AppendUint32(dib, s.masks[i])
			}
		}
		if s.dibSize >= 56 {
			dib = le.AppendUint32(dib, s.masks[3])
		}
		for len(dib) < s.dibSize {
			dib = append(dib, 0)
		}
	}
	var masks []byte
	if s.dibSize == 40 && (s.compression == compressionBitfields || s.compression == compressionAlphaBitfields) {
		count := 3
		if s.compression == compressionAlphaBitfields {
			count = 4
		}
		for i := 0; i < count; i++ {
			masks = le.AppendUint32(masks, s.masks[i])
		}
	}
	var palette []byte
	for _, c := range s.palette {
		palette = append(palette, c.B, c.G, c.R)
		if s.dibSize != 12 {
			palette = append(palette, 0)
		}
	}
	offset := fileHeaderSize + len(dib) + len(masks) + len(palette) + len(s.gap)
	out := []byte{'B', 'M'}
	out = le.AppendUint32(out, uint32(offset+len(s.pixels)))
	out = le.AppendUint32(out, 0)
	out = le.AppendUint32(out, uint32(offset))
	out = append(out, dib...)
	out = append(out, masks...)
	out = append(out, palette...)
	out = append(out, s.gap...)
	return append(out, s.pixels...)
}

func pack(rows [][]byte, topDown bool) []byte {
	var out []byte
	for i := range rows {
		row := rows[i]
		if !topDown {
			row = rows[len(rows)-1-i]
		}
		out = append(out, row...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}
	return out
}

func rows24(img *image.NRGBA, topDown bool) []byte {
	var rows [][]byte
	for y := 0; y < img.Bounds().Dy(); y++ {
		var row []byte
		for x := 0; x < img.Bounds().Dx(); x++ {
			c := img.NRGBAAt(x, y)
			row = append(row, c.B, c.G, c.R)
		}
		rows = append(rows, row)
	}
	return pack(rows, topDown)
}

func rows32(img *image.NRGBA, topDown bool) []byte {
	var rows [][]byte
	for y := 0; y < img.Bounds().Dy(); y++ {
		var row []byte
		for x := 0; x < img.Bounds().Dx(); x++ {
			c := img.NRGBAAt(x, y)
			row = append(row, c.B, c.G, c.R, c.A)
		}
		rows = append(rows, row)
	}
	return pack(rows, topDown)
}

func rowsIndexed(indices [][]byte, bpp int, topDown bool) []byte {
	var rows [][]byte
	for _, line := range indices {
		row := make([]byte, (len(line)*bpp+7)/8)
		for x, index := range line {
			switch bpp {
			case 1:
				row[x>>3] |= index << (7 - uint(x&7))
			case 4:
				if x&1 == 0 {
					row[x>>1] |= index << 4
				} else {
					row[x>>1] |= index
				}
			default:
				row[x] = index
			}
		}
		rows = append(rows, row)
	}
	return pack(rows, topDown)
}

func randomPalette(rng *rand.Rand, n int) []color.RGBA {
	palette := make([]color.RGBA, n)
	for i := range palette {
		palette[i] = color.RGBA{R: uint8(rng.Intn(256)), G: uint8(rng.Intn(256)), B: uint8(rng.Intn(256)), A: 255}
	}
	return palette
}

func decodeData(data []byte) (*codec.Image, error) {
	return Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
}

func mustDecode(t *testing.T, data []byte) *codec.Image {
	t.Helper()
	img, err := decodeData(data)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func probeData(data []byte) (codec.Info, error) {
	return Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
}

func scaleTo8(v, bits uint32) uint8 {
	largest := uint32(1)<<bits - 1
	return uint8((v*255 + largest/2) / largest)
}

func TestSniffAndIdentity(t *testing.T) {
	d := Decoder{}
	valid := build(spec{width: 1, height: 1, bpp: 24, pixels: []byte{1, 2, 3, 0}})
	if !d.Sniff(valid) {
		t.Fatal("BMP not recognised")
	}
	for _, size := range []int{12, 40, 52, 56, 64, 108, 124} {
		head := append([]byte("BM"), make([]byte, 16)...)
		binary.LittleEndian.PutUint32(head[14:], uint32(size))
		if !d.Sniff(head) {
			t.Fatalf("header size %d rejected", size)
		}
	}
	for _, head := range [][]byte{nil, []byte("BM"), []byte("BM\x00\x00"), append([]byte("BM"), make([]byte, 20)...), []byte("GIF89a")} {
		if d.Sniff(head) {
			t.Fatalf("%q sniffed as BMP", head)
		}
	}
	if d.ID() != codec.BMP || d.Name() != "BMP" {
		t.Fatal("identity")
	}
	if got := d.Extensions(); len(got) != 2 || got[0] != ".bmp" || got[1] != ".dib" {
		t.Fatalf("extensions %v", got)
	}
}

func TestTwentyFourBitAllPaddings(t *testing.T) {
	for width := 1; width <= 9; width++ {
		for _, topDown := range []bool{false, true} {
			src := testimg.Noise(width, 5, uint64(width), false)
			height := 5
			if topDown {
				height = -5
			}
			data := build(spec{width: width, height: height, bpp: 24, pixels: rows24(src, topDown)})
			img := mustDecode(t, data)
			if !testimg.Identical(img.First(), src) {
				t.Fatalf("width %d topDown=%v: pixels differ", width, topDown)
			}
			info, err := probeData(data)
			if err != nil || info.Width != width || info.Height != 5 || info.BitDepth != 24 || info.HasAlpha || info.Frames != 1 || info.FormatName != "BMP" {
				t.Fatalf("width %d: %+v %v", width, info, err)
			}
		}
	}
}

func TestMatchesStandardLibraryOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	for _, bpp := range []int{1, 4, 8} {
		width, height := 13, 7
		palette := randomPalette(rng, 1<<bpp)
		indices := make([][]byte, height)
		for y := range indices {
			indices[y] = make([]byte, width)
			for x := range indices[y] {
				indices[y][x] = byte(rng.Intn(1 << bpp))
			}
		}
		data := build(spec{width: width, height: height, bpp: bpp, palette: palette, pixels: rowsIndexed(indices, bpp, false)})
		reference, err := xbmp.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("bpp %d oracle: %v", bpp, err)
		}
		img := mustDecode(t, data)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				want := color.NRGBAModel.Convert(reference.At(x, y)).(color.NRGBA)
				if have := img.First().NRGBAAt(x, y); have != want {
					t.Fatalf("bpp %d (%d,%d) = %v want %v", bpp, x, y, have, want)
				}
			}
		}
	}
	src := testimg.Noise(11, 6, 8, false)
	data := build(spec{width: 11, height: 6, bpp: 24, pixels: rows24(src, false)})
	reference, err := xbmp.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	img := mustDecode(t, data)
	for y := 0; y < 6; y++ {
		for x := 0; x < 11; x++ {
			want := color.NRGBAModel.Convert(reference.At(x, y)).(color.NRGBA)
			if have := img.First().NRGBAAt(x, y); have != want {
				t.Fatalf("24 bit (%d,%d) = %v want %v", x, y, have, want)
			}
		}
	}
}

func TestIndexedFormatsWithBothHeaderStyles(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	for _, dibSize := range []int{12, 40, 108} {
		for _, bpp := range []int{1, 4, 8} {
			for width := 1; width <= 17; width += 4 {
				palette := randomPalette(rng, 1<<bpp)
				indices := make([][]byte, 4)
				for y := range indices {
					indices[y] = make([]byte, width)
					for x := range indices[y] {
						indices[y][x] = byte(rng.Intn(1 << bpp))
					}
				}
				topDown := dibSize == 108
				height := 4
				if topDown {
					height = -4
				}
				data := build(spec{dibSize: dibSize, width: width, height: height, bpp: bpp, palette: palette, pixels: rowsIndexed(indices, bpp, topDown)})
				img := mustDecode(t, data)
				for y := range indices {
					for x, index := range indices[y] {
						c := palette[index]
						want := color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}
						if have := img.First().NRGBAAt(x, y); have != want {
							t.Fatalf("dib %d bpp %d width %d (%d,%d) = %v want %v", dibSize, bpp, width, x, y, have, want)
						}
					}
				}
			}
		}
	}
}

func TestShortPaletteAndOutOfRangeIndices(t *testing.T) {
	palette := []color.RGBA{{R: 10, G: 20, B: 30, A: 255}, {R: 40, G: 50, B: 60, A: 255}}
	data := build(spec{width: 4, height: 1, bpp: 8, colorsUsed: 2, palette: palette, pixels: rowsIndexed([][]byte{{0, 1, 2, 200}}, 8, false)})
	img := mustDecode(t, data).First()
	want := []color.NRGBA{{10, 20, 30, 255}, {40, 50, 60, 255}, {0, 0, 0, 255}, {0, 0, 0, 255}}
	for x, w := range want {
		if have := img.NRGBAAt(x, 0); have != w {
			t.Fatalf("pixel %d = %v, want %v", x, have, w)
		}
	}
}

func TestGapBeforePixelDataAndOffsetInsidePalette(t *testing.T) {
	src := testimg.Noise(5, 3, 9, false)
	gapped := build(spec{width: 5, height: 3, bpp: 24, gap: []byte{1, 2, 3, 4, 5, 6, 7}, pixels: rows24(src, false)})
	if !testimg.Identical(mustDecode(t, gapped).First(), src) {
		t.Fatal("gap before pixel data not honoured")
	}
	palette := randomPalette(rand.New(rand.NewSource(1)), 256)
	indices := [][]byte{{0, 1, 2, 3}}
	data := build(spec{width: 4, height: 1, bpp: 8, palette: palette, pixels: rowsIndexed(indices, 8, false)})
	binary.LittleEndian.PutUint32(data[10:], uint32(fileHeaderSize+infoHeaderSize+100))
	img := mustDecode(t, data).First()
	for x := 0; x < 4; x++ {
		c := palette[x]
		if have := img.NRGBAAt(x, 0); have != (color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}) {
			t.Fatalf("pixel %d = %v", x, have)
		}
	}
}

func TestThirtyTwoBitAlphaUsedWhenPresent(t *testing.T) {
	src := testimg.Noise(7, 5, 10, true)
	src.SetNRGBA(0, 0, color.NRGBA{R: 1, G: 2, B: 3, A: 1})
	data := build(spec{width: 7, height: 5, bpp: 32, pixels: rows32(src, false)})
	img := mustDecode(t, data)
	if !testimg.Identical(img.First(), src) || !img.Info.HasAlpha {
		t.Fatalf("alpha lost: has=%v", img.Info.HasAlpha)
	}
	info, err := probeData(data)
	if err != nil || info.BitDepth != 32 || info.HasAlpha {
		t.Fatalf("probe of BI_RGB 32 bit: %+v %v", info, err)
	}
}

func TestThirtyTwoBitWithAllZeroAlphaIsOpaque(t *testing.T) {
	src := testimg.Noise(6, 4, 11, false)
	zeroAlpha := image.NewNRGBA(src.Bounds())
	copy(zeroAlpha.Pix, src.Pix)
	for i := 3; i < len(zeroAlpha.Pix); i += 4 {
		zeroAlpha.Pix[i] = 0
	}
	data := build(spec{width: 6, height: 4, bpp: 32, pixels: rows32(zeroAlpha, false)})
	img := mustDecode(t, data)
	if !testimg.Identical(img.First(), src) || img.Info.HasAlpha {
		t.Fatalf("zero alpha must become opaque; has alpha %v", img.Info.HasAlpha)
	}
}

func TestSixteenBitDefaultAndBitfields(t *testing.T) {
	rng := rand.New(rand.NewSource(12))
	width, height := 9, 4
	values := make([][]byte, height)
	words := make([][]uint16, height)
	for y := range values {
		for x := 0; x < width; x++ {
			v := uint16(rng.Intn(65536))
			words[y] = append(words[y], v)
			values[y] = append(values[y], byte(v), byte(v>>8))
		}
	}
	check := func(name string, data []byte, want func(v uint16) color.NRGBA) {
		t.Helper()
		img := mustDecode(t, data).First()
		for y := range words {
			for x, v := range words[y] {
				if have := img.NRGBAAt(x, y); have != want(v) {
					t.Fatalf("%s (%d,%d) v=%#04x: %v want %v", name, x, y, v, have, want(v))
				}
			}
		}
	}
	check("555", build(spec{width: width, height: height, bpp: 16, pixels: pack(values, false)}), func(v uint16) color.NRGBA {
		return color.NRGBA{R: scaleTo8(uint32(v>>10)&31, 5), G: scaleTo8(uint32(v>>5)&31, 5), B: scaleTo8(uint32(v)&31, 5), A: 255}
	})
	masks565 := [4]uint32{0xf800, 0x07e0, 0x001f, 0}
	want565 := func(v uint16) color.NRGBA {
		return color.NRGBA{R: scaleTo8(uint32(v>>11)&31, 5), G: scaleTo8(uint32(v>>5)&63, 6), B: scaleTo8(uint32(v)&31, 5), A: 255}
	}
	for _, dibSize := range []int{40, 52, 56, 108, 124} {
		check("565/"+string(rune('0'+dibSize/10%10)), build(spec{dibSize: dibSize, width: width, height: height, bpp: 16, compression: compressionBitfields, masks: masks565, pixels: pack(values, false)}), want565)
	}
	masks4444 := [4]uint32{0x0f00, 0x00f0, 0x000f, 0xf000}
	data4444 := build(spec{dibSize: 108, width: width, height: height, bpp: 16, compression: compressionBitfields, masks: masks4444, pixels: pack(values, false)})
	check("4444", data4444, func(v uint16) color.NRGBA {
		return color.NRGBA{R: scaleTo8(uint32(v>>8)&15, 4), G: scaleTo8(uint32(v>>4)&15, 4), B: scaleTo8(uint32(v)&15, 4), A: scaleTo8(uint32(v>>12)&15, 4)}
	})
	info, err := probeData(data4444)
	if err != nil || !info.HasAlpha || info.BitDepth != 16 {
		t.Fatalf("probe 4444: %+v %v", info, err)
	}
}

func TestThirtyTwoBitBitfields(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	width, height := 5, 3
	var values [][]byte
	var words [][]uint32
	for y := 0; y < height; y++ {
		var row []byte
		var line []uint32
		for x := 0; x < width; x++ {
			v := rng.Uint32()
			line = append(line, v)
			row = binary.LittleEndian.AppendUint32(row, v)
		}
		values = append(values, row)
		words = append(words, line)
	}
	masks := [4]uint32{0x3ff00000, 0x000ffc00, 0x000003ff, 0xc0000000}
	for _, dibSize := range []int{56, 108, 124} {
		data := build(spec{dibSize: dibSize, width: width, height: height, bpp: 32, compression: compressionBitfields, masks: masks, pixels: pack(values, false)})
		img := mustDecode(t, data)
		for y := range words {
			for x, v := range words[y] {
				want := color.NRGBA{R: uint8((v >> 20 & 0x3ff) >> 2), G: uint8((v >> 10 & 0x3ff) >> 2), B: uint8((v & 0x3ff) >> 2), A: scaleTo8(v>>30, 2)}
				if have := img.First().NRGBAAt(x, y); have != want {
					t.Fatalf("dib %d (%d,%d) = %v want %v", dibSize, x, y, have, want)
				}
			}
		}
	}
	alphaBitfields := build(spec{dibSize: 40, width: width, height: height, bpp: 32, compression: compressionAlphaBitfields, masks: masks, pixels: pack(values, false)})
	if img := mustDecode(t, alphaBitfields); img.Info.BitDepth != 32 {
		t.Fatalf("%+v", img.Info)
	}
}

func TestThirtyTwoBitBitfieldsWithoutAlphaMaskIsOpaque(t *testing.T) {
	src := testimg.Noise(4, 4, 14, true)
	data := build(spec{dibSize: 40, width: 4, height: 4, bpp: 32, compression: compressionBitfields, masks: [4]uint32{0xff0000, 0xff00, 0xff, 0}, pixels: rows32(src, false)})
	img := mustDecode(t, data)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			want := src.NRGBAAt(x, y)
			want.A = 255
			if have := img.First().NRGBAAt(x, y); have != want {
				t.Fatalf("(%d,%d) = %v want %v", x, y, have, want)
			}
		}
	}
	if img.Info.HasAlpha {
		t.Fatal("no alpha mask, no alpha")
	}
}

func TestThirtyTwoBitStandardMasksInV4AndV5(t *testing.T) {
	src := testimg.Noise(6, 3, 15, true)
	for _, dibSize := range []int{108, 124} {
		data := build(spec{dibSize: dibSize, width: 6, height: 3, bpp: 32, compression: compressionBitfields, masks: [4]uint32{0xff0000, 0xff00, 0xff, 0xff000000}, pixels: rows32(src, false)})
		img := mustDecode(t, data)
		if !testimg.Identical(img.First(), src) {
			t.Fatalf("dib %d: pixels differ", dibSize)
		}
		info, err := probeData(data)
		if err != nil || !info.HasAlpha {
			t.Fatalf("dib %d: %+v %v", dibSize, info, err)
		}
	}
}

func TestRLE8(t *testing.T) {
	palette := []color.RGBA{{R: 1, G: 1, B: 1, A: 255}, {R: 200, A: 255}, {G: 200, A: 255}, {B: 200, A: 255}}
	stream := []byte{
		6, 1, 0, 0,
		3, 2, 0, 3, 3, 1, 2, 0, 0, 0,
		0, 2, 2, 0, 4, 3, 0, 1,
	}
	data := build(spec{width: 6, height: 3, bpp: 8, compression: compressionRLE8, colorsUsed: uint32(len(palette)), palette: palette, pixels: stream})
	img := mustDecode(t, data).First()
	want := [][]int{
		{0, 0, 3, 3, 3, 3},
		{2, 2, 2, 3, 1, 2},
		{1, 1, 1, 1, 1, 1},
	}
	for y := range want {
		for x, index := range want[y] {
			c := palette[index]
			if have := img.NRGBAAt(x, y); have != (color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}) {
				t.Fatalf("(%d,%d) = %v want palette %d", x, y, have, index)
			}
		}
	}
}

func TestRLE4(t *testing.T) {
	palette := randomPalette(rand.New(rand.NewSource(3)), 16)
	stream := []byte{
		5, 0x12, 0, 0,
		0, 5, 0x12, 0x34, 0x50, 0, 0, 1,
	}
	data := build(spec{width: 5, height: 2, bpp: 4, compression: compressionRLE4, palette: palette, pixels: stream})
	img := mustDecode(t, data).First()
	want := [][]int{
		{1, 2, 3, 4, 5},
		{1, 2, 1, 2, 1},
	}
	for y := range want {
		for x, index := range want[y] {
			c := palette[index]
			if have := img.NRGBAAt(x, y); have != (color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}) {
				t.Fatalf("(%d,%d) = %v want palette %d", x, y, have, index)
			}
		}
	}
}

func TestRLETopDownAndEarlyEnd(t *testing.T) {
	palette := []color.RGBA{{A: 255}, {R: 255, A: 255}}
	data := build(spec{width: 3, height: -2, bpp: 8, compression: compressionRLE8, colorsUsed: 2, palette: palette, pixels: []byte{3, 1, 0, 1}})
	img := mustDecode(t, data).First()
	if img.NRGBAAt(0, 0) != (color.NRGBA{R: 255, A: 255}) || img.NRGBAAt(0, 1) != (color.NRGBA{A: 255}) {
		t.Fatal("top-down RLE rows misplaced")
	}
}

func TestRLEHostileStreams(t *testing.T) {
	palette := []color.RGBA{{A: 255}, {R: 255, A: 255}}
	cases := map[string][]byte{
		"literal run cut off":   {0, 200, 1, 2, 3},
		"delta cut off":         {0, 2, 5},
		"command cut off":       {0, 0, 4},
		"no data":               nil,
		"incomplete image":      {3, 1, 0, 0},
		"huge delta then pixel": {0, 2, 255, 255, 5, 1, 0, 1},
		"unbounded run":         bytes.Repeat([]byte{255, 1}, 5000),
	}
	for name, stream := range cases {
		data := build(spec{width: 3, height: 3, bpp: 8, compression: compressionRLE8, colorsUsed: uint32(len(palette)), palette: palette, pixels: stream})
		_, err := decodeData(data)
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
		if name != "huge delta then pixel" && name != "unbounded run" && err == nil {
			t.Fatalf("%s: decoded without error", name)
		}
	}
	data := build(spec{width: 3, height: 3, bpp: 4, compression: compressionRLE8, pixels: nil})
	if _, err := decodeData(data); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("RLE8 with 4 bpp: %v", err)
	}
	data = build(spec{width: 3, height: 3, bpp: 8, compression: compressionRLE4, pixels: nil})
	if _, err := decodeData(data); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("RLE4 with 8 bpp: %v", err)
	}
}

func TestEncoderRoundTrip(t *testing.T) {
	for _, withAlpha := range []bool{false, true} {
		src := testimg.Noise(19, 11, 16, withAlpha)
		var buf bytes.Buffer
		if err := (Encoder{}).Encode(&buf, src, codec.EncodeOptions{}); err != nil {
			t.Fatal(err)
		}
		img := mustDecode(t, buf.Bytes())
		if !testimg.Identical(img.First(), src) {
			t.Fatalf("alpha=%v: round trip differs", withAlpha)
		}
		if img.Info.HasAlpha != withAlpha {
			t.Fatalf("alpha=%v: info %+v", withAlpha, img.Info)
		}
	}
	whole := testimg.Noise(20, 20, 17, false)
	sub := whole.SubImage(image.Rect(3, 4, 15, 18)).(*image.NRGBA)
	var buf bytes.Buffer
	if err := (Encoder{}).Encode(&buf, sub, codec.EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	img := mustDecode(t, buf.Bytes()).First()
	for y := 0; y < sub.Bounds().Dy(); y++ {
		for x := 0; x < sub.Bounds().Dx(); x++ {
			if img.NRGBAAt(x, y) != sub.NRGBAAt(sub.Bounds().Min.X+x, sub.Bounds().Min.Y+y) {
				t.Fatalf("sub image pixel (%d,%d) differs", x, y)
			}
		}
	}
	if err := (Encoder{}).Encode(&bytes.Buffer{}, nil, codec.EncodeOptions{}); err == nil {
		t.Fatal("nil accepted")
	}
	e := Encoder{}
	if e.ID() != codec.BMP || e.Name() != "BMP" || e.Extension() != ".bmp" {
		t.Fatal("identity")
	}
}

func TestUnsupportedAndMalformedHeaders(t *testing.T) {
	pixels := []byte{1, 2, 3, 0}
	good := build(spec{width: 1, height: 1, bpp: 24, pixels: pixels})
	with := func(mutate func(b []byte)) []byte {
		b := append([]byte(nil), good...)
		mutate(b)
		return b
	}
	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"jpeg inside", build(spec{width: 1, height: 1, bpp: 24, compression: compressionJPEG, pixels: pixels}), codec.ErrUnsupported},
		{"png inside", build(spec{width: 1, height: 1, bpp: 24, compression: compressionPNG, pixels: pixels}), codec.ErrUnsupported},
		{"compression 9", build(spec{width: 1, height: 1, bpp: 24, compression: 9, pixels: pixels}), codec.ErrUnsupported},
		{"bitfields 24 bit", build(spec{width: 1, height: 1, bpp: 24, compression: compressionBitfields, pixels: pixels}), codec.ErrUnsupported},
		{"2 bit", build(spec{width: 1, height: 1, bpp: 2, pixels: pixels, palette: randomPalette(rand.New(rand.NewSource(1)), 4)}), codec.ErrUnsupported},
		{"odd header size", with(func(b []byte) { b[14] = 16 }), codec.ErrUnsupported},
		{"tiny header size", with(func(b []byte) { b[14] = 8 }), codec.ErrCorrupt},
		{"header longer than file", build(spec{dibSize: 124, width: 1, height: 1, bpp: 24})[:40], codec.ErrCorrupt},
		{"offset past file", with(func(b []byte) { binary.LittleEndian.PutUint32(b[10:], 1000) }), codec.ErrCorrupt},
		{"offset inside header", with(func(b []byte) { binary.LittleEndian.PutUint32(b[10:], 20) }), codec.ErrCorrupt},
		{"zero width", build(spec{width: 0, height: 1, bpp: 24, pixels: pixels}), codec.ErrCorrupt},
		{"negative width", build(spec{width: -4, height: 1, bpp: 24, pixels: pixels}), codec.ErrCorrupt},
		{"zero height", build(spec{width: 1, height: 0, bpp: 24, pixels: pixels}), codec.ErrCorrupt},
		{"minimum int32 height", build(spec{width: 1, height: -1 << 31, bpp: 24, pixels: pixels}), codec.ErrTooLarge},
		{"huge both", build(spec{width: 1 << 30, height: 1 << 30, bpp: 24, pixels: pixels}), codec.ErrTooLarge},
		{"pixel data missing", build(spec{width: 100, height: 100, bpp: 24, pixels: pixels}), codec.ErrCorrupt},
		{"palette missing", build(spec{width: 1, height: 1, bpp: 8})[:fileHeaderSize+infoHeaderSize+8], codec.ErrCorrupt},
		{"empty masks", build(spec{dibSize: 40, width: 1, height: 1, bpp: 32, compression: compressionBitfields, pixels: pixels}), codec.ErrCorrupt},
		{"split mask", build(spec{dibSize: 40, width: 1, height: 1, bpp: 32, compression: compressionBitfields, masks: [4]uint32{0xf0f0, 0xf00, 0xf}, pixels: pixels}), codec.ErrUnsupported},
		{"wide 16 bit mask", build(spec{dibSize: 40, width: 1, height: 1, bpp: 16, compression: compressionBitfields, masks: [4]uint32{0xf0000, 0xf00, 0xf}, pixels: pixels}), codec.ErrUnsupported},
		{"alpha bitfields in 52 byte header", build(spec{dibSize: 52, width: 1, height: 1, bpp: 32, compression: compressionAlphaBitfields, masks: [4]uint32{0xff0000, 0xff00, 0xff}, pixels: pixels}), codec.ErrCorrupt},
		{"empty", nil, codec.ErrCorrupt},
		{"bad signature", with(func(b []byte) { b[0] = 'X' }), codec.ErrCorrupt},
	}
	for _, c := range cases {
		testimg.Within(t, 10*time.Second, func() {
			if _, err := decodeData(c.data); !errors.Is(err, c.want) {
				t.Errorf("%s: decode got %v, want %v", c.name, err, c.want)
			}
			if _, err := probeData(c.data); err != nil && !errors.Is(err, c.want) {
				t.Errorf("%s: probe got %v, want %v", c.name, err, c.want)
			}
		})
	}
}

func TestLimitsAndCancellation(t *testing.T) {
	src := testimg.Noise(50, 50, 18, false)
	data := build(spec{width: 50, height: 50, bpp: 24, pixels: rows24(src, false)})
	lim := codec.DefaultLimits()
	lim.MaxPixels = 100
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("pixels: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxFileBytes = 100
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("file: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Decoder{}).Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	rle := build(spec{width: 3, height: 1, bpp: 8, compression: compressionRLE8, palette: randomPalette(rand.New(rand.NewSource(1)), 256), pixels: []byte{3, 1, 0, 1}})
	if _, err := (Decoder{}).Decode(ctx, bytes.NewReader(rle), int64(len(rle)), codec.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel RLE: %v", err)
	}
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{}); err != nil {
		t.Fatalf("zero limits: %v", err)
	}
}

func TestEveryTruncatedPrefix(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	indices := [][]byte{{0, 1, 2}, {3, 2, 1}, {1, 1, 0}}
	samples := map[string][]byte{
		"24 bit":  build(spec{width: 3, height: 3, bpp: 24, pixels: rows24(testimg.Noise(3, 3, 1, false), false)}),
		"8 bit":   build(spec{width: 3, height: 3, bpp: 8, palette: randomPalette(rng, 256), pixels: rowsIndexed(indices, 8, false)}),
		"4 bit":   build(spec{dibSize: 12, width: 3, height: 3, bpp: 4, palette: randomPalette(rng, 16), pixels: rowsIndexed(indices, 4, false)}),
		"bitmask": build(spec{dibSize: 108, width: 3, height: 3, bpp: 32, compression: compressionBitfields, masks: [4]uint32{0xff0000, 0xff00, 0xff, 0xff000000}, pixels: rows32(testimg.Noise(3, 3, 2, true), false)}),
		"rle8":    build(spec{width: 6, height: 3, bpp: 8, compression: compressionRLE8, palette: randomPalette(rng, 256), pixels: []byte{6, 1, 0, 0, 3, 2, 0, 3, 3, 1, 2, 0, 0, 0, 0, 2, 2, 0, 4, 3, 0, 1}}),
	}
	for name, data := range samples {
		for n := 0; n <= len(data); n++ {
			prefix := data[:n]
			if _, err := probeData(prefix); err != nil {
				testimg.RequireCodecError(t, err)
			}
			_, err := decodeData(prefix)
			if err != nil {
				testimg.RequireCodecError(t, err)
			}
			if err == nil && n < len(data)-4 {
				t.Fatalf("%s: prefix %d/%d decoded without error", name, n, len(data))
			}
		}
		if _, err := decodeData(data); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestCorruptedBytesNeverPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	base := [][]byte{
		build(spec{width: 7, height: 5, bpp: 24, pixels: rows24(testimg.Noise(7, 5, 3, false), false)}),
		build(spec{dibSize: 124, width: 4, height: -4, bpp: 32, compression: compressionBitfields, masks: [4]uint32{0xff0000, 0xff00, 0xff, 0xff000000}, pixels: rows32(testimg.Noise(4, 4, 4, true), true)}),
		build(spec{width: 6, height: 3, bpp: 8, compression: compressionRLE8, palette: randomPalette(rng, 256), pixels: []byte{6, 1, 0, 0, 3, 2, 0, 3, 3, 1, 2, 0, 0, 0, 0, 2, 2, 0, 4, 3, 0, 1}}),
	}
	for _, data := range base {
		for i := 0; i < 800; i++ {
			mutated := append([]byte(nil), data...)
			for k := 0; k < 1+rng.Intn(4); k++ {
				mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
			}
			_, err := Decoder{}.Decode(context.Background(), bytes.NewReader(mutated), int64(len(mutated)), codec.Options{Limits: testimg.SmallLimits()})
			if err != nil {
				testimg.RequireCodecError(t, err)
			}
		}
	}
}

func FuzzDecode(f *testing.F) {
	rng := rand.New(rand.NewSource(9))
	f.Add(build(spec{width: 3, height: 3, bpp: 24, pixels: rows24(testimg.Noise(3, 3, 1, false), false)}))
	f.Add(build(spec{dibSize: 108, width: 3, height: -3, bpp: 32, compression: compressionBitfields, masks: [4]uint32{0xff0000, 0xff00, 0xff, 0xff000000}, pixels: rows32(testimg.Noise(3, 3, 2, true), true)}))
	f.Add(build(spec{width: 6, height: 3, bpp: 8, compression: compressionRLE8, palette: randomPalette(rng, 256), pixels: []byte{6, 1, 0, 0, 3, 2, 0, 3, 3, 1, 2, 0, 0, 0, 0, 2, 2, 0, 4, 3, 0, 1}}))
	f.Add(build(spec{width: 5, height: 2, bpp: 4, compression: compressionRLE4, palette: randomPalette(rng, 16), pixels: []byte{5, 0x12, 0, 0, 0, 5, 0x12, 0x34, 0x50, 0, 0, 1}}))
	f.Add(build(spec{dibSize: 12, width: 2, height: 2, bpp: 1, palette: randomPalette(rng, 2), pixels: []byte{0x80, 0, 0, 0, 0x40, 0, 0, 0}}))
	f.Add(build(spec{dibSize: 40, width: 2, height: 2, bpp: 16, compression: compressionBitfields, masks: [4]uint32{0xf800, 0x7e0, 0x1f}, pixels: []byte{1, 2, 3, 4, 0, 0, 0, 0, 5, 6, 7, 8, 0, 0, 0, 0}}))
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := testimg.SmallLimits()
		Decoder{}.Sniff(data)
		Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
		img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim})
		if err == nil && (img == nil || img.First() == nil) {
			t.Fatal("success without pixels")
		}
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
	})
}
