package tiff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"math/rand"
	"runtime"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/reduce"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func (f fixture) expectedDeep() *image.NRGBA64 {
	img := image.NewNRGBA64(image.Rect(0, 0, f.width, f.height))
	for y := 0; y < f.height; y++ {
		for x := 0; x < f.width; x++ {
			s := func(c int) uint32 { return uint32(f.sampleAt(x, y, c)) }
			var p [4]uint32
			switch {
			case f.photometric == 0 || f.photometric == 1:
				g := s(0)
				if f.photometric == 0 {
					g = 0xffff - g
				}
				p = [4]uint32{g, g, g, 0xffff}
			case f.samples == 3:
				p = [4]uint32{s(0), s(1), s(2), 0xffff}
			case f.extra == 2:
				p = [4]uint32{s(0), s(1), s(2), s(3)}
			default:
				a := s(3)
				p = [4]uint32{s(0), s(1), s(2), a}
				switch a {
				case 0xffff:
				case 0:
					p = [4]uint32{}
				default:
					for c := 0; c < 3; c++ {
						p[c] = min(p[c]*0xffff/a, 0xffff)
					}
				}
			}
			o := img.PixOffset(x, y)
			for c := 0; c < 4; c++ {
				img.Pix[o+2*c], img.Pix[o+2*c+1] = byte(p[c]>>8), byte(p[c])
			}
		}
	}
	return img
}

func deepKinds() []kind {
	var out []kind
	for _, k := range kinds {
		if k.bits == 16 {
			out = append(out, k)
		}
	}
	return out
}

func requireDeepIdentical(t testing.TB, got, want *image.NRGBA64, what string) {
	t.Helper()
	if got == nil || got.Rect != want.Rect {
		t.Fatalf("%s: bounds %v, want %v", what, got, want.Rect)
	}
	if bytes.Equal(got.Pix, want.Pix) {
		return
	}
	for y := 0; y < want.Rect.Dy(); y++ {
		for x := 0; x < want.Rect.Dx(); x++ {
			if got.NRGBA64At(x, y) != want.NRGBA64At(x, y) {
				t.Fatalf("%s: first difference at (%d,%d): got %v want %v", what, x, y, got.NRGBA64At(x, y), want.NRGBA64At(x, y))
			}
		}
	}
	t.Fatalf("%s: pixels differ", what)
}

func deepOptions() codec.Options {
	return codec.Options{Limits: codec.DefaultLimits(), Deep: true}
}

func decodeDeep(t testing.TB, data []byte, o codec.Options) *codec.Image {
	t.Helper()
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), o)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func openDeep(t testing.TB, data []byte, cacheBytes int64) codec.DeepTileReader {
	t.Helper()
	rd, err := Decoder{CacheBytes: cacheBytes}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rd.Close() })
	deep, ok := rd.(codec.DeepTileReader)
	if !ok {
		t.Fatal("tile reader does not implement DeepTileReader")
	}
	return deep
}

func TestDeepDecodeReturnsEverySampleExactly(t *testing.T) {
	for _, k := range deepKinds() {
		t.Run(k.name, func(t *testing.T) {
			for _, comp := range compressions {
				for _, v := range matrixVariants {
					for _, predictor := range k.predictors() {
						for _, bigEndian := range []bool{false, true} {
							f := k.fixture(53, 41, int64(comp)*31+int64(predictor))
							f.compression, f.predictor, f.bigEndian = comp, predictor, bigEndian
							f.bigTIFF = predictor == 2 && bigEndian
							v.apply(&f)
							name := fmt.Sprintf("comp%d/%s/pred%d/be%v", comp, v.name, predictor, bigEndian)
							img := decodeDeep(t, f.bytes(), deepOptions())
							frame := img.Frames[0]
							if frame.Pix != nil || frame.Deep == nil {
								t.Fatalf("%s: deep %v pix %v", name, frame.Deep != nil, frame.Pix != nil)
							}
							requireDeepIdentical(t, frame.Deep, f.expectedDeep(), name)
							if img.Scale != 1 {
								t.Fatalf("%s: scale %d", name, img.Scale)
							}
						}
					}
				}
			}
		})
	}
}

