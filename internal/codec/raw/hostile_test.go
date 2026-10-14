package raw

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

var knownSizes = map[[2]int]bool{{96, 64}: true, {48, 32}: true, {24, 16}: true, {16, 12}: true}

func requireSane(t *testing.T, name string, data []byte) {
	t.Helper()
	info, err := probeData(data)
	if err != nil {
		testimg.RequireCodecError(t, err)
	} else if !knownSizes[[2]int{info.Width, info.Height}] {
		t.Fatalf("%s: probe invented a %dx%d preview", name, info.Width, info.Height)
	}
	img, err := decodeData(data)
	if err != nil {
		testimg.RequireCodecError(t, err)
		return
	}
	b := img.First().Bounds()
	if !knownSizes[[2]int{b.Dx(), b.Dy()}] {
		t.Fatalf("%s: decode invented a %dx%d image", name, b.Dx(), b.Dy())
	}
}

func lastPreviewFile(p previews) []byte {
	f := standardTIFF(little)
	prev := f.later(p.large)
	ifd0, _ := f.ifd(join(jpegTags(prev), []entry{f.asciiEntry(tagMake, "SONY")})...)
	f.setFirst(ifd0)
	return f.data()
}

func TestEveryTruncatedPrefixIsRejectedOrYieldsARealPreview(t *testing.T) {
	for _, s := range allSamples(t) {
		for n := 0; n < len(s.data); n++ {
			requireSane(t, s.name, s.data[:n])
			(Decoder{}).Sniff(head(s.data[:n]))
		}
	}
}

func TestNoStrictPrefixDecodesWhenThePreviewIsLast(t *testing.T) {
	data := lastPreviewFile(makePreviews(t))
	for n := 0; n < len(data); n++ {
		if _, err := decodeData(data[:n]); err == nil {
			t.Fatalf("prefix of %d/%d bytes decoded", n, len(data))
		} else {
			testimg.RequireCodecError(t, err)
		}
	}
	if _, err := decodeData(data); err != nil {
		t.Fatal(err)
	}
}

func TestRandomDamageNeverPanicsOrInventsImages(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	for _, s := range allSamples(t) {
		for i := 0; i < 150; i++ {
			mutated := append([]byte(nil), s.data...)
			for k := 0; k < 1+rng.Intn(6); k++ {
				mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
			}
			info, err := probeData(mutated)
			if err != nil {
				testimg.RequireCodecError(t, err)
			}
			img, err := decodeData(mutated)
			if err != nil {
				testimg.RequireCodecError(t, err)
			} else if img.First() == nil || info.Width == 0 {
				t.Fatalf("%s: success without pixels", s.name)
			}
		}
	}
}

type cancelReader struct {
	r      *bytes.Reader
	after  int
	reads  int
	cancel context.CancelFunc
}

func (c *cancelReader) ReadAt(p []byte, off int64) (int, error) {
	c.reads++
	if c.after > 0 && c.reads >= c.after {
		c.cancel()
	}
	return c.r.ReadAt(p, off)
}

func TestCancellationAtEveryReadStopsTheWork(t *testing.T) {
	for _, s := range allSamples(t) {
		ctx, cancel := context.WithCancel(context.Background())
		counting := &cancelReader{r: bytes.NewReader(s.data), cancel: cancel}
		if _, err := (Decoder{}).Decode(ctx, counting, int64(len(s.data)), codec.Options{}); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		cancel()
		stopped := 0
		for after := 1; after <= counting.reads; after++ {
			ctx, cancel := context.WithCancel(context.Background())
			reader := &cancelReader{r: bytes.NewReader(s.data), after: after, cancel: cancel}
			_, err := Decoder{}.Decode(ctx, reader, int64(len(s.data)), codec.Options{})
			cancel()
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("%s: cancel at read %d: %v", s.name, after, err)
				}
				stopped++
			}
		}
		if stopped == 0 {
			t.Fatalf("%s: cancellation never took effect in %d reads", s.name, counting.reads)
		}
	}
}

type flippingReader struct {
	first, second []byte
	switchAfter   int
	reads         int
}

