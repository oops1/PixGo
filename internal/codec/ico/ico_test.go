package ico

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

type iconEntry struct {
	dirWidth  byte
	dirHeight byte
	field1    uint16
	field2    uint16
	data      []byte
}

func buildICO(kind uint16, entries ...iconEntry) []byte {
	le := binary.LittleEndian
	out := le.AppendUint16(nil, 0)
	out = le.AppendUint16(out, kind)
	out = le.AppendUint16(out, uint16(len(entries)))
	offset := directorySize + entrySize*len(entries)
	for _, e := range entries {
		out = append(out, e.dirWidth, e.dirHeight, 0, 0)
		out = le.AppendUint16(out, e.field1)
		out = le.AppendUint16(out, e.field2)
		out = le.AppendUint32(out, uint32(len(e.data)))
		out = le.AppendUint32(out, uint32(offset))
		offset += len(e.data)
	}
	for _, e := range entries {
		out = append(out, e.data...)
	}
	return out
}

func dibHeader(width, heightField, bpp, colorsUsed int) []byte {
	le := binary.LittleEndian
	out := le.AppendUint32(nil, 40)
	out = le.AppendUint32(out, uint32(width))
	out = le.AppendUint32(out, uint32(int32(heightField)))
	out = le.AppendUint16(out, 1)
	out = le.AppendUint16(out, uint16(bpp))
	out = le.AppendUint32(out, 0)
	out = le.AppendUint32(out, 0)
	out = le.AppendUint32(out, 0)
	out = le.AppendUint32(out, 0)
	out = le.AppendUint32(out, uint32(colorsUsed))
	return le.AppendUint32(out, 0)
}

func padRow(row []byte) []byte {
	for len(row)%4 != 0 {
		row = append(row, 0)
	}
	return row
}

func maskRows(mask [][]bool) []byte {
	var out []byte
	for y := len(mask) - 1; y >= 0; y-- {
		row := make([]byte, (len(mask[y])+7)/8)
		for x, transparent := range mask[y] {
			if transparent {
				row[x>>3] |= 0x80 >> uint(x&7)
			}
		}
		out = append(out, padRow(row)...)
	}
	return out
}

func clearMask(width, height int) [][]bool {
	mask := make([][]bool, height)
	for y := range mask {
		mask[y] = make([]bool, width)
	}
	return mask
}

func dib32(img *image.NRGBA, mask [][]bool, doubled bool) []byte {
	b := img.Bounds()
	heightField := b.Dy()
	if doubled {
		heightField *= 2
	}
	out := dibHeader(b.Dx(), heightField, 32, 0)
	for y := b.Dy() - 1; y >= 0; y-- {
		for x := 0; x < b.Dx(); x++ {
			c := img.NRGBAAt(x, y)
			out = append(out, c.B, c.G, c.R, c.A)
		}
	}
	if doubled {
		out = append(out, maskRows(mask)...)
	}
	return out
}

func dib24(img *image.NRGBA, mask [][]bool) []byte {
	b := img.Bounds()
	out := dibHeader(b.Dx(), b.Dy()*2, 24, 0)
	for y := b.Dy() - 1; y >= 0; y-- {
		var row []byte
		for x := 0; x < b.Dx(); x++ {
			c := img.NRGBAAt(x, y)
			row = append(row, c.B, c.G, c.R)
		}
		out = append(out, padRow(row)...)
	}
	return append(out, maskRows(mask)...)
}

