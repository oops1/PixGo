package tiff

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"math/rand"
	"testing"
	"time"

	xtiff "golang.org/x/image/tiff"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

type tiffSpec struct {
	bigEndian   bool
	width       int
	height      int
	samples     int
	orientation uint16
	compression uint16
	pixels      []byte
	selfLoop    bool
}

type order interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

type entry struct {
	tag   uint16
	typ   uint16
	count uint32
	value []byte
}

func buildTIFF(s tiffSpec) []byte {
	var o order = binary.LittleEndian
	out := []byte{'I', 'I'}
	if s.bigEndian {
		o = binary.BigEndian
		out = []byte{'M', 'M'}
	}
	if s.compression == 0 {
		s.compression = 1
	}
	short := func(v uint16) []byte { return o.AppendUint16(nil, v) }
	long := func(v uint32) []byte { return o.AppendUint32(nil, v) }
	var bits []byte
	for i := 0; i < s.samples; i++ {
		bits = o.AppendUint16(bits, 8)
	}
	photometric := uint16(2)
	if s.samples == 1 {
		photometric = 1
	}
	entries := []entry{
		{0x100, 4, 1, long(uint32(s.width))},
		{0x101, 4, 1, long(uint32(s.height))},
		{0x102, 3, uint32(s.samples), bits},
		{0x103, 3, 1, short(s.compression)},
		{0x106, 3, 1, short(photometric)},
		{0x111, 4, 1, nil},
	}
	if s.orientation != 0 {
		entries = append(entries, entry{0x112, 3, 1, short(s.orientation)})
	}
	entries = append(entries,
		entry{0x115, 3, 1, short(uint16(s.samples))},
		entry{0x116, 4, 1, long(uint32(s.height))},
		entry{0x117, 4, 1, long(uint32(len(s.pixels)))},
	)
	if s.samples == 4 {
		entries = append(entries, entry{0x152, 3, 1, short(2)})
	}
	tableSize := 2 + len(entries)*12 + 4
	extraStart := 8 + tableSize
	var extra []byte
	for i := range entries {
		if len(entries[i].value) > 4 {
			offset := uint32(extraStart + len(extra))
			extra = append(extra, entries[i].value...)
			entries[i].value = long(offset)
		}
	}
	pixelStart := uint32(extraStart + len(extra))
	for i := range entries {
		if entries[i].tag == 0x111 {
			entries[i].value = long(pixelStart)
		}
	}
	out = o.AppendUint16(out, 42)
	out = o.AppendUint32(out, 8)
	out = o.AppendUint16(out, uint16(len(entries)))
	for _, e := range entries {
		out = o.AppendUint16(out, e.tag)
		out = o.AppendUint16(out, e.typ)
		out = o.AppendUint32(out, e.count)
		slot := make([]byte, 4)
		copy(slot, e.value)
		out = append(out, slot...)
	}
	next := uint32(0)
	if s.selfLoop {
		next = 8
	}
	out = o.AppendUint32(out, next)
	out = append(out, extra...)
	return append(out, s.pixels...)
}

func rgbBytes(img *image.NRGBA, samples int) []byte {
	var out []byte
	for i := 0; i < len(img.Pix); i += 4 {
		out = append(out, img.Pix[i:i+samples]...)
	}
	return out
}

func probe(data []byte) (codec.Info, error) {
	return Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
}

func decode(data []byte) (*codec.Image, error) {
	return Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
}

func TestSniffAndExtensions(t *testing.T) {
	d := Decoder{}
	if !d.Sniff([]byte{'I', 'I', 42, 0, 8, 0, 0, 0}) || !d.Sniff([]byte{'M', 'M', 0, 42}) {
		t.Fatal("TIFF signatures not recognised")
	}
	for _, head := range [][]byte{nil, {'I', 'I', 42}, {'I', 'I', 43, 0}, {'M', 'M', 42, 0}, {'I', 'I', 0, 42}, []byte("BM\x00\x00")} {
		if d.Sniff(head) {
			t.Fatalf("%v sniffed as TIFF", head)
		}
	}
	if d.ID() != codec.TIFF || d.Name() != "TIFF" {
		t.Fatal("identity")
	}
	if got := d.Extensions(); len(got) != 2 || got[0] != ".tif" || got[1] != ".tiff" {
		t.Fatalf("extensions %v", got)
	}
}

