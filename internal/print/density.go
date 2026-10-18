package print

import (
	"context"
	"encoding/binary"
	"io"
	"math"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	minReadDPI = 10
	maxReadDPI = 20000

	tagXResolution    = 0x011a
	tagYResolution    = 0x011b
	tagResolutionUnit = 0x0128

	maxScanSteps = 4096
)

func validDPI(v float64) bool {
	return !math.IsNaN(v) && v >= minReadDPI && v <= maxReadDPI
}

func ReadDPI(ctx context.Context, r io.ReaderAt, size int64, id codec.ID) (float64, bool) {
	if r == nil || size <= 0 {
		return 0, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if dpi, ok := exifDPI(ctx, r, size, id); ok {
		return dpi, true
	}
	switch id {
	case codec.JPEG:
		return jfifDPI(r, size)
	case codec.PNG, codec.APNG:
		return pngDPI(r, size)
	case codec.BMP:
		return bmpDPI(r, size)
	}
	return 0, false
}

func exifDPI(ctx context.Context, r io.ReaderAt, size int64, id codec.ID) (float64, bool) {
	switch id {
	case codec.JPEG, codec.PNG, codec.APNG, codec.WebP, codec.TIFF, codec.HEIC, codec.AVIF, codec.JXL:
	default:
		return 0, false
	}
	m, err := meta.ReadBlocksContext(ctx, r, size, id)
	if err != nil && len(m.EXIF) == 0 {
		return 0, false
	}
	if len(m.EXIF) == 0 {
		return 0, false
	}
	return DPIFromEXIF(m.EXIF)
}

func DPIFromEXIF(block []byte) (float64, bool) {
	rd, err := exif.NewReader(exif.StripExifHeader(block))
	if err != nil {
		return 0, false
	}
	tree, err := rd.Tree()
	if err != nil || tree.IFD0 == nil {
		return 0, false
	}
	unit := 2
	if e, ok := tree.IFD0.Find(tagResolutionUnit); ok {
		if v, ok := e.Uint(0); ok {
			unit = int(v)
		}
	}
	per := 1.0
	switch unit {
	case 2:
	case 3:
		per = 2.54
	default:
		return 0, false
	}
	x, okx := rational(tree.IFD0, tagXResolution)
	y, oky := rational(tree.IFD0, tagYResolution)
	switch {
	case okx && validDPI(x*per):
		return x * per, true
	case oky && validDPI(y*per):
		return y * per, true
	}
	return 0, false
}

func rational(d *exif.Directory, tag uint16) (float64, bool) {
	e, ok := d.Find(tag)
	if !ok {
		return 0, false
	}
	n, den, ok := e.Rational(0)
	if !ok || den == 0 {
		return 0, false
	}
	return float64(n) / float64(den), true
}

func readAt(r io.ReaderAt, size, off int64, n int) ([]byte, bool) {
	if off < 0 || n < 0 || off+int64(n) > size {
		return nil, false
	}
	buf := make([]byte, n)
	got, _ := r.ReadAt(buf, off)
	if got == n {
		return buf, true
	}
	return nil, false
}

func jfifDPI(r io.ReaderAt, size int64) (float64, bool) {
	head, ok := readAt(r, size, 0, 2)
	if !ok || head[0] != 0xff || head[1] != 0xd8 {
		return 0, false
	}
	pos := int64(2)
	for step := 0; step < maxScanSteps; step++ {
		h, ok := readAt(r, size, pos, 4)
		if !ok || h[0] != 0xff {
			return 0, false
		}
		marker := h[1]
		if marker == 0xff {
			pos++
			continue
		}
		if marker == 0xda || marker == 0xd9 {
			return 0, false
		}
		if marker == 0x01 || (marker >= 0xd0 && marker <= 0xd8) {
			pos += 2
			continue
		}
		length := int64(binary.BigEndian.Uint16(h[2:4]))
		if length < 2 {
			return 0, false
		}
		if marker == 0xe0 && length >= 16 {
			body, ok := readAt(r, size, pos+4, 12)
			if ok && string(body[:5]) == "JFIF\x00" {
				units := body[7]
				xd := float64(binary.BigEndian.Uint16(body[8:10]))
				yd := float64(binary.BigEndian.Uint16(body[10:12]))
				per := 0.0
				switch units {
				case 1:
					per = 1
				case 2:
					per = 2.54
				}
				if per > 0 {
					if validDPI(xd * per) {
						return xd * per, true
					}
					if validDPI(yd * per) {
						return yd * per, true
					}
				}
				return 0, false
			}
		}
		pos += 2 + length
	}
	return 0, false
}

func pngDPI(r io.ReaderAt, size int64) (float64, bool) {
	sig, ok := readAt(r, size, 0, 8)
	if !ok || string(sig[1:4]) != "PNG" {
		return 0, false
	}
	pos := int64(8)
	for step := 0; step < maxScanSteps; step++ {
		h, ok := readAt(r, size, pos, 8)
		if !ok {
			return 0, false
		}
		length := int64(binary.BigEndian.Uint32(h[:4]))
		kind := string(h[4:8])
		switch kind {
		case "IDAT", "IEND":
			return 0, false
		case "pHYs":
			if length < 9 {
				return 0, false
			}
			body, ok := readAt(r, size, pos+8, 9)
			if !ok || body[8] != 1 {
				return 0, false
			}
			xp := float64(binary.BigEndian.Uint32(body[0:4])) * 0.0254
			yp := float64(binary.BigEndian.Uint32(body[4:8])) * 0.0254
			if validDPI(xp) {
				return xp, true
			}
			if validDPI(yp) {
				return yp, true
			}
			return 0, false
		}
		pos += 12 + length
	}
	return 0, false
}

func bmpDPI(r io.ReaderAt, size int64) (float64, bool) {
	h, ok := readAt(r, size, 0, 46)
	if !ok || h[0] != 'B' || h[1] != 'M' {
		return 0, false
	}
	if binary.LittleEndian.Uint32(h[14:18]) < 40 {
		return 0, false
	}
	xp := float64(int32(binary.LittleEndian.Uint32(h[38:42]))) * 0.0254
	yp := float64(int32(binary.LittleEndian.Uint32(h[42:46]))) * 0.0254
	if validDPI(xp) {
		return xp, true
	}
	if validDPI(yp) {
		return yp, true
	}
	return 0, false
}