func dibIndexed(indices [][]byte, bpp int, palette []color.RGBA, mask [][]bool) []byte {
	width, height := len(indices[0]), len(indices)
	out := dibHeader(width, height*2, bpp, len(palette))
	for _, c := range palette {
		out = append(out, c.B, c.G, c.R, 0)
	}
	for y := height - 1; y >= 0; y-- {
		row := make([]byte, (width*bpp+7)/8)
		for x, index := range indices[y] {
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
		out = append(out, padRow(row)...)
	}
	return append(out, maskRows(mask)...)
}

func pngData(t testing.TB, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func probeICO(data []byte) (codec.Info, error) {
	return Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
}

func decodeICO(data []byte) (*codec.Image, error) {
	return Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
}

func TestSniffAndIdentity(t *testing.T) {
	img := testimg.Noise(4, 4, 1, true)
	icon := buildICO(typeIcon, iconEntry{data: dib32(img, clearMask(4, 4), true)})
	cursor := buildICO(typeCursor, iconEntry{data: dib32(img, clearMask(4, 4), true)})
	if !(Decoder{}).Sniff(icon) || (Decoder{}).Sniff(cursor) {
		t.Fatal("ICO sniffing")
	}
	if (CursorDecoder{}).Sniff(icon) || !(CursorDecoder{}).Sniff(cursor) {
		t.Fatal("CUR sniffing")
	}
	broken := [][]byte{nil, icon[:10], append([]byte{1, 0}, icon[2:]...), append(append([]byte(nil), icon[:4]...), 0, 0), append(append([]byte(nil), icon[:4]...), 0, 1)}
	for _, head := range broken {
		if (Decoder{}).Sniff(head) {
			t.Fatalf("%v sniffed as ICO", head)
		}
	}
	if len(icon) > 20 {
		bad := append([]byte(nil), icon...)
		binary.LittleEndian.PutUint32(bad[18:], 2)
		if (Decoder{}).Sniff(bad) {
			t.Fatal("data offset inside the directory accepted")
		}
	}
	if (Decoder{}).ID() != codec.ICO || (CursorDecoder{}).ID() != codec.CUR {
		t.Fatal("ids")
	}
	if (Decoder{}).Name() != "ICO" || (CursorDecoder{}).Name() != "CUR" {
		t.Fatal("names")
	}
	if e := (Decoder{}).Extensions(); len(e) != 1 || e[0] != ".ico" {
		t.Fatalf("%v", e)
	}
	if e := (CursorDecoder{}).Extensions(); len(e) != 1 || e[0] != ".cur" {
		t.Fatalf("%v", e)
	}
}

func TestPNGEntriesSortedLargestFirst(t *testing.T) {
	small, medium, large := testimg.Noise(16, 16, 1, true), testimg.Noise(32, 32, 2, true), testimg.Noise(48, 40, 3, true)
	data := buildICO(typeIcon,
		iconEntry{dirWidth: 16, dirHeight: 16, data: pngData(t, small)},
		iconEntry{dirWidth: 48, dirHeight: 40, data: pngData(t, large)},
		iconEntry{dirWidth: 32, dirHeight: 32, data: pngData(t, medium)},
	)
	img, err := decodeICO(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Frames) != 3 || img.Info.Frames != 3 || img.Info.Animated || img.Info.Width != 48 || img.Info.Height != 40 {
		t.Fatalf("%+v", img.Info)
	}
	for i, want := range []*image.NRGBA{large, medium, small} {
		if !testimg.Identical(img.Frames[i].Pix, want) {
			t.Fatalf("frame %d is not in size order", i)
		}
	}
	info, err := probeICO(data)
	if err != nil || info != img.Info {
		t.Fatalf("probe %+v vs decode %+v (%v)", info, img.Info, err)
	}
	if info.Format != codec.ICO || info.FormatName != "ICO" || info.BitDepth != 32 || !info.HasAlpha {
		t.Fatalf("%+v", info)
	}
}

func TestRealSizeWinsOverDirectorySize(t *testing.T) {
	img := testimg.Noise(20, 10, 4, true)
	for name, entry := range map[string]iconEntry{
		"zero means 256": {dirWidth: 0, dirHeight: 0, data: pngData(t, img)},
		"lying":          {dirWidth: 99, dirHeight: 77, data: pngData(t, img)},
		"dib zero":       {dirWidth: 0, dirHeight: 0, data: dib32(img, clearMask(20, 10), true)},
	} {
		info, err := probeICO(buildICO(typeIcon, entry))
		if err != nil || info.Width != 20 || info.Height != 10 {
			t.Fatalf("%s: %+v %v", name, info, err)
		}
	}
}

func TestSortByBitDepthWhenSizesMatch(t *testing.T) {
	palette := []color.RGBA{{R: 255, A: 255}, {B: 255, A: 255}}
	indexed := dibIndexed([][]byte{{0, 1, 0, 1}, {1, 0, 1, 0}, {0, 0, 1, 1}, {1, 1, 0, 0}}, 1, palette, clearMask(4, 4))
	truecolour := dib32(testimg.Noise(4, 4, 5, true), clearMask(4, 4), true)
	mid := dib24(testimg.Noise(4, 4, 6, false), clearMask(4, 4))
	data := buildICO(typeIcon, iconEntry{data: indexed}, iconEntry{data: truecolour}, iconEntry{data: mid})
	img, err := decodeICO(data)
	if err != nil {
		t.Fatal(err)
	}
	if img.Info.BitDepth != 32 {
		t.Fatalf("%+v", img.Info)
	}
	if !testimg.Identical(img.Frames[0].Pix, testimg.Noise(4, 4, 5, true)) || !testimg.Identical(img.Frames[1].Pix, testimg.Noise(4, 4, 6, false)) {
		t.Fatal("order by bit depth wrong")
	}
}

func TestDIB32UsesItsAlphaChannel(t *testing.T) {
	src := testimg.Noise(9, 7, 7, true)
	src.SetNRGBA(0, 0, color.NRGBA{R: 5, G: 6, B: 7, A: 1})
	mask := clearMask(9, 7)
	for x := range mask[2] {
		mask[2][x] = true
	}
	img, err := decodeICO(buildICO(typeIcon, iconEntry{data: dib32(src, mask, true)}))
	if err != nil {
		t.Fatal(err)
	}
	if !testimg.Identical(img.First(), src) {
		t.Fatal("the AND mask must be ignored when alpha is present")
	}
}

func TestDIB32WithAllZeroAlphaFallsBackToMask(t *testing.T) {
	src := testimg.Noise(8, 6, 8, false)
	zero := image.NewNRGBA(src.Bounds())
	copy(zero.Pix, src.Pix)
	for i := 3; i < len(zero.Pix); i += 4 {
		zero.Pix[i] = 0
	}
	mask := clearMask(8, 6)
	mask[0][0], mask[3][5], mask[5][7] = true, true, true
	img, err := decodeICO(buildICO(typeIcon, iconEntry{data: dib32(zero, mask, true)}))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			want := src.NRGBAAt(x, y)
			if mask[y][x] {
				want.A = 0
			}
			if have := img.First().NRGBAAt(x, y); have != want {
				t.Fatalf("(%d,%d) = %v want %v", x, y, have, want)
			}
		}
	}
	noMask := buildICO(typeIcon, iconEntry{data: dib32(zero, nil, false)})
	img, err = decodeICO(noMask)
	if err != nil || !testimg.Identical(img.First(), src) {
		t.Fatalf("no mask and no alpha must be opaque: %v", err)
	}
}

