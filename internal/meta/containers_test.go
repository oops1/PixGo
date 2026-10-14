package meta

import (
	"encoding/binary"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

const (
	heifXMPContentType = "application/rdf+xml"
	cr3Header          = "\x00\x00\x00\x18ftypcrx \x00\x00\x00\x01crx isom"
)

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func heifExifData(tiff []byte, offset uint32, prefix bool) []byte {
	out := isotest.U32(offset)
	if prefix {
		out = append(out, "Exif\x00\x00"...)
	}
	return append(out, tiff...)
}

type heifOptions struct {
	itemType string
	exif     []byte
	profile  []byte
	kind     string
	xmp      string
	noLink   bool
	extra    []isotest.Item
	props    []isotest.Prop
}

func heifFile(o heifOptions) []byte {
	if o.itemType == "" {
		o.itemType = "hvc1"
	}
	props := []isotest.Prop{isotest.Ispe(6000, 4000)}
	if o.profile != nil {
		kind := o.kind
		if kind == "" {
			kind = "prof"
		}
		props = append(props, isotest.ColrProfile(kind, o.profile))
	}
	props = append(props, o.props...)
	f := isotest.File{
		Major:      "heic",
		Compatible: []string{"mif1", "heic"},
		Primary:    1,
		Items:      []isotest.Item{{ID: 1, Type: o.itemType, Data: []byte("pixels"), Props: props}},
	}
	next := uint32(2)
	add := func(item isotest.Item) {
		item.ID = next
		f.Items = append(f.Items, item)
		if !o.noLink {
			f.Refs = append(f.Refs, isotest.Ref{Type: "cdsc", From: next, To: []uint32{1}})
		}
		next++
	}
	if o.exif != nil {
		add(isotest.Item{Type: "Exif", Data: o.exif, Hidden: true})
	}
	if o.xmp != "" {
		add(isotest.Item{Type: "mime", ContentType: heifXMPContentType, Data: []byte(o.xmp), Hidden: true})
	}
	f.Items = append(f.Items, o.extra...)
	return f.Bytes()
}

func fullHEIF(t testing.TB, e enc, below bool, itemType string) ([]byte, []byte) {
	t.Helper()
	profile := sRGBProfile()
	data := heifFile(heifOptions{
		itemType: itemType,
		exif:     heifExifData(e.tiff(photoParts(e, below)), 6, true),
		profile:  profile,
		xmp:      xmpSample,
	})
	return data, profile
}

func jxlContainer(boxes ...[]byte) []byte {
	out := append([]byte(nil), jxlSignature...)
	out = append(out, isotest.Box("ftyp", []byte("jxl "), isotest.U32(0), []byte("jxl "))...)
	for _, box := range boxes {
		out = append(out, box...)
	}
	return out
}

func jxlCodestream() []byte {
	return isotest.Box("jxlc", []byte{0xff, 0x0a, 0x00, 0x01, 0x02, 0x03})
}

func fullJXL(t testing.TB, e enc, below bool) []byte {
	t.Helper()
	return jxlContainer(
		isotest.Box("jxll", []byte{5}),
		isotest.Box("Exif", heifExifData(e.tiff(photoParts(e, below)), 0, false)),
		isotest.Box("xml ", []byte(xmpSample)),
		isotest.Box("jbrd", []byte("reconstruction data")),
		jxlCodestream(),
	)
}

func patchMagic(e enc, tiff []byte, magic uint16) []byte {
	out := append([]byte(nil), tiff...)
	e.o.PutUint16(out[2:], magic)
	return out
}

func rawTIFF(e enc, magic uint16, parts tiffParts) []byte {
	return patchMagic(e, e.tiff(parts), magic)
}

func rawParts(t testing.TB, e enc, below bool) tiffParts {
	t.Helper()
	parts := photoParts(e, below)
	profile := sRGBProfile()
	parts.ifd0 = append(parts.ifd0,
		e.long(0x0100, 160),
		e.long(0x0101, 120),
		e.undefined(0x8773, profile),
		e.bytesVal(0x02bc, []byte(xmpSample)...),
	)
	return parts
}

type cr3Options struct {
	skipCMT1, skipCMT2, skipCMT4 bool
	noCanon                      bool
	noXMP                        bool
	badCMT2                      bool
}

func cr3File(e enc, below bool, o cr3Options) []byte {
	parts := photoParts(e, below)
	canon := []byte{}
	canon = append(canon, cr3CanonUUID...)
	canon = append(canon, isotest.Box("CNCV", []byte("CanonCR3_001/00.09.00/00.00.00"))...)
	if !o.skipCMT1 {
		canon = append(canon, isotest.Box("CMT1", e.tiff(tiffParts{ifd0: parts.ifd0}))...)
	}
	if !o.skipCMT2 {
		cmt2 := e.tiff(tiffParts{ifd0: parts.exif})
		if o.badCMT2 {
			cmt2 = []byte("not a TIFF block")
		}
		canon = append(canon, isotest.Box("CMT2", cmt2)...)
	}
	canon = append(canon, isotest.Box("CMT3", []byte("maker note"))...)
	if !o.skipCMT4 {
		canon = append(canon, isotest.Box("CMT4", e.tiff(tiffParts{ifd0: parts.gps}))...)
	}
	canon = append(canon, isotest.Box("THMB", []byte("thumbnail"))...)
	moovChildren := [][]byte{
		isotest.Box("mvhd", make([]byte, 100)),
		isotest.Box("uuid", make([]byte, 16), []byte("other")),
	}
	if !o.noCanon {
		moovChildren = append(moovChildren, isotest.Box("uuid", canon))
	}
	moov := isotest.Box("moov", moovChildren...)
	out := []byte(cr3Header)
	out = append(out, moov...)
	if !o.noXMP {
		out = append(out, isotest.Box("uuid", cr3XMPUUID, []byte(xmpSample))...)
	}
	out = append(out, isotest.Box("uuid", make([]byte, 16), []byte("PRVW"))...)
	return append(out, isotest.Box("mdat", make([]byte, 64))...)
}

func rafFile(jpeg []byte, offset, length uint32) []byte {
	head := make([]byte, 160)
	copy(head, rafSignature)
	copy(head[16:], "0201FF129501")
	binary.BigEndian.PutUint32(head[84:], offset)
	binary.BigEndian.PutUint32(head[88:], length)
	out := append(head, jpeg...)
	return append(out, make([]byte, 32)...)
}

func fullRAF(t testing.TB, e enc, below bool) ([]byte, []byte) {
	t.Helper()
	jpeg, profile := fullJPEG(t, e, below)
	return rafFile(jpeg, 160, uint32(len(jpeg))), profile
}

func newContainerSamples(t testing.TB) []sample {
	t.Helper()
	var out []sample
	for _, o := range orders {
		e := enc{o.order}
		heic, _ := fullHEIF(t, e, false, "hvc1")
		avif, _ := fullHEIF(t, e, true, "av01")
		raf, _ := fullRAF(t, e, false)
		out = append(out,
			sample{"heic/" + o.name, codec.HEIC, heic},
			sample{"avif/" + o.name, codec.AVIF, avif},
			sample{"jxl/" + o.name, codec.JXL, fullJXL(t, e, false)},
			sample{"cr3/" + o.name, codec.RAW, cr3File(e, false, cr3Options{})},
			sample{"raf/" + o.name, codec.RAW, raf},
			sample{"nef/" + o.name, codec.RAW, rawTIFF(e, 42, rawParts(t, e, false))},
			sample{"orf/" + o.name, codec.RAW, rawTIFF(e, 0x4f52, rawParts(t, e, true))},
			sample{"rw2/" + o.name, codec.RAW, rawTIFF(e, 0x55, rawParts(t, e, false))},
		)
	}
	return out
}
