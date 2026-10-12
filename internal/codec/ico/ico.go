package ico

import (
	"bytes"
	"context"
	"encoding/binary"
	stdimage "image"
	stdpng "image/png"
	"io"
	"sort"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

const (
	directorySize    = 6
	entrySize        = 16
	maxEntries       = 256
	pngSignatureSize = 8
	dibProbeSize     = 40
	typeIcon         = 1
	typeCursor       = 2
	maxPaletteColors = 256
)

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

type variant struct {
	id        codec.ID
	name      string
	extension string
	iconType  uint16
}

var (
	iconVariant   = variant{id: codec.ICO, name: "ICO", extension: ".ico", iconType: typeIcon}
	cursorVariant = variant{id: codec.CUR, name: "CUR", extension: ".cur", iconType: typeCursor}
)

type Decoder struct{}

func (Decoder) ID() codec.ID { return iconVariant.id }

func (Decoder) Name() string { return iconVariant.name }

func (Decoder) Extensions() []string { return []string{iconVariant.extension} }

func (Decoder) Sniff(head []byte) bool { return iconVariant.sniff(head) }

func (Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	return iconVariant.probe(ctx, r, size, lim)
}

func (Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	return iconVariant.decode(ctx, r, size, o)
}

type CursorDecoder struct{}

func (CursorDecoder) ID() codec.ID { return cursorVariant.id }

func (CursorDecoder) Name() string { return cursorVariant.name }

func (CursorDecoder) Extensions() []string { return []string{cursorVariant.extension} }

func (CursorDecoder) Sniff(head []byte) bool { return cursorVariant.sniff(head) }

func (CursorDecoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	return cursorVariant.probe(ctx, r, size, lim)
}

func (CursorDecoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	return cursorVariant.decode(ctx, r, size, o)
}

func (v variant) sniff(head []byte) bool {
	if len(head) < directorySize+entrySize {
		return false
	}
	if head[0] != 0 || head[1] != 0 || binary.LittleEndian.Uint16(head[2:4]) != v.iconType {
		return false
	}
	count := int(binary.LittleEndian.Uint16(head[4:6]))
	if count < 1 || count > maxEntries {
		return false
	}
	if head[directorySize+3] != 0 {
		return false
	}
	offset := int64(binary.LittleEndian.Uint32(head[directorySize+12 : directorySize+16]))
	return offset >= int64(directorySize+count*entrySize)
}

type entry struct {
	index      int
	offset     int64
	length     int64
	isPNG      bool
	width      int
	height     int
	bitCount   int
	hasMask    bool
	headerSize int
	colors     int
}

func pngBitsPerPixel(colorType, depth byte) int {
	switch colorType {
	case 2:
		return 3 * int(depth)
	case 4:
		return 2 * int(depth)
	case 6:
		return 4 * int(depth)
	}
	return int(depth)
}

func inspectEntry(r io.ReaderAt, size int64, e *entry, lim codec.Limits) error {
	if e.length < dibProbeSize {
		return guard.Corrupt("ico: entry %d has only %d bytes", e.index, e.length)
	}
	head, err := guard.ReadAt(r, e.offset, dibProbeSize, size)
	if err != nil {
		return err
	}
	if bytes.Equal(head[:pngSignatureSize], pngSignature) {
		if binary.BigEndian.Uint32(head[8:12]) != 13 || string(head[12:16]) != "IHDR" {
			return guard.Corrupt("ico: entry %d has a broken PNG header", e.index)
		}
		width := int64(binary.BigEndian.Uint32(head[16:20]))
		height := int64(binary.BigEndian.Uint32(head[20:24]))
		if width == 0 || height == 0 || width > 1<<31-1 || height > 1<<31-1 {
			return guard.Corrupt("ico: entry %d PNG dimensions %dx%d", e.index, width, height)
		}
		e.isPNG = true
		e.width, e.height = int(width), int(height)
		e.bitCount = pngBitsPerPixel(head[25], head[24])
		return lim.Check(e.width, e.height)
	}
	return inspectDIB(head, e, lim)
}

func inspectDIB(head []byte, e *entry, lim codec.Limits) error {
	headerSize := int(binary.LittleEndian.Uint32(head[0:4]))
	switch headerSize {
	case 40, 52, 56, 64, 108, 124:
	default:
		return guard.Unsupported("ico: entry %d has a %d byte DIB header", e.index, headerSize)
	}
	if int64(headerSize) > e.length {
		return guard.Corrupt("ico: entry %d header longer than the entry", e.index)
	}
	width := int64(int32(binary.LittleEndian.Uint32(head[4:8])))
	field := int64(int32(binary.LittleEndian.Uint32(head[8:12])))
	bitCount := int(binary.LittleEndian.Uint16(head[14:16]))
	compression := binary.LittleEndian.Uint32(head[16:20])
	colorsUsed := binary.LittleEndian.Uint32(head[32:36])
	if width <= 0 || field <= 0 {
		return guard.Corrupt("ico: entry %d DIB dimensions %dx%d", e.index, width, field)
	}
	if width > 1<<31-1 || field > 1<<31-1 {
		return guard.TooLarge("ico: entry %d DIB dimensions %dx%d", e.index, width, field)
	}
	switch bitCount {
	case 1, 4, 8, 24, 32:
	default:
		return guard.Unsupported("ico: entry %d has %d bits per pixel", e.index, bitCount)
	}
	if compression != 0 {
		return guard.Unsupported("ico: entry %d DIB compression %d", e.index, compression)
	}
	e.width, e.bitCount, e.headerSize = int(width), bitCount, headerSize
	if bitCount <= 8 {
		e.colors = 1 << bitCount
		if colorsUsed != 0 && int(colorsUsed) < e.colors {
			e.colors = int(colorsUsed)
		}
	} else if colorsUsed <= maxPaletteColors {
		e.colors = int(colorsUsed)
	}
	candidates := []struct {
		height  int64
		hasMask bool
	}{{field / 2, true}, {field, false}}
	if field%2 == 1 {
		candidates = candidates[1:]
	}
	bestSlack := int64(-1)
	var limitErr error
	var best entry
	for _, c := range candidates {
		if c.height <= 0 {
			continue
		}
		e.height, e.hasMask = int(c.height), c.hasMask
		if err := lim.Check(e.width, e.height); err != nil {
			limitErr = err
			continue
		}
		slack := e.length - e.bytesNeeded()
		if slack >= 0 && (bestSlack < 0 || slack < bestSlack) {
			bestSlack, best = slack, *e
		}
	}
	if bestSlack < 0 {
		if limitErr != nil {
			return limitErr
		}
		return guard.Corrupt("ico: entry %d pixel data is cut off", e.index)
	}
	*e = best
	return nil
}

func (e *entry) xorStride() int64 {
	return (int64(e.width)*int64(e.bitCount) + 31) / 32 * 4
}

func (e *entry) andStride() int64 {
	return (int64(e.width) + 31) / 32 * 4
}

func (e *entry) bytesNeeded() int64 {
	total := int64(e.headerSize) + int64(e.colors)*4 + e.xorStride()*int64(e.height)
	if e.hasMask {
		total += e.andStride() * int64(e.height)
	}
	return total
}

func readEntries(ctx context.Context, r io.ReaderAt, size int64, v variant, lim codec.Limits) ([]entry, error) {
	if size < directorySize+entrySize {
		return nil, guard.Corrupt("ico: %d bytes is too short", size)
	}
	var head [directorySize]byte
	if err := guard.ReadAtInto(r, head[:], 0, size); err != nil {
		return nil, err
	}
	if head[0] != 0 || head[1] != 0 {
		return nil, guard.Corrupt("ico: reserved field is not zero")
	}
	if kind := binary.LittleEndian.Uint16(head[2:4]); kind != v.iconType {
		return nil, guard.Corrupt("ico: type %d, want %d", kind, v.iconType)
	}
	count := int(binary.LittleEndian.Uint16(head[4:6]))
	if count < 1 || count > maxEntries {
		return nil, guard.Corrupt("ico: %d entries", count)
	}
	directory, err := guard.ReadAt(r, directorySize, int64(count)*entrySize, size)
	if err != nil {
		return nil, err
	}
	directoryEnd := int64(directorySize + count*entrySize)
	var entries []entry
	var firstErr error
	for i := 0; i < count; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw := directory[i*entrySize : (i+1)*entrySize]
		e := entry{
			index:  i,
			length: int64(binary.LittleEndian.Uint32(raw[8:12])),
			offset: int64(binary.LittleEndian.Uint32(raw[12:16])),
		}
		var err error
		switch {
		case e.offset < directoryEnd || e.offset > size || e.length > size-e.offset:
			err = guard.Corrupt("ico: entry %d range %d+%d outside the %d byte file", i, e.offset, e.length, size)
		default:
			err = inspectEntry(r, size, &e, lim)
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return nil, firstErr
	}
	sort.SliceStable(entries, func(a, b int) bool {
		areaA := int64(entries[a].width) * int64(entries[a].height)
		areaB := int64(entries[b].width) * int64(entries[b].height)
		if areaA != areaB {
			return areaA > areaB
		}
		return entries[a].bitCount > entries[b].bitCount
	})
	return entries, nil
}

func (v variant) probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	lim = guard.Limits(lim)
	if err := guard.CheckSize(size, lim); err != nil {
		return codec.Info{}, err
	}
	if err := ctx.Err(); err != nil {
		return codec.Info{}, err
	}
	entries, err := readEntries(ctx, r, size, v, lim)
	if err != nil {
		return codec.Info{}, err
	}
	return v.info(entries), nil
}