func TestDIBWithoutDoubledHeight(t *testing.T) {
	src := testimg.Noise(6, 5, 9, true)
	img, err := decodeICO(buildICO(typeIcon, iconEntry{data: dib32(src, nil, false)}))
	if err != nil {
		t.Fatal(err)
	}
	if !testimg.Identical(img.First(), src) {
		t.Fatal("single height DIB decoded wrongly")
	}
}

func TestLowDepthDIBsApplyTheMask(t *testing.T) {
	rng := rand.New(rand.NewSource(10))
	width, height := 11, 6
	for _, bpp := range []int{1, 4, 8} {
		palette := make([]color.RGBA, 1<<bpp)
		for i := range palette {
			palette[i] = color.RGBA{R: uint8(rng.Intn(256)), G: uint8(rng.Intn(256)), B: uint8(rng.Intn(256)), A: 255}
		}
		indices := make([][]byte, height)
		mask := clearMask(width, height)
		for y := range indices {
			indices[y] = make([]byte, width)
			for x := range indices[y] {
				indices[y][x] = byte(rng.Intn(1 << bpp))
				mask[y][x] = rng.Intn(3) == 0
			}
		}
		img, err := decodeICO(buildICO(typeIcon, iconEntry{data: dibIndexed(indices, bpp, palette, mask)}))
		if err != nil {
			t.Fatalf("bpp %d: %v", bpp, err)
		}
		for y := range indices {
			for x, index := range indices[y] {
				c := palette[index]
				want := color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}
				if mask[y][x] {
					want.A = 0
				}
				if have := img.First().NRGBAAt(x, y); have != want {
					t.Fatalf("bpp %d (%d,%d) = %v want %v", bpp, x, y, have, want)
				}
			}
		}
		if img.Info.BitDepth != bpp {
			t.Fatalf("bpp %d: %+v", bpp, img.Info)
		}
	}
}

