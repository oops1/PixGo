package raw

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	xtiff "golang.org/x/image/tiff"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func head(data []byte) []byte {
	return data[:min(len(data), codec.SniffLen)]
}

func structural(tb testing.TB, data []byte) []candidate {
	tb.Helper()
	ctx := context.Background()
	reader := bytes.NewReader(data)
	size := int64(len(data))
	f := newFinder(ctx, reader, size)
	var err error
	switch identify(head(data)) {
	case containerTIFF:
		_, _, err = discoverTIFF(ctx, reader, size, f)
	case containerCR3:
		_, _, err = discoverCR3(ctx, reader, size, f)
	case containerRAF:
		err = discoverRAF(reader, size, f)
	default:
		tb.Fatal("unknown container")
	}
	if err != nil {
		tb.Fatal(err)
	}
	return f.sorted()
}

type sample struct {
	name string
	data []byte
}

func allSamples(tb testing.TB) []sample {
	p := makePreviews(tb)
	return []sample{
		{"CR2", cr2File(p, 0)},
		{"NEF little endian", nefFile(little, p, 0)},
		{"NEF big endian", nefFile(big, p, 0)},
		{"ARW", arwFile(p)},
		{"DNG little endian", dngFile(little, p, 0)},
		{"DNG big endian", dngFile(big, p, 0)},
		{"RW2", rw2File(p)},
		{"ORF new maker note", orfFile(p, "new")},
		{"ORF old maker note", orfFile(p, "old")},
		{"NEF Nikon maker note", nikonMakerNoteFile(p)},
		{"CR3", cr3File(p, 0)},
		{"RAF", rafFile(p.large)},
	}
}

func TestIdentity(t *testing.T) {
	d := Decoder{}
	if d.ID() != codec.RAW || d.Name() != "RAW (встроенный просмотр)" {
		t.Fatalf("%s %q", d.ID(), d.Name())
	}
	want := ".cr2 .cr3 .nef .arw .dng .orf .rw2 .pef .raf"
	if got := strings.Join(d.Extensions(), " "); got != want {
		t.Fatalf("extensions %q, want %q", got, want)
	}
}

func TestEveryContainerYieldsItsLargestPreviewWithoutScanning(t *testing.T) {
	for _, s := range allSamples(t) {
		found := structural(t, s.data)
		if len(found) == 0 || found[0].head.width != 96 || found[0].head.height != 64 {
			t.Errorf("%s: candidates %+v", s.name, found)
			continue
		}
		if s.name != "CR3" && found[0].carved {
			t.Errorf("%s: the preview was found by scanning, not by structure", s.name)
		}
	}
}

func TestProbeAndDecodeOfEveryContainer(t *testing.T) {
	for _, s := range allSamples(t) {
		info, err := probeData(s.data)
		if err != nil {
			t.Fatalf("%s: probe: %v", s.name, err)
		}
		want := codec.Info{Format: codec.RAW, FormatName: "RAW", Width: 96, Height: 64, Frames: 1, Orientation: codec.OrientNormal, BitDepth: 24, Preview: true}
		if info != want {
			t.Fatalf("%s: %+v, want %+v", s.name, info, want)
		}
		img, err := decodeData(s.data)
		if err != nil {
			t.Fatalf("%s: decode: %v", s.name, err)
		}
		if img.Info != info || len(img.Frames) != 1 || img.Scale != 1 || img.Loop != 0 {
			t.Fatalf("%s: decoded %+v", s.name, img.Info)
		}
		pix := img.First()
		if pix.Bounds().Dx() != 96 || pix.Bounds().Dy() != 64 {
			t.Fatalf("%s: %v", s.name, pix.Bounds())
		}
		if got := pix.NRGBAAt(48, 32); !closeTo(got, tomato, 12) {
			t.Fatalf("%s: centre pixel %v, want about %v", s.name, got, tomato)
		}
	}
}

func TestVendorSpecificPreviewsAreOptionalBecauseTheFileIsScanned(t *testing.T) {
	p := makePreviews(t)
	broken := orfFile(p, "new")
	broken = bytes.Replace(broken, []byte("OLYMPUS\x00"), []byte("OLYMPXS\x00"), 1)
	if found := structural(t, broken); found[0].head.width != 16 {
		t.Fatalf("structure alone should only see the thumbnail: %+v", found[0].head)
	}
	if info := mustProbe(t, broken); info.Width != 96 || info.Height != 64 {
		t.Fatalf("scanning missed the preview: %+v", info)
	}
	carved := carvedFile(p, 3000)
	if found := structural(t, carved); found[0].head.width != 16 {
		t.Fatalf("%+v", found)
	}
	if info := mustProbe(t, carved); info.Width != 96 {
		t.Fatalf("%+v", info)
	}
}

