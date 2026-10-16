package tiff

import (
	"encoding/binary"
	"io"

	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

const (
	typeByte   = 1
	typeASCII  = 2
	typeShort  = 3
	typeLong   = 4
	typeRat    = 5
	typeSByte  = 6
	typeUndef  = 7
	typeSShort = 8
	typeSLong  = 9
	typeSRat   = 10
	typeFloat  = 11
	typeDouble = 12
	typeIFD    = 13
	typeLong8  = 16
	typeSLong8 = 17
	typeIFD8   = 18
)

const (
	tagImageWidth      = 0x100
	tagImageLength     = 0x101
	tagBitsPerSample   = 0x102
	tagCompression     = 0x103
	tagPhotometric     = 0x106
	tagFillOrder       = 0x10a
	tagStripOffsets    = 0x111
	tagOrientation     = 0x112
	tagSamplesPerPixel = 0x115
	tagRowsPerStrip    = 0x116
	tagStripByteCounts = 0x117
	tagPlanar          = 0x11c
	tagPredictor       = 0x13d
	tagColorMap        = 0x140
	tagTileWidth       = 0x142
	tagTileLength      = 0x143
	tagTileOffsets     = 0x144
	tagTileByteCounts  = 0x145
	tagExtraSamples    = 0x152
	tagSampleFormat    = 0x153
)

const (
	maxIFDEntries = 4096
	classicInline = 4
	bigInline     = 8
)

type ifdEntry struct {
	tag   uint16
	typ   uint16
	count uint64
	field [8]byte
}

type file struct {
	r       io.ReaderAt
	size    int64
	order   binary.ByteOrder
	big     bool
	entries map[uint16]ifdEntry
}

func typeSize(typ uint16) int64 {
	switch typ {
	case typeByte, typeASCII, typeSByte, typeUndef:
		return 1
	case typeShort, typeSShort:
		return 2
	case typeLong, typeSLong, typeFloat, typeIFD:
		return 4
	case typeRat, typeSRat, typeDouble, typeLong8, typeSLong8, typeIFD8:
		return 8
	}
	return 0
}

func readFile(r io.ReaderAt, size int64) (*file, error) {
	if size < 8 {
		return nil, guard.Corrupt("tiff: %d bytes cannot hold a header", size)
	}
	headLen := int64(16)
	if size < headLen {
		headLen = size
	}
	head, err := guard.ReadAt(r, 0, headLen, size)
	if err != nil {
		return nil, err
	}
	f := &file{r: r, size: size, entries: make(map[uint16]ifdEntry)}
	switch {
	case head[0] == 'I' && head[1] == 'I':
		f.order = binary.LittleEndian
	case head[0] == 'M' && head[1] == 'M':
		f.order = binary.BigEndian
	default:
		return nil, guard.Corrupt("tiff: byte order mark %q", head[:2])
	}
	var firstIFD uint64
	switch f.order.Uint16(head[2:4]) {
	case 42:
		firstIFD = uint64(f.order.Uint32(head[4:8]))
	case 43:
		if len(head) < 16 || f.order.Uint16(head[4:6]) != 8 || f.order.Uint16(head[6:8]) != 0 {
			return nil, guard.Corrupt("tiff: malformed BigTIFF header")
		}
		f.big = true
		firstIFD = f.order.Uint64(head[8:16])
	default:
		return nil, guard.Corrupt("tiff: magic %d", f.order.Uint16(head[2:4]))
	}
	if err := f.readIFD(firstIFD); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *file) readIFD(offset uint64) error {
	countSize, entrySize := uint64(2), uint64(12)
	if f.big {
		countSize, entrySize = 8, 20
	}
	if offset < 8 || offset > uint64(f.size) || countSize > uint64(f.size)-offset {
		return guard.Corrupt("tiff: IFD offset %d outside the file", offset)
	}
	raw, err := guard.ReadAt(f.r, int64(offset), int64(countSize), f.size)
	if err != nil {
		return err
	}
	var count uint64
	if f.big {
		count = f.order.Uint64(raw)
	} else {
		count = uint64(f.order.Uint16(raw))
	}
	if count == 0 || count > maxIFDEntries {
		return guard.Corrupt("tiff: IFD declares %d entries", count)
	}
	tableStart := offset + countSize
	if count*entrySize > uint64(f.size)-tableStart {
		return guard.Corrupt("tiff: IFD table cut off")
	}
	table, err := guard.ReadAt(f.r, int64(tableStart), int64(count*entrySize), f.size)
	if err != nil {
		return err
	}
	for i := uint64(0); i < count; i++ {
		raw := table[i*entrySize : (i+1)*entrySize]
		e := ifdEntry{tag: f.order.Uint16(raw[0:2]), typ: f.order.Uint16(raw[2:4])}
		if f.big {
			e.count = f.order.Uint64(raw[4:12])
			copy(e.field[:], raw[12:20])
		} else {
			e.count = uint64(f.order.Uint32(raw[4:8]))
			copy(e.field[:], raw[8:12])
		}
		if _, dup := f.entries[e.tag]; !dup {
			f.entries[e.tag] = e
		}
	}
	return nil
}

func (f *file) inlineSize() uint64 {
	if f.big {
		return bigInline
	}
	return classicInline
}

func (f *file) valueBytes(e ifdEntry, want uint64) ([]byte, error) {
	elem := typeSize(e.typ)
	if elem == 0 {
		return nil, guard.Corrupt("tiff: tag %#x has unknown type %d", e.tag, e.typ)
	}
	if e.count > (1<<62)/uint64(elem) {
		return nil, guard.Corrupt("tiff: tag %#x declares %d values", e.tag, e.count)
	}
	total := e.count * uint64(elem)
	if total <= f.inlineSize() {
		return e.field[:want], nil
	}
	var offset uint64
	if f.big {
		offset = f.order.Uint64(e.field[:8])
	} else {
		offset = uint64(f.order.Uint32(e.field[:4]))
	}
	if offset > uint64(f.size) || want > uint64(f.size)-offset {
		return nil, guard.Corrupt("tiff: tag %#x values lie outside the file", e.tag)
	}
	return guard.ReadAt(f.r, int64(offset), int64(want), f.size)
}

func (f *file) uints(e ifdEntry, limit uint64) ([]uint64, error) {
	size := typeSize(e.typ)
	switch e.typ {
	case typeByte, typeShort, typeLong, typeLong8, typeIFD, typeIFD8:
	default:
		return nil, guard.Corrupt("tiff: tag %#x has type %d, expected an unsigned integer", e.tag, e.typ)
	}
	count := min(e.count, limit)
	raw, err := f.valueBytes(e, count*uint64(size))
	if err != nil {
		return nil, err
	}
	out := make([]uint64, count)
	for i := range out {
		chunk := raw[uint64(i)*uint64(size):]
		switch size {
		case 1:
			out[i] = uint64(chunk[0])
		case 2:
			out[i] = uint64(f.order.Uint16(chunk))
		case 4:
			out[i] = uint64(f.order.Uint32(chunk))
		default:
			out[i] = f.order.Uint64(chunk)
		}
	}
	return out, nil
}

func (f *file) first(tag uint16) (uint64, bool, error) {
	e, ok := f.entries[tag]
	if !ok {
		return 0, false, nil
	}
	values, err := f.uints(e, 1)
	if err != nil {
		return 0, false, err
	}
	if len(values) == 0 {
		return 0, false, nil
	}
	return values[0], true, nil
}