func TestTrueColourDIBApplyTheMask(t *testing.T) {
	src := testimg.Noise(7, 5, 11, false)
	mask := clearMask(7, 5)
	mask[1][1], mask[4][6] = true, true
	img, err := decodeICO(buildICO(typeIcon, iconEntry{data: dib24(src, mask)}))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 5; y++ {
		for x := 0; x < 7; x++ {
			want := src.NRGBAAt(x, y)
			if mask[y][x] {
				want.A = 0
			}
			if have := img.First().NRGBAAt(x, y); have != want {
				t.Fatalf("(%d,%d) = %v want %v", x, y, have, want)
			}
		}
	}
}

func TestCursorFilesUseDataForDepthAndSize(t *testing.T) {
	src := testimg.Noise(12, 12, 12, true)
	data := buildICO(typeCursor, iconEntry{dirWidth: 12, dirHeight: 12, field1: 3, field2: 4, data: dib32(src, clearMask(12, 12), true)})
	info, err := CursorDecoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil || info.Format != codec.CUR || info.FormatName != "CUR" || info.BitDepth != 32 || info.Width != 12 {
		t.Fatalf("%+v %v", info, err)
	}
	img, err := CursorDecoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
	if err != nil || !testimg.Identical(img.First(), src) {
		t.Fatalf("cursor decode: %v", err)
	}
	if _, err := decodeICO(data); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("ICO decoder on a cursor: %v", err)
	}
	icon := buildICO(typeIcon, iconEntry{data: dib32(src, clearMask(12, 12), true)})
	if _, err := (CursorDecoder{}).Decode(context.Background(), bytes.NewReader(icon), int64(len(icon)), codec.Options{}); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("CUR decoder on an icon: %v", err)
	}
}

func TestBadEntriesAreSkippedWhenOthersWork(t *testing.T) {
	good := testimg.Noise(8, 8, 13, true)
	data := buildICO(typeIcon,
		iconEntry{data: dib32(good, clearMask(8, 8), true)},
		iconEntry{data: []byte("not an image at all, but long enough to pass the length check....")},
		iconEntry{data: pngData(t, testimg.Noise(30, 30, 1, true))},
	)
	truncated := data[:len(data)-30]
	for name, input := range map[string][]byte{"garbage entry": data[:len(data)], "cut last entry": truncated} {
		img, err := decodeICO(input)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if img.Info.Frames != len(img.Frames) || len(img.Frames) < 1 {
			t.Fatalf("%s: %+v", name, img.Info)
		}
	}
	img, _ := decodeICO(truncated)
	if len(img.Frames) != 1 || !testimg.Identical(img.First(), good) {
		t.Fatalf("expected only the good entry, got %d frames", len(img.Frames))
	}
	onlyBad := buildICO(typeIcon, iconEntry{data: bytes.Repeat([]byte{0x42}, 64)})
	if _, err := decodeICO(onlyBad); err == nil {
		t.Fatal("nothing decodable must be an error")
	} else {
		testimg.RequireCodecError(t, err)
	}
}

func TestMalformedDirectories(t *testing.T) {
	good := buildICO(typeIcon, iconEntry{data: dib32(testimg.Noise(4, 4, 1, true), clearMask(4, 4), true)})
	with := func(mutate func(b []byte)) []byte {
		b := append([]byte(nil), good...)
		mutate(b)
		return b
	}
	cases := map[string][]byte{
		"empty":             nil,
		"directory only":    good[:6],
		"reserved":          with(func(b []byte) { b[0] = 1 }),
		"type 3":            with(func(b []byte) { b[2] = 3 }),
		"zero entries":      with(func(b []byte) { b[4], b[5] = 0, 0 }),
		"257 entries":       with(func(b []byte) { b[4], b[5] = 1, 1 }),
		"offset in dir":     with(func(b []byte) { binary.LittleEndian.PutUint32(b[18:], 4) }),
		"offset past end":   with(func(b []byte) { binary.LittleEndian.PutUint32(b[18:], uint32(len(b)+10)) }),
		"length past end":   with(func(b []byte) { binary.LittleEndian.PutUint32(b[14:], uint32(len(b))) }),
		"huge length":       with(func(b []byte) { binary.LittleEndian.PutUint32(b[14:], 0xffffffff) }),
		"tiny length":       with(func(b []byte) { binary.LittleEndian.PutUint32(b[14:], 3) }),
		"directory cut off": buildICO(typeIcon, iconEntry{data: good[22:]})[:20],
	}
	for name, data := range cases {
		_, probeErr := probeICO(data)
		_, decodeErr := decodeICO(data)
		if probeErr == nil || decodeErr == nil {
			t.Fatalf("%s: probe=%v decode=%v", name, probeErr, decodeErr)
		}
		testimg.RequireCodecError(t, probeErr)
		testimg.RequireCodecError(t, decodeErr)
	}
}

