package apng

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	stdpng "image/png"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

type rawChunk struct {
	kind    string
	body    []byte
	badCRC  bool
	badSize *uint32
}

type chunkList []rawChunk

func (c chunkList) bytes() []byte {
	out := append([]byte(nil), signature...)
	for _, ch := range c {
		length := uint32(len(ch.body))
		if ch.badSize != nil {
			length = *ch.badSize
		}
		out = binary.BigEndian.AppendUint32(out, length)
		start := len(out)
		out = append(out, ch.kind...)
		out = append(out, ch.body...)
		sum := crc32.ChecksumIEEE(out[start:])
		if ch.badCRC {
			sum ^= 0xdeadbeef
		}
		out = binary.BigEndian.AppendUint32(out, sum)
	}
	return out
}

func (c chunkList) index(kind string, nth int) int {
	for i, ch := range c {
		if ch.kind == kind {
			if nth == 0 {
				return i
			}
			nth--
		}
	}
	panic("chunk " + kind + " not found")
}

func (c chunkList) count(kind string) int {
	n := 0
	for _, ch := range c {
		if ch.kind == kind {
			n++
		}
	}
	return n
}

func (c chunkList) clone() chunkList {
	out := make(chunkList, len(c))
	for i, ch := range c {
		ch.body = append([]byte(nil), ch.body...)
		out[i] = ch
	}
	return out
}

func (c chunkList) insertAt(i int, extra ...rawChunk) chunkList {
	out := append(chunkList(nil), c[:i]...)
	out = append(out, extra...)
	return append(out, c[i:]...)
}

func (c chunkList) removeAt(i int) chunkList {
	return append(append(chunkList(nil), c[:i]...), c[i+1:]...)
}

func (c chunkList) edit(kind string, nth int, change func(*rawChunk)) chunkList {
	out := c.clone()
	change(&out[out.index(kind, nth)])
	return out
}

func (c chunkList) corruptCRC(kind string, nth int) chunkList {
	return c.edit(kind, nth, func(ch *rawChunk) { ch.badCRC = true })
}

func (c chunkList) truncated(keep int) []byte {
	data := c.bytes()
	return data[:len(data)-keep]
}

func chunk(kind string, body []byte) []byte {
	return chunkList{{kind: kind, body: body}}.bytes()[len(signature):]
}

type pngParts struct {
	header  []byte
	palette []byte
	trans   []byte
	data    []byte
}

func splitPNG(tb testing.TB, data []byte) pngParts {
	tb.Helper()
	var p pngParts
	position := len(signature)
	for position+12 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[position:]))
		kind := string(data[position+4 : position+8])
		body := data[position+8 : position+8+length]
		switch kind {
		case "IHDR":
			p.header = body
		case "PLTE":
			p.palette = body
		case "tRNS":
			p.trans = body
		case "IDAT":
			p.data = append(p.data, body...)
		}
		position += 12 + length
	}
	if p.header == nil || p.data == nil {
		tb.Fatal("not a PNG file")
	}
	return p
}

func encodeStd(tb testing.TB, img image.Image) []byte {
	tb.Helper()
	var buf bytes.Buffer
	if err := stdpng.Encode(&buf, img); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

func deflate(raw []byte) []byte {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write(raw)
	w.Close()
	return buf.Bytes()
}

func ihdr(width, height int, depth, colourType, interlace byte) []byte {
	body := binary.BigEndian.AppendUint32(nil, uint32(width))
	body = binary.BigEndian.AppendUint32(body, uint32(height))
	return append(body, depth, colourType, 0, 0, interlace)
}

func rgbaPNG(img *image.NRGBA) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	var raw []byte
	for y := 0; y < h; y++ {
		raw = append(raw, 0)
		raw = append(raw, img.Pix[y*img.Stride:y*img.Stride+w*4]...)
	}
	return chunkList{
		{kind: "IHDR", body: ihdr(w, h, 8, 6, 0)},
		{kind: "IDAT", body: deflate(raw)},
		{kind: "IEND"},
	}.bytes()
}

func interlacedRGBAPNG(img *image.NRGBA) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	passes := [7][4]int{{0, 0, 8, 8}, {4, 0, 8, 8}, {0, 4, 4, 8}, {2, 0, 4, 4}, {0, 2, 2, 4}, {1, 0, 2, 2}, {0, 1, 1, 2}}
	var raw []byte
	for _, p := range passes {
		if p[0] >= w || p[1] >= h {
			continue
		}
		for y := p[1]; y < h; y += p[3] {
			raw = append(raw, 0)
			for x := p[0]; x < w; x += p[2] {
				raw = append(raw, img.Pix[y*img.Stride+x*4:y*img.Stride+x*4+4]...)
			}
		}
	}
	return chunkList{
		{kind: "IHDR", body: ihdr(w, h, 8, 6, 1)},
		{kind: "IDAT", body: deflate(raw)},
		{kind: "IEND"},
	}.bytes()
}