func TestDecodeRGBAndRGBAInBothByteOrders(t *testing.T) {
	for _, bigEndian := range []bool{false, true} {
		rgb := testimg.Noise(11, 7, 1, false)
		data := buildTIFF(tiffSpec{bigEndian: bigEndian, width: 11, height: 7, samples: 3, pixels: rgbBytes(rgb, 3)})
		img, err := decode(data)
		if err != nil {
			t.Fatal(err)
		}
		if !testimg.Identical(img.First(), rgb) {
			t.Fatalf("bigEndian=%v: RGB pixels differ", bigEndian)
		}
		info, err := probe(data)
		if err != nil || info.Width != 11 || info.Height != 7 || info.BitDepth != 24 || info.HasAlpha || info.Frames != 1 || info.FormatName != "TIFF" {
			t.Fatalf("%+v %v", info, err)
		}
		rgba := testimg.Noise(11, 7, 2, true)
		data = buildTIFF(tiffSpec{bigEndian: bigEndian, width: 11, height: 7, samples: 4, pixels: rgbBytes(rgba, 4)})
		img, err = decode(data)
		if err != nil {
			t.Fatal(err)
		}
		if !testimg.Identical(img.First(), rgba) {
			t.Fatalf("bigEndian=%v: RGBA pixels differ", bigEndian)
		}
		info, err = probe(data)
		if err != nil || info.BitDepth != 32 || !info.HasAlpha {
			t.Fatalf("%+v %v", info, err)
		}
	}
}

func TestDecodeGray(t *testing.T) {
	pixels := make([]byte, 6*4)
	rand.New(rand.NewSource(3)).Read(pixels)
	data := buildTIFF(tiffSpec{width: 6, height: 4, samples: 1, pixels: pixels})
	img, err := decode(data)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 6; x++ {
			v := pixels[y*6+x]
			if c := img.First().NRGBAAt(x, y); c.R != v || c.G != v || c.B != v || c.A != 255 {
				t.Fatalf("(%d,%d) = %v want %d", x, y, c, v)
			}
		}
	}
	info, _ := probe(data)
	if info.BitDepth != 8 {
		t.Fatalf("%+v", info)
	}
}

func TestOrientationFromIFD0(t *testing.T) {
	rgb := testimg.Noise(4, 3, 4, false)
	for _, bigEndian := range []bool{false, true} {
		for v := uint16(1); v <= 8; v++ {
			data := buildTIFF(tiffSpec{bigEndian: bigEndian, width: 4, height: 3, samples: 3, orientation: v, pixels: rgbBytes(rgb, 3)})
			info, err := probe(data)
			if err != nil || info.Orientation != codec.Orientation(v) {
				t.Fatalf("bigEndian=%v v=%d: %+v %v", bigEndian, v, info, err)
			}
			img, err := decode(data)
			if err != nil || img.Info.Orientation != codec.Orientation(v) {
				t.Fatalf("decode v=%d: %v", v, err)
			}
		}
	}
	none := buildTIFF(tiffSpec{width: 4, height: 3, samples: 3, pixels: rgbBytes(rgb, 3)})
	if info, _ := probe(none); info.Orientation != codec.OrientNormal {
		t.Fatalf("absent orientation: %d", info.Orientation)
	}
	bad := buildTIFF(tiffSpec{width: 4, height: 3, samples: 3, orientation: 12, pixels: rgbBytes(rgb, 3)})
	if info, _ := probe(bad); info.Orientation != codec.OrientNormal {
		t.Fatalf("invalid orientation: %d", info.Orientation)
	}
}

func TestStandardEncoderOutputWithCompression(t *testing.T) {
	src := testimg.Noise(33, 21, 5, true)
	for name, opts := range map[string]*xtiff.Options{
		"uncompressed": {Compression: xtiff.Uncompressed},
		"deflate":      {Compression: xtiff.Deflate},
		"predictor":    {Compression: xtiff.Deflate, Predictor: true},
	} {
		var buf bytes.Buffer
		if err := xtiff.Encode(&buf, src, opts); err != nil {
			t.Fatal(err)
		}
		img, err := decode(buf.Bytes())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !testimg.Identical(img.First(), src) {
			t.Fatalf("%s: pixels differ", name)
		}
		info, err := probe(buf.Bytes())
		if err != nil || !info.HasAlpha || info.Width != 33 || info.Height != 21 {
			t.Fatalf("%s: %+v %v", name, info, err)
		}
	}
}

func TestSixteenBitSourceIsReducedToEightBits(t *testing.T) {
	src := image.NewRGBA64(image.Rect(0, 0, 5, 4))
	rand.New(rand.NewSource(6)).Read(src.Pix)
	for i := 6; i < len(src.Pix); i += 8 {
		src.Pix[i], src.Pix[i+1] = 0xff, 0xff
	}
	var buf bytes.Buffer
	if err := xtiff.Encode(&buf, src, nil); err != nil {
		t.Fatal(err)
	}
	img, err := decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 5; x++ {
			p := src.RGBA64At(x, y)
			c := img.First().NRGBAAt(x, y)
			if c.R != uint8(p.R>>8) || c.G != uint8(p.G>>8) || c.B != uint8(p.B>>8) || c.A != 255 {
				t.Fatalf("(%d,%d) = %v want %v", x, y, c, p)
			}
		}
	}
}