func TestHostileEntryContents(t *testing.T) {
	src := testimg.Noise(4, 4, 1, true)
	good := dib32(src, clearMask(4, 4), true)
	with := func(mutate func(b []byte)) []byte {
		b := append([]byte(nil), good...)
		mutate(b)
		return b
	}
	cases := map[string][]byte{
		"zero width":      with(func(b []byte) { binary.LittleEndian.PutUint32(b[4:], 0) }),
		"negative width":  with(func(b []byte) { binary.LittleEndian.PutUint32(b[4:], 0xfffffff0) }),
		"negative height": with(func(b []byte) { binary.LittleEndian.PutUint32(b[8:], 0xfffffff0) }),
		"zero height":     with(func(b []byte) { binary.LittleEndian.PutUint32(b[8:], 0) }),
		"huge width":      with(func(b []byte) { binary.LittleEndian.PutUint32(b[4:], 1<<30) }),
		"huge both": with(func(b []byte) {
			binary.LittleEndian.PutUint32(b[4:], 1<<30)
			binary.LittleEndian.PutUint32(b[8:], 1<<30)
		}),
		"2 bits":             with(func(b []byte) { b[14] = 2 }),
		"compressed":         with(func(b []byte) { b[16] = 1 }),
		"header 41":          with(func(b []byte) { b[0] = 41 }),
		"header beyond data": with(func(b []byte) { b[0] = 124 }),
		"palette too large":  with(func(b []byte) { b[14] = 8; binary.LittleEndian.PutUint32(b[32:], 200) }),
	}
	for name, dibData := range cases {
		data := buildICO(typeIcon, iconEntry{data: dibData})
		testimg.Within(t, 10*time.Second, func() {
			_, probeErr := probeICO(data)
			_, decodeErr := decodeICO(data)
			if probeErr == nil || decodeErr == nil {
				t.Errorf("%s: probe=%v decode=%v", name, probeErr, decodeErr)
				return
			}
			testimg.RequireCodecError(t, probeErr)
			testimg.RequireCodecError(t, decodeErr)
		})
	}
	huge := testimg.Noise(2, 2, 1, true)
	pngBytes := pngData(t, huge)
	binary.BigEndian.PutUint32(pngBytes[16:], 100000)
	binary.BigEndian.PutUint32(pngBytes[20:], 100000)
	data := buildICO(typeIcon, iconEntry{data: pngBytes})
	testimg.Within(t, 10*time.Second, func() {
		if _, err := probeICO(data); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("huge PNG probe: %v", err)
		}
		if _, err := decodeICO(data); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("huge PNG decode: %v", err)
		}
	})
	zeroPNG := pngData(t, huge)
	binary.BigEndian.PutUint32(zeroPNG[16:], 0)
	if _, err := probeICO(buildICO(typeIcon, iconEntry{data: zeroPNG})); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("zero width PNG: %v", err)
	}
	brokenPNG := pngData(t, huge)
	copy(brokenPNG[12:16], "XXXX")
	if _, err := probeICO(buildICO(typeIcon, iconEntry{data: brokenPNG})); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("broken PNG header: %v", err)
	}
}

