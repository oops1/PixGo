package app

import (
	"bytes"

	"github.com/oops1/PixGo/internal/codec"
)

func sameOptions(a, b codec.EncodeOptions) bool {
	return a.Quality == b.Quality && a.Lossless == b.Lossless &&
		bytes.Equal(a.Metadata.EXIF, b.Metadata.EXIF) &&
		bytes.Equal(a.Metadata.ICC, b.Metadata.ICC) &&
		bytes.Equal(a.Metadata.XMP, b.Metadata.XMP)
}