func TestScanningIsSkippedWhenTheStructureFoundALargePreview(t *testing.T) {
	p := previews{large: jpegOf(t, 1200, 900, tomato), thumb: jpegOf(t, 16, 12, mustard)}
	data := cr2File(p, 0)
	data = append(data, jpegOf(t, 2000, 1500, navy)...)
	if info := mustProbe(t, data); info.Width != 1200 {
		t.Fatalf("the unreferenced JPEG was used: %+v", info)
	}
}

func TestOrientationComesFromTheRawThenFromThePreview(t *testing.T) {
	p := makePreviews(t)
	for v := uint16(1); v <= 8; v++ {
		for name, data := range map[string][]byte{
			"CR2":        cr2File(p, v),
			"NEF little": nefFile(little, p, v),
			"NEF big":    nefFile(big, p, v),
			"DNG little": dngFile(little, p, v),
			"DNG big":    dngFile(big, p, v),
			"CR3":        cr3File(p, v),
		} {
			info := mustProbe(t, data)
			if info.Orientation != codec.Orientation(v) {
				t.Fatalf("%s %d: %v", name, v, info.Orientation)
			}
			if img := mustDecode(t, data); img.Info.Orientation != codec.Orientation(v) {
				t.Fatalf("%s %d decode: %v", name, v, img.Info.Orientation)
			}
		}
	}
	tilted := previews{large: withExifOrientation(p.large, 6, false), thumb: p.thumb}
	for name, data := range map[string][]byte{
		"ARW without its own orientation": arwFile(tilted),
		"RAF":                             rafFile(tilted.large),
		"CR3 without CMT1":                cr3File(tilted, 0),
	} {
		if info := mustProbe(t, data); info.Orientation != codec.OrientRotate90 {
			t.Fatalf("%s: preview orientation not used: %v", name, info.Orientation)
		}
		if img := mustDecode(t, data); img.Info.Orientation != codec.OrientRotate90 {
			t.Fatalf("%s: decode: %v", name, img.Info.Orientation)
		}
	}
	both := nefFile(little, tilted, 3)
	if info := mustProbe(t, both); info.Orientation != codec.OrientRotate180 {
		t.Fatalf("the RAW orientation must win over the preview one: %v", info.Orientation)
	}
	if info := mustProbe(t, rafFile(p.large)); info.Orientation != codec.OrientNormal {
		t.Fatalf("no orientation anywhere: %v", info.Orientation)
	}
}

func TestLargestPreviewWithinTheLimitsIsChosen(t *testing.T) {
	data := nefFile(little, makePreviews(t), 0)
	cases := []struct {
		name   string
		pixels int64
		width  int
		err    error
	}{
		{"no limit", 0, 96, nil},
		{"fits exactly", 96 * 64, 96, nil},
		{"just below the largest", 96*64 - 1, 48, nil},
		{"only the thumbnail fits", 200, 16, nil},
		{"nothing fits", 100, 0, codec.ErrTooLarge},
	}
	for _, c := range cases {
		lim := codec.DefaultLimits()
		if c.pixels > 0 {
			lim.MaxPixels = c.pixels
		}
		reader := bytes.NewReader(data)
		info, err := Decoder{}.Probe(context.Background(), reader, int64(len(data)), lim)
		img, derr := Decoder{}.Decode(context.Background(), reader, int64(len(data)), codec.Options{Limits: lim})
		if c.err != nil {
			if !errors.Is(err, c.err) || !errors.Is(derr, c.err) {
				t.Errorf("%s: %v / %v", c.name, err, derr)
			}
			continue
		}
		if err != nil || derr != nil || info.Width != c.width || img.First().Bounds().Dx() != c.width {
			t.Errorf("%s: %+v %v %v", c.name, info, err, derr)
		}
	}
}

func rafWithOffsets(offset, length uint32) []byte {
	return patchRAF(rafFile([]byte("not a jpeg")), offset, length)
}

func patchRAF(base []byte, offset, length uint32) []byte {
	data := append([]byte(nil), base...)
	putBE32(data[84:], offset)
	putBE32(data[88:], length)
	return data
}

func putBE32(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
}

