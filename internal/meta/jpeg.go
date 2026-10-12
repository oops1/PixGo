package meta

import (
	"bytes"
	"context"
)

var (
	jpegXMPPrefix = []byte("http://ns.adobe.com/xap/1.0/\x00")
	jpegICCPrefix = []byte("ICC_PROFILE\x00")
)

type iccChunk struct {
	seq, count int
	offset     int64
	length     int
}

func isSOF(marker byte) bool {
	return marker >= 0xc0 && marker <= 0xcf && marker != 0xc4 && marker != 0xc8 && marker != 0xcc
}

func scanJPEG(ctx context.Context, in input, b *blocks) error {
	head, err := in.read(0, 2)
	if err != nil {
		return err
	}
	if head[0] != 0xff || head[1] != 0xd8 {
		return corrupt("missing JPEG SOI")
	}
	var (
		scanErr  error
		chunks   []iccChunk
		total    int
		iccBad   bool
		haveExif bool
		haveXMP  bool
		step     int
	)
	pos := int64(2)
walk:
	for ; step < maxJPEGSegments; step++ {
		if step%ctxCheckEvery == 0 {
			if err := ctx.Err(); err != nil {
				scanErr = err
				break
			}
		}
		marker, err := in.read(pos, 2)
		if err != nil {
			scanErr = corrupt("JPEG ends at %d without EOI", pos)
			break
		}
		if marker[0] != 0xff {
			scanErr = corrupt("expected JPEG marker at %d", pos)
			break
		}
		code := marker[1]
		switch {
		case code == 0xff:
			pos++
			continue
		case code == 0x00:
			scanErr = corrupt("stuffed byte outside scan at %d", pos)
			break walk
		case code == 0xd9 || code == 0xda:
			break walk
		case code == 0x01 || code == 0xd8 || (code >= 0xd0 && code <= 0xd7):
			pos += 2
			continue
		}
		lenBytes, err := in.read(pos+2, 2)
		if err != nil {
			scanErr = corrupt("JPEG segment header cut off at %d", pos)
			break
		}
		length := int64(lenBytes[0])<<8 | int64(lenBytes[1])
		if length < 2 {
			scanErr = corrupt("JPEG segment length %d at %d", length, pos)
			break
		}
		end := pos + 2 + length
		if end > in.size {
			scanErr = corrupt("JPEG segment at %d exceeds file", pos)
			break
		}
		dataStart := pos + 4
		dataLen := length - 2
		switch {
		case code == 0xe1:
			prefixLen := dataLen
			if prefixLen > int64(len(jpegXMPPrefix)) {
				prefixLen = int64(len(jpegXMPPrefix))
			}
			prefix, err := in.read(dataStart, int(prefixLen))
			if err != nil {
				scanErr = err
				break walk
			}
			if skip := exifPrefixLen(prefix); skip > 0 {
				if !haveExif && dataLen-skip >= 8 {
					haveExif = true
					b.setTIFF(dataStart+skip, dataLen-skip)
				}
			} else if bytes.HasPrefix(prefix, jpegXMPPrefix) {
				payload := dataLen - int64(len(jpegXMPPrefix))
				if !haveXMP && payload > 0 && payload <= MaxXMPBytes {
					data, err := in.read(dataStart+int64(len(jpegXMPPrefix)), int(payload))
					if err != nil {
						scanErr = err
						break walk
					}
					haveXMP = true
					b.xmp = data
				}
			}
		case code == 0xe2:
			prefixLen := dataLen
			if prefixLen > int64(len(jpegICCPrefix))+2 {
				prefixLen = int64(len(jpegICCPrefix)) + 2
			}
			prefix, err := in.read(dataStart, int(prefixLen))
			if err != nil {
				scanErr = err
				break walk
			}
			if len(prefix) == len(jpegICCPrefix)+2 && bytes.HasPrefix(prefix, jpegICCPrefix) && !iccBad {
				n := int(dataLen) - len(prefix)
				total += n
				if total > MaxICCBytes || len(chunks) >= 1024 {
					iccBad = true
					chunks = nil
				} else {
					chunks = append(chunks, iccChunk{
						seq:    int(prefix[len(jpegICCPrefix)]),
						count:  int(prefix[len(jpegICCPrefix)+1]),
						offset: dataStart + int64(len(prefix)),
						length: n,
					})
				}
			}
		case isSOF(code):
			if dataLen >= 5 {
				sof, err := in.read(dataStart, 5)
				if err != nil {
					scanErr = err
					break walk
				}
				h := int64(sof[1])<<8 | int64(sof[2])
				w := int64(sof[3])<<8 | int64(sof[4])
				b.setSize(w, h)
			}
		}
		pos = end
	}
	if scanErr == nil && step >= maxJPEGSegments {
		scanErr = corrupt("too many JPEG segments")
	}
	if !iccBad && len(chunks) > 0 {
		b.icc = assembleICC(in, chunks)
	}
	return scanErr
}

func assembleICC(in input, chunks []iccChunk) []byte {
	count := chunks[0].count
	if count < 1 || len(chunks) != count {
		return nil
	}
	ordered := make([]iccChunk, count)
	total := 0
	for _, c := range chunks {
		if c.count != count || c.seq < 1 || c.seq > count || ordered[c.seq-1].seq != 0 {
			return nil
		}
		ordered[c.seq-1] = c
		total += c.length
	}
	if total <= 0 || total > MaxICCBytes {
		return nil
	}
	out := make([]byte, 0, total)
	for _, c := range ordered {
		data, err := in.read(c.offset, c.length)
		if err != nil {
			return nil
		}
		out = append(out, data...)
	}
	return out
}
