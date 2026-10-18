package meta

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta/exif"
)

var testThumbnail = append([]byte{0xff, 0xd8, 0xff, 0xdb, 0, 4, 1, 2}, bytes.Repeat([]byte{0x55}, 300)...)

func richParts(e enc) tiffParts {
	parts := photoParts(e, false)
	parts.ifd0 = append(parts.ifd0, e.long(0x0100, 6000), e.long(0x0101, 4000), e.rational(0x011a, 72, 1), e.rational(0x011b, 72, 1), e.short(0x0128, 2))
	parts.exif = append(parts.exif, e.undefined(0x9286, append([]byte("ASCII\x00\x00\x00"), "a comment"...)))
	return parts
}

func withThumbnail(e enc, tiff []byte, thumb []byte) []byte {
	n := int(e.o.Uint16(tiff[8:]))
	nextAt := 8 + 2 + 12*n
	out := append([]byte(nil), tiff...)
	for len(out)%2 == 1 {
		out = append(out, 0)
	}
	ifd1At := len(out)
	e.o.PutUint32(out[nextAt:], uint32(ifd1At))
	thumbAt := ifd1At + 2 + 3*12 + 4
	out = e.writeIFD(out, []tentry{e.short(0x0103, 6), e.long(0x0201, uint32(thumbAt)), e.long(0x0202, uint32(len(thumb)))}, 0)
	return append(out, thumb...)
}

func mustParse(t testing.TB, tiff []byte) *tiffDoc {
	t.Helper()
	doc, err := parseTIFF(tiff)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return doc
}

func readEXIF(t testing.TB, tiff []byte) *Metadata {
	t.Helper()
	return mustRead(t, wrapEXIF(tiff), codec.JPEG)
}

func TestRewriteFixesOrientationAndSize(t *testing.T) {
	for _, o := range orders {
		e := enc{o.order}
		src := e.tiff(richParts(e))
		before := readEXIF(t, src)
		if before.Exif.Orientation != codec.OrientRotate90 || before.Exif.PixelWidth != 6000 {
			t.Fatal("the fixture changed")
		}
		out := Rewrite(codec.Metadata{EXIF: src}, RewriteOptions{Width: 1234, Height: 70000})
		m := readEXIF(t, out.EXIF)
		if m.Exif.Orientation != codec.OrientNormal {
			t.Fatalf("%s: orientation %d, want 1", o.name, m.Exif.Orientation)
		}
		if m.Exif.PixelWidth != 1234 || m.Exif.PixelHeight != 70000 {
			t.Fatalf("%s: pixel dimensions %dx%d", o.name, m.Exif.PixelWidth, m.Exif.PixelHeight)
		}
		doc := mustParse(t, out.EXIF)
		for tag, want := range map[uint16]uint32{0x0100: 1234, 0x0101: 70000} {
			i := doc.ifd0.index(tag)
			if i < 0 {
				t.Fatalf("IFD0 tag %#x lost", tag)
			}
			r, _ := exif.NewReader(out.EXIF)
			d, _ := r.Directory(r.FirstOffset())
			e, _ := d.Find(tag)
			if got, _ := e.Uint(0); got != want {
				t.Fatalf("%s: IFD0 tag %#x = %d, want %d", o.name, tag, got, want)
			}
		}
		if m.Exif.Make != "Canon" || m.Exif.Lens != "RF 24-70mm F2.8 L" || m.Exif.ISO != 100 || m.GPS == nil || m.GPS.DMS() != "55°45′21.1″ N, 37°37′02.5″ E" {
			t.Fatalf("%s: unrelated tags changed: %+v %+v", o.name, m.Exif, m.GPS)
		}
		if doc.ifd0.index(0x011a) < 0 || doc.exif.index(0x9286) < 0 {
			t.Fatalf("%s: resolution or user comment lost", o.name)
		}
	}
}

