package jpeg

import (
	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta"
)

const (
	soiLength        = 2
	maxSegmentData   = 65533
	iccChunkOverhead = 14
)

var (
	exifHeader = []byte("Exif\x00\x00")
	xmpHeader  = []byte("http://ns.adobe.com/xap/1.0/\x00")
	iccHeader  = []byte("ICC_PROFILE\x00")
)

func appendSegment(dst []byte, marker byte, parts ...[]byte) []byte {
	n := 2
	for _, p := range parts {
		n += len(p)
	}
	dst = append(dst, 0xff, marker, byte(n>>8), byte(n))
	for _, p := range parts {
		dst = append(dst, p...)
	}
	return dst
}

func metadataSegments(m codec.Metadata) []byte {
	m, _ = meta.Fit(m, codec.JPEG)
	var out []byte
	if len(m.EXIF) > 0 {
		out = appendSegment(out, 0xe1, exifHeader, m.EXIF)
	}
	if len(m.XMP) > 0 {
		out = appendSegment(out, 0xe1, xmpHeader, m.XMP)
	}
	if len(m.ICC) > 0 {
		chunk := maxSegmentData - iccChunkOverhead
		count := (len(m.ICC) + chunk - 1) / chunk
		for i := 0; i < count; i++ {
			part := m.ICC[i*chunk : min(len(m.ICC), (i+1)*chunk)]
			out = appendSegment(out, 0xe2, iccHeader, []byte{byte(i + 1), byte(count)}, part)
		}
	}
	return out
}