func TestFilesWithoutAUsablePreviewAreReportedHonestly(t *testing.T) {
	bare := func() []byte {
		f := standardTIFF(little)
		ifd0, _ := f.ifd(f.asciiEntry(tagMake, "NIKON CORPORATION"), longEntry(0x0100, 100))
		f.setFirst(ifd0)
		return f.data()
	}()
	losslessOnly := func() []byte {
		f := standardTIFF(little)
		raw := f.later(losslessJPEG(300, 200))
		ifd0, _ := f.ifd(join(stripTags(raw, compressionJPEG), []entry{dngVersion()})...)
		f.setFirst(ifd0)
		return f.data()
	}()
	twelveBit := func() []byte {
		f := standardTIFF(little)
		prev := f.later(frameJPEG(0xc1, 12, 100, 80, 3))
		ifd0, _ := f.ifd(join(jpegTags(prev), []entry{dngVersion()})...)
		f.setFirst(ifd0)
		return f.data()
	}()
	beyond := func() []byte {
		f := standardTIFF(little)
		ifd0, _ := f.ifd(longEntry(tagJPEGOffset, 0x7ffffff0), longEntry(tagJPEGLength, 5000), f.asciiEntry(tagMake, "SONY"))
		f.setFirst(ifd0)
		return f.data()
	}()
	cases := []struct {
		name string
		data []byte
		want error
		text string
	}{
		{"no JPEG at all", bare, codec.ErrUnsupported, "no embedded JPEG"},
		{"only a lossless raw stream", losslessOnly, codec.ErrUnsupported, "unsupported JPEG"},
		{"12-bit JPEG", twelveBit, codec.ErrUnsupported, "unsupported JPEG"},
		{"preview offset beyond the file", beyond, codec.ErrCorrupt, "outside the file"},
		{"RAF pointing nowhere", rafWithOffsets(0x7fff0000, 400), codec.ErrCorrupt, "outside the file"},
		{"RAF with zero offsets", rafWithOffsets(0, 0), codec.ErrUnsupported, "no embedded JPEG"},
	}
	for _, c := range cases {
		_, err := probeData(c.data)
		if !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.text) {
			t.Errorf("%s: probe: %v", c.name, err)
		}
		if _, err := decodeData(c.data); !errors.Is(err, c.want) {
			t.Errorf("%s: decode: %v", c.name, err)
		}
	}
}

func TestForeignAndDamagedContainersAreCorrupt(t *testing.T) {
	plain := func() []byte {
		var buf bytes.Buffer
		if err := xtiff.Encode(&buf, testimg.Noise(8, 8, 1, false), nil); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}()
	for name, data := range map[string][]byte{
		"jpeg":        jpegOf(t, 8, 8, tomato),
		"empty":       nil,
		"one byte":    {'I'},
		"text":        []byte("this is not a raw file at all, just text, long enough to pass"),
		"bigtiff":     {'I', 'I', 43, 0, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		"bad IFD":     {'I', 'I', 42, 0, 0xff, 0xff, 0xff, 0x7f},
		"cut RAF":     []byte(rafSignature + "0201"),
		"cut CR3 box": append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'c', 'r', 'x', ' '}, 0, 0),
	} {
		if _, err := probeData(data); err == nil {
			t.Errorf("%s: probed", name)
		} else {
			testimg.RequireCodecError(t, err)
		}
		if _, err := decodeData(data); err == nil {
			t.Errorf("%s: decoded", name)
		} else {
			testimg.RequireCodecError(t, err)
		}
	}
	if _, err := probeData(plain); !errors.Is(err, codec.ErrUnsupported) {
		t.Errorf("plain TIFF: %v", err)
	}
}

func TestLimitsAndCancellation(t *testing.T) {
	data := nefFile(little, makePreviews(t), 0)
	lim := codec.DefaultLimits()
	lim.MaxFileBytes = 100
	if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("file size: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Decoder{}).Probe(ctx, bytes.NewReader(data), int64(len(data)), codec.DefaultLimits()); !errors.Is(err, context.Canceled) {
		t.Fatalf("probe: %v", err)
	}
	if _, err := (Decoder{}).Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("decode: %v", err)
	}
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{}); err != nil {
		t.Fatalf("zero options: %v", err)
	}
}

func TestGrayPreviewsAreReportedAsEightBit(t *testing.T) {
	p := previews{large: grayJPEG(t, 40, 30), thumb: grayJPEG(t, 8, 6)}
	info := mustProbe(t, arwFile(p))
	if info.BitDepth != 8 || info.Width != 40 {
		t.Fatalf("%+v", info)
	}
	if img := mustDecode(t, arwFile(p)); img.First().Bounds().Dx() != 40 {
		t.Fatalf("%v", img.First().Bounds())
	}
}