func TestRewriteSizeUsesShortWhenItFits(t *testing.T) {
	e := enc{orders[0].order}
	parts := richParts(e)
	for i := range parts.exif {
		if parts.exif[i].tag == 0xa002 || parts.exif[i].tag == 0xa003 {
			parts.exif[i] = e.short(parts.exif[i].tag, 10)
		}
	}
	out := Rewrite(codec.Metadata{EXIF: e.tiff(parts)}, RewriteOptions{Width: 640, Height: 480})
	doc := mustParse(t, out.EXIF)
	i := doc.exif.index(0xa002)
	if doc.exif.entries[i].typ != exif.TypeShort || doc.order.Uint16(doc.exif.entries[i].data) != 640 {
		t.Fatalf("entry %+v", doc.exif.entries[i])
	}
}

func TestRewriteOnlyTouchesDimensionsThatArePresent(t *testing.T) {
	e := enc{orders[0].order}
	parts := photoParts(e, false)
	var exifTags []tentry
	for _, en := range parts.exif {
		if en.tag != 0xa002 && en.tag != 0xa003 {
			exifTags = append(exifTags, en)
		}
	}
	parts.exif = exifTags
	out := Rewrite(codec.Metadata{EXIF: e.tiff(parts)}, RewriteOptions{Width: 10, Height: 20})
	doc := mustParse(t, out.EXIF)
	if doc.exif.index(0xa002) >= 0 || doc.exif.index(0xa003) >= 0 || doc.ifd0.index(0x0100) >= 0 {
		t.Fatal("dimension tags were invented")
	}
}

func TestRewriteWithoutSizeKeepsDimensions(t *testing.T) {
	e := enc{orders[0].order}
	out := Rewrite(codec.Metadata{EXIF: e.tiff(richParts(e))}, RewriteOptions{})
	m := readEXIF(t, out.EXIF)
	if m.Exif.PixelWidth != 6000 || m.Exif.PixelHeight != 4000 || m.Exif.Orientation != codec.OrientNormal {
		t.Fatalf("%+v", m.Exif)
	}
}

func TestRewriteThumbnail(t *testing.T) {
	for _, o := range orders {
		e := enc{o.order}
		src := withThumbnail(e, e.tiff(richParts(e)), testThumbnail)
		if th, ok := exif.JPEGThumbnail(src); !ok || !bytes.Equal(th, testThumbnail) {
			t.Fatal("fixture thumbnail not found")
		}
		dropped := Rewrite(codec.Metadata{EXIF: src}, RewriteOptions{Width: 100, Height: 50})
		if _, ok := exif.JPEGThumbnail(dropped.EXIF); ok {
			t.Fatalf("%s: the thumbnail survived", o.name)
		}
		if len(dropped.EXIF) >= len(src)-len(testThumbnail)+10 {
			t.Fatalf("%s: the block was not cut: %d bytes of %d", o.name, len(dropped.EXIF), len(src))
		}
		r, _ := exif.NewReader(dropped.EXIF)
		tree, _ := r.Tree()
		if tree.IFD0.Next != 0 || tree.IFD1 != nil {
			t.Fatalf("%s: IFD1 is still linked", o.name)
		}
		kept := Rewrite(codec.Metadata{EXIF: src}, RewriteOptions{Width: 100, Height: 50, KeepThumbnail: true})
		th, ok := exif.JPEGThumbnail(kept.EXIF)
		if !ok || !bytes.Equal(th, testThumbnail) {
			t.Fatalf("%s: the kept thumbnail is damaged", o.name)
		}
		if readEXIF(t, kept.EXIF).Exif.PixelWidth != 100 {
			t.Fatalf("%s: dimensions not updated with a thumbnail", o.name)
		}
	}
}

