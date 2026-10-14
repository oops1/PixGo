package raw

import (
	"encoding/binary"
	"io"

	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

const (
	rafSignature    = "FUJIFILMCCD-RAW "
	rafJPEGOffsetAt = 84
	rafHeaderSize   = 92
)

func discoverRAF(r io.ReaderAt, size int64, f *finder) error {
	if size < rafHeaderSize {
		return guard.Corrupt("raw: RAF header is cut off")
	}
	var fields [8]byte
	if err := guard.ReadAtInto(r, fields[:], rafJPEGOffsetAt, size); err != nil {
		return err
	}
	f.add(int64(binary.BigEndian.Uint32(fields[0:4])), int64(binary.BigEndian.Uint32(fields[4:8])), false)
	return nil
}
