package meta

import (
	"encoding/binary"
	"slices"

	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	tagOrientation   uint16 = 0x0112
	tagRelatedWidth  uint16 = 0x1001
	tagRelatedLength uint16 = 0x1002
	tagInterop       uint16 = 0xa005
	tagMakerNote     uint16 = 0x927c
	tagSubIFDs       uint16 = 0x014a
	tagStripOffsets  uint16 = 0x0111
	tagStripCounts   uint16 = 0x0117
	tagTileOffsets   uint16 = 0x0144
	tagTileCounts    uint16 = 0x0145
	tagThumbOffset   uint16 = 0x0201
	tagThumbLength   uint16 = 0x0202

	maxTIFFDirs      = 16
	maxTIFFEntryData = 8 << 20
)

type tiffOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

func orderOf(o binary.ByteOrder) tiffOrder {
	if o == binary.BigEndian {
		return binary.BigEndian
	}
	return binary.LittleEndian
}

type tiffEntry struct {
	tag   uint16
	typ   exif.Type
	count uint32
	data  []byte
	pinAt int
}

type tiffDir struct {
	entries []tiffEntry
}

func (d *tiffDir) index(tag uint16) int {
	if d == nil {
		return -1
	}
	for i := range d.entries {
		if d.entries[i].tag == tag {
			return i
		}
	}
	return -1
}

func (d *tiffDir) remove(tag uint16) {
	if i := d.index(tag); i >= 0 {
		d.entries = slices.Delete(d.entries, i, i+1)
	}
}

type tiffDoc struct {
	order   tiffOrder
	ifd0    *tiffDir
	exif    *tiffDir
	gps     *tiffDir
	interop *tiffDir
	ifd1    *tiffDir
	thumb   []byte
}

func skippedTag(tag uint16) bool {
	switch tag {
	case 0x8769, 0x8825, tagInterop, tagSubIFDs, tagStripOffsets, tagStripCounts, tagTileOffsets, tagTileCounts, tagThumbOffset, tagThumbLength:
		return true
	}
	return false
}

func swapValue(typ exif.Type, data []byte) {
	width := 0
	switch typ {
	case exif.TypeShort, exif.TypeSShort:
		width = 2
	case exif.TypeLong, exif.TypeSLong, exif.TypeFloat, exif.TypeRational, exif.TypeSRational:
		width = 4
	case exif.TypeDouble:
		width = 8
	}
	if width == 0 {
		return
	}
	for i := 0; i+width <= len(data); i += width {
		slices.Reverse(data[i : i+width])
	}
}

func convertDir(dir *exif.Directory, order tiffOrder, keep func(tag uint16) bool) *tiffDir {
	out := &tiffDir{}
	if dir == nil {
		return out
	}
	seen := map[uint16]bool{}
	for _, e := range dir.Entries {
		if seen[e.Tag] || skippedTag(e.Tag) || e.Type.Size() == 0 || (keep != nil && !keep(e.Tag)) {
			continue
		}
		data, ok := e.RawMax(maxTIFFEntryData)
		if !ok {
			continue
		}
		seen[e.Tag] = true
		te := tiffEntry{tag: e.Tag, typ: e.Type, count: e.Count, data: slices.Clone(data)}
		if e.ByteOrder() != binary.ByteOrder(order) {
			swapValue(e.Type, te.data)
		}
		if at, ok := e.InlineOffset(); ok && e.Tag == tagMakerNote {
			te.pinAt = int(at)
		}
		out.entries = append(out.entries, te)
	}
	return out
}

func subDirectory(r *exif.Reader, dir *exif.Directory, tag uint16, visited map[uint32]bool) *exif.Directory {
	e, ok := dir.Find(tag)
	if !ok {
		return nil
	}
	off, ok := e.Uint(0)
	if !ok || off == 0 || visited[off] || len(visited) >= maxTIFFDirs {
		return nil
	}
	visited[off] = true
	sub, err := r.Directory(off)
	if err != nil {
		return nil
	}
	return sub
}