func TestRewriteStripGPS(t *testing.T) {
	for _, o := range orders {
		e := enc{o.order}
		src := e.tiff(richParts(e))
		kept := Rewrite(codec.Metadata{EXIF: src}, RewriteOptions{})
		if readEXIF(t, kept.EXIF).GPS == nil {
			t.Fatal("GPS must survive when not stripped")
		}
		stripped := Rewrite(codec.Metadata{EXIF: src}, RewriteOptions{StripGPS: true})
		m := readEXIF(t, stripped.EXIF)
		if m.GPS != nil {
			t.Fatalf("%s: GPS survived", o.name)
		}
		if m.Exif == nil || m.Exif.Make != "Canon" {
			t.Fatal("the rest of the EXIF was damaged")
		}
		doc := mustParse(t, stripped.EXIF)
		if doc.gps != nil || doc.ifd0.index(0x8825) >= 0 {
			t.Fatal("GPS pointer or directory is still in the structure")
		}
		if len(stripped.EXIF) >= len(kept.EXIF) {
			t.Fatalf("%s: the GPS block was not cut: %d >= %d", o.name, len(stripped.EXIF), len(kept.EXIF))
		}
		for _, secret := range [][]byte{{55, 0, 0, 0, 1, 0, 0, 0}, []byte("N\x00")} {
			_ = secret
		}
	}
}

func canonLikeNote(n int) []byte {
	note := []byte{0x16, 0x00}
	for len(note) < n {
		note = append(note, byte(len(note)*7), 0x01)
	}
	return note[:n]
}

func TestRewriteKeepsHeaderlessMakerNoteAtItsOffset(t *testing.T) {
	for _, o := range orders {
		e := enc{o.order}
		parts := richParts(e)
		note := canonLikeNote(900)
		parts.exif = append(parts.exif, e.undefined(0x927c, note))
		src := withThumbnail(e, e.tiff(parts), testThumbnail)
		before := mustParse(t, src)
		at := before.exif.entries[before.exif.index(tagMakerNote)].pinAt
		if at < 8 {
			t.Fatal("fixture has no out-of-line maker note")
		}
		out := Rewrite(codec.Metadata{EXIF: src}, RewriteOptions{Width: 10, Height: 10, StripGPS: true})
		after := mustParse(t, out.EXIF)
		moved := after.exif.entries[after.exif.index(tagMakerNote)]
		if moved.pinAt != at {
			t.Fatalf("%s: maker note moved from %d to %d", o.name, at, moved.pinAt)
		}
		if !bytes.Equal(out.EXIF[at:at+len(note)], note) {
			t.Fatalf("%s: maker note bytes differ at the old offset", o.name)
		}
	}
}

func TestRewriteMovesSelfContainedMakerNote(t *testing.T) {
	e := enc{orders[0].order}
	parts := richParts(e)
	note := append([]byte("Nikon\x00\x02\x10\x00\x00"), canonLikeNote(500)...)
	parts.exif = append(parts.exif, e.undefined(0x927c, note))
	src := withThumbnail(e, e.tiff(parts), testThumbnail)
	out := Rewrite(codec.Metadata{EXIF: src}, RewriteOptions{StripGPS: true})
	after := mustParse(t, out.EXIF)
	got := after.exif.entries[after.exif.index(tagMakerNote)].data
	if !bytes.Equal(got, note) {
		t.Fatal("maker note bytes changed")
	}
	if len(out.EXIF) > len(src)-len(testThumbnail) {
		t.Fatalf("the block did not shrink: %d of %d", len(out.EXIF), len(src))
	}
}

func TestRewriteDropsMakerNoteThatCannotFit(t *testing.T) {
	e := enc{orders[0].order}
	parts := richParts(e)
	parts.exif = append(parts.exif, e.undefined(0x927c, canonLikeNote(70000)))
	out, err := RewriteFor(codec.Metadata{EXIF: e.tiff(parts)}, RewriteOptions{Width: 8, Height: 8}, codec.JPEG)
	if err != nil {
		t.Fatalf("dropping only the maker note is not an error: %v", err)
	}
	if len(out.EXIF) == 0 || len(out.EXIF) > MaxJPEGExifBytes {
		t.Fatalf("EXIF %d bytes", len(out.EXIF))
	}
	if mustParse(t, out.EXIF).exif.index(tagMakerNote) >= 0 {
		t.Fatal("maker note kept")
	}
	if readEXIF(t, out.EXIF).Exif.Make != "Canon" {
		t.Fatal("rest of EXIF lost")
	}
}

