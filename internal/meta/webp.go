package meta

import (
	"context"
	"encoding/binary"
)

func scanWebP(ctx context.Context, in input, b *blocks) error {
	head, err := in.read(0, 12)
	if err != nil {
		return err
	}
	if string(head[:4]) != "RIFF" || string(head[8:12]) != "WEBP" {
		return corrupt("missing RIFF/WEBP signature")
	}
	var (
		scanErr  error
		haveExif bool
		haveICC  bool
		haveXMP  bool
		canvas   bool
		chunks   int
	)
	pos := int64(12)
walk:
	for ; chunks < maxContainerOps; chunks++ {
		if chunks%1024 == 0 {
			if err := ctx.Err(); err != nil {
				scanErr = err
				break
			}
		}
		if pos >= in.size {
			break
		}
		header, err := in.read(pos, 8)
		if err != nil {
			scanErr = corrupt("WebP chunk header cut off at %d", pos)
			break
		}
		kind := string(header[:4])
		length := int64(binary.LittleEndian.Uint32(header[4:8]))
		dataStart := pos + 8
		if dataStart+length > in.size {
			scanErr = corrupt("WebP chunk %q at %d exceeds file", kind, pos)
			break
		}
		switch kind {
		case "VP8X":
			if length >= 10 {
				data, err := in.read(dataStart, 10)
				if err != nil {
					scanErr = err
					break walk
				}
				if !canvas {
					canvas = true
					w := 1 + (int64(data[4]) | int64(data[5])<<8 | int64(data[6])<<16)
					h := 1 + (int64(data[7]) | int64(data[8])<<8 | int64(data[9])<<16)
					b.setSize(w, h)
				}
			}
		case "VP8 ":
			if length >= 10 && !canvas {
				data, err := in.read(dataStart, 10)
				if err != nil {
					scanErr = err
					break walk
				}
				if data[3] == 0x9d && data[4] == 0x01 && data[5] == 0x2a {
					b.setSize(int64(binary.LittleEndian.Uint16(data[6:8])&0x3fff), int64(binary.LittleEndian.Uint16(data[8:10])&0x3fff))
				}
			}
		case "VP8L":
			if length >= 5 && !canvas {
				data, err := in.read(dataStart, 5)
				if err != nil {
					scanErr = err
					break walk
				}
				if data[0] == 0x2f {
					bits := binary.LittleEndian.Uint32(data[1:5])
					b.setSize(int64(bits&0x3fff)+1, int64(bits>>14&0x3fff)+1)
				}
			}
		case "EXIF":
			if !haveExif && length >= 8 {
				prefixLen := int64(6)
				if length < prefixLen {
					prefixLen = length
				}
				prefix, err := in.read(dataStart, int(prefixLen))
				if err != nil {
					scanErr = err
					break walk
				}
				skip := exifPrefixLen(prefix)
				if length-skip >= 8 {
					haveExif = true
					b.setTIFF(dataStart+skip, length-skip)
				}
			}
		case "ICCP":
			if !haveICC && length > 0 && length <= MaxICCBytes {
				data, err := in.read(dataStart, int(length))
				if err != nil {
					scanErr = err
					break walk
				}
				haveICC = true
				b.icc = data
			}
		case "XMP ":
			if !haveXMP && length > 0 && length <= MaxXMPBytes {
				data, err := in.read(dataStart, int(length))
				if err != nil {
					scanErr = err
					break walk
				}
				haveXMP = true
				b.xmp = data
			}
		}
		pos = dataStart + length + length&1
	}
	if scanErr == nil && chunks >= maxContainerOps {
		scanErr = corrupt("too many WebP chunks")
	}
	return scanErr
}
