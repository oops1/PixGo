package tiff

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"math/rand"
	"testing"

	xtiff "golang.org/x/image/tiff"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/reduce"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

type kind struct {
	name                              string
	bits, samples, photometric, extra int
}

var kinds = []kind{
	{"gray1", 1, 1, 1, 0},
	{"gray2", 2, 1, 1, 0},
	{"gray4", 4, 1, 1, 0},
	{"gray8", 8, 1, 1, 0},
	{"gray16", 16, 1, 1, 0},
	{"white1", 1, 1, 0, 0},
	{"white8", 8, 1, 0, 0},
	{"white16", 16, 1, 0, 0},
	{"palette1", 1, 1, 3, 0},
	{"palette2", 2, 1, 3, 0},
	{"palette4", 4, 1, 3, 0},
	{"palette8", 8, 1, 3, 0},
	{"rgb8", 8, 3, 2, 0},
	{"rgb16", 16, 3, 2, 0},
	{"rgba8", 8, 4, 2, 2},
	{"rgba8associated", 8, 4, 2, 1},
	{"rgba16", 16, 4, 2, 2},
	{"rgba16associated", 16, 4, 2, 1},
}

type variant struct {
	name         string
	tileW, tileH int
	rows         int
}

var variants = []variant{
	{"strip-whole", 0, 0, 0},
	{"strip-1", 0, 0, 1},
	{"strip-3", 0, 0, 3},
	{"strip-7", 0, 0, 7},
	{"strip-16", 0, 0, 16},
	{"tile-16", 16, 16, 0},
	{"tile-32x16", 32, 16, 0},
	{"tile-16x48", 16, 48, 0},
	{"tile-64", 64, 64, 0},
}

var matrixVariants = []variant{variants[0], variants[1], variants[2], variants[3], variants[5], variants[6], variants[8]}

var compressions = []int{1, 5, 8, 32773}

func (k kind) fixture(width, height int, seed int64) fixture {
	f := fixture{width: width, height: height, bits: k.bits, samples: k.samples, photometric: k.photometric, extra: k.extra, compression: 1}
	f.fill(seed)
	if k.photometric == 3 {
		f.palette = makePalette(k.bits, seed+1)
	}
	return f
}

func (k kind) predictors() []int {
	if k.bits >= 8 {
		return []int{0, 2}
	}
	return []int{0}
}

func (v variant) apply(f *fixture) {
	f.tileW, f.tileH, f.rowsPerStrip = v.tileW, v.tileH, v.rows
}

func openTiled(t testing.TB, data []byte) codec.TileReader {
	t.Helper()
	rd, err := Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatalf("OpenTiled: %v", err)
	}
	t.Cleanup(func() { rd.Close() })
	return rd
}

func requireIdentical(t testing.TB, got, want *image.NRGBA, what string) {
	t.Helper()
	if got == nil || got.Bounds() != want.Bounds() {
		t.Fatalf("%s: bounds %v, want %v", what, got.Bounds(), want.Bounds())
	}
	if !testimg.Identical(got, want) {
		bounds := want.Bounds()
		for y := 0; y < bounds.Dy(); y++ {
			for x := 0; x < bounds.Dx(); x++ {
				if got.NRGBAAt(x, y) != want.NRGBAAt(x, y) {
					t.Fatalf("%s: first difference at (%d,%d): got %v want %v", what, x, y, got.NRGBAAt(x, y), want.NRGBAAt(x, y))
				}
			}
		}
	}
}

func TestSupportedLayoutsMatchReference(t *testing.T) {
	crossChecked := 0
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			for _, comp := range compressions {
				for _, v := range matrixVariants {
					for _, predictor := range k.predictors() {
						f := k.fixture(53, 41, int64(comp)*31+int64(predictor))
						f.compression = comp
						f.predictor = predictor
						v.apply(&f)
						name := fmt.Sprintf("comp%d/%s/pred%d", comp, v.name, predictor)
						data := f.bytes()
						want := f.expected()
						rd := openTiled(t, data)
						got, err := rd.Overview(context.Background(), 0)
						if err != nil {
							t.Fatalf("%s: %v", name, err)
						}
						requireIdentical(t, got, want, name)
						info := rd.Info()
						if info.Width != 53 || info.Height != 41 || info.Format != codec.TIFF || info.Frames != 1 {
							t.Fatalf("%s: info %+v", name, info)
						}
						img, err := decode(data)
						if err != nil {
							t.Fatalf("%s: Decode: %v", name, err)
						}
						requireIdentical(t, img.First(), want, name+" Decode")
						if img.Scale != 1 {
							t.Fatalf("%s: scale %d", name, img.Scale)
						}
						if reference, err := xtiff.Decode(bytes.NewReader(data)); err == nil {
							crossChecked++
							requireIdentical(t, convert.NRGBA(reference), want, name+" x/image")
						}
						level := want
						for shift := 1; shift <= 3; shift++ {
							level = reduce.Halve(level)
							got, err := rd.Overview(context.Background(), shift)
							if err != nil {
								t.Fatalf("%s: Overview(%d): %v", name, shift, err)
							}
							requireIdentical(t, got, level, fmt.Sprintf("%s Overview(%d)", name, shift))
						}
					}
				}
			}
		})
	}
	if crossChecked < 600 {
		t.Fatalf("only %d files were cross-checked against x/image", crossChecked)
	}
}