func TestRewriteReportsExifThatCannotFit(t *testing.T) {
	e := enc{orders[0].order}
	parts := richParts(e)
	parts.exif = append(parts.exif, e.undefined(0x9999, bytes.Repeat([]byte{'x'}, 70000)))
	out, err := RewriteFor(codec.Metadata{EXIF: e.tiff(parts), XMP: []byte(xmpSample)}, RewriteOptions{}, codec.JPEG)
	var dropped *DroppedError
	if !errors.As(err, &dropped) || !errors.Is(err, ErrDropped) || !dropped.EXIF || dropped.XMP || dropped.ICC {
		t.Fatalf("err %v", err)
	}
	if len(out.EXIF) != 0 || len(out.XMP) == 0 {
		t.Fatal("only the EXIF block must be dropped")
	}
	if !strings.Contains(dropped.Error(), "EXIF") {
		t.Fatal(dropped.Error())
	}
	big, err := RewriteFor(codec.Metadata{EXIF: e.tiff(parts)}, RewriteOptions{}, codec.PNG)
	if err != nil || len(big.EXIF) == 0 {
		t.Fatalf("PNG has no 64 KB limit: %v", err)
	}
}

func TestRewriteDropsUnreadableExif(t *testing.T) {
	m := codec.Metadata{EXIF: []byte("II*\x00\xff\xff\xff\xffgarbage"), ICC: sRGBProfile()}
	out, err := RewriteFor(m, RewriteOptions{}, codec.JPEG)
	var dropped *DroppedError
	if !errors.As(err, &dropped) || !dropped.EXIF {
		t.Fatalf("err %v", err)
	}
	if len(out.EXIF) != 0 || !bytes.Equal(out.ICC, m.ICC) {
		t.Fatal("garbage EXIF must go, the profile must stay")
	}
	if got := Rewrite(m, RewriteOptions{}); len(got.EXIF) != 0 {
		t.Fatal("Rewrite must drop garbage EXIF too")
	}
}

func TestRewriteAcceptsExifPrefix(t *testing.T) {
	e := enc{orders[0].order}
	src := append([]byte("Exif\x00\x00"), e.tiff(richParts(e))...)
	out := Rewrite(codec.Metadata{EXIF: src}, RewriteOptions{Width: 5, Height: 6})
	if !validTIFFHeader(out.EXIF) || readEXIF(t, out.EXIF).Exif.PixelWidth != 5 {
		t.Fatal("prefixed EXIF not handled")
	}
}

func TestRewriteIsIdempotent(t *testing.T) {
	for _, o := range orders {
		e := enc{o.order}
		parts := richParts(e)
		parts.exif = append(parts.exif, e.undefined(0x927c, canonLikeNote(333)))
		src := withThumbnail(e, e.tiff(parts), testThumbnail)
		opts := RewriteOptions{Width: 77, Height: 55, StripGPS: true}
		once := Rewrite(codec.Metadata{EXIF: src}, opts)
		twice := Rewrite(once, opts)
		if !bytes.Equal(once.EXIF, twice.EXIF) {
			t.Fatalf("%s: a second pass changed the block", o.name)
		}
	}
}

