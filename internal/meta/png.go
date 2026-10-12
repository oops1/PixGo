package meta

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"io"
)

var (
	pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	pngXMPKey    = []byte("XML:com.adobe.xmp\x00")
)

func inflate(data []byte, limit int) ([]byte, bool) {
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	defer zr.Close()
	var out bytes.Buffer
	if _, err := io.Copy(&out, io.LimitReader(zr, int64(limit)+1)); err != nil && out.Len() == 0 {
		return nil, false
	}
	if out.Len() > limit || out.Len() == 0 {
		return nil, false
	}
	return out.Bytes(), true
}

func pngICCPayload(data []byte) []byte {
	nul := bytes.IndexByte(data, 0)
	if nul < 1 || nul > 79 || nul+2 > len(data) || data[nul+1] != 0 {
		return nil
	}
	out, ok := inflate(data[nul+2:], MaxICCBytes)
	if !ok {
		return nil
	}
	return out
}

func pngXMPPayload(data []byte) []byte {
	if !bytes.HasPrefix(data, pngXMPKey) {
		return nil
	}
	rest := data[len(pngXMPKey):]
	if len(rest) < 2 {
		return nil
	}
	compressed, method := rest[0], rest[1]
	rest = rest[2:]
	for i := 0; i < 2; i++ {
		nul := bytes.IndexByte(rest, 0)
		if nul < 0 {
			return nil
		}
		rest = rest[nul+1:]
	}
	switch {
	case compressed == 0:
		if len(rest) > MaxXMPBytes {
			return nil
		}
		return rest
	case compressed == 1 && method == 0:
		out, ok := inflate(rest, MaxXMPBytes)
		if !ok {
			return nil
		}
		return out
	}
	return nil
}

func scanPNG(ctx context.Context, in input, b *blocks) error {
	sig, err := in.read(0, len(pngSignature))
	if err != nil {
		return err
	}
	if !bytes.Equal(sig, pngSignature) {
		return corrupt("missing PNG signature")
	}
	var (
		scanErr  error
		haveExif bool
		haveICC  bool
		haveXMP  bool
		chunks   int
	)
	pos := int64(len(pngSignature))
walk:
	for ; chunks < maxContainerOps; chunks++ {
		if chunks%1024 == 0 {
			if err := ctx.Err(); err != nil {
				scanErr = err
				break
			}
		}
		head, err := in.read(pos, 8)
		if err != nil {
			scanErr = corrupt("PNG ends at %d without IEND", pos)
			break
		}
		length := int64(binary.BigEndian.Uint32(head[:4]))
		kind := string(head[4:8])
		if length > 1<<31-1 || pos+8+length+4 > in.size {
			scanErr = corrupt("PNG chunk %q at %d exceeds file", kind, pos)
			break
		}
		dataStart := pos + 8
		switch kind {
		case "IEND":
			break walk
		case "IHDR":
			if length >= 8 {
				dims, err := in.read(dataStart, 8)
				if err != nil {
					scanErr = err
					break walk
				}
				b.setSize(int64(binary.BigEndian.Uint32(dims[:4])), int64(binary.BigEndian.Uint32(dims[4:])))
			}
		case "eXIf":
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
		case "iCCP":
			if !haveICC && length > 0 && length <= MaxICCBytes {
				data, err := in.read(dataStart, int(length))
				if err != nil {
					scanErr = err
					break walk
				}
				if icc := pngICCPayload(data); icc != nil {
					haveICC = true
					b.icc = icc
				}
			}
		case "iTXt":
			if !haveXMP && length > int64(len(pngXMPKey)) && length <= MaxXMPBytes+1024 {
				prefix, err := in.read(dataStart, len(pngXMPKey))
				if err != nil {
					scanErr = err
					break walk
				}
				if bytes.Equal(prefix, pngXMPKey) {
					data, err := in.read(dataStart, int(length))
					if err != nil {
						scanErr = err
						break walk
					}
					if xmp := pngXMPPayload(data); xmp != nil {
						haveXMP = true
						b.xmp = xmp
					}
				}
			}
		}
		pos = dataStart + length + 4
	}
	if scanErr == nil && chunks >= maxContainerOps {
		scanErr = corrupt("too many PNG chunks")
	}
	return scanErr
}
