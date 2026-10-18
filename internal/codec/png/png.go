package png

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	stdpng "image/png"
	"io"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/codec/internal/splice"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	signatureSize        = 8
	chunkHeaderSize      = 8
	chunkTrailerSize     = 4
	headerChunkSize      = 13
	animationChunkSize   = 8
	maxChunksBeforeData  = 100_000
	maxChunksAfterData   = 4096
	contextCheckInterval = 256
	maxChunkLength       = 1<<31 - 1
)

var signature = []byte("\x89PNG\r\n\x1a\n")

type Decoder struct{}

func (Decoder) ID() codec.ID { return codec.PNG }

func (Decoder) Name() string { return "PNG" }

func (Decoder) Extensions() []string { return []string{".png"} }

func (Decoder) Sniff(head []byte) bool {
	return len(head) >= signatureSize && bytes.Equal(head[:signatureSize], signature)
}

func isUnsupported(err error) bool {
	var unsupported stdpng.UnsupportedError
	return errors.As(err, &unsupported)
}

type header struct {
	width       uint32
	height      uint32
	bitDepth    uint8
	colorType   uint8
	interlace   uint8
	transparent bool
	frames      uint32
	orientation codec.Orientation
}

func (h header) bitsPerPixel() int {
	depth := int(h.bitDepth)
	switch h.colorType {
	case 2:
		return 3 * depth
	case 4:
		return 2 * depth
	case 6:
		return 4 * depth
	}
	return depth
}

func (h header) hasAlpha() bool {
	return h.colorType == 4 || h.colorType == 6 || h.transparent
}

func validHeader(bitDepth, colorType, compression, filter, interlace uint8) bool {
	if compression != 0 || filter != 0 || interlace > 1 {
		return false
	}
	switch colorType {
	case 0:
		return bitDepth == 1 || bitDepth == 2 || bitDepth == 4 || bitDepth == 8 || bitDepth == 16
	case 2, 4, 6:
		return bitDepth == 8 || bitDepth == 16
	case 3:
		return bitDepth == 1 || bitDepth == 2 || bitDepth == 4 || bitDepth == 8
	}
	return false
}

func walkChunks(ctx context.Context, r io.ReaderAt, size int64) (header, error) {
	var h header
	h.orientation = codec.OrientNormal
	h.frames = 1
	if size < signatureSize+chunkHeaderSize+headerChunkSize+chunkTrailerSize {
		return h, guard.Corrupt("png: %d bytes is too short", size)
	}
	var lead [signatureSize + chunkHeaderSize + headerChunkSize]byte
	if err := guard.ReadAtInto(r, lead[:], 0, size); err != nil {
		return h, err
	}
	if !bytes.Equal(lead[:signatureSize], signature) {
		return h, guard.Corrupt("png: bad signature")
	}
	if binary.BigEndian.Uint32(lead[8:12]) != headerChunkSize || string(lead[12:16]) != "IHDR" {
		return h, guard.Corrupt("png: IHDR must come first")
	}
	fields := lead[16:]
	h.width = binary.BigEndian.Uint32(fields[0:4])
	h.height = binary.BigEndian.Uint32(fields[4:8])
	h.bitDepth, h.colorType, h.interlace = fields[8], fields[9], fields[12]
	if h.width == 0 || h.height == 0 || h.width > maxChunkLength || h.height > maxChunkLength {
		return h, guard.Corrupt("png: dimensions %dx%d", h.width, h.height)
	}
	if !validHeader(h.bitDepth, h.colorType, fields[10], fields[11], h.interlace) {
		return h, guard.Corrupt("png: invalid IHDR (depth %d, colour type %d)", h.bitDepth, h.colorType)
	}

	position := int64(len(lead) + chunkTrailerSize)
	seenData := false
	budget := maxChunksBeforeData
	for chunks := 0; chunks < budget; chunks++ {
		if chunks%contextCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return h, err
			}
		}
		var chunkHead [chunkHeaderSize]byte
		if position+chunkHeaderSize > size {
			break
		}
		if err := guard.ReadAtInto(r, chunkHead[:], position, size); err != nil {
			return h, err
		}
		length := int64(binary.BigEndian.Uint32(chunkHead[0:4]))
		kind := string(chunkHead[4:8])
		dataStart := position + chunkHeaderSize
		if length > maxChunkLength || dataStart+length+chunkTrailerSize > size {
			break
		}
		switch kind {
		case "IDAT":
			if !seenData {
				seenData = true
				budget = chunks + 1 + maxChunksAfterData
			}
		case "IEND":
			return h, nil
		case "tRNS":
			if !seenData {
				h.transparent = true
			}
		case "acTL":
			if !seenData && length == animationChunkSize {
				var fields [animationChunkSize]byte
				if err := guard.ReadAtInto(r, fields[:], dataStart, size); err != nil {
					return h, err
				}
				if frames := binary.BigEndian.Uint32(fields[0:4]); frames >= 1 {
					h.frames = frames
				}
			}
		case "eXIf":
			if h.orientation == codec.OrientNormal {
				if orientation, ok := exif.OrientationAt(r, dataStart, length); ok {
					h.orientation = orientation
				}
			}
		}
		position = dataStart + length + chunkTrailerSize
	}
	return h, nil
}

func (Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	lim = guard.Limits(lim)
	if err := guard.CheckSize(size, lim); err != nil {
		return codec.Info{}, err
	}
	if err := ctx.Err(); err != nil {
		return codec.Info{}, err
	}
	h, err := walkChunks(ctx, r, size)
	if err != nil {
		return codec.Info{}, err
	}
	if err := lim.Check(int(h.width), int(h.height)); err != nil {
		return codec.Info{}, err
	}
	return codec.Info{
		Format:      codec.PNG,
		FormatName:  "PNG",
		Width:       int(h.width),
		Height:      int(h.height),
		Frames:      int(h.frames),
		Animated:    h.frames > 1,
		HasAlpha:    h.hasAlpha(),
		Orientation: h.orientation,
		BitDepth:    h.bitsPerPixel(),
	}, nil
}

func (d Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	info, err := d.Probe(ctx, r, size, o.Limits)
	if err != nil {
		return nil, err
	}
	decoded, err := stdpng.Decode(guard.Stream(ctx, r, size))
	if err != nil {
		return nil, guard.Classify(ctx, err, isUnsupported)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pix := convert.NRGBA(decoded)
	return &codec.Image{Info: info, Frames: []codec.Frame{{Pix: pix}}, Scale: 1}, nil
}

type Encoder struct{}

func (Encoder) ID() codec.ID { return codec.PNG }

func (Encoder) Name() string { return "PNG" }

func (Encoder) Extension() string { return ".png" }

func (Encoder) Encode(w io.Writer, img *image.NRGBA, o codec.EncodeOptions) error {
	if img == nil || img.Bounds().Empty() {
		return errors.New("png: nothing to encode")
	}
	encoder := stdpng.Encoder{CompressionLevel: stdpng.DefaultCompression}
	out := splice.New(w, metadataOffset, metadataChunks(o.Metadata))
	if err := encoder.Encode(out, img); err != nil {
		return err
	}
	return out.Flush()
}