func parseTIFF(raw []byte) (*tiffDoc, error) {
	r, err := exif.NewReader(raw)
	if err != nil {
		return nil, err
	}
	order := orderOf(r.ByteOrder())
	visited := map[uint32]bool{r.FirstOffset(): true}
	d0, err := r.Directory(r.FirstOffset())
	if err != nil {
		return nil, err
	}
	doc := &tiffDoc{order: order, ifd0: convertDir(d0, order, nil)}
	exifDir := subDirectory(r, d0, 0x8769, visited)
	if exifDir != nil {
		doc.exif = convertDir(exifDir, order, nil)
		if inter := subDirectory(r, exifDir, tagInterop, visited); inter != nil {
			doc.interop = convertDir(inter, order, nil)
		}
	}
	if gps := subDirectory(r, d0, 0x8825, visited); gps != nil {
		doc.gps = convertDir(gps, order, nil)
	}
	if d0.Next != 0 && !visited[d0.Next] {
		visited[d0.Next] = true
		if d1, err := r.Directory(d0.Next); err == nil {
			doc.attachThumbnail(raw, d1)
		}
	}
	return doc, nil
}

func (d *tiffDoc) attachThumbnail(raw []byte, d1 *exif.Directory) {
	offEntry, ok1 := d1.Find(tagThumbOffset)
	lenEntry, ok2 := d1.Find(tagThumbLength)
	if !ok1 || !ok2 {
		return
	}
	off, ok1 := offEntry.Uint(0)
	length, ok2 := lenEntry.Uint(0)
	if !ok1 || !ok2 || length < 4 || uint64(off) > uint64(len(raw)) || uint64(length) > uint64(len(raw))-uint64(off) {
		return
	}
	data := raw[off : off+length]
	if data[0] != 0xff || data[1] != 0xd8 {
		return
	}
	d.thumb = slices.Clone(data)
	d.ifd1 = convertDir(d1, d.order, nil)
}

func (d *tiffDoc) put16(v uint16) []byte { return d.order.AppendUint16(nil, v) }

func (d *tiffDoc) put32(v uint32) []byte { return d.order.AppendUint32(nil, v) }

func (d *tiffDoc) uintEntry(tag uint16, typ exif.Type, v uint32) tiffEntry {
	if typ == exif.TypeShort {
		return tiffEntry{tag: tag, typ: typ, count: 1, data: d.put16(uint16(v))}
	}
	return tiffEntry{tag: tag, typ: exif.TypeLong, count: 1, data: d.put32(v)}
}

func (d *tiffDoc) setOrientation(dir *tiffDir) {
	if i := dir.index(tagOrientation); i >= 0 {
		dir.entries[i] = d.uintEntry(tagOrientation, exif.TypeShort, 1)
	}
}

func (d *tiffDoc) setSize(dir *tiffDir, tag uint16, v int) {
	i := dir.index(tag)
	if i < 0 || v <= 0 {
		return
	}
	typ := exif.TypeLong
	if v <= 0xffff {
		typ = exif.TypeShort
	}
	dir.entries[i] = d.uintEntry(tag, typ, uint32(v))
}

type dirPlan struct {
	entries []tiffEntry
	at      int
	offs    []int
}

func (d *tiffDoc) plan(dir *tiffDir, extra ...tiffEntry) *dirPlan {
	if dir == nil {
		return nil
	}
	p := &dirPlan{entries: append(slices.Clone(dir.entries), extra...)}
	slices.SortStableFunc(p.entries, func(a, b tiffEntry) int { return int(a.tag) - int(b.tag) })
	p.offs = make([]int, len(p.entries))
	return p
}

func (p *dirPlan) size() int { return 2 + 12*len(p.entries) + 4 }

func (p *dirPlan) setPointer(d *tiffDoc, tag uint16, to int) {
	for i := range p.entries {
		if p.entries[i].tag == tag {
			p.entries[i].data = d.put32(uint32(to))
		}
	}
}

type layout struct {
	cursor int
	pinAt  int
	pinEnd int
}

func (l *layout) place(n int) int {
	off := (l.cursor + 1) &^ 1
	if l.pinEnd > l.pinAt && off < l.pinEnd && off+n > l.pinAt {
		off = (l.pinEnd + 1) &^ 1
	}
	l.cursor = off + n
	return off
}

func (l *layout) end() int { return max(l.cursor, l.pinEnd) }

