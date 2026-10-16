package jpegdec

import "errors"

type FormatError string

func (e FormatError) Error() string { return "invalid JPEG format: " + string(e) }

type UnsupportedError string

func (e UnsupportedError) Error() string { return "unsupported JPEG feature: " + string(e) }

var (
	ErrTooLarge = errors.New("jpegdec: image exceeds the pixel limit")
	ErrClosed   = errors.New("jpegdec: decoder is closed")
)

const (
	errShortData        = FormatError("short Huffman data")
	errBadHuffmanCode   = FormatError("bad Huffman code")
	errUnsupportedRatio = UnsupportedError("luma/chroma subsampling ratio")
)