func TestRewriteBlocksAreStructurallySound(t *testing.T) {
	e := enc{orders[1].order}
	parts := richParts(e)
	parts.exif = append(parts.exif, e.undefined(0x927c, canonLikeNote(40)))
	src := withThumbnail(e, e.tiff(parts), testThumbnail)
	for _, keep := range []bool{false, true} {
		out := Rewrite(codec.Metadata{EXIF: src}, RewriteOptions{Width: 9, Height: 9, KeepThumbnail: keep})
		r, err := exif.NewReader(out.EXIF)
		if err != nil {
			t.Fatal(err)
		}
		chain, err := r.Chain()
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range chain {
			last := uint16(0)
			for i, en := range d.Entries {
				if i > 0 && en.Tag <= last {
					t.Fatalf("tags out of order: %#x after %#x", en.Tag, last)
				}
				last = en.Tag
				if _, ok := en.Raw(); !ok && en.Count > 0 {
					t.Fatalf("tag %#x has a dangling value offset", en.Tag)
				}
			}
		}
	}
}

func TestFitLimits(t *testing.T) {
	big := codec.Metadata{
		EXIF: append([]byte("II*\x00\x08\x00\x00\x00"), make([]byte, MaxJPEGExifBytes)...),
		XMP:  bytes.Repeat([]byte{'x'}, MaxJPEGXMPBytes+1),
		ICC:  make([]byte, MaxJPEGICCBytes+1),
	}
	out, err := Fit(big, codec.JPEG)
	var dropped *DroppedError
	if !errors.As(err, &dropped) || !dropped.EXIF || !dropped.XMP || !dropped.ICC {
		t.Fatalf("err %v", err)
	}
	if !out.Empty() {
		t.Fatal("everything over the limit must go")
	}
	fits := codec.Metadata{
		EXIF: append([]byte("MM\x00*\x00\x00\x00\x08"), make([]byte, MaxJPEGExifBytes-8)...),
		XMP:  bytes.Repeat([]byte{'x'}, MaxJPEGXMPBytes),
		ICC:  make([]byte, MaxJPEGICCBytes),
	}
	got, err := Fit(fits, codec.JPEG)
	if err != nil || len(got.EXIF) != len(fits.EXIF) || len(got.XMP) != len(fits.XMP) || len(got.ICC) != len(fits.ICC) {
		t.Fatalf("limits are inclusive: %v", err)
	}
	bad, err := Fit(codec.Metadata{EXIF: []byte("not tiff data")}, codec.PNG)
	if err == nil || len(bad.EXIF) != 0 {
		t.Fatal("a non-TIFF EXIF block must be refused")
	}
}

