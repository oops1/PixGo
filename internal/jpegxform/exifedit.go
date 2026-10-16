package jpegxform

import (
	"bytes"
	"encoding/binary"
)

const (
	tagImageWidth     = 0x0100
	tagImageLength    = 0x0101
	tagOrientation    = 0x0112
	tagXResolution    = 0x011A
	tagYResolution    = 0x011B
	tagExifPointer    = 0x8769
	tagPixelXDim      = 0xA002
	tagPixelYDim      = 0xA003
	typeShort         = 3
	typeLong          = 4
	typeRational      = 5
	exifPrefix        = 6
	tiffHeader        = 8
	ifdEntry          = 12
	maxExifEntries    = 4096
	xmpOrientationTag = "tiff:Orientation"
	xmpPrefix         = "http://ns.adobe.com/xap/1.0/\x00"
)

type exifChange struct {
	oldWidth, oldHeight int
	newWidth, newHeight int
	swapResolution      bool
	transformed         bool
}

type tiffView struct {
	data  []byte
	order binary.ByteOrder
}

type ifdEntryRef struct {
	at    int
	tag   uint16
	typ   uint16
	count uint32
}

func newTiffView(tiff []byte) (tiffView, bool) {
	if len(tiff) < tiffHeader {
		return tiffView{}, false
	}
	var order binary.ByteOrder
	switch {
	case tiff[0] == 'I' && tiff[1] == 'I':
		order = binary.LittleEndian
	case tiff[0] == 'M' && tiff[1] == 'M':
		order = binary.BigEndian
	default:
		return tiffView{}, false
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return tiffView{}, false
	}
	return tiffView{data: tiff, order: order}, true
}

func (t tiffView) entries(ifd uint32) (list []ifdEntryRef, nextAt int, ok bool) {
	if ifd < tiffHeader || uint64(ifd)+2 > uint64(len(t.data)) {
		return nil, 0, false
	}
	n := int(t.order.Uint16(t.data[ifd:]))
	if n > maxExifEntries || uint64(ifd)+2+uint64(n)*ifdEntry+4 > uint64(len(t.data)) {
		return nil, 0, false
	}
	for i := 0; i < n; i++ {
		at := int(ifd) + 2 + i*ifdEntry
		list = append(list, ifdEntryRef{
			at:    at,
			tag:   t.order.Uint16(t.data[at:]),
			typ:   t.order.Uint16(t.data[at+2:]),
			count: t.order.Uint32(t.data[at+4:]),
		})
	}
	return list, int(ifd) + 2 + n*ifdEntry, true
}

func find(list []ifdEntryRef, tag uint16) (ifdEntryRef, bool) {
	for _, e := range list {
		if e.tag == tag {
			return e, true
		}
	}
	return ifdEntryRef{}, false
}

func (t tiffView) value(e ifdEntryRef) (uint32, bool) {
	if e.count != 1 {
		return 0, false
	}
	switch e.typ {
	case typeShort:
		return uint32(t.order.Uint16(t.data[e.at+8:])), true
	case typeLong:
		return t.order.Uint32(t.data[e.at+8:]), true
	}
	return 0, false
}

func (t tiffView) replace(e ifdEntryRef, from, to int) {
	if v, ok := t.value(e); ok && int(v) == from {
		t.setValue(e, to)
	}
}

func (t tiffView) setValue(e ifdEntryRef, v int) bool {
	if e.count != 1 || v < 0 {
		return false
	}
	switch e.typ {
	case typeShort:
		if v > 0xFFFF {
			return false
		}
		t.order.PutUint16(t.data[e.at+8:], uint16(v))
		t.data[e.at+10], t.data[e.at+11] = 0, 0
		return true
	case typeLong:
		t.order.PutUint32(t.data[e.at+8:], uint32(v))
		return true
	}
	return false
}

func (t tiffView) rationalAt(e ifdEntryRef) (int, bool) {
	if e.typ != typeRational || e.count != 1 {
		return 0, false
	}
	off := t.order.Uint32(t.data[e.at+8:])
	if uint64(off)+8 > uint64(len(t.data)) {
		return 0, false
	}
	return int(off), true
}

func (t tiffView) swapRationals(a, b ifdEntryRef) {
	oa, okA := t.rationalAt(a)
	ob, okB := t.rationalAt(b)
	if !okA || !okB || oa == ob {
		return
	}
	var tmp [8]byte
	copy(tmp[:], t.data[oa:oa+8])
	copy(t.data[oa:oa+8], t.data[ob:ob+8])
	copy(t.data[ob:ob+8], tmp[:])
}

func (t tiffView) fix(ch exifChange) {
	ifd0 := t.order.Uint32(t.data[4:])
	list, nextAt, ok := t.entries(ifd0)
	if !ok {
		return
	}
	changed := ch.transformed
	if e, ok := find(list, tagOrientation); ok {
		if v, ok := t.value(e); ok && v != 1 {
			changed = true
			t.setValue(e, 1)
		}
	}
	if e, ok := find(list, tagImageWidth); ok {
		t.replace(e, ch.oldWidth, ch.newWidth)
	}
	if e, ok := find(list, tagImageLength); ok {
		t.replace(e, ch.oldHeight, ch.newHeight)
	}
	if ch.swapResolution {
		x, okX := find(list, tagXResolution)
		y, okY := find(list, tagYResolution)
		if okX && okY {
			t.swapRationals(x, y)
		}
	}
	if e, ok := find(list, tagExifPointer); ok {
		if sub, ok := t.value(e); ok {
			t.fixExifIFD(sub, ch)
		}
	}
	if changed && nextAt+4 <= len(t.data) {
		t.order.PutUint32(t.data[nextAt:], 0)
	}
}

func (t tiffView) fixExifIFD(offset uint32, ch exifChange) {
	list, _, ok := t.entries(offset)
	if !ok {
		return
	}
	if e, ok := find(list, tagPixelXDim); ok {
		t.replace(e, ch.oldWidth, ch.newWidth)
	}
	if e, ok := find(list, tagPixelYDim); ok {
		t.replace(e, ch.oldHeight, ch.newHeight)
	}
}

func fixExifSegment(payload []byte, ch exifChange) {
	if !isExif(payload) {
		return
	}
	if t, ok := newTiffView(payload[exifPrefix:]); ok {
		t.fix(ch)
	}
}

func fixXMPSegment(payload []byte) {
	if !isXMP(payload) {
		return
	}
	body := payload[len(xmpPrefix):]
	tag := []byte(xmpOrientationTag)
	for from := 0; from < len(body); {
		i := bytes.Index(body[from:], tag)
		if i < 0 {
			return
		}
		start := from + i
		at := start + len(tag)
		from = at
		digit := -1
		switch {
		case at+2 < len(body) && body[at] == '=' && (body[at+1] == '"' || body[at+1] == '\''):
			digit = at + 2
		case start > 0 && body[start-1] == '<' && at+1 < len(body) && body[at] == '>':
			digit = at + 1
		}
		if digit < 0 || digit+1 >= len(body) {
			continue
		}
		next := body[digit+1]
		if body[digit] >= '1' && body[digit] <= '8' && (next == '"' || next == '\'' || next == '<') {
			body[digit] = '1'
		}
	}
}