func TestEveryTruncatedPrefix(t *testing.T) {
	samples := map[string][]byte{
		"dib": buildICO(typeIcon, iconEntry{data: dib32(testimg.Noise(5, 4, 1, true), clearMask(5, 4), true)}),
		"png": buildICO(typeCursor, iconEntry{data: pngData(t, testimg.Noise(5, 4, 2, true))}),
	}
	for name, data := range samples {
		for n := 0; n < len(data); n++ {
			prefix := data[:n]
			_, probeErr := (variantFor(data)).probe(context.Background(), bytes.NewReader(prefix), int64(n), codec.DefaultLimits())
			_, decodeErr := (variantFor(data)).decode(context.Background(), bytes.NewReader(prefix), int64(n), codec.Options{})
			if probeErr == nil || decodeErr == nil {
				t.Fatalf("%s: prefix %d/%d probe=%v decode=%v", name, n, len(data), probeErr, decodeErr)
			}
			testimg.RequireCodecError(t, probeErr)
			testimg.RequireCodecError(t, decodeErr)
		}
	}
	multi := buildICO(typeIcon,
		iconEntry{data: dib32(testimg.Noise(5, 4, 1, true), clearMask(5, 4), true)},
		iconEntry{data: pngData(t, testimg.Noise(6, 6, 2, true))},
	)
	for n := 0; n <= len(multi); n++ {
		if _, err := decodeICO(multi[:n]); err != nil {
			testimg.RequireCodecError(t, err)
		}
	}
}

func variantFor(data []byte) variant {
	if binary.LittleEndian.Uint16(data[2:4]) == typeCursor {
		return cursorVariant
	}
	return iconVariant
}

func TestLimitsAndCancellation(t *testing.T) {
	data := buildICO(typeIcon,
		iconEntry{data: dib32(testimg.Noise(10, 10, 1, true), clearMask(10, 10), true)},
		iconEntry{data: dib32(testimg.Noise(12, 12, 2, true), clearMask(12, 12), true)},
		iconEntry{data: dib32(testimg.Noise(14, 14, 3, true), clearMask(14, 14), true)},
	)
	lim := codec.DefaultLimits()
	lim.MaxFrames = 2
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("frames: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxFramePixels = 400
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("total pixels: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxPixels = 150
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim})
	if err != nil || len(img.Frames) != 2 {
		t.Fatalf("entries over the pixel limit are dropped: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxFileBytes = 40
	if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("file: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Decoder{}).Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{}); err != nil {
		t.Fatalf("zero limits: %v", err)
	}
}

func TestCorruptedBytesNeverPanic(t *testing.T) {
	data := buildICO(typeIcon,
		iconEntry{data: dib32(testimg.Noise(6, 5, 1, true), clearMask(6, 5), true)},
		iconEntry{data: pngData(t, testimg.Noise(5, 5, 2, true))},
		iconEntry{data: dibIndexed([][]byte{{0, 1, 0}, {1, 0, 1}}, 4, []color.RGBA{{R: 1, A: 255}, {G: 1, A: 255}}, clearMask(3, 2))},
	)
	rng := rand.New(rand.NewSource(12))
	for i := 0; i < 1500; i++ {
		mutated := append([]byte(nil), data...)
		for k := 0; k < 1+rng.Intn(4); k++ {
			mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
		}
		if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(mutated), int64(len(mutated)), codec.Options{Limits: testimg.SmallLimits()}); err != nil {
			testimg.RequireCodecError(t, err)
		}
	}
}

func FuzzDecode(f *testing.F) {
	f.Add(buildICO(typeIcon, iconEntry{data: dib32(testimg.Noise(4, 4, 1, true), clearMask(4, 4), true)}))
	f.Add(buildICO(typeCursor, iconEntry{data: pngData(f, testimg.Noise(4, 3, 2, true))}))
	f.Add(buildICO(typeIcon, iconEntry{data: dibIndexed([][]byte{{0, 1}, {1, 0}}, 1, []color.RGBA{{R: 1, A: 255}, {G: 1, A: 255}}, clearMask(2, 2))}))
	f.Add(buildICO(typeIcon, iconEntry{data: dib24(testimg.Noise(3, 3, 3, false), clearMask(3, 3))}, iconEntry{data: dib32(testimg.Noise(2, 2, 4, true), nil, false)}))
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := testimg.SmallLimits()
		for _, v := range []variant{iconVariant, cursorVariant} {
			v.sniff(data)
			v.probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
			img, err := v.decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim})
			if err == nil && (img == nil || img.First() == nil || img.Info.Frames != len(img.Frames)) {
				t.Fatal("inconsistent success")
			}
			if err != nil {
				testimg.RequireCodecError(t, err)
			}
		}
	})
}
