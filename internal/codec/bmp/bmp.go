package bmp

import (
	"bufio"
	"context"
	"encoding/binary"
	"image"
	"io"
	"math/bits"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/meta"
)

const (
	fileHeaderSize   = 14
	coreHeaderSize   = 12
	infoHeaderSize   = 40
	os2HeaderSize    = 64
	maxHeaderSize    = 124
	maxPaletteColors = 256
	bandTargetBytes  = 4 << 20
	rleBufferSize    = 64 << 10
	rleCheckInterval = 1 << 16
	maxRowBytes      = 1 << 31
)

const (
	compressionRGB            = 0
	compressionRLE8           = 1
	compressionRLE4           = 2
	compressionBitfields      = 3
	compressionJPEG           = 4
	compressionPNG            = 5
	compressionAlphaBitfields = 6
)

type Decoder struct{}

func (Decoder) ID() codec.ID { return codec.BMP }

func (Decoder) Name() string { return "BMP" }

func (Decoder) Extensions() []string { return []string{".bmp", ".dib"} }

func knownHeaderSize(n uint32) bool {
	switch n {
	case coreHeaderSize, infoHeaderSize, 52, 56, os2HeaderSize, 108, maxHeaderSize:
		return true
	}
	return false
}

func (Decoder) Sniff(head []byte) bool {
	if len(head) < fileHeaderSize+4 || head[0] != 'B' || head[1] != 'M' {
		return false
	}
	return knownHeaderSize(binary.LittleEndian.Uint32(head[fileHeaderSize:]))
}

type channel struct {
	mask  uint32
	shift uint
	bits  uint
	table [256]uint8
}

func newChannel(mask uint32) (channel, error) {
	var c channel
	if mask == 0 {
		return c, nil
	}
	c.mask = mask
	c.shift = uint(bits.TrailingZeros32(mask))
	shifted := mask >> c.shift
	if shifted&(shifted+1) != 0 {
		return c, guard.Unsupported("bmp: non-contiguous colour mask %#x", mask)
	}
	c.bits = uint(bits.Len32(shifted))
	if c.bits <= 8 {
		largest := uint32(1)<<c.bits - 1
		for v := uint32(0); v <= largest; v++ {
			c.table[v] = uint8((v*255 + largest/2) / largest)
		}
	}
	return c, nil
}

func (c *channel) value(pixel uint32) uint8 {
	v := (pixel & c.mask) >> c.shift
	if c.bits <= 8 {
		return c.table[v]
	}
	return uint8(v >> (c.bits - 8))
}

type header struct {
	width         int
	height        int
	topDown       bool
	bitCount      int
	compression   uint32
	masks         [4]uint32
	alphaCapable  bool
	explicitAlpha bool
	palette       [maxPaletteColors][4]uint8
	paletteLen    int
	dataOffset    int64
	color         codec.ColorInfo
}

func (h *header) indexed() bool { return h.bitCount <= 8 }

func (h *header) info() codec.Info {
	return codec.Info{
		Format:      codec.BMP,
		FormatName:  "BMP",
		Width:       h.width,
		Height:      h.height,
		Frames:      1,
		HasAlpha:    h.explicitAlpha,
		Orientation: codec.OrientNormal,
		BitDepth:    h.bitCount,
	}
}

func (h *header) rowBytes() int64 {
	return (int64(h.width)*int64(h.bitCount) + 7) / 8
}

func (h *header) stride() int64 {
	return (int64(h.width)*int64(h.bitCount) + 31) / 32 * 4
}