func (v variant) info(entries []entry) codec.Info {
	best := entries[0]
	return codec.Info{
		Format:      v.id,
		FormatName:  v.name,
		Width:       best.width,
		Height:      best.height,
		Frames:      len(entries),
		HasAlpha:    true,
		Orientation: codec.OrientNormal,
		BitDepth:    best.bitCount,
	}
}

func decodePNGEntry(ctx context.Context, r io.ReaderAt, size int64, e entry) (*stdimage.NRGBA, error) {
	data, err := guard.ReadAt(r, e.offset, e.length, size)
	if err != nil {
		return nil, err
	}
	decoded, err := stdpng.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, guard.Classify(ctx, err, nil)
	}
	return convert.NRGBA(decoded), nil
}

func decodeDIBEntry(r io.ReaderAt, size int64, e entry) (*stdimage.NRGBA, error) {
	data, err := guard.ReadAt(r, e.offset, e.bytesNeeded(), size)
	if err != nil {
		return nil, err
	}
	var palette [maxPaletteColors][4]uint8
	for i := range palette {
		palette[i] = [4]uint8{0, 0, 0, 0xff}
	}
	paletteStart := e.headerSize
	for i := 0; i < e.colors; i++ {
		p := data[paletteStart+i*4:]
		palette[i] = [4]uint8{p[2], p[1], p[0], 0xff}
	}
	xorStart := paletteStart + e.colors*4
	xorStride := int(e.xorStride())
	andStart := xorStart + xorStride*e.height
	andStride := int(e.andStride())

	dst := stdimage.NewNRGBA(stdimage.Rect(0, 0, e.width, e.height))
	var alphaSeen uint8
	for y := 0; y < e.height; y++ {
		stored := e.height - 1 - y
		in := data[xorStart+stored*xorStride : xorStart+(stored+1)*xorStride]
		out := dst.Pix[y*dst.Stride : y*dst.Stride+e.width*4]
		for x := 0; x < e.width; x++ {
			o := x * 4
			switch e.bitCount {
			case 1:
				copy(out[o:o+4], palette[(in[x>>3]>>(7-uint(x&7)))&1][:])
			case 4:
				index := in[x>>1]
				if x&1 == 0 {
					index >>= 4
				} else {
					index &= 15
				}
				copy(out[o:o+4], palette[index][:])
			case 8:
				copy(out[o:o+4], palette[in[x]][:])
			case 24:
				out[o], out[o+1], out[o+2], out[o+3] = in[x*3+2], in[x*3+1], in[x*3], 0xff
			default:
				out[o], out[o+1], out[o+2], out[o+3] = in[x*4+2], in[x*4+1], in[x*4], in[x*4+3]
				alphaSeen |= in[x*4+3]
			}
		}
	}
	useMask := e.hasMask && (e.bitCount < 32 || alphaSeen == 0)
	if useMask {
		for y := 0; y < e.height; y++ {
			stored := e.height - 1 - y
			mask := data[andStart+stored*andStride : andStart+(stored+1)*andStride]
			out := dst.Pix[y*dst.Stride : y*dst.Stride+e.width*4]
			for x := 0; x < e.width; x++ {
				if (mask[x>>3]>>(7-uint(x&7)))&1 != 0 {
					out[x*4+3] = 0
				} else {
					out[x*4+3] = 0xff
				}
			}
		}
	} else if e.bitCount == 32 && alphaSeen == 0 {
		convert.SetOpaque(dst)
	}
	return dst, nil
}

func (v variant) decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	lim := guard.Limits(o.Limits)
	if err := guard.CheckSize(size, lim); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := readEntries(ctx, r, size, v, lim)
	if err != nil {
		return nil, err
	}
	budget := guard.NewFrameBudget(lim)
	var frames []codec.Frame
	var decoded []entry
	var firstErr error
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := budget.Add(e.width, e.height); err != nil {
			return nil, err
		}
		var pix *stdimage.NRGBA
		var err error
		if e.isPNG {
			pix, err = decodePNGEntry(ctx, r, size, e)
		} else {
			pix, err = decodeDIBEntry(r, size, e)
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		frames = append(frames, codec.Frame{Pix: pix})
		decoded = append(decoded, e)
	}
	if len(frames) == 0 {
		return nil, firstErr
	}
	info := v.info(decoded)
	info.Width, info.Height = frames[0].Pix.Bounds().Dx(), frames[0].Pix.Bounds().Dy()
	return &codec.Image{Info: info, Frames: frames, Scale: 1}, nil
}