func solid(width, height int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

type frameSpec struct {
	png      []byte
	x, y     int
	num, den uint16
	dispose  byte
	blend    byte
}

type animation struct {
	width, height int
	plays         uint32
	defaultPNG    []byte
	frames        []frameSpec
	pieces        int
	before        []rawChunk
}

func pieces(data []byte, n int) [][]byte {
	if n <= 1 || len(data) < n {
		return [][]byte{data}
	}
	var out [][]byte
	step := len(data) / n
	for i := 0; i < n-1; i++ {
		out = append(out, data[i*step:(i+1)*step])
	}
	return append(out, data[(n-1)*step:])
}

func frameControl(sequence uint32, f frameSpec, width, height int) []byte {
	body := binary.BigEndian.AppendUint32(nil, sequence)
	body = binary.BigEndian.AppendUint32(body, uint32(width))
	body = binary.BigEndian.AppendUint32(body, uint32(height))
	body = binary.BigEndian.AppendUint32(body, uint32(f.x))
	body = binary.BigEndian.AppendUint32(body, uint32(f.y))
	body = binary.BigEndian.AppendUint16(body, f.num)
	body = binary.BigEndian.AppendUint16(body, f.den)
	return append(body, f.dispose, f.blend)
}

func animationControl(frames, plays uint32) []byte {
	return binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint32(nil, frames), plays)
}

func (a animation) chunks(tb testing.TB) chunkList {
	tb.Helper()
	first := a.frames[0].png
	if a.defaultPNG != nil {
		first = a.defaultPNG
	}
	lead := splitPNG(tb, first)
	width, height := a.width, a.height
	if width == 0 {
		width = int(binary.BigEndian.Uint32(lead.header[0:4]))
		height = int(binary.BigEndian.Uint32(lead.header[4:8]))
	}
	header := append(ihdr(width, height, 0, 0, 0)[:8], lead.header[8:]...)
	list := chunkList{{kind: "IHDR", body: header}}
	list = append(list, a.before...)
	if lead.palette != nil {
		list = append(list, rawChunk{kind: "PLTE", body: lead.palette})
	}
	if lead.trans != nil {
		list = append(list, rawChunk{kind: "tRNS", body: lead.trans})
	}
	list = append(list, rawChunk{kind: "acTL", body: animationControl(uint32(len(a.frames)), a.plays)})
	sequence := uint32(0)
	size := func(f frameSpec) (int, int) {
		parts := splitPNG(tb, f.png)
		return int(binary.BigEndian.Uint32(parts.header[0:4])), int(binary.BigEndian.Uint32(parts.header[4:8]))
	}
	addFdAT := func(f frameSpec) {
		w, h := size(f)
		list = append(list, rawChunk{kind: "fcTL", body: frameControl(sequence, f, w, h)})
		sequence++
		for _, piece := range pieces(splitPNG(tb, f.png).data, a.pieces) {
			list = append(list, rawChunk{kind: "fdAT", body: append(binary.BigEndian.AppendUint32(nil, sequence), piece...)})
			sequence++
		}
	}
	frames := a.frames
	if a.defaultPNG != nil {
		for _, piece := range pieces(lead.data, a.pieces) {
			list = append(list, rawChunk{kind: "IDAT", body: piece})
		}
	} else {
		w, h := size(frames[0])
		list = append(list, rawChunk{kind: "fcTL", body: frameControl(sequence, frames[0], w, h)})
		sequence++
		for _, piece := range pieces(lead.data, a.pieces) {
			list = append(list, rawChunk{kind: "IDAT", body: piece})
		}
		frames = frames[1:]
	}
	for _, f := range frames {
		addFdAT(f)
	}
	return append(list, rawChunk{kind: "IEND"})
}

func (a animation) bytes(tb testing.TB) []byte {
	return a.chunks(tb).bytes()
}

func probe(data []byte) (codec.Info, error) {
	return Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
}

func decodeWith(data []byte, frames codec.FrameSel, lim codec.Limits) (*codec.Image, error) {
	return Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim, Frames: frames})
}

func decodeAll(data []byte) (*codec.Image, error) {
	return decodeWith(data, codec.AllFrames, codec.DefaultLimits())
}

func mustDecodeAll(tb testing.TB, data []byte) *codec.Image {
	tb.Helper()
	img, err := decodeAll(data)
	if err != nil {
		tb.Fatal(err)
	}
	return img
}

func at(img *image.NRGBA, x, y int) color.NRGBA {
	return img.NRGBAAt(x, y)
}