func (f *flippingReader) ReadAt(p []byte, off int64) (int, error) {
	f.reads++
	source := f.first
	if f.reads > f.switchAfter {
		source = f.second
	}
	if off >= int64(len(source)) {
		return 0, errors.New("EOF")
	}
	n := copy(p, source[off:])
	if n < len(p) {
		return n, errors.New("short read")
	}
	return n, nil
}

func TestFileChangingWhileItIsReadNeverPanics(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	for _, s := range allSamples(t) {
		others := [][]byte{nil, s.data[:len(s.data)/2], bytes.Repeat([]byte{0xff}, len(s.data))}
		for i := 0; i < 20; i++ {
			mutated := append([]byte(nil), s.data...)
			for k := 0; k < 8; k++ {
				mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
			}
			others = append(others, mutated)
		}
		for _, other := range others {
			for switchAfter := 0; switchAfter < 14; switchAfter++ {
				reader := &flippingReader{first: s.data, second: other, switchAfter: switchAfter}
				if _, err := (Decoder{}).Decode(context.Background(), reader, int64(len(s.data)), codec.Options{}); err != nil {
					testimg.RequireCodecError(t, err)
				}
			}
		}
	}
}

func nextOffset(f *tiffFile) uint32 {
	f.align()
	return uint32(len(f.buf))
}

func TestIFDCyclesTerminate(t *testing.T) {
	p := makePreviews(t)
	f := standardTIFF(little)
	prev := f.later(p.large)
	self := nextOffset(f)
	ifd0, next0 := f.ifd(join(jpegTags(prev), []entry{longEntry(tagSubIFDs, self), longEntry(tagExifIFD, self)})...)
	f.link(next0, ifd0)
	f.setFirst(ifd0)
	data := f.data()
	testimg.Within(t, 5*time.Second, func() {
		found := structural(t, data)
		if len(found) != 1 || found[0].head.width != 96 {
			t.Errorf("%+v", found)
		}
	})
}

func TestNestedAndLongIFDChainsAreBounded(t *testing.T) {
	p := makePreviews(t)
	nested := func(depth int) []byte {
		f := standardTIFF(little)
		prev := f.later(p.large)
		offset, _ := f.ifd(jpegTags(prev)...)
		for i := 0; i < depth; i++ {
			offset, _ = f.ifd(longEntry(tagSubIFDs, offset))
		}
		f.setFirst(offset)
		return f.data()
	}
	if found := structural(t, nested(maxIFDDepth-1)); len(found) != 1 {
		t.Fatalf("shallow nesting: %+v", found)
	}
	if found := structural(t, nested(maxIFDDepth+5)); len(found) != 0 {
		t.Fatalf("deep nesting should not reach the preview: %+v", found)
	}
	if found := structural(t, nested(500)); len(found) != 0 {
		t.Fatalf("%+v", found)
	}
	chain := func(length, previewAt int) []byte {
		f := standardTIFF(little)
		prev := f.later(p.large)
		var first uint32
		prevNext := 0
		for i := 0; i < length; i++ {
			entries := []entry{longEntry(0x0100, uint32(i+1))}
			if i == previewAt {
				entries = append(entries, jpegTags(prev)...)
			}
			offset, next := f.ifd(entries...)
			if i == 0 {
				first = offset
			} else {
				f.link(prevNext, offset)
			}
			prevNext = next
		}
		f.setFirst(first)
		return f.data()
	}
	if found := structural(t, chain(200, 10)); len(found) != 1 {
		t.Fatalf("an early preview in a long chain: %+v", found)
	}
	if found := structural(t, chain(200, 150)); len(found) != 0 {
		t.Fatalf("the IFD budget was exceeded: %+v", found)
	}
}

