package png

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta"
)

const (
	metadataOffset = signatureSize + chunkHeaderSize + headerChunkSize + chunkTrailerSize
	iccName        = "ICC Profile"
	xmpKeyword     = "XML:com.adobe.xmp"
)

func appendChunk(dst []byte, kind string, data []byte) []byte {
	dst = binary.BigEndian.AppendUint32(dst, uint32(len(data)))
	start := len(dst)
	dst = append(dst, kind...)
	dst = append(dst, data...)
	return binary.BigEndian.AppendUint32(dst, crc32.ChecksumIEEE(dst[start:]))
}

func deflate(data []byte) []byte {
	var buf bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	zw.Write(data)
	zw.Close()
	return buf.Bytes()
}

func metadataChunks(m codec.Metadata) []byte {
	m, _ = meta.Fit(m, codec.PNG)
	var out []byte
	if len(m.ICC) > 0 {
		data := append([]byte(iccName), 0, 0)
		out = appendChunk(out, "iCCP", append(data, deflate(m.ICC)...))
	}
	if len(m.EXIF) > 0 {
		out = appendChunk(out, "eXIf", m.EXIF)
	}
	if len(m.XMP) > 0 {
		data := append([]byte(xmpKeyword), 0, 0, 0, 0, 0)
		out = appendChunk(out, "iTXt", append(data, m.XMP...))
	}
	return out
}
