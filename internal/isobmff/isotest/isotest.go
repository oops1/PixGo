package isotest

import (
	"encoding/binary"
)

func U16(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }

func U32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }

func U64(v uint64) []byte { return binary.BigEndian.AppendUint64(nil, v) }

func join(parts [][]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func Box(kind string, parts ...[]byte) []byte {
	body := join(parts)
	out := U32(uint32(8 + len(body)))
	out = append(out, kind...)
	return append(out, body...)
}

func LargeBox(kind string, parts ...[]byte) []byte {
	body := join(parts)
	out := U32(1)
	out = append(out, kind...)
	out = append(out, U64(uint64(16+len(body)))...)
	return append(out, body...)
}

func OpenEndedBox(kind string, parts ...[]byte) []byte {
	out := U32(0)
	out = append(out, kind...)
	return append(out, join(parts)...)
}

func FullBox(kind string, version byte, flags uint32, parts ...[]byte) []byte {
	head := U32(uint32(version)<<24 | flags&0xffffff)
	return Box(kind, append([][]byte{head}, parts...)...)
}

type Prop struct {
	Box       []byte
	Essential bool
}

func Ispe(width, height uint32) Prop {
	return Prop{Box: FullBox("ispe", 0, 0, U32(width), U32(height)), Essential: true}
}

func Irot(quarterTurns byte) Prop {
	return Prop{Box: Box("irot", []byte{quarterTurns}), Essential: true}
}

func Imir(axis byte) Prop { return Prop{Box: Box("imir", []byte{axis}), Essential: true} }

func Clap(widthNum, widthDen, heightNum, heightDen, horizNum, horizDen, vertNum, vertDen int32) Prop {
	var body []byte
	for _, v := range []int32{widthNum, widthDen, heightNum, heightDen, horizNum, horizDen, vertNum, vertDen} {
		body = append(body, U32(uint32(v))...)
	}
	return Prop{Box: Box("clap", body), Essential: true}
}

func Pixi(bits ...byte) Prop {
	return Prop{Box: FullBox("pixi", 0, 0, []byte{byte(len(bits))}, bits)}
}

func AuxC(urn string) Prop {
	return Prop{Box: FullBox("auxC", 0, 0, []byte(urn), []byte{0}), Essential: true}
}

func AV1C(highBitDepth, twelveBit, monochrome bool) Prop {
	var flags byte
	if highBitDepth {
		flags |= 1 << 6
	}
	if twelveBit {
		flags |= 1 << 5
	}
	if monochrome {
		flags |= 1 << 4
	}
	return Prop{Box: Box("av1C", []byte{0x81, 0x00, flags, 0x00}), Essential: true}
}

func HVCC(chromaFormat, bitDepth byte) Prop {
	body := []byte{1}
	body = append(body, make([]byte, 12)...)
	body = append(body, 0xf0, 0x00, 0xfc)
	body = append(body, 0xfc|chromaFormat, 0xf8|(bitDepth-8), 0xf8|(bitDepth-8))
	body = append(body, 0, 0, 0x03, 0)
	return Prop{Box: Box("hvcC", body), Essential: true}
}

func Colr() Prop {
	return Prop{Box: Box("colr", []byte("nclx"), U16(1), U16(13), U16(6), []byte{0x80})}
}

func ColrProfile(kind string, profile []byte) Prop {
	return Prop{Box: Box("colr", []byte(kind), profile)}
}

func Unknown(kind string, essential bool) Prop {
	return Prop{Box: Box(kind, []byte{1, 2, 3, 4}), Essential: essential}
}

func GridDescriptor(rows, cols int, width, height uint32, wide bool) []byte {
	flags := byte(0)
	if wide {
		flags = 1
	}
	out := []byte{0, flags, byte(rows - 1), byte(cols - 1)}
	if wide {
		out = append(out, U32(width)...)
		return append(out, U32(height)...)
	}
	out = append(out, U16(uint16(width))...)
	return append(out, U16(uint16(height))...)
}

type Item struct {
	ID          uint32
	Type        string
	Data        []byte
	Props       []Prop
	Hidden      bool
	ContentType string
	InIdat      bool
	NoData      bool
}

type Ref struct {
	Type string
	From uint32
	To   []uint32
}

type Track struct {
	Handler string
	Samples uint32
	Width   uint32
	Height  uint32
}

type File struct {
	Major      string
	Minor      uint32
	Compatible []string
	Primary    uint32
	Items      []Item
	Refs       []Ref
	Tracks     []Track
	Extra      [][]byte
	NoMeta     bool
}

func (f File) ftyp() []byte {
	body := append([]byte(f.Major), U32(f.Minor)...)
	for _, brand := range f.Compatible {
		body = append(body, brand...)
	}
	return Box("ftyp", body)
}

func (f File) infe(item Item) []byte {
	flags := uint32(0)
	if item.Hidden {
		flags = 1
	}
	body := append(U16(uint16(item.ID)), 0, 0)
	body = append(body, item.Type...)
	body = append(body, 0)
	if item.Type == "mime" {
		body = append(body, item.ContentType...)
		body = append(body, 0)
	}
	return FullBox("infe", 2, flags, body)
}

func (f File) iloc(items []Item, mdatStart uint64) []byte {
	body := []byte{0x44, 0x00}
	body = append(body, U16(uint16(len(items)))...)
	var mdatOffset, idatOffset uint64
	for _, item := range items {
		method := uint16(0)
		offset := mdatStart + mdatOffset
		if item.InIdat {
			method = 1
			offset = idatOffset
			idatOffset += uint64(len(item.Data))
		} else if !item.NoData {
			mdatOffset += uint64(len(item.Data))
		}
		body = append(body, U16(uint16(item.ID))...)
		body = append(body, U16(method)...)
		body = append(body, U16(0)...)
		if item.NoData {
			body = append(body, U16(0)...)
			continue
		}
		body = append(body, U16(1)...)
		body = append(body, U32(uint32(offset))...)
		body = append(body, U32(uint32(len(item.Data)))...)
	}
	return FullBox("iloc", 1, 0, body)
}

func (f File) iref() []byte {
	var body []byte
	for _, ref := range f.Refs {
		entry := append(U32(ref.From), U16(uint16(len(ref.To)))...)
		for _, to := range ref.To {
			entry = append(entry, U32(to)...)
		}
		body = append(body, Box(ref.Type, entry)...)
	}
	return FullBox("iref", 1, 0, body)
}

func (f File) iprp() []byte {
	var ipco, ipma []byte
	var associations [][]byte
	count := 0
	for _, item := range f.Items {
		if len(item.Props) == 0 {
			continue
		}
		entry := append(U32(item.ID), byte(len(item.Props)))
		for _, prop := range item.Props {
			ipco = append(ipco, prop.Box...)
			count++
			value := uint16(count)
			if prop.Essential {
				value |= 0x8000
			}
			entry = append(entry, U16(value)...)
		}
		associations = append(associations, entry)
	}
	ipma = append(ipma, U32(uint32(len(associations)))...)
	ipma = append(ipma, join(associations)...)
	return Box("iprp", Box("ipco", ipco), FullBox("ipma", 1, 1, ipma))
}

func (f File) meta(mdatStart uint64) []byte {
	var parts [][]byte
	parts = append(parts, FullBox("hdlr", 0, 0, U32(0), []byte("pict"), make([]byte, 12), []byte{0}))
	if f.Primary != 0 {
		parts = append(parts, FullBox("pitm", 0, 0, U16(uint16(f.Primary))))
	}
	var entries []byte
	for _, item := range f.Items {
		entries = append(entries, f.infe(item)...)
	}
	parts = append(parts, FullBox("iinf", 0, 0, U16(uint16(len(f.Items))), entries))
	var idat []byte
	for _, item := range f.Items {
		if item.InIdat {
			idat = append(idat, item.Data...)
		}
	}
	parts = append(parts, f.iloc(f.Items, mdatStart))
	if len(f.Refs) > 0 {
		parts = append(parts, f.iref())
	}
	parts = append(parts, f.iprp())
	if len(idat) > 0 {
		parts = append(parts, Box("idat", idat))
	}
	return FullBox("meta", 0, 0, parts...)
}

func (f File) movie() []byte {
	var traks []byte
	for i, track := range f.Tracks {
		tkhd := FullBox("tkhd", 0, 7, U32(0), U32(0), U32(uint32(i+1)), U32(0), U32(0), make([]byte, 52), U32(track.Width<<16), U32(track.Height<<16))
		hdlr := FullBox("hdlr", 0, 0, U32(0), []byte(track.Handler), make([]byte, 12), []byte{0})
		stsz := FullBox("stsz", 0, 0, U32(1), U32(track.Samples))
		stbl := Box("stbl", stsz)
		mdia := Box("mdia", hdlr, Box("minf", stbl))
		traks = append(traks, Box("trak", tkhd, mdia)...)
	}
	return Box("moov", traks)
}

func (f File) Bytes() []byte {
	build := func(mdatStart uint64) []byte {
		out := f.ftyp()
		if !f.NoMeta {
			out = append(out, f.meta(mdatStart)...)
		}
		if len(f.Tracks) > 0 {
			out = append(out, f.movie()...)
		}
		for _, extra := range f.Extra {
			out = append(out, extra...)
		}
		return out
	}
	head := build(0)
	var data []byte
	for _, item := range f.Items {
		if !item.InIdat && !item.NoData {
			data = append(data, item.Data...)
		}
	}
	mdatStart := uint64(len(head)) + 8
	out := build(mdatStart)
	return append(out, Box("mdat", data)...)
}