func TestDeepIsNotReturnedUnlessAskedFor(t *testing.T) {
	for _, k := range deepKinds() {
		f := k.fixture(30, 20, 3)
		img := decodeDeep(t, f.bytes(), codec.Options{Limits: codec.DefaultLimits()})
		if img.Frames[0].Deep != nil || img.Frames[0].Pix == nil {
			t.Fatalf("%s: Deep leaked into the default path", k.name)
		}
		requireIdentical(t, img.Frames[0].Pix, f.expected(), k.name)
	}
}

func TestEightBitAndIndexedKindsStayEightBitWithDeep(t *testing.T) {
	for _, k := range kinds {
		if k.bits == 16 {
			continue
		}
		f := k.fixture(30, 20, 3)
		img := decodeDeep(t, f.bytes(), deepOptions())
		if img.Frames[0].Deep != nil || img.Frames[0].Pix == nil {
			t.Fatalf("%s became deep", k.name)
		}
		requireIdentical(t, img.Frames[0].Pix, f.expected(), k.name)
		rd := openDeep(t, f.bytes(), 0)
		if rd.Deep() {
			t.Fatalf("%s: Deep() is true", k.name)
		}
		if _, err := rd.OverviewDeep(context.Background(), 0); !errors.Is(err, codec.ErrUnsupported) {
			t.Fatalf("%s: OverviewDeep %v", k.name, err)
		}
		if _, err := rd.RegionDeep(context.Background(), 0, image.Rect(0, 0, 4, 4)); !errors.Is(err, codec.ErrUnsupported) {
			t.Fatalf("%s: RegionDeep %v", k.name, err)
		}
	}
}

func TestDeepScaleAndRegionOfTheWholeReader(t *testing.T) {
	f := deepKinds()[2].fixture(103, 77, 9)
	f.compression, f.predictor = 8, 2
	variants[5].apply(&f)
	data := f.bytes()
	want := f.expectedDeep()
	level := want
	for shift := 1; shift <= 3; shift++ {
		level = reduce.HalveDeep(level)
		o := deepOptions()
		o.Scale = 1 << uint(shift)
		img := decodeDeep(t, data, o)
		requireDeepIdentical(t, img.Frames[0].Deep, level, fmt.Sprintf("scale %d", 1<<shift))
	}
	region := image.Rect(10, 6, 60, 50)
	o := deepOptions()
	o.Region = &region
	img := decodeDeep(t, data, o)
	requireDeepIdentical(t, img.Frames[0].Deep, cropDeep(want, region), "region")
}

func TestTileReaderDeepLevelsEqualTheReducedReference(t *testing.T) {
	ctx := context.Background()
	for _, k := range deepKinds() {
		for _, v := range []variant{variants[0], variants[2], variants[5], variants[6], variants[8]} {
			for _, comp := range []int{1, 8} {
				name := fmt.Sprintf("%s/%s/comp%d", k.name, v.name, comp)
				f := k.fixture(211, 157, 77)
				f.compression = comp
				if comp == 8 {
					f.predictor = 2
				}
				v.apply(&f)
				data := f.bytes()
				rd := openDeep(t, data, 0)
				if !rd.Deep() {
					t.Fatalf("%s: Deep() is false", name)
				}
				level := f.expectedDeep()
				for shift := 0; shift <= 6; shift++ {
					if shift > 0 {
						level = reduce.HalveDeep(level)
					}
					got, err := rd.OverviewDeep(ctx, shift)
					if err != nil {
						t.Fatalf("%s: OverviewDeep(%d): %v", name, shift, err)
					}
					requireDeepIdentical(t, got, level, fmt.Sprintf("%s OverviewDeep(%d)", name, shift))
				}
			}
		}
	}
}

