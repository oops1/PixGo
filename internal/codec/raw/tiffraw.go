package raw

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	tagCompression   uint16 = 0x0103
	tagStripOffsets  uint16 = 0x0111
	tagStripCounts   uint16 = 0x0117
	tagSubIFDs       uint16 = 0x014a
	tagJPEGOffset    uint16 = 0x0201
	tagJPEGLength    uint16 = 0x0202
	tagExifIFD       uint16 = 0x8769
	tagMakerNote     uint16 = 0x927c
	tagJpgFromRaw    uint16 = 0x002e
	tagNikonPreview  uint16 = 0x0011
	tagOlympusThumb  uint16 = 0x0100
	tagOlympusCamera uint16 = 0x2020
	tagOlympusStart  uint16 = 0x0101
	tagOlympusLength uint16 = 0x0102
	tagDNGVersion    uint16 = 0xc612
	tagMake          uint16 = 0x010f

	compressionOldJPEG = 6
	compressionJPEG    = 7
	maxIFDs            = 64
	maxIFDDepth        = 6
	maxSubIFDs         = 32
	maxStrips          = 4096
	makerNoteHeader    = 16
)

type ifdKey struct {
	base   int64
	offset uint32
}

type walker struct {
	ctx     context.Context
	f       *finder
	visited map[ifdKey]bool
	count   int
}

func newWalker(ctx context.Context, f *finder) *walker {
	return &walker{ctx: ctx, f: f, visited: map[ifdKey]bool{}}
}

type scope struct {
	rd   *exif.Reader
	base int64
}

func (w *walker) directory(s scope, offset uint32) (*exif.Directory, bool) {
	key := ifdKey{base: s.base, offset: offset}
	if offset == 0 || w.visited[key] || w.count >= maxIFDs || w.ctx.Err() != nil {
		return nil, false
	}
	w.visited[key] = true
	w.count++
	dir, err := s.rd.Directory(offset)
	if err != nil {
		return nil, false
	}
	return dir, true
}

func (w *walker) chain(s scope, offset uint32, depth int) {
	for {
		dir, ok := w.directory(s, offset)
		if !ok {
			return
		}
		w.scan(s, dir, depth)
		offset = dir.Next
	}
}

func (w *walker) single(s scope, offset uint32, depth int) {
	if dir, ok := w.directory(s, offset); ok {
		w.scan(s, dir, depth)
	}
}

func (w *walker) addRange(s scope, offset, length uint64) {
	w.f.add(s.base+int64(offset), int64(length), false)
}

func firstUint(dir *exif.Directory, tag uint16) (uint32, bool) {
	e, ok := dir.Find(tag)
	if !ok {
		return 0, false
	}
	return e.Uint(0)
}

func (w *walker) scan(s scope, dir *exif.Directory, depth int) {
	if offset, ok := firstUint(dir, tagJPEGOffset); ok {
		if length, ok := firstUint(dir, tagJPEGLength); ok {
			w.addRange(s, uint64(offset), uint64(length))
		}
	}
	if compression, ok := firstUint(dir, tagCompression); ok && (compression == compressionOldJPEG || compression == compressionJPEG) {
		w.scanStrips(s, dir)
	}
	if e, ok := dir.Find(tagJpgFromRaw); ok && (e.Type == exif.TypeUndefined || e.Type == exif.TypeByte) {
		if offset, length, ok := e.Location(); ok {
			w.addRange(s, offset, length)
		}
	}
	if e, ok := dir.Find(tagSubIFDs); ok && depth < maxIFDDepth {
		for i := 0; i < int(e.Count) && i < maxSubIFDs; i++ {
			if offset, ok := e.Uint(i); ok {
				w.single(s, offset, depth+1)
			}
		}
	}
	if offset, ok := firstUint(dir, tagExifIFD); ok && depth < maxIFDDepth {
		if exifDir, ok := w.directory(s, offset); ok {
			w.scan(s, exifDir, depth+1)
			if e, ok := exifDir.Find(tagMakerNote); ok {
				w.makerNote(s, e)
			}
		}
	}
}