func TestSubIFDArraysAreBounded(t *testing.T) {
	p := makePreviews(t)
	f := standardTIFF(little)
	prev := f.later(p.large)
	var offsets []uint32
	for i := 0; i < maxSubIFDs+8; i++ {
		var entries []entry
		if i == maxSubIFDs+4 {
			entries = jpegTags(prev)
		} else {
			entries = []entry{longEntry(0x0100, uint32(i))}
		}
		offset, _ := f.ifd(entries...)
		offsets = append(offsets, offset)
	}
	ifd0, _ := f.ifd(f.longsEntry(tagSubIFDs, offsets...))
	f.setFirst(ifd0)
	if found := structural(t, f.data()); len(found) != 0 {
		t.Fatalf("only %d SubIFDs are followed: %+v", maxSubIFDs, found)
	}
	huge := standardTIFF(little)
	ifd, _ := huge.ifd(entry{tag: tagSubIFDs, typ: typeLong, count: 5_000_000, value: 8})
	huge.setFirst(ifd)
	if found := structural(t, huge.data()); len(found) != 0 {
		t.Fatalf("%+v", found)
	}
}

func TestDirectoriesWithAbsurdEntryCountsYieldNothing(t *testing.T) {
	data := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 0xff, 0xff}
	data = append(data, make([]byte, 200)...)
	if found := structural(t, data); len(found) != 0 {
		t.Fatalf("%+v", found)
	}
	if _, err := probeData(data); !errors.Is(err, codec.ErrUnsupported) {
		t.Fatalf("%v", err)
	}
}

func TestStripTagsAreValidatedBeforeUse(t *testing.T) {
	p := makePreviews(t)
	build := func(entries func(f *tiffFile, prev *blobRef) []entry) []byte {
		f := standardTIFF(little)
		prev := f.later(p.large)
		ifd0, _ := f.ifd(entries(f, prev)...)
		f.setFirst(ifd0)
		return f.data()
	}
	good := build(func(f *tiffFile, prev *blobRef) []entry { return stripTags(prev, compressionJPEG) })
	if found := structural(t, good); len(found) != 1 {
		t.Fatalf("control case: %+v", found)
	}
	cases := map[string][]byte{
		"uncompressed strips": build(func(f *tiffFile, prev *blobRef) []entry { return stripTags(prev, 1) }),
		"no compression tag": build(func(f *tiffFile, prev *blobRef) []entry {
			return []entry{refEntry(tagStripOffsets, prev), longEntry(tagStripCounts, uint32(len(prev.data)))}
		}),
		"no byte counts": build(func(f *tiffFile, prev *blobRef) []entry {
			return []entry{shortEntry(tagCompression, compressionJPEG), refEntry(tagStripOffsets, prev)}
		}),
		"count mismatch": build(func(f *tiffFile, prev *blobRef) []entry {
			return []entry{shortEntry(tagCompression, compressionJPEG), f.longsEntry(tagStripOffsets, 100, 200), longEntry(tagStripCounts, 50)}
		}),
		"too many strips": build(func(f *tiffFile, prev *blobRef) []entry {
			return []entry{shortEntry(tagCompression, compressionJPEG), entry{tag: tagStripOffsets, typ: typeLong, count: maxStrips + 1, value: 8}, entry{tag: tagStripCounts, typ: typeLong, count: maxStrips + 1, value: 8}}
		}),
		"no strips": build(func(f *tiffFile, prev *blobRef) []entry {
			return []entry{shortEntry(tagCompression, compressionJPEG), entry{tag: tagStripOffsets, typ: typeLong, count: 0, value: 8}, entry{tag: tagStripCounts, typ: typeLong, count: 0, value: 8}}
		}),
		"offsets of the wrong type": build(func(f *tiffFile, prev *blobRef) []entry {
			return []entry{shortEntry(tagCompression, compressionJPEG), entry{tag: tagStripOffsets, typ: typeASCIIType, count: 1, value: 8}, longEntry(tagStripCounts, 10)}
		}),
		"counts of the wrong type": build(func(f *tiffFile, prev *blobRef) []entry {
			return []entry{shortEntry(tagCompression, compressionJPEG), refEntry(tagStripOffsets, prev), entry{tag: tagStripCounts, typ: typeASCIIType, count: 1, value: 8}}
		}),
		"offsets beyond the file": build(func(f *tiffFile, prev *blobRef) []entry {
			return []entry{shortEntry(tagCompression, compressionJPEG), longEntry(tagStripOffsets, 0xfffffff0), longEntry(tagStripCounts, 0xffffffff)}
		}),
	}
	for name, data := range cases {
		if found := structural(t, data); len(found) != 0 {
			t.Errorf("%s: %+v", name, found)
		}
	}
	split := build(func(f *tiffFile, prev *blobRef) []entry {
		half := len(prev.data) / 2
		return []entry{shortEntry(tagCompression, compressionOldJPEG), refEntry(tagStripOffsets, prev), longEntry(tagStripCounts, uint32(half))}
	})
	if found := structural(t, split); len(found) != 1 || found[0].length != int64(len(p.large)/2) {
		t.Fatalf("%+v", found)
	}
	multi := func() []byte {
		f := standardTIFF(little)
		prev := f.later(p.large)
		ifd0, _ := f.ifd(shortEntry(tagCompression, compressionJPEG), refEntry(tagStripOffsets, prev), longEntry(tagStripCounts, uint32(len(p.large))))
		f.setFirst(ifd0)
		return f.data()
	}()
	if found := structural(t, multi); len(found) != 1 {
		t.Fatalf("%+v", found)
	}
}

