package webp

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"

	xwebp "golang.org/x/image/webp"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	riffHeaderSize       = 12
	chunkHeaderSize      = 8
	maxChunks            = 1 << 20
	contextCheckInterval = 256
	flagAnimation        = 0x02
	flagAlpha            = 0x10
)

type Decoder struct{}

func (Decoder) ID() codec.ID { return codec.WebP }

func (Decoder) Name() string { return "WebP" }

func (Decoder) Extensions() []string { return []string{".webp"} }

func (Decoder) Sniff(head []byte) bool {
	return len(head) >= riffHeaderSize && bytes.Equal(head[:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP"))
}

type layout struct {
	width       int
	height      int
	hasAlpha    bool
	animated    bool
	frames      int
	orientation codec.Orientation
}

func readUint24(b []byte) int {
	return int(b[0]) | int(b[1])<<8 | int(b[2])<<16
}

func walkChunks(ctx context.Context, r io.ReaderAt, size int64) (layout, error) {
	result := layout{orientation: codec.OrientNormal}
	var riff [riffHeaderSize]byte
	if size < riffHeaderSize {
		return result, guard.Corrupt("webp: %d bytes is too short", size)
	}
	if err := guard.ReadAtInto(r, riff[:], 0, size); err != nil {
		return result, err
	}
	if string(riff[:4]) != "RIFF" || string(riff[8:12]) != "WEBP" {
		return result, guard.Corrupt("webp: not a RIFF WEBP container")
	}
	declared := int64(binary.LittleEndian.Uint32(riff[4:8]))
	if declared < 4 {
		return result, guard.Corrupt("webp: RIFF size %d", declared)
	}
	end := size
	if declared+8 < end {
		end = declared + 8
	}

	var canvasWidth, canvasHeight int
	haveCanvas, haveBitstream := false, false
	position := int64(riffHeaderSize)
	for chunks := 0; chunks < maxChunks; chunks++ {
		if chunks%contextCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return result, err
			}
		}
		if position+chunkHeaderSize > end {
			break
		}
		var head [chunkHeaderSize]byte
		if err := guard.ReadAtInto(r, head[:], position, size); err != nil {
			return result, err
		}
		length := int64(binary.LittleEndian.Uint32(head[4:8]))
		payload := position + chunkHeaderSize
		next := payload + length + length&1
		if payload+length > end {
			break
		}
		switch string(head[:4]) {
		case "VP8X":
			if length < 10 {
				return result, guard.Corrupt("webp: VP8X chunk of %d bytes", length)
			}
			var body [10]byte
			if err := guard.ReadAtInto(r, body[:], payload, size); err != nil {
				return result, err
			}
			result.animated = body[0]&flagAnimation != 0
			result.hasAlpha = result.hasAlpha || body[0]&flagAlpha != 0
			canvasWidth = readUint24(body[4:7]) + 1
			canvasHeight = readUint24(body[7:10]) + 1
			haveCanvas = true
		case "VP8 ":
			if haveBitstream {
				break
			}
			if length < 10 {
				return result, guard.Corrupt("webp: VP8 chunk of %d bytes", length)
			}
			var body [10]byte
			if err := guard.ReadAtInto(r, body[:], payload, size); err != nil {
				return result, err
			}
			if body[0]&1 != 0 || body[3] != 0x9d || body[4] != 0x01 || body[5] != 0x2a {
				return result, guard.Corrupt("webp: bad VP8 key frame header")
			}
			width := int(binary.LittleEndian.Uint16(body[6:8]) & 0x3fff)
			height := int(binary.LittleEndian.Uint16(body[8:10]) & 0x3fff)
			if err := adoptBitstream(&result, width, height, haveCanvas, canvasWidth, canvasHeight); err != nil {
				return result, err
			}
			haveBitstream = true
		case "VP8L":
			if haveBitstream {
				break
			}
			if length < 5 {
				return result, guard.Corrupt("webp: VP8L chunk of %d bytes", length)
			}
			var body [5]byte
			if err := guard.ReadAtInto(r, body[:], payload, size); err != nil {
				return result, err
			}
			if body[0] != 0x2f {
				return result, guard.Corrupt("webp: bad VP8L signature")
			}
			bitsWord := binary.LittleEndian.Uint32(body[1:5])
			width := int(bitsWord&0x3fff) + 1
			height := int(bitsWord>>14&0x3fff) + 1
			if bitsWord>>28&1 != 0 {
				result.hasAlpha = true
			}
			if err := adoptBitstream(&result, width, height, haveCanvas, canvasWidth, canvasHeight); err != nil {
				return result, err
			}
			haveBitstream = true
		case "ALPH":
			result.hasAlpha = true
		case "ANMF":
			result.frames++
		case "EXIF":
			if result.orientation == codec.OrientNormal && length >= 8 {
				base := payload
				window := length
				var prefix [6]byte
				if length >= 6 {
					if err := guard.ReadAtInto(r, prefix[:], payload, size); err == nil && bytes.Equal(prefix[:6], []byte("Exif\x00\x00")) {
						base += 6
						window -= 6
					}
				}
				if orientation, ok := exif.OrientationAt(r, base, window); ok {
					result.orientation = orientation
				}
			}
		}
		position = next
	}
	if haveCanvas {
		result.width, result.height = canvasWidth, canvasHeight
	}
	if result.width <= 0 || result.height <= 0 {
		return result, guard.Corrupt("webp: no image dimensions")
	}
	if result.animated {
		if result.frames < 1 {
			result.frames = 1
		}
	} else {
		result.frames = 1
	}
	return result, nil
}

func adoptBitstream(result *layout, width, height int, haveCanvas bool, canvasWidth, canvasHeight int) error {
	if width <= 0 || height <= 0 {
		return guard.Corrupt("webp: bitstream dimensions %dx%d", width, height)
	}
	if haveCanvas && !result.animated && (width != canvasWidth || height != canvasHeight) {
		return guard.Corrupt("webp: bitstream %dx%d does not match canvas %dx%d", width, height, canvasWidth, canvasHeight)
	}
	result.width, result.height = width, height
	return nil
}

func (Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	lim = guard.Limits(lim)
	if err := guard.CheckSize(size, lim); err != nil {
		return codec.Info{}, err
	}
	if err := ctx.Err(); err != nil {
		return codec.Info{}, err
	}
	found, err := walkChunks(ctx, r, size)
	if err != nil {
		return codec.Info{}, err
	}
	if err := lim.Check(found.width, found.height); err != nil {
		return codec.Info{}, err
	}
	depth := 24
	if found.hasAlpha {
		depth = 32
	}
	return codec.Info{
		Format:      codec.WebP,
		FormatName:  "WebP",
		Width:       found.width,
		Height:      found.height,
		Frames:      found.frames,
		Animated:    found.animated,
		HasAlpha:    found.hasAlpha,
		Orientation: found.orientation,
		BitDepth:    depth,
	}, nil
}

func (d Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	info, err := d.Probe(ctx, r, size, o.Limits)
	if err != nil {
		return nil, err
	}
	if info.Animated {
		return nil, guard.Unsupported("webp: animated images are not supported yet")
	}
	decoded, err := xwebp.Decode(guard.Stream(ctx, r, size))
	if err != nil {
		return nil, guard.Classify(ctx, err, nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pix := convert.NRGBA(decoded)
	return &codec.Image{Info: info, Frames: []codec.Frame{{Pix: pix}}, Scale: 1}, nil
}