func TestChoiceOrderPrefersStructureOnEqualSize(t *testing.T) {
	f := newFinder(context.Background(), bytes.NewReader(nil), 0)
	f.list = []candidate{
		{offset: 900, head: jpegHead{width: 10, height: 10}, carved: true},
		{offset: 500, head: jpegHead{width: 10, height: 10}},
		{offset: 100, head: jpegHead{width: 10, height: 10}, carved: true},
		{offset: 700, head: jpegHead{width: 20, height: 10}, carved: true},
	}
	got := f.sorted()
	order := []int64{got[0].offset, got[1].offset, got[2].offset, got[3].offset}
	want := []int64{700, 500, 100, 900}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order %v, want %v", order, want)
		}
	}
	chosen, err := f.choose(codec.DefaultLimits())
	if err != nil || chosen.offset != 700 {
		t.Fatalf("%+v %v", chosen, err)
	}
	if span := (candidate{offset: 100}).span(1000); span != 900 {
		t.Fatalf("span %d", span)
	}
	if span := (candidate{offset: 100, length: 50}).span(1000); span != 50 {
		t.Fatalf("span %d", span)
	}
}

func TestCandidateCountIsBounded(t *testing.T) {
	jpegData := jpegOf(t, 8, 8, tomato)
	var data []byte
	for i := 0; i < maxCandidates+50; i++ {
		data = append(data, jpegData...)
	}
	f := newFinder(context.Background(), bytes.NewReader(data), int64(len(data)))
	err := carve(context.Background(), f.r, f.size, 0, f.size, func(at int64) bool { return f.add(at, 0, true) })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.list) != maxCandidates {
		t.Fatalf("%d candidates", len(f.list))
	}
	if f.add(0, 0, true) || f.add(int64(len(jpegData)*3+1), 0, true) {
		t.Fatal("candidate accepted past the cap")
	}
}

func TestPreviewLengthBeyondTheFileIsRescuedByScanning(t *testing.T) {
	p := makePreviews(t)
	f := standardTIFF(little)
	prev := f.later(p.large)
	ifd0, _ := f.ifd(refEntry(tagJPEGOffset, prev), longEntry(tagJPEGLength, uint32(len(p.large))+500), f.asciiEntry(tagMake, "SONY"))
	f.setFirst(ifd0)
	data := f.data()
	if found := structural(t, data); len(found) != 0 {
		t.Fatalf("the tag should be rejected: %+v", found)
	}
	if info := mustProbe(t, data); info.Width != 96 {
		t.Fatalf("%+v", info)
	}
	if img := mustDecode(t, data); img.First().Bounds().Dx() != 96 {
		t.Fatalf("%v", img.First().Bounds())
	}
}

func TestCutPreviewIsCorruptWhenDecoded(t *testing.T) {
	p := makePreviews(t)
	data := arwFile(p)
	cut := data[:len(data)-len(p.thumb)-len(p.large)/2]
	info, err := probeData(cut)
	if err != nil || info.Width != 96 {
		t.Fatalf("the cut preview still has a header: %+v %v", info, err)
	}
	if _, err := decodeData(cut); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("decode: %v", err)
	}
}

func TestFourComponentPreviewsAreThirtyTwoBit(t *testing.T) {
	f := standardTIFF(little)
	prev := f.later(frameJPEG(0xc0, 8, 50, 40, 4))
	ifd0, _ := f.ifd(join(jpegTags(prev), []entry{dngVersion()})...)
	f.setFirst(ifd0)
	info := mustProbe(t, f.data())
	if info.BitDepth != 32 || info.Width != 50 || info.Height != 40 {
		t.Fatalf("%+v", info)
	}
}

func TestBigEndianOlympusMakerNote(t *testing.T) {
	p := makePreviews(t)
	data := makerNoteFile(big, [4]byte{'M', 'M', 'O', 'R'}, "OLYMPUS", func(*tiffFile) []byte { return olympusNote(big, true, 0, p) }, p)
	if found := structural(t, data); len(found) == 0 || found[0].head.width != 96 || found[0].carved {
		t.Fatalf("%+v", found)
	}
}

func TestRangesOutsideTheFileMarkTheFinderAsTruncated(t *testing.T) {
	data := make([]byte, 100)
	for name, args := range map[string][2]int64{"negative offset": {-1, 5}, "no length": {10, 0}, "offset beyond": {500, 5}, "length beyond": {50, 500}} {
		f := newFinder(context.Background(), bytes.NewReader(data), int64(len(data)))
		if err := f.addWithin(args[0], args[1]); err != nil || !f.truncated {
			t.Errorf("%s: truncated=%v err=%v", name, f.truncated, err)
		}
	}
}