func TestEnormousPreviewsAreRefusedBeforeDecoding(t *testing.T) {
	for _, size := range [][2]int{{65535, 65535}, {20000, 20000}, {100000 / 2, 4000}} {
		f := standardTIFF(little)
		prev := f.later(frameJPEG(0xc0, 8, size[0], size[1], 3))
		ifd0, _ := f.ifd(join(jpegTags(prev), []entry{dngVersion()})...)
		f.setFirst(ifd0)
		data := f.data()
		testimg.Within(t, 5*time.Second, func() {
			if _, err := probeData(data); !errors.Is(err, codec.ErrTooLarge) {
				t.Errorf("%v: probe: %v", size, err)
			}
			if _, err := decodeData(data); !errors.Is(err, codec.ErrTooLarge) {
				t.Errorf("%v: decode: %v", size, err)
			}
		})
	}
}

func TestSignatureFloodsFinishQuickly(t *testing.T) {
	f := standardTIFF(little)
	ifd0, _ := f.ifd(f.asciiEntry(tagMake, "NIKON CORPORATION"))
	f.setFirst(ifd0)
	base := f.data()
	flood := append(append([]byte(nil), base...), bytes.Repeat([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x02}, (17<<20)/6)...)
	testimg.Within(t, 30*time.Second, func() {
		if _, err := probeData(flood); !errors.Is(err, codec.ErrUnsupported) {
			t.Errorf("empty signatures: %v", err)
		}
	})
	small := jpegOf(t, 8, 8, tomato)
	many := append([]byte(nil), base...)
	for len(many) < 8<<20 {
		many = append(many, small...)
	}
	testimg.Within(t, 30*time.Second, func() {
		info, err := probeData(many)
		if err != nil || info.Width != 8 {
			t.Errorf("%+v %v", info, err)
		}
	})
}

func TestMakerNotesAreDefensive(t *testing.T) {
	p := makePreviews(t)
	notes := map[string][]byte{
		"tiny":                         []byte("Nikon"),
		"Nikon with a bad TIFF":        append([]byte("Nikon\x00\x02\x10\x00\x00"), bytes.Repeat([]byte{0xee}, 40)...),
		"Nikon version 1":              append([]byte("Nikon\x00\x01\x00"), bytes.Repeat([]byte{0}, 40)...),
		"Olympus bad byte order":       append([]byte("OLYMPUS\x00ZZ\x03\x00"), bytes.Repeat([]byte{0}, 60)...),
		"Olympus IFD beyond file":      append([]byte("OLYMPUS\x00II\x03\x00"), bytes.Repeat([]byte{0xff}, 60)...),
		"old Olympus zeros":            append([]byte("OLYMP\x00\x01\x00"), bytes.Repeat([]byte{0}, 60)...),
		"old Olympus huge count":       append([]byte("OLYMP\x00\x01\x00\xff\xff"), bytes.Repeat([]byte{0xaa}, 60)...),
		"unknown vendor":               bytes.Repeat([]byte("PENTAX \x00"), 8),
		"Olympus camera settings loop": olympusLoop(),
	}
	for name, note := range notes {
		note := note
		data := makerNoteFile(little, [4]byte{'I', 'I', 42, 0}, "OLYMPUS", func(*tiffFile) []byte { return note }, p)
		found := structural(t, data)
		if len(found) != 1 || found[0].head.width != 16 {
			t.Errorf("%s: %+v", name, found)
		}
		requireSane(t, name, data)
	}
	empty := makerNoteFile(little, [4]byte{'I', 'I', 42, 0}, "OLYMPUS", func(*tiffFile) []byte { return nil }, p)
	requireSane(t, "no maker note", empty)
}

