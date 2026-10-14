package meta

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/oops1/PixGo/internal/codec"
)

func containerErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, codec.ErrCorrupt) {
		return err
	}
	return corrupt("%v", err)
}

func isoExifTIFF(data []byte) []byte {
	if len(data) < 4+8 {
		return nil
	}
	payload := data[4:]
	offset := uint64(binary.BigEndian.Uint32(data))
	if offset <= uint64(len(payload)) {
		if tiff := tiffAt(payload[offset:]); tiff != nil {
			return tiff
		}
	}
	return tiffAt(payload)
}

func tiffAt(data []byte) []byte {
	data = data[exifPrefixLen(data):]
	if len(data) >= 8 && (string(data[:2]) == "II" || string(data[:2]) == "MM") {
		return data
	}
	return nil
}

func (in input) readBlock(offset, length, limit int64) ([]byte, bool, error) {
	if length <= 0 || length > limit {
		return nil, false, nil
	}
	data, err := in.read(offset, int(length))
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}
