package exif

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/oops1/PixGo/internal/codec"
)

const (
	TagBitsPerSample   uint16 = 0x0102
	TagPhotometric     uint16 = 0x0106
	TagOrientation     uint16 = 0x0112
	TagSamplesPerPixel uint16 = 0x0115
	TagExifPointer     uint16 = 0x8769
	TagGPSPointer      uint16 = 0x8825
)

const (
	headerSize        = 8
	entrySize         = 12
	maxEntriesPerIFD  = 1024
	maxChainLength    = 1024
	maxValueBytes     = 1 << 20
	maxJPEGSteps      = 1024
	jpegSegmentHeader = 4
)

var ErrNoExif = errors.New("exif: no Exif segment")

var exifSegmentPrefix = []byte("Exif\x00")

type Type uint16

const (
	TypeByte      Type = 1
	TypeASCII     Type = 2
	TypeShort     Type = 3
	TypeLong      Type = 4
	TypeRational  Type = 5
	TypeSByte     Type = 6
	TypeUndefined Type = 7
	TypeSShort    Type = 8
	TypeSLong     Type = 9
	TypeSRational Type = 10
	TypeFloat     Type = 11
	TypeDouble    Type = 12
)

func (t Type) Size() int {
	switch t {
	case TypeByte, TypeASCII, TypeSByte, TypeUndefined:
		return 1
	case TypeShort, TypeSShort:
		return 2
	case TypeLong, TypeSLong, TypeFloat:
		return 4
	case TypeRational, TypeSRational, TypeDouble:
		return 8
	}
	return 0
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: exif: %s", codec.ErrCorrupt, fmt.Sprintf(format, args...))
}

type source interface {
	length() uint64
	read(offset, n uint64) ([]byte, error)
}

type memorySource []byte

func (m memorySource) length() uint64 { return uint64(len(m)) }

func (m memorySource) read(offset, n uint64) ([]byte, error) {
	if offset > uint64(len(m)) || n > uint64(len(m))-offset {
		return nil, invalid("range %d+%d outside %d bytes", offset, n, len(m))
	}
	return m[offset : offset+n : offset+n], nil
}

type readerSource struct {
	r      io.ReaderAt
	base   uint64
	extent uint64
}

func (s readerSource) length() uint64 { return s.extent }

func (s readerSource) read(offset, n uint64) ([]byte, error) {
	if offset > s.extent || n > s.extent-offset {
		return nil, invalid("range %d+%d outside %d bytes", offset, n, s.extent)
	}
	buf := make([]byte, n)
	if n == 0 {
		return buf, nil
	}
	got, err := s.r.ReadAt(buf, int64(s.base+offset))
	if got == len(buf) {
		return buf, nil
	}
	if err == nil || err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return nil, fmt.Errorf("%w: exif: %v", codec.ErrCorrupt, err)
}

type Reader struct {
	src   source
	order binary.ByteOrder
	first uint32
}

func NewReader(tiff []byte) (*Reader, error) {
	return newReader(memorySource(tiff))
}

func NewReaderAt(r io.ReaderAt, base, length int64) (*Reader, error) {
	if base < 0 || length < 0 || r == nil {
		return nil, invalid("bad TIFF window %d+%d", base, length)
	}
	return newReader(readerSource{r: r, base: uint64(base), extent: uint64(length)})
}

func newReader(src source) (*Reader, error) {
	return newReaderAccepting(src, func(magic uint16) bool { return magic == 42 })
}

func newReaderAccepting(src source, accept func(magic uint16) bool) (*Reader, error) {
	head, err := src.read(0, headerSize)
	if err != nil {
		return nil, err
	}
	var order binary.ByteOrder
	switch {
	case head[0] == 'I' && head[1] == 'I':
		order = binary.LittleEndian
	case head[0] == 'M' && head[1] == 'M':
		order = binary.BigEndian
	default:
		return nil, invalid("byte order mark %q", head[:2])
	}
	if !accept(order.Uint16(head[2:4])) {
		return nil, invalid("TIFF magic %d", order.Uint16(head[2:4]))
	}
	return &Reader{src: src, order: order, first: order.Uint32(head[4:8])}, nil
}

func (r *Reader) ByteOrder() binary.ByteOrder { return r.order }

func (r *Reader) FirstOffset() uint32 { return r.first }

type Entry struct {
	Tag    uint16
	Type   Type
	Count  uint32
	inline [4]byte
	reader *Reader
}

func (e Entry) byteLen() uint64 {
	return uint64(e.Count) * uint64(e.Type.Size())
}

func (e Entry) Raw() ([]byte, bool) {
	return e.RawMax(maxValueBytes)
}

func (e Entry) RawMax(limit uint64) ([]byte, bool) {
	n := e.byteLen()
	if n == 0 || n > limit {
		return nil, false
	}
	if n <= 4 {
		return e.inline[:n], true
	}
	offset := uint64(e.reader.order.Uint32(e.inline[:]))
	data, err := e.reader.src.read(offset, n)
	if err != nil {
		return nil, false
	}
	return data, true
}