func olympusLoop() []byte {
	note := []byte("OLYMPUS\x00II\x03\x00")
	note = binary.LittleEndian.AppendUint16(note, 1)
	note = binary.LittleEndian.AppendUint16(note, tagOlympusCamera)
	note = binary.LittleEndian.AppendUint16(note, typeLong)
	note = binary.LittleEndian.AppendUint32(note, 1)
	note = binary.LittleEndian.AppendUint32(note, 12)
	return binary.LittleEndian.AppendUint32(note, 12)
}

func TestHostileCR3Boxes(t *testing.T) {
	p := makePreviews(t)
	jpegBox := mp4Box("mdat", p.large)
	ftyp := mp4Box("ftyp", []byte("crx "), []byte{0, 1, 0, 0}, []byte("crx isom"))
	tiny := bytes.Repeat([]byte{0, 0, 0, 8, 'f', 'r', 'e', 'e'}, maxBoxes+100)
	cases := map[string][]byte{
		"box smaller than its header":   bytes.Join([][]byte{ftyp, {0, 0, 0, 4, 'f', 'r', 'e', 'e'}, jpegBox}, nil),
		"large box with a tiny size":    bytes.Join([][]byte{ftyp, {0, 0, 0, 1, 'f', 'r', 'e', 'e', 0, 0, 0, 0, 0, 0, 0, 3}, jpegBox}, nil),
		"large box beyond the file":     bytes.Join([][]byte{ftyp, {0, 0, 0, 1, 'f', 'r', 'e', 'e', 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, jpegBox}, nil),
		"large box header cut off":      bytes.Join([][]byte{ftyp, {0, 0, 0, 1, 'f', 'r', 'e', 'e', 0, 0}}, nil),
		"box to the end of the file":    bytes.Join([][]byte{ftyp, {0, 0, 0, 0, 'm', 'd', 'a', 't'}, p.large}, nil),
		"mdat longer than the file":     bytes.Join([][]byte{ftyp, {0x7f, 0xff, 0xff, 0xff, 'm', 'd', 'a', 't'}, p.large}, nil),
		"moov with a short uuid":        bytes.Join([][]byte{ftyp, mp4Box("moov", mp4Box("uuid", []byte{1, 2, 3})), jpegBox}, nil),
		"moov with another uuid":        bytes.Join([][]byte{ftyp, mp4Box("moov", mp4Box("uuid", previewUUID, p.small)), jpegBox}, nil),
		"canon uuid with broken boxes":  bytes.Join([][]byte{ftyp, mp4Box("moov", mp4Box("uuid", canonUUID, []byte{0, 0, 0, 2, 'C', 'M', 'T', '1', 1, 2, 3})), jpegBox}, nil),
		"CMT1 with garbage":             bytes.Join([][]byte{ftyp, mp4Box("moov", mp4Box("uuid", canonUUID, mp4Box("CMT1", []byte("garbage garbage")))), jpegBox}, nil),
		"uuid with nothing after id":    bytes.Join([][]byte{ftyp, mp4Box("uuid", previewUUID), jpegBox}, nil),
		"thousands of empty boxes":      bytes.Join([][]byte{ftyp, tiny, jpegBox}, nil),
		"nothing but the brand":         ftyp,
		"moov holding the preview only": bytes.Join([][]byte{ftyp, mp4Box("moov", mp4Box("uuid", canonUUID, mp4Box("THMB", make([]byte, 16), p.thumb)))}, nil),
	}
	for name, data := range cases {
		requireSane(t, name, data)
	}
	if info := mustProbe(t, cases["box to the end of the file"]); info.Width != 96 {
		t.Fatalf("%+v", info)
	}
	if info := mustProbe(t, cases["mdat longer than the file"]); info.Width != 96 {
		t.Fatalf("%+v", info)
	}
	if info := mustProbe(t, cases["moov holding the preview only"]); info.Width != 16 {
		t.Fatalf("%+v", info)
	}
	if found := structural(t, cases["thousands of empty boxes"]); len(found) != 0 {
		t.Fatalf("the box budget should hide the preview from the structure walk: %+v", found)
	}
	if info := mustProbe(t, cases["thousands of empty boxes"]); info.Width != 96 {
		t.Fatalf("scanning should rescue it: %+v", info)
	}
	if _, err := probeData(cases["nothing but the brand"]); !errors.Is(err, codec.ErrUnsupported) {
		t.Fatalf("%v", err)
	}
	if _, err := probeData(cases["box smaller than its header"]); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("%v", err)
	}
}