func (d *tiffDoc) encode(pin bool) []byte {
	pointer := func(tag uint16) tiffEntry { return d.uintEntry(tag, exif.TypeLong, 0) }
	var p0Extra, peExtra, p1Extra []tiffEntry
	if d.exif != nil {
		p0Extra = append(p0Extra, pointer(0x8769))
	}
	if d.gps != nil {
		p0Extra = append(p0Extra, pointer(0x8825))
	}
	if d.interop != nil && d.exif != nil {
		peExtra = append(peExtra, pointer(tagInterop))
	}
	if d.ifd1 != nil && d.thumb != nil {
		p1Extra = append(p1Extra, pointer(tagThumbOffset), pointer(tagThumbLength))
	}
	p0 := d.plan(d.ifd0, p0Extra...)
	pe := d.plan(d.exif, peExtra...)
	pi := d.plan(d.interop)
	pg := d.plan(d.gps)
	p1 := d.plan(d.ifd1, p1Extra...)
	if p0 == nil {
		p0 = d.plan(&tiffDir{})
	}
	if d.exif == nil {
		pe, pi = nil, nil
	}
	if d.interop == nil || d.exif == nil {
		pi = nil
	}
	if d.ifd1 == nil || d.thumb == nil {
		p1 = nil
	}
	l := &layout{cursor: 8}
	pinned := -1
	if pin && pe != nil {
		for i, e := range pe.entries {
			if e.tag == tagMakerNote && e.pinAt >= 8 && len(e.data) > 4 && pinWorthKeeping(e.data) {
				pinned = i
				l.pinAt, l.pinEnd = e.pinAt, e.pinAt+len(e.data)
			}
		}
	}
	plans := []*dirPlan{p0, pe, pi, pg, p1}
	for _, p := range plans {
		if p == nil {
			continue
		}
		p.at = l.place(p.size())
		for i, e := range p.entries {
			switch {
			case len(e.data) <= 4:
				p.offs[i] = -1
			case p == pe && i == pinned:
				p.offs[i] = e.pinAt
			default:
				p.offs[i] = l.place(len(e.data))
			}
		}
	}
	thumbAt := 0
	if p1 != nil {
		thumbAt = l.place(len(d.thumb))
	}
	if p0 != nil && pe != nil {
		p0.setPointer(d, 0x8769, pe.at)
	}
	if p0 != nil && pg != nil {
		p0.setPointer(d, 0x8825, pg.at)
	}
	if pe != nil && pi != nil {
		pe.setPointer(d, tagInterop, pi.at)
	}
	if p1 != nil {
		p1.setPointer(d, tagThumbOffset, thumbAt)
		p1.setPointer(d, tagThumbLength, len(d.thumb))
	}
	buf := make([]byte, l.end())
	if d.order == binary.LittleEndian {
		copy(buf, "II")
	} else {
		copy(buf, "MM")
	}
	d.order.PutUint16(buf[2:], 42)
	d.order.PutUint32(buf[4:], uint32(p0.at))
	for _, p := range plans {
		if p == nil {
			continue
		}
		at := p.at
		d.order.PutUint16(buf[at:], uint16(len(p.entries)))
		for i, e := range p.entries {
			rec := buf[at+2+12*i:]
			d.order.PutUint16(rec[0:], e.tag)
			d.order.PutUint16(rec[2:], uint16(e.typ))
			d.order.PutUint32(rec[4:], e.count)
			if p.offs[i] < 0 {
				copy(rec[8:12], e.data)
				continue
			}
			d.order.PutUint32(rec[8:], uint32(p.offs[i]))
			copy(buf[p.offs[i]:], e.data)
		}
		if p == p0 && p1 != nil {
			d.order.PutUint32(buf[at+2+12*len(p.entries):], uint32(p1.at))
		}
	}
	if p1 != nil {
		copy(buf[thumbAt:], d.thumb)
	}
	return buf
}

func pinWorthKeeping(note []byte) bool {
	if len(note) < 4 {
		return false
	}
	for _, c := range note[:3] {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
			return true
		}
	}
	return false
}

func swapEntries(dir *tiffDir, a, b uint16) {
	i, j := dir.index(a), dir.index(b)
	if i < 0 || j < 0 {
		return
	}
	dir.entries[i].data, dir.entries[j].data = dir.entries[j].data, dir.entries[i].data
	dir.entries[i].typ, dir.entries[j].typ = dir.entries[j].typ, dir.entries[i].typ
	dir.entries[i].count, dir.entries[j].count = dir.entries[j].count, dir.entries[i].count
}