func readHeader(r io.ReaderAt, size int64, lim codec.Limits) (*header, error) {
	if size < fileHeaderSize+coreHeaderSize {
		return nil, guard.Corrupt("bmp: %d bytes is too short", size)
	}
	var fixed [fileHeaderSize + 4]byte
	if err := guard.ReadAtInto(r, fixed[:], 0, size); err != nil {
		return nil, err
	}
	if fixed[0] != 'B' || fixed[1] != 'M' {
		return nil, guard.Corrupt("bmp: bad signature")
	}
	dataOffset := int64(binary.LittleEndian.Uint32(fixed[10:14]))
	dibSize := binary.LittleEndian.Uint32(fixed[14:18])
	if dibSize < coreHeaderSize {
		return nil, guard.Corrupt("bmp: header of %d bytes", dibSize)
	}
	if !knownHeaderSize(dibSize) {
		return nil, guard.Unsupported("bmp: header of %d bytes", dibSize)
	}
	dib, err := guard.ReadAt(r, fileHeaderSize, int64(dibSize), size)
	if err != nil {
		return nil, err
	}

	h := &header{}
	h.color = embeddedProfileColor(r, size, dib)
	core := dibSize == coreHeaderSize
	var widthRaw, heightRaw int64
	var colorsUsed uint32
	if core {
		widthRaw = int64(binary.LittleEndian.Uint16(dib[4:6]))
		heightRaw = int64(binary.LittleEndian.Uint16(dib[6:8]))
		h.bitCount = int(binary.LittleEndian.Uint16(dib[10:12]))
		h.compression = compressionRGB
	} else {
		widthRaw = int64(int32(binary.LittleEndian.Uint32(dib[4:8])))
		heightRaw = int64(int32(binary.LittleEndian.Uint32(dib[8:12])))
		h.bitCount = int(binary.LittleEndian.Uint16(dib[14:16]))
		h.compression = binary.LittleEndian.Uint32(dib[16:20])
		colorsUsed = binary.LittleEndian.Uint32(dib[32:36])
	}
	if widthRaw <= 0 || heightRaw == 0 {
		return nil, guard.Corrupt("bmp: dimensions %dx%d", widthRaw, heightRaw)
	}
	h.topDown = heightRaw < 0
	if heightRaw < 0 {
		heightRaw = -heightRaw
	}
	if widthRaw > 1<<31-1 || heightRaw > 1<<31 {
		return nil, guard.TooLarge("bmp: dimensions %dx%d", widthRaw, heightRaw)
	}
	h.width, h.height = int(widthRaw), int(heightRaw)
	if err := lim.Check(h.width, h.height); err != nil {
		return nil, err
	}

	switch h.compression {
	case compressionRGB:
		switch h.bitCount {
		case 1, 4, 8, 16, 24, 32:
		default:
			return nil, guard.Unsupported("bmp: %d bits per pixel", h.bitCount)
		}
	case compressionRLE8:
		if h.bitCount != 8 {
			return nil, guard.Corrupt("bmp: RLE8 with %d bits per pixel", h.bitCount)
		}
	case compressionRLE4:
		if h.bitCount != 4 {
			return nil, guard.Corrupt("bmp: RLE4 with %d bits per pixel", h.bitCount)
		}
	case compressionBitfields, compressionAlphaBitfields:
		if dibSize == os2HeaderSize || (h.bitCount != 16 && h.bitCount != 32) {
			return nil, guard.Unsupported("bmp: compression %d with %d bits per pixel", h.compression, h.bitCount)
		}
	case compressionJPEG, compressionPNG:
		return nil, guard.Unsupported("bmp: embedded JPEG or PNG data")
	default:
		return nil, guard.Unsupported("bmp: compression %d", h.compression)
	}

	maskBytes := int64(0)
	switch h.compression {
	case compressionBitfields, compressionAlphaBitfields:
		wantAlpha := h.compression == compressionAlphaBitfields
		switch {
		case dibSize >= 56:
			for i := 0; i < 4; i++ {
				h.masks[i] = binary.LittleEndian.Uint32(dib[40+4*i:])
			}
		case dibSize == 52:
			for i := 0; i < 3; i++ {
				h.masks[i] = binary.LittleEndian.Uint32(dib[40+4*i:])
			}
			if wantAlpha {
				return nil, guard.Corrupt("bmp: alpha bit fields need a 56 byte header")
			}
		default:
			count := 3
			if wantAlpha {
				count = 4
			}
			maskBytes = int64(count) * 4
			raw, err := guard.ReadAt(r, fileHeaderSize+int64(dibSize), maskBytes, size)
			if err != nil {
				return nil, err
			}
			for i := 0; i < count; i++ {
				h.masks[i] = binary.LittleEndian.Uint32(raw[4*i:])
			}
		}
		if h.masks[0]|h.masks[1]|h.masks[2] == 0 {
			return nil, guard.Corrupt("bmp: empty colour masks")
		}
		if h.bitCount == 16 {
			for _, m := range h.masks {
				if m > 0xffff {
					return nil, guard.Unsupported("bmp: mask %#x wider than 16 bits", m)
				}
			}
		}
		h.explicitAlpha = h.masks[3] != 0
		h.alphaCapable = h.explicitAlpha
	case compressionRGB:
		switch h.bitCount {
		case 16:
			h.masks = [4]uint32{0x7c00, 0x03e0, 0x001f, 0}
		case 32:
			h.masks = [4]uint32{0x00ff0000, 0x0000ff00, 0x000000ff, 0xff000000}
			h.alphaCapable = true
		}
	}

	headerEnd := fileHeaderSize + int64(dibSize) + maskBytes
	if h.indexed() {
		count := 1 << h.bitCount
		if colorsUsed != 0 && int(colorsUsed) < count && colorsUsed <= maxPaletteColors {
			count = int(colorsUsed)
		}
		entrySize := int64(4)
		if core {
			entrySize = 3
		}
		raw, err := guard.ReadAt(r, headerEnd, int64(count)*entrySize, size)
		if err != nil {
			return nil, err
		}
		for i := range h.palette {
			h.palette[i] = [4]uint8{0, 0, 0, 0xff}
		}
		for i := 0; i < count; i++ {
			e := raw[int64(i)*entrySize:]
			h.palette[i] = [4]uint8{e[2], e[1], e[0], 0xff}
		}
		h.paletteLen = count
		paletteEnd := headerEnd + int64(count)*entrySize
		if dataOffset != 0 && dataOffset < paletteEnd && dataOffset >= headerEnd {
			dataOffset = paletteEnd
		}
		headerEnd = paletteEnd
	}
	if dataOffset == 0 {
		dataOffset = headerEnd
	}
	if dataOffset < fileHeaderSize+int64(dibSize)+maskBytes || dataOffset > size {
		return nil, guard.Corrupt("bmp: pixel data offset %d in a %d byte file", dataOffset, size)
	}
	h.dataOffset = dataOffset
	if h.compression == compressionRLE8 || h.compression == compressionRLE4 {
		return h, nil
	}
	stride := h.stride()
	if stride > maxRowBytes {
		return nil, guard.TooLarge("bmp: rows of %d bytes", stride)
	}
	last := dataOffset + int64(h.height-1)*stride + h.rowBytes()
	if last > size {
		return nil, guard.Corrupt("bmp: pixel data needs %d bytes, file has %d", last, size)
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
	h, err := readHeader(r, size, lim)
	if err != nil {
		return codec.Info{}, err
	}
	return h.info(), nil
}

type rowConverter func(dst, src []byte) uint8

func (h *header) rowConverter() (rowConverter, error) {
	width := h.width
	palette := &h.palette
	switch h.bitCount {
	case 1:
		return func(dst, src []byte) uint8 {
			for x := 0; x < width; x++ {
				copy(dst[x*4:x*4+4], palette[(src[x>>3]>>(7-uint(x&7)))&1][:])
			}
			return 0xff
		}, nil
	case 4:
		return func(dst, src []byte) uint8 {
			for x := 0; x < width; x++ {
				index := src[x>>1]
				if x&1 == 0 {
					index >>= 4
				} else {
					index &= 15
				}
				copy(dst[x*4:x*4+4], palette[index][:])
			}
			return 0xff
		}, nil
	case 8:
		return func(dst, src []byte) uint8 {
			for x := 0; x < width; x++ {
				copy(dst[x*4:x*4+4], palette[src[x]][:])
			}
			return 0xff
		}, nil
	case 24:
		return func(dst, src []byte) uint8 {
			for x := 0; x < width; x++ {
				dst[x*4] = src[x*3+2]
				dst[x*4+1] = src[x*3+1]
				dst[x*4+2] = src[x*3]
				dst[x*4+3] = 0xff
			}
			return 0xff
		}, nil
	}
	red, err := newChannel(h.masks[0])
	if err != nil {
		return nil, err
	}
	green, err := newChannel(h.masks[1])
	if err != nil {
		return nil, err
	}
	blue, err := newChannel(h.masks[2])
	if err != nil {
		return nil, err
	}
	alpha, err := newChannel(h.masks[3])
	if err != nil {
		return nil, err
	}
	hasAlpha := h.masks[3] != 0
	if h.bitCount == 16 {
		return func(dst, src []byte) uint8 {
			var seen uint8
			for x := 0; x < width; x++ {
				pixel := uint32(src[x*2]) | uint32(src[x*2+1])<<8
				a := uint8(0xff)
				if hasAlpha {
					a = alpha.value(pixel)
				}
				dst[x*4] = red.value(pixel)
				dst[x*4+1] = green.value(pixel)
				dst[x*4+2] = blue.value(pixel)
				dst[x*4+3] = a
				seen |= a
			}
			return seen
		}, nil
	}
	if h.masks == [4]uint32{0x00ff0000, 0x0000ff00, 0x000000ff, 0xff000000} {
		return func(dst, src []byte) uint8 {
			var seen uint8
			for x := 0; x < width; x++ {
				dst[x*4] = src[x*4+2]
				dst[x*4+1] = src[x*4+1]
				dst[x*4+2] = src[x*4]
				dst[x*4+3] = src[x*4+3]
				seen |= src[x*4+3]
			}
			return seen
		}, nil
	}
	return func(dst, src []byte) uint8 {
		var seen uint8
		for x := 0; x < width; x++ {
			pixel := binary.LittleEndian.Uint32(src[x*4:])
			a := uint8(0xff)
			if hasAlpha {
				a = alpha.value(pixel)
			}
			dst[x*4] = red.value(pixel)
			dst[x*4+1] = green.value(pixel)
			dst[x*4+2] = blue.value(pixel)
			dst[x*4+3] = a
			seen |= a
		}
		return seen
	}, nil
}

func (h *header) imageRow(stored int) int {
	if h.topDown {
		return stored
	}
	return h.height - 1 - stored
}

func decodeRows(ctx context.Context, r io.ReaderAt, size int64, h *header, dst *image.NRGBA) (uint8, error) {
	convertRow, err := h.rowConverter()
	if err != nil {
		return 0, err
	}
	rowBytes, stride := int(h.rowBytes()), int(h.stride())
	bandRows := bandTargetBytes / stride
	if bandRows < 1 {
		bandRows = 1
	}
	if bandRows > h.height {
		bandRows = h.height
	}
	buf := make([]byte, (bandRows-1)*stride+rowBytes)
	var alphaSeen atomic.Uint32
	for first := 0; first < h.height; first += bandRows {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		rows := bandRows
		if first+rows > h.height {
			rows = h.height - first
		}
		length := (rows-1)*stride + rowBytes
		if err := guard.ReadAtInto(r, buf[:length], h.dataOffset+int64(first)*int64(stride), size); err != nil {
			return 0, err
		}
		convert.Bands(h.width, rows, func(y0, y1 int) {
			var seen uint8
			for i := y0; i < y1; i++ {
				row := h.imageRow(first + i)
				seen |= convertRow(dst.Pix[row*dst.Stride:row*dst.Stride+h.width*4], buf[i*stride:i*stride+rowBytes])
			}
			alphaSeen.Or(uint32(seen))
		})
	}
	return uint8(alphaSeen.Load()), nil
}

func decodeRLE(ctx context.Context, r io.ReaderAt, size int64, h *header, dst *image.NRGBA) error {
	indices := make([]byte, h.width*h.height)
	stream := bufio.NewReaderSize(io.NewSectionReader(r, h.dataOffset, size-h.dataOffset), rleBufferSize)
	nibbles := h.compression == compressionRLE4
	x, y := 0, 0
	put := func(index byte) {
		if x < h.width && y < h.height {
			indices[y*h.width+x] = index
		}
		x++
	}
	complete := func() bool {
		return y >= h.height || (y == h.height-1 && x >= h.width)
	}
	for ops := 0; ; ops++ {
		if ops%rleCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		count, err := stream.ReadByte()
		if err != nil {
			if complete() {
				break
			}
			return guard.Corrupt("bmp: RLE data ends inside the image")
		}
		value, err := stream.ReadByte()
		if err != nil {
			return guard.Corrupt("bmp: RLE data ends inside a command")
		}
		if count > 0 {
			for i := 0; i < int(count); i++ {
				if nibbles {
					if i&1 == 0 {
						put(value >> 4)
					} else {
						put(value & 15)
					}
				} else {
					put(value)
				}
			}
			continue
		}
		switch value {
		case 0:
			x = 0
			y++
		case 1:
			goto done
		case 2:
			dx, err1 := stream.ReadByte()
			dy, err2 := stream.ReadByte()
			if err1 != nil || err2 != nil {
				return guard.Corrupt("bmp: RLE delta cut off")
			}
			x += int(dx)
			y += int(dy)
		default:
			pixels := int(value)
			dataBytes := pixels
			if nibbles {
				dataBytes = (pixels + 1) / 2
			}
			for i := 0; i < dataBytes; i++ {
				b, err := stream.ReadByte()
				if err != nil {
					return guard.Corrupt("bmp: RLE literal run cut off")
				}
				if nibbles {
					put(b >> 4)
					if 2*i+1 < pixels {
						put(b & 15)
					}
				} else {
					put(b)
				}
			}
			if dataBytes&1 == 1 {
				if _, err := stream.ReadByte(); err != nil {
					if !complete() {
						return guard.Corrupt("bmp: RLE literal padding missing")
					}
				}
			}
		}
	}
done:
	for stored := 0; stored < h.height; stored++ {
		row := h.imageRow(stored)
		out := dst.Pix[row*dst.Stride : row*dst.Stride+h.width*4]
		in := indices[stored*h.width : (stored+1)*h.width]
		for i, index := range in {
			copy(out[i*4:i*4+4], h.palette[index][:])
		}
	}
	return nil
}

func (Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	lim := guard.Limits(o.Limits)
	if err := guard.CheckSize(size, lim); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h, err := readHeader(r, size, lim)
	if err != nil {
		return nil, err
	}
	dst := image.NewNRGBA(image.Rect(0, 0, h.width, h.height))
	info := h.info()
	if h.compression == compressionRLE8 || h.compression == compressionRLE4 {
		if err := decodeRLE(ctx, r, size, h, dst); err != nil {
			return nil, guard.Classify(ctx, err, nil)
		}
	} else {
		alphaSeen, err := decodeRows(ctx, r, size, h, dst)
		if err != nil {
			return nil, guard.Classify(ctx, err, nil)
		}
		if h.alphaCapable {
			if alphaSeen == 0 {
				convert.SetOpaque(dst)
			}
			info.HasAlpha = alphaSeen != 0
		}
	}
	return &codec.Image{Info: info, Color: h.color, Frames: []codec.Frame{{Pix: dst}}, Scale: 1}, nil
}

const (
	v5HeaderSize    = 124
	v5ColorSpace    = 56
	v5ProfileData   = 112
	v5ProfileSize   = 116
	profileEmbedded = 0x4d424544
)

func embeddedProfileColor(r io.ReaderAt, size int64, dib []byte) codec.ColorInfo {
	if len(dib) < v5HeaderSize || binary.LittleEndian.Uint32(dib[v5ColorSpace:]) != profileEmbedded {
		return codec.ColorInfo{}
	}
	offset := int64(binary.LittleEndian.Uint32(dib[v5ProfileData:]))
	length := int64(binary.LittleEndian.Uint32(dib[v5ProfileSize:]))
	if length <= 0 || length > meta.MaxICCBytes {
		return codec.ColorInfo{}
	}
	data, err := guard.ReadAt(r, fileHeaderSize+offset, length, size)
	if err != nil {
		return codec.ColorInfo{}
	}
	return meta.NewColor(data, nil)
}