func TestScanWindowOfCR3MediaIsBounded(t *testing.T) {
	p := makePreviews(t)
	ftyp := mp4Box("ftyp", []byte("crx "), []byte{0, 1, 0, 0}, []byte("crx isom"))
	filler := bytes.Repeat([]byte{0x33}, maxCarveBytes+1000)
	data := bytes.Join([][]byte{ftyp, mp4Box("mdat", p.medium, filler, p.large)}, nil)
	if info := mustProbe(t, data); info.Width != 48 {
		t.Fatalf("a preview beyond the scan window was used: %+v", info)
	}
}

func TestHostileRAFHeaders(t *testing.T) {
	p := makePreviews(t)
	good := rafFile(p.large)
	for name, data := range map[string][]byte{
		"offset past the end":   patchRAF(good, uint32(len(good))+10, 100),
		"length past the end":   patchRAF(good, 160, uint32(len(good))),
		"maximal offsets":       patchRAF(good, 0xffffffff, 0xffffffff),
		"offset inside header":  patchRAF(good, 4, 100),
		"zero length":           patchRAF(good, 160, 0),
		"only the header":       good[:rafHeaderSize],
		"header cut off":        good[:rafHeaderSize-1],
		"signature only":        []byte(rafSignature),
		"preview of zero bytes": append(append([]byte(nil), good[:160]...), make([]byte, 200)...),
	} {
		requireSane(t, name, data)
	}
	if info := mustProbe(t, patchRAF(good, 160, 0)); info.Width != 96 {
		t.Fatalf("an unknown length means to the end of the file: %+v", info)
	}
}

func TestPanasonicPreviewMustBeABlob(t *testing.T) {
	p := makePreviews(t)
	inline := standardTIFF(little)
	ifd0, _ := inline.ifd(entry{tag: tagJpgFromRaw, typ: typeUndefined, count: 4, inline: []byte{0xff, 0xd8, 0xff, 0xe0}}, entry{tag: tagJpgFromRaw + 1, typ: typeShort, count: 1, value: 3})
	inline.setFirst(ifd0)
	if found := structural(t, inline.data()); len(found) != 0 {
		t.Fatalf("%+v", found)
	}
	wrongType := standardTIFF(little)
	ref := wrongType.later(p.large)
	wifd, _ := wrongType.ifd(entry{tag: tagJpgFromRaw, typ: typeShort, count: uint32(len(p.large) / 2), ref: ref})
	wrongType.setFirst(wifd)
	if found := structural(t, wrongType.data()); len(found) != 0 {
		t.Fatalf("%+v", found)
	}
	bytesType := standardTIFF(little)
	bref := bytesType.later(p.large)
	bifd, _ := bytesType.ifd(entry{tag: tagJpgFromRaw, typ: typeByte, count: uint32(len(p.large)), ref: bref})
	bytesType.setFirst(bifd)
	if found := structural(t, bytesType.data()); len(found) != 1 {
		t.Fatalf("%+v", found)
	}
}