const xmpGPSSample = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about=""
    xmlns:tiff="http://ns.adobe.com/tiff/1.0/"
    xmlns:exif="http://ns.adobe.com/exif/1.0/"
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    tiff:Orientation="6"
    tiff:ImageWidth="6000"
    tiff:ImageLength='4000'
    exif:PixelXDimension="6000"
    exif:GPSLatitude="55,45.35N"
    exif:GPSLongitude='37,37.04E'
    exif:ColorSpace="1">
   <exif:PixelYDimension>4000</exif:PixelYDimension>
   <exif:GPSAltitude>156/1</exif:GPSAltitude>
   <exif:GPSTimeStamp>2024-09-14T15:27:03Z</exif:GPSTimeStamp>
   <exif:GPSVersionID/>
   <dc:title><rdf:Alt><rdf:li xml:lang="x-default">Keep me</rdf:li></rdf:Alt></dc:title>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`

func TestRewriteXMP(t *testing.T) {
	out := Rewrite(codec.Metadata{XMP: []byte(xmpGPSSample)}, RewriteOptions{Width: 321, Height: 123, StripGPS: true})
	s := string(out.XMP)
	for _, want := range []string{
		`tiff:Orientation="1"`, `tiff:ImageWidth="321"`, `tiff:ImageLength='123'`, `exif:PixelXDimension="321"`,
		`<exif:PixelYDimension>123</exif:PixelYDimension>`, `exif:ColorSpace="1"`, `Keep me`, `<?xpacket end="w"?>`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	if strings.Contains(s, "GPS") || strings.Contains(s, "55,45") || strings.Contains(s, "156/1") {
		t.Errorf("GPS data survived:\n%s", s)
	}
	if x := parseXMP(out.XMP); x == nil || x.Title != "Keep me" {
		t.Fatalf("rewritten packet no longer parses: %+v", x)
	}
	kept := string(Rewrite(codec.Metadata{XMP: []byte(xmpGPSSample)}, RewriteOptions{}).XMP)
	if !strings.Contains(kept, `exif:GPSLatitude="55,45.35N"`) || !strings.Contains(kept, "<exif:GPSAltitude>156/1</exif:GPSAltitude>") {
		t.Fatal("GPS must stay without StripGPS")
	}
	if !strings.Contains(kept, `tiff:ImageWidth="6000"`) || !strings.Contains(kept, `tiff:Orientation="1"`) {
		t.Fatal("sizes stay, orientation resets")
	}
}

func TestRewriteXMPElementOrientationAndAbsentFields(t *testing.T) {
	src := `<x:xmpmeta><rdf:RDF><rdf:Description><tiff:Orientation> 8 </tiff:Orientation><dc:format>image/jpeg</dc:format></rdf:Description></rdf:RDF></x:xmpmeta>`
	out := string(Rewrite(codec.Metadata{XMP: []byte(src)}, RewriteOptions{Width: 4, Height: 4, StripGPS: true}).XMP)
	if !strings.Contains(out, "<tiff:Orientation> 1 </tiff:Orientation>") {
		t.Fatal(out)
	}
	if strings.Contains(out, "PixelXDimension") || strings.Contains(out, "ImageWidth") {
		t.Fatal("fields were invented: " + out)
	}
}

func TestRewriteXMPUTF16(t *testing.T) {
	units := utf16.Encode([]rune(xmpGPSSample))
	raw := []byte{0xff, 0xfe}
	for _, u := range units {
		raw = append(raw, byte(u), byte(u>>8))
	}
	out := Rewrite(codec.Metadata{XMP: raw}, RewriteOptions{Width: 5, Height: 6, StripGPS: true})
	if !strings.Contains(string(out.XMP), `tiff:ImageWidth="5"`) || strings.Contains(string(out.XMP), "GPSLatitude") {
		t.Fatalf("UTF-16 packet not edited:\n%s", out.XMP)
	}
}

func TestRewriteLeavesICCAlone(t *testing.T) {
	profile := sRGBProfile()
	out := Rewrite(codec.Metadata{ICC: profile}, RewriteOptions{Width: 3, Height: 3, StripGPS: true})
	if !bytes.Equal(out.ICC, profile) || len(out.EXIF) != 0 || len(out.XMP) != 0 {
		t.Fatal("only the profile exists")
	}
	profile[0] ^= 0xff
	if out.ICC[0] == profile[0] {
		t.Fatal("the profile is shared with the input")
	}
}

func TestRewriteEmptyMetadata(t *testing.T) {
	if got, err := RewriteFor(codec.Metadata{}, RewriteOptions{Width: 1, Height: 1}, codec.JPEG); err != nil || !got.Empty() {
		t.Fatalf("%v %v", got, err)
	}
}

func TestRewriteThroughRealFileFormats(t *testing.T) {
	for _, o := range orders {
		e := enc{o.order}
		data, _ := fullJPEG(t, e, false)
		blocks := readBlocks(t, data, codec.JPEG)
		out, err := RewriteFor(blocks, RewriteOptions{Width: 600, Height: 400, StripGPS: true}, codec.JPEG)
		if err != nil {
			t.Fatal(err)
		}
		file := buildJPEG(600, 400, append(append([][]byte{jpegExifSeg(out.EXIF)}, jpegICCSegs(out.ICC, 500)...), jpegXMPSeg(string(out.XMP)))...)
		m := mustRead(t, file, codec.JPEG)
		if m.GPS != nil || m.Exif.Orientation != codec.OrientNormal || m.Exif.PixelWidth != 600 || m.ICC == nil || m.XMP == nil {
			t.Fatalf("%s: %+v %+v", o.name, m.Exif, m.GPS)
		}
	}
}