func TestDeepRegionEqualsCropOfOverviewWithAndWithoutCache(t *testing.T) {
	ctx := context.Background()
	for _, k := range deepKinds() {
		for _, v := range []variant{variants[1], variants[3], variants[5], variants[7]} {
			name := fmt.Sprintf("%s/%s", k.name, v.name)
			f := k.fixture(171, 133, 5)
			f.compression, f.predictor = 5, 2
			v.apply(&f)
			data := f.bytes()
			for _, cacheBytes := range []int64{0, 3000, 8 << 20} {
				rd := openDeep(t, data, cacheBytes)
				rng := rand.New(rand.NewSource(int64(len(name)) + cacheBytes))
				for shift := 0; shift <= 5; shift++ {
					overview, err := rd.OverviewDeep(ctx, shift)
					if err != nil {
						t.Fatal(err)
					}
					lw, lh := codec.LevelSize(f.width, f.height, shift)
					rects := []image.Rectangle{image.Rect(0, 0, lw, lh), image.Rect(0, 0, 1, 1), image.Rect(lw-1, lh-1, lw, lh), image.Rect(-5, -5, lw+5, lh+5), image.Rect(lw, lh, lw+4, lh+4)}
					for i := 0; i < 6; i++ {
						rects = append(rects, randomRect(rng, lw, lh))
					}
					for _, r := range rects {
						region, err := rd.RegionDeep(ctx, shift, r)
						if err != nil {
							t.Fatalf("%s: RegionDeep(%d, %v): %v", name, shift, r, err)
						}
						want := cropDeep(overview, r.Canon())
						if region.Rect.Empty() && want.Rect.Empty() {
							continue
						}
						requireDeepIdentical(t, region, want, fmt.Sprintf("%s cache %d RegionDeep(%d, %v)", name, cacheBytes, shift, r))
					}
				}
			}
		}
	}
}

func TestDeepAndEightBitPathsOfTheSameReaderAgree(t *testing.T) {
	ctx := context.Background()
	for _, k := range deepKinds() {
		f := k.fixture(97, 61, 11)
		f.compression = 8
		variants[5].apply(&f)
		rd := openDeep(t, f.bytes(), 4<<20)
		for pass := 0; pass < 2; pass++ {
			eight, err := rd.Region(ctx, 0, image.Rect(5, 5, 70, 50))
			if err != nil {
				t.Fatal(err)
			}
			deep, err := rd.RegionDeep(ctx, 0, image.Rect(5, 5, 70, 50))
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < eight.Rect.Dy(); y++ {
				for x := 0; x < eight.Rect.Dx(); x++ {
					e := eight.NRGBAAt(x, y)
					d := deep.NRGBA64At(x, y)
					if uint16(e.R) != d.R>>8 || uint16(e.G) != d.G>>8 || uint16(e.B) != d.B>>8 || uint16(e.A) != d.A>>8 {
						if k.extra == 1 && d.A != 0xffff {
							continue
						}
						t.Fatalf("%s pass %d: 8 bit %v deep %v at %d,%d", k.name, pass, e, d, x, y)
					}
				}
			}
		}
	}
}

func TestDeepStripWithAHugeRowKeepsBandsSmall(t *testing.T) {
	f := deepKinds()[2].fixture(4000, 300, 2)
	f.compression = 1
	variants[0].apply(&f)
	data := f.bytes()
	rd := openDeep(t, data, 0)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got, err := rd.OverviewDeep(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	resultBytes := uint64(len(got.Pix))
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > resultBytes+8<<20 {
		t.Fatalf("allocated %d KiB for a %d KiB result", allocated>>10, resultBytes>>10)
	}
	want := f.expectedDeep()
	want = reduce.HalveDeep(reduce.HalveDeep(want))
	requireDeepIdentical(t, got, want, "single strip level 2")
}

func profileTag(profile []byte) rawTag {
	values := make([]uint64, len(profile))
	for i, b := range profile {
		values[i] = uint64(b)
	}
	return rawTag{tag: 0x8773, typ: 1, values: values}
}

func TestColorFromTheProfileTag(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 700)
	for _, k := range []kind{kinds[12], kinds[13], kinds[4]} {
		for _, big := range []bool{false, true} {
			for _, tileW := range []int{0, 16} {
				f := k.fixture(40, 30, 1)
				f.bigTIFF, f.custom = big, []rawTag{profileTag(profile)}
				if tileW > 0 {
					variants[5].apply(&f)
				}
				data := f.bytes()
				img := decodeDeep(t, data, deepOptions())
				if !bytes.Equal(img.Color.ICC, profile) || img.Color.CICP != nil {
					t.Fatalf("%s big=%v tile=%d: %d bytes", k.name, big, tileW, len(img.Color.ICC))
				}
				colored, ok := openDeep(t, data, 0).(codec.ColorTileReader)
				if !ok || !bytes.Equal(colored.Color().ICC, profile) {
					t.Fatalf("%s big=%v tile=%d: tile reader colour", k.name, big, tileW)
				}
			}
		}
	}
}