func (w *walker) scanStrips(s scope, dir *exif.Directory) {
	offsets, ok := dir.Find(tagStripOffsets)
	if !ok || offsets.Count == 0 || offsets.Count > maxStrips {
		return
	}
	counts, ok := dir.Find(tagStripCounts)
	if !ok || counts.Count != offsets.Count {
		return
	}
	first, ok := offsets.Uint(0)
	if !ok {
		return
	}
	var total uint64
	for i := 0; i < int(counts.Count); i++ {
		n, ok := counts.Uint(i)
		if !ok {
			return
		}
		total += uint64(n)
	}
	w.addRange(s, uint64(first), total)
}

func (w *walker) makerNote(s scope, e exif.Entry) {
	offset, length, ok := e.Location()
	if !ok || length < makerNoteHeader {
		return
	}
	start := s.base + int64(offset)
	var head [makerNoteHeader]byte
	if err := guard.ReadAtInto(w.f.r, head[:], start, w.f.size); err != nil {
		return
	}
	switch {
	case bytes.HasPrefix(head[:], []byte("Nikon\x00")) && head[6] == 2:
		w.nikon(start + 10)
	case bytes.HasPrefix(head[:], []byte("OLYMPUS\x00")):
		order, ok := byteOrderOf(head[8:10])
		if !ok {
			return
		}
		rd, err := exif.NewReaderAtOffset(w.f.r, start, w.f.size-start, order, 12)
		if err == nil {
			w.olympus(scope{rd: rd, base: start}, 12)
		}
	case bytes.HasPrefix(head[:], []byte("OLYMP\x00")):
		rd, err := exif.NewReaderAtOffset(w.f.r, s.base, w.f.size-s.base, s.rd.ByteOrder(), uint32(offset)+8)
		if err == nil {
			w.olympus(scope{rd: rd, base: s.base}, uint32(offset)+8)
		}
	}
}

func byteOrderOf(mark []byte) (binary.ByteOrder, bool) {
	switch string(mark) {
	case "II":
		return binary.LittleEndian, true
	case "MM":
		return binary.BigEndian, true
	}
	return nil, false
}

func (w *walker) nikon(base int64) {
	rd, err := exif.NewReaderAtCamera(w.f.r, base, w.f.size-base)
	if err != nil {
		return
	}
	s := scope{rd: rd, base: base}
	dir, ok := w.directory(s, rd.FirstOffset())
	if !ok {
		return
	}
	if offset, ok := firstUint(dir, tagNikonPreview); ok {
		w.single(s, offset, maxIFDDepth-1)
	}
}

func (w *walker) olympus(s scope, first uint32) {
	dir, ok := w.directory(s, first)
	if !ok {
		return
	}
	if e, ok := dir.Find(tagOlympusThumb); ok && (e.Type == exif.TypeUndefined || e.Type == exif.TypeByte) {
		if offset, length, ok := e.Location(); ok {
			w.addRange(s, offset, length)
		}
	}
	offset, ok := firstUint(dir, tagOlympusCamera)
	if !ok {
		return
	}
	settings, ok := w.directory(s, offset)
	if !ok {
		return
	}
	if start, ok := firstUint(settings, tagOlympusStart); ok {
		if length, ok := firstUint(settings, tagOlympusLength); ok {
			w.addRange(s, uint64(start), uint64(length))
		}
	}
}

func discoverTIFF(ctx context.Context, r io.ReaderAt, size int64, f *finder) (codec.Orientation, bool, error) {
	rd, err := exif.NewReaderAtCamera(r, 0, size)
	if err != nil {
		return 0, false, guard.Corrupt("raw: %v", err)
	}
	newWalker(ctx, f).chain(scope{rd: rd}, rd.FirstOffset(), 0)
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	orientation, ok := rd.Orientation()
	return orientation, ok, nil
}
