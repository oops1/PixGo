package meta

import (
	"encoding/binary"

	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	tagImageWidth    uint16 = 0x0100
	tagImageLength   uint16 = 0x0101
	tagXMPPacket     uint16 = 0x02bc
	tagICCProfile    uint16 = 0x8773
	tiffMagicClassic        = 42
	tiffMagicBig            = 43
)

func scanTIFF(in input, b *blocks) error {
	head, err := in.read(0, 4)
	if err != nil {
		return err
	}
	var order binary.ByteOrder
	switch string(head[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return corrupt("TIFF byte order mark %q", head[:2])
	}
	switch order.Uint16(head[2:4]) {
	case tiffMagicClassic:
		b.wholeTIFF = true
		b.setTIFF(0, in.size)
		return nil
	case tiffMagicBig:
		return nil
	}
	return corrupt("TIFF magic %d", order.Uint16(head[2:4]))
}

func tiffPayloads(ifd0 *exif.Directory) (width, height int, icc, xmp []byte) {
	if e, ok := ifd0.Find(tagImageWidth); ok {
		if v, ok := e.Uint(0); ok && v <= maxDimension {
			width = int(v)
		}
	}
	if e, ok := ifd0.Find(tagImageLength); ok {
		if v, ok := e.Uint(0); ok && v <= maxDimension {
			height = int(v)
		}
	}
	if width == 0 || height == 0 {
		width, height = 0, 0
	}
	if e, ok := ifd0.Find(tagICCProfile); ok && byteLike(e) {
		if data, ok := e.RawMax(MaxICCBytes); ok {
			icc = data
		}
	}
	if e, ok := ifd0.Find(tagXMPPacket); ok && byteLike(e) {
		if data, ok := e.RawMax(MaxXMPBytes); ok {
			xmp = data
		}
	}
	return width, height, icc, xmp
}

func byteLike(e exif.Entry) bool {
	return e.Type == exif.TypeByte || e.Type == exif.TypeUndefined || e.Type == exif.TypeASCII
}