func (e Entry) Uint(index int) (uint32, bool) {
	if index < 0 || uint64(index) >= uint64(e.Count) {
		return 0, false
	}
	size := e.Type.Size()
	switch e.Type {
	case TypeByte, TypeUndefined, TypeShort, TypeLong:
	default:
		return 0, false
	}
	data, ok := e.Raw()
	if !ok || (index+1)*size > len(data) {
		return 0, false
	}
	chunk := data[index*size : (index+1)*size]
	switch size {
	case 1:
		return uint32(chunk[0]), true
	case 2:
		return uint32(e.reader.order.Uint16(chunk)), true
	}
	return e.reader.order.Uint32(chunk), true
}

func (e Entry) Int(index int) (int32, bool) {
	if index < 0 || uint64(index) >= uint64(e.Count) {
		return 0, false
	}
	size := e.Type.Size()
	switch e.Type {
	case TypeSByte, TypeSShort, TypeSLong:
	default:
		return 0, false
	}
	data, ok := e.Raw()
	if !ok || (index+1)*size > len(data) {
		return 0, false
	}
	chunk := data[index*size : (index+1)*size]
	switch size {
	case 1:
		return int32(int8(chunk[0])), true
	case 2:
		return int32(int16(e.reader.order.Uint16(chunk))), true
	}
	return int32(e.reader.order.Uint32(chunk)), true
}

func (e Entry) Rational(index int) (numerator, denominator uint32, ok bool) {
	if e.Type != TypeRational || index < 0 || uint64(index) >= uint64(e.Count) {
		return 0, 0, false
	}
	data, present := e.Raw()
	if !present || (index+1)*8 > len(data) {
		return 0, 0, false
	}
	chunk := data[index*8 : index*8+8]
	return e.reader.order.Uint32(chunk[:4]), e.reader.order.Uint32(chunk[4:]), true
}

func (e Entry) SRational(index int) (numerator, denominator int32, ok bool) {
	if e.Type != TypeSRational || index < 0 || uint64(index) >= uint64(e.Count) {
		return 0, 0, false
	}
	data, present := e.Raw()
	if !present || (index+1)*8 > len(data) {
		return 0, 0, false
	}
	chunk := data[index*8 : index*8+8]
	return int32(e.reader.order.Uint32(chunk[:4])), int32(e.reader.order.Uint32(chunk[4:])), true
}

func (e Entry) Text() (string, bool) {
	if e.Type != TypeASCII {
		return "", false
	}
	data, ok := e.Raw()
	if !ok {
		return "", false
	}
	if end := bytes.IndexByte(data, 0); end >= 0 {
		data = data[:end]
	}
	return string(data), true
}

type Directory struct {
	Offset  uint32
	Entries []Entry
	Next    uint32
}

func (d *Directory) Find(tag uint16) (Entry, bool) {
	if d == nil {
		return Entry{}, false
	}
	for _, e := range d.Entries {
		if e.Tag == tag {
			return e, true
		}
	}
	return Entry{}, false
}

func (r *Reader) Directory(offset uint32) (*Directory, error) {
	countBytes, err := r.src.read(uint64(offset), 2)
	if err != nil {
		return nil, err
	}
	declared := int(r.order.Uint16(countBytes))
	if declared > maxEntriesPerIFD {
		return nil, invalid("IFD at %d declares %d entries", offset, declared)
	}
	tableStart := uint64(offset) + 2
	available := 0
	if length := r.src.length(); tableStart <= length {
		available = int((length - tableStart) / entrySize)
	}
	count := declared
	if count > available {
		count = available
	}
	dir := &Directory{Offset: offset}
	if count > 0 {
		table, err := r.src.read(tableStart, uint64(count)*entrySize)
		if err != nil {
			return nil, err
		}
		dir.Entries = make([]Entry, count)
		for i := range dir.Entries {
			raw := table[i*entrySize : (i+1)*entrySize]
			e := Entry{
				Tag:    r.order.Uint16(raw[0:2]),
				Type:   Type(r.order.Uint16(raw[2:4])),
				Count:  r.order.Uint32(raw[4:8]),
				reader: r,
			}
			copy(e.inline[:], raw[8:12])
			dir.Entries[i] = e
		}
	}
	if count == declared {
		if next, err := r.src.read(tableStart+uint64(count)*entrySize, 4); err == nil {
			dir.Next = r.order.Uint32(next)
		}
	}
	return dir, nil
}

func (r *Reader) Chain() ([]*Directory, error) {
	var chain []*Directory
	visited := make(map[uint32]bool)
	offset := r.first
	for offset != 0 && len(chain) < maxChainLength && !visited[offset] {
		visited[offset] = true
		dir, err := r.Directory(offset)
		if err != nil {
			if len(chain) == 0 {
				return nil, err
			}
			break
		}
		chain = append(chain, dir)
		offset = dir.Next
	}
	if len(chain) == 0 {
		return nil, invalid("no IFD0")
	}
	return chain, nil
}