func TestColorIsEmptyWithoutAUsableProfile(t *testing.T) {
	f := kinds[12].fixture(20, 20, 1)
	if got := decodeDeep(t, f.bytes(), deepOptions()).Color; !got.Empty() {
		t.Fatalf("%+v", got)
	}
	for _, space := range []string{"CMYK", "Lab "} {
		f.custom = []rawTag{profileTag(testimg.SampleICCSpace(space, 8))}
		if got := decodeDeep(t, f.bytes(), deepOptions()).Color; !got.Empty() {
			t.Fatalf("%s profile reported: %+v", space, got)
		}
	}
	f.custom = []rawTag{profileTag([]byte("not a profile"))}
	if got := decodeDeep(t, f.bytes(), deepOptions()).Color; !got.Empty() {
		t.Fatalf("%+v", got)
	}
}

func TestLegacyPathReportsDeepAndColor(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 100)
	f := kinds[13].fixture(24, 18, 6)
	f.compression = 1
	f.custom = []rawTag{profileTag(profile)}
	data := f.bytes()
	img, err := legacyDecode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits(), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if img.Frames[0].Deep == nil || !bytes.Equal(img.Color.ICC, profile) {
		t.Fatal("legacy decode lost the deep picture or the profile")
	}
	requireDeepIdentical(t, img.Frames[0].Deep, f.expectedDeep(), "legacy")
	region := image.Rect(3, 2, 15, 11)
	img, err = legacyDecode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits(), &region, true)
	if err != nil {
		t.Fatal(err)
	}
	requireDeepIdentical(t, img.Frames[0].Deep, cropDeep(f.expectedDeep(), region), "legacy region")
}

func TestDeepHostileInputsFailCleanly(t *testing.T) {
	f := deepKinds()[2].fixture(64, 64, 1)
	f.compression = 8
	variants[5].apply(&f)
	data := f.bytes()
	for cut := 1; cut < len(data); cut += 97 {
		img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data[:cut]), int64(cut), deepOptions())
		if err == nil && (img == nil || len(img.Frames) == 0) {
			t.Fatalf("cut %d: no error and no picture", cut)
		}
	}
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 200; i++ {
		bad := append([]byte(nil), data...)
		for j := 0; j < 8; j++ {
			bad[rng.Intn(len(bad))] ^= byte(1 + rng.Intn(255))
		}
		Decoder{}.Decode(context.Background(), bytes.NewReader(bad), int64(len(bad)), deepOptions())
		if rd, err := (Decoder{}).OpenTiled(context.Background(), bytes.NewReader(bad), int64(len(bad)), codec.DefaultLimits()); err == nil {
			rd.(codec.DeepTileReader).OverviewDeep(context.Background(), 1)
			rd.(codec.DeepTileReader).RegionDeep(context.Background(), 0, image.Rect(0, 0, 30, 30))
			rd.Close()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rd := openDeep(t, data, 0)
	if _, err := rd.OverviewDeep(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	rd.Close()
	if _, err := rd.OverviewDeep(context.Background(), 0); err == nil {
		t.Fatal("closed reader still decodes")
	}
}
