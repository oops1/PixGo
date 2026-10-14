package meta

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"

	"github.com/oops1/PixGo/internal/isobmff"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	rafSignature    = "FUJIFILMCCD-RAW "
	rafJPEGOffsetAt = 84
	rafHeaderSize   = 92
	rawHeadLen      = 16
	maxCR3Boxes     = 4096
	maxCMTBytes     = 1 << 20
)

var (
	cr3CanonUUID = []byte{0x85, 0xc0, 0xb6, 0x87, 0x82, 0x0f, 0x11, 0xe0, 0x81, 0x11, 0xf4, 0xce, 0x46, 0x2b, 0x6a, 0x48}
	cr3XMPUUID   = []byte{0xbe, 0x7a, 0xcf, 0xcb, 0x97, 0xa9, 0x42, 0xe8, 0x9c, 0x71, 0x99, 0x94, 0x91, 0xe3, 0xaf, 0xac}
)

func cameraTIFFHeader(head []byte) bool {
	if len(head) < 8 {
		return false
	}
	var order binary.ByteOrder
	switch string(head[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return false
	}
	switch order.Uint16(head[2:4]) {
	case 42, exif.MagicOlympus, exif.MagicOlympusAlt, exif.MagicPanasonic:
		return true
	}
	return false
}

func scanRAW(ctx context.Context, in input, b *blocks) error {
	head, err := in.read(0, int(min(in.size, rawHeadLen)))
	if err != nil {
		return err
	}
	switch {
	case bytes.HasPrefix(head, []byte(rafSignature)):
		return scanRAF(ctx, in, b)
	case len(head) >= 12 && string(head[4:12]) == "ftypcrx ":
		return scanCR3(ctx, in, b)
	case cameraTIFFHeader(head):
		b.camera, b.wholeTIFF, b.noSize = true, true, true
		b.setTIFF(0, in.size)
	}
	return nil
}

func scanRAF(ctx context.Context, in input, b *blocks) error {
	if in.size < rafHeaderSize {
		return corrupt("RAF header cut off at %d bytes", in.size)
	}
	fields, err := in.read(rafJPEGOffsetAt, 8)
	if err != nil {
		return err
	}
	offset := int64(binary.BigEndian.Uint32(fields[0:4]))
	length := int64(binary.BigEndian.Uint32(fields[4:8]))
	if offset < rafHeaderSize || length < 2 || offset > in.size || length > in.size-offset {
		return corrupt("RAF JPEG preview %d+%d outside %d bytes", offset, length, in.size)
	}
	preview := input{r: io.NewSectionReader(in.r, offset, length), size: length}
	err = scanJPEG(ctx, preview, b)
	if b.tiffLen > 0 {
		b.tiffBase += offset
	}
	b.width, b.height = 0, 0
	return err
}

type cr3Tree struct {
	ifd0, exif, gps *exif.Directory
}

func (t cr3Tree) found() bool { return t.ifd0 != nil || t.exif != nil || t.gps != nil }

func (t cr3Tree) tree() *exif.Tree {
	ifd0 := t.ifd0
	if ifd0 == nil {
		ifd0 = &exif.Directory{}
	}
	return &exif.Tree{IFD0: ifd0, Exif: t.exif, GPS: t.gps}
}

func cmtDirectory(in input, s isobmff.Span) *exif.Directory {
	if s.Len() < 8 || s.Len() > maxCMTBytes {
		return nil
	}
	reader, err := exif.NewReaderAt(in.r, s.Payload, s.Len())
	if err != nil {
		return nil
	}
	dir, err := reader.Directory(reader.FirstOffset())
	if err != nil {
		return nil
	}
	return dir
}

func uuidOf(in input, s isobmff.Span) ([]byte, bool) {
	if s.Len() < int64(len(cr3CanonUUID)) {
		return nil, false
	}
	id, err := in.read(s.Payload, len(cr3CanonUUID))
	return id, err == nil
}

func scanCR3(ctx context.Context, in input, b *blocks) error {
	var (
		scanErr error
		parts   cr3Tree
		haveXMP bool
	)
	fail := func(err error) {
		if scanErr == nil {
			scanErr = containerErr(ctx, err)
		}
	}
	canonChild := func(s isobmff.Span) error {
		switch s.Type {
		case "CMT1":
			if parts.ifd0 == nil {
				parts.ifd0 = cmtDirectory(in, s)
			}
		case "CMT2":
			if parts.exif == nil {
				parts.exif = cmtDirectory(in, s)
			}
		case "CMT4":
			if parts.gps == nil {
				parts.gps = cmtDirectory(in, s)
			}
		}
		return nil
	}
	movieChild := func(s isobmff.Span) error {
		if s.Type != "uuid" {
			return nil
		}
		if id, ok := uuidOf(in, s); ok && bytes.Equal(id, cr3CanonUUID) {
			fail(isobmff.Walk(ctx, in.r, in.size, s.Payload+int64(len(id)), s.End, maxCR3Boxes, canonChild))
		}
		return nil
	}
	err := isobmff.Walk(ctx, in.r, in.size, 0, in.size, maxCR3Boxes, func(s isobmff.Span) error {
		switch s.Type {
		case "moov":
			fail(isobmff.Walk(ctx, in.r, in.size, s.Payload, s.End, maxCR3Boxes, movieChild))
		case "uuid":
			id, ok := uuidOf(in, s)
			if !ok || haveXMP || !bytes.Equal(id, cr3XMPUUID) {
				return nil
			}
			haveXMP = true
			data, ok, err := in.readBlock(s.Payload+int64(len(id)), s.Len()-int64(len(id)), MaxXMPBytes)
			if err != nil {
				fail(err)
			} else if ok {
				b.xmp = data
			}
		}
		return nil
	})
	fail(err)
	if parts.found() {
		b.tree = parts.tree()
	}
	return scanErr
}