type Tree struct {
	IFD0 *Directory
	Exif *Directory
	GPS  *Directory
	IFD1 *Directory
}

func (r *Reader) Tree() (Tree, error) {
	var tree Tree
	visited := make(map[uint32]bool)
	read := func(offset uint32) *Directory {
		if offset == 0 || visited[offset] {
			return nil
		}
		visited[offset] = true
		dir, err := r.Directory(offset)
		if err != nil {
			return nil
		}
		return dir
	}
	tree.IFD0 = read(r.first)
	if tree.IFD0 == nil {
		return tree, invalid("no IFD0")
	}
	if e, ok := tree.IFD0.Find(TagExifPointer); ok {
		if offset, ok := e.Uint(0); ok {
			tree.Exif = read(offset)
		}
	}
	if e, ok := tree.IFD0.Find(TagGPSPointer); ok {
		if offset, ok := e.Uint(0); ok {
			tree.GPS = read(offset)
		}
	}
	tree.IFD1 = read(tree.IFD0.Next)
	return tree, nil
}

func orientationFrom(r *Reader) (codec.Orientation, bool) {
	dir, err := r.Directory(r.first)
	if err != nil {
		return codec.OrientNormal, false
	}
	entry, ok := dir.Find(TagOrientation)
	if !ok || entry.Type != TypeShort {
		return codec.OrientNormal, false
	}
	value, ok := entry.Uint(0)
	if !ok || value < uint32(codec.OrientNormal) || value > uint32(codec.OrientRotate270) {
		return codec.OrientNormal, false
	}
	return codec.Orientation(value), true
}

func Orientation(tiff []byte) (codec.Orientation, bool) {
	r, err := NewReader(tiff)
	if err != nil {
		return codec.OrientNormal, false
	}
	return orientationFrom(r)
}

func OrientationAt(r io.ReaderAt, base, length int64) (codec.Orientation, bool) {
	reader, err := NewReaderAt(r, base, length)
	if err != nil {
		return codec.OrientNormal, false
	}
	return orientationFrom(reader)
}

func StripExifHeader(payload []byte) []byte {
	if len(payload) >= 6 && bytes.HasPrefix(payload, exifSegmentPrefix) && (payload[5] == 0 || payload[5] == 0xff) {
		return payload[6:]
	}
	return payload
}

func FromJPEG(r io.ReaderAt, size int64) ([]byte, error) {
	var soi [2]byte
	if err := readExactly(r, soi[:], 0, size); err != nil || soi[0] != 0xff || soi[1] != 0xd8 {
		return nil, invalid("missing JPEG SOI")
	}
	position := int64(2)
	for step := 0; step < maxJPEGSteps; step++ {
		var header [jpegSegmentHeader]byte
		available := size - position
		if available < 2 {
			return nil, ErrNoExif
		}
		if available > jpegSegmentHeader {
			available = jpegSegmentHeader
		}
		if err := readExactly(r, header[:available], position, size); err != nil {
			return nil, err
		}
		if header[0] != 0xff {
			return nil, invalid("expected marker at %d", position)
		}
		marker := header[1]
		if marker == 0xff {
			position++
			continue
		}
		switch {
		case marker == 0xd9 || marker == 0xda:
			return nil, ErrNoExif
		case marker == 0x01 || marker == 0xd8 || (marker >= 0xd0 && marker <= 0xd7):
			position += 2
			continue
		}
		if available < jpegSegmentHeader {
			return nil, invalid("segment header cut off at %d", position)
		}
		segmentLength := int64(header[2])<<8 | int64(header[3])
		if segmentLength < 2 || position+2+segmentLength > size {
			return nil, invalid("segment of %d bytes at %d exceeds file", segmentLength, position)
		}
		if marker == 0xe1 && segmentLength >= 2+6 {
			var prefix [6]byte
			if err := readExactly(r, prefix[:], position+4, size); err != nil {
				return nil, err
			}
			if bytes.HasPrefix(prefix[:], exifSegmentPrefix) && (prefix[5] == 0 || prefix[5] == 0xff) {
				payload := make([]byte, segmentLength-2-6)
				if err := readExactly(r, payload, position+4+6, size); err != nil {
					return nil, err
				}
				return payload, nil
			}
		}
		position += 2 + segmentLength
	}
	return nil, ErrNoExif
}

func readExactly(r io.ReaderAt, buf []byte, offset, size int64) error {
	if offset < 0 || offset > size || int64(len(buf)) > size-offset {
		return invalid("read of %d bytes at %d outside %d byte file", len(buf), offset, size)
	}
	if len(buf) == 0 {
		return nil
	}
	got, err := r.ReadAt(buf, offset)
	if got == len(buf) {
		return nil
	}
	if err == nil || err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return fmt.Errorf("%w: exif: %v", codec.ErrCorrupt, err)
}