func TestEndiannessAndBigTIFF(t *testing.T) {
	for _, k := range kinds {
		for _, bigTIFF := range []bool{false, true} {
			for _, bigEndian := range []bool{false, true} {
				for _, v := range []variant{variants[0], variants[2], variants[5], variants[7]} {
					for _, comp := range []int{1, 5, 8, 32773} {
						f := k.fixture(37, 29, 7)
						f.bigTIFF, f.bigEndian, f.compression = bigTIFF, bigEndian, comp
						if k.bits >= 8 {
							f.predictor = 2
						}
						v.apply(&f)
						name := fmt.Sprintf("%s big=%v be=%v %s comp=%d", k.name, bigTIFF, bigEndian, v.name, comp)
						data := f.bytes()
						rd := openTiled(t, data)
						got, err := rd.Overview(context.Background(), 0)
						if err != nil {
							t.Fatalf("%s: %v", name, err)
						}
						requireIdentical(t, got, f.expected(), name)
						img, err := decode(data)
						if err != nil {
							t.Fatalf("%s: Decode %v", name, err)
						}
						requireIdentical(t, img.First(), f.expected(), name+" Decode")
					}
				}
			}
		}
	}
}

func TestBigTIFFIsDecodedByTheNewReaderOnly(t *testing.T) {
	f := kinds[12].fixture(20, 10, 3)
	f.bigTIFF = true
	data := f.bytes()
	if _, err := xtiff.Decode(bytes.NewReader(data)); err == nil {
		t.Fatal("x/image unexpectedly reads BigTIFF")
	}
	info, err := probe(data)
	if err != nil || info.Width != 20 || info.Height != 10 || info.BitDepth != 24 {
		t.Fatalf("%+v %v", info, err)
	}
	img, err := decode(data)
	if err != nil {
		t.Fatal(err)
	}
	requireIdentical(t, img.First(), f.expected(), "bigtiff")
}

func TestLZWTableResetsAndWidthChanges(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	f := fixture{width: 300, height: 200, bits: 8, samples: 3, photometric: 2, compression: 5}
	f.values = make([]uint16, f.width*f.height*f.samples)
	for i := range f.values {
		f.values[i] = uint16(rng.Intn(256))
		if i%11 < 6 && i > 3 {
			f.values[i] = f.values[i-3]
		}
	}
	for _, v := range []variant{variants[0], variants[3], variants[7]} {
		v.apply(&f)
		data := f.bytes()
		rd := openTiled(t, data)
		got, err := rd.Overview(context.Background(), 0)
		if err != nil {
			t.Fatal(err)
		}
		requireIdentical(t, got, f.expected(), v.name)
		if reference, err := xtiff.Decode(bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		} else {
			requireIdentical(t, convert.NRGBA(reference), f.expected(), v.name+" x/image")
		}
	}
}

func TestPackBitsWithNoOpBytes(t *testing.T) {
	f := kinds[12].fixture(64, 33, 4)
	f.compression = 32773
	f.noops = true
	for _, v := range variants {
		v.apply(&f)
		rd := openTiled(t, f.bytes())
		got, err := rd.Overview(context.Background(), 0)
		if err != nil {
			t.Fatal(err)
		}
		requireIdentical(t, got, f.expected(), v.name)
	}
}

func TestDeflateOldTagAndMissingCompressionTag(t *testing.T) {
	f := kinds[12].fixture(30, 20, 5)
	f.compression = 32946
	rd := openTiled(t, f.bytes())
	got, err := rd.Overview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	requireIdentical(t, got, f.expected(), "adobe deflate")
	f = kinds[12].fixture(30, 20, 5)
	f.compression = 1
	f.omit = []uint16{0x103}
	rd = openTiled(t, f.bytes())
	got, err = rd.Overview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	requireIdentical(t, got, f.expected(), "no compression tag")
}

func TestUnsortedTagsAreAccepted(t *testing.T) {
	f := kinds[12].fixture(10, 10, 6)
	data := f.bytes()
	o := f.byteOrder()
	count := int(o.Uint16(data[o.Uint32(data[4:]):]))
	start := int(o.Uint32(data[4:])) + 2
	first := append([]byte(nil), data[start:start+12]...)
	copy(data[start:], data[start+12:start+24])
	copy(data[start+12:], first)
	if count < 2 {
		t.Fatal("not enough tags")
	}
	img, err := decode(data)
	if err != nil {
		t.Fatal(err)
	}
	requireIdentical(t, img.First(), f.expected(), "unsorted")
}

func TestMetadataFromTheFirstDirectory(t *testing.T) {
	for _, bigTIFF := range []bool{false, true} {
		for value := 1; value <= 8; value++ {
			f := kinds[14].fixture(9, 7, 8)
			f.bigTIFF = bigTIFF
			f.orientation = value
			info, err := probe(f.bytes())
			if err != nil {
				t.Fatal(err)
			}
			if info.Orientation != codec.Orientation(value) || !info.HasAlpha || info.BitDepth != 32 || info.Format != codec.TIFF || info.FormatName != "TIFF" || info.Frames != 1 {
				t.Fatalf("big=%v value=%d: %+v", bigTIFF, value, info)
			}
		}
	}
	f := kinds[3].fixture(9, 7, 9)
	f.orientation = 9
	if info, _ := probe(f.bytes()); info.Orientation != codec.OrientNormal || info.BitDepth != 8 || info.HasAlpha {
		t.Fatalf("%+v", info)
	}
}