func TestUnsupportedCompression(t *testing.T) {
	rgb := testimg.Noise(4, 4, 7, false)
	data := buildTIFF(tiffSpec{width: 4, height: 4, samples: 3, compression: 7, pixels: rgbBytes(rgb, 3)})
	if _, err := decode(data); !errors.Is(err, codec.ErrUnsupported) {
		t.Fatalf("got %v", err)
	}
}

func TestSelfReferencingIFDChain(t *testing.T) {
	rgb := testimg.Noise(4, 4, 8, false)
	data := buildTIFF(tiffSpec{width: 4, height: 4, samples: 3, orientation: 6, selfLoop: true, pixels: rgbBytes(rgb, 3)})
	testimg.Within(t, 10*time.Second, func() {
		info, err := probe(data)
		if err != nil || info.Orientation != codec.OrientRotate90 {
			t.Errorf("%+v %v", info, err)
		}
		if _, err := decode(data); err != nil {
			t.Errorf("decode: %v", err)
		}
	})
}

func TestHugeDimensionsRejectedQuickly(t *testing.T) {
	data := buildTIFF(tiffSpec{width: 1 << 24, height: 1 << 24, samples: 3, pixels: []byte{1, 2, 3}})
	testimg.Within(t, 10*time.Second, func() {
		if _, err := probe(data); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("probe: %v", err)
		}
		if _, err := decode(data); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("decode: %v", err)
		}
	})
}

func TestStripLargerThanFile(t *testing.T) {
	rgb := testimg.Noise(8, 8, 9, false)
	data := buildTIFF(tiffSpec{width: 8, height: 8, samples: 3, pixels: rgbBytes(rgb, 3)})
	for _, tag := range []uint16{0x111, 0x117} {
		mutated := append([]byte(nil), data...)
		count := int(binary.LittleEndian.Uint16(mutated[8:10]))
		for i := 0; i < count; i++ {
			base := 10 + 12*i
			if binary.LittleEndian.Uint16(mutated[base:]) == tag {
				binary.LittleEndian.PutUint32(mutated[base+8:], 0xfffffff0)
			}
		}
		testimg.Within(t, 10*time.Second, func() {
			if _, err := decode(mutated); err == nil {
				t.Errorf("tag %#x: oversized strip accepted", tag)
			} else {
				testimg.RequireCodecError(t, err)
			}
		})
	}
}

func TestEveryTruncatedPrefix(t *testing.T) {
	rgb := testimg.Noise(5, 4, 10, false)
	data := buildTIFF(tiffSpec{width: 5, height: 4, samples: 3, orientation: 3, pixels: rgbBytes(rgb, 3)})
	for n := 0; n < len(data); n++ {
		prefix := data[:n]
		if _, err := probe(prefix); err != nil {
			testimg.RequireCodecError(t, err)
		}
		if _, err := decode(prefix); err == nil {
			t.Fatalf("prefix %d/%d decoded", n, len(data))
		} else {
			testimg.RequireCodecError(t, err)
		}
	}
}

func TestLimitsAndCancellation(t *testing.T) {
	rgb := testimg.Noise(20, 20, 11, false)
	data := buildTIFF(tiffSpec{width: 20, height: 20, samples: 3, pixels: rgbBytes(rgb, 3)})
	lim := codec.DefaultLimits()
	lim.MaxPixels = 399
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("pixels: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxFileBytes = 50
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
	rgb := testimg.Noise(9, 6, 12, true)
	data := buildTIFF(tiffSpec{width: 9, height: 6, samples: 4, orientation: 6, pixels: rgbBytes(rgb, 4)})
	rng := rand.New(rand.NewSource(10))
	for i := 0; i < 600; i++ {
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

func FuzzDecode(f *testing.F) {
	rgb := testimg.Noise(5, 4, 13, true)
	f.Add(buildTIFF(tiffSpec{width: 5, height: 4, samples: 4, orientation: 6, pixels: rgbBytes(rgb, 4)}))
	f.Add(buildTIFF(tiffSpec{bigEndian: true, width: 5, height: 4, samples: 3, pixels: rgbBytes(rgb, 3)}))
	f.Add(buildTIFF(tiffSpec{width: 5, height: 4, samples: 1, selfLoop: true, pixels: make([]byte, 20)}))
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
