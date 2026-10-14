package webp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	vpx "github.com/gen2brain/vpx/webp"

	"github.com/oops1/PixGo/internal/codec"
)

func readTestdata(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func chunk(id string, payload []byte) []byte {
	out := append([]byte(id), binary.LittleEndian.AppendUint32(nil, uint32(len(payload)))...)
	out = append(out, payload...)
	if len(payload)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

func riff(chunks ...[]byte) []byte {
	var body []byte
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := []byte("RIFF")
	out = binary.LittleEndian.AppendUint32(out, uint32(4+len(body)))
	out = append(out, "WEBP"...)
	return append(out, body...)
}

func uint24(v int) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16)} }

func vp8x(flags byte, width, height int) []byte {
	payload := []byte{flags, 0, 0, 0}
	payload = append(payload, uint24(width-1)...)
	payload = append(payload, uint24(height-1)...)
	return chunk("VP8X", payload)
}

func vp8(width, height int) []byte {
	payload := []byte{0x10, 0, 0, 0x9d, 0x01, 0x2a}
	payload = binary.LittleEndian.AppendUint16(payload, uint16(width))
	payload = binary.LittleEndian.AppendUint16(payload, uint16(height))
	return chunk("VP8 ", payload)
}

func vp8l(width, height int, alpha bool) []byte {
	bitsWord := uint32(width-1) | uint32(height-1)<<14
	if alpha {
		bitsWord |= 1 << 28
	}
	payload := []byte{0x2f}
	payload = binary.LittleEndian.AppendUint32(payload, bitsWord)
	return chunk("VP8L", payload)
}

func exifChunk(tiff []byte, prefixed bool) []byte {
	if prefixed {
		tiff = append([]byte("Exif\x00\x00"), tiff...)
	}
	return chunk("EXIF", tiff)
}

func animChunk(loop int) []byte {
	return chunk("ANIM", []byte{0, 0, 0, 0, byte(loop), byte(loop >> 8)})
}

const (
	frameDispose = 1
	frameNoBlend = 2
)

func frameChunk(x, y, width, height, milliseconds int, flags byte, inner ...[]byte) []byte {
	payload := append(uint24(x/2), uint24(y/2)...)
	payload = append(payload, uint24(width-1)...)
	payload = append(payload, uint24(height-1)...)
	payload = append(payload, uint24(milliseconds)...)
	payload = append(payload, flags)
	for _, c := range inner {
		payload = append(payload, c...)
	}
	return chunk("ANMF", payload)
}

func probe(data []byte) (codec.Info, error) {
	return Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
}

func decodeWith(data []byte, o codec.Options) (*codec.Image, error) {
	return Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), o)
}

func decode(data []byte) (*codec.Image, error) {
	return decodeWith(data, codec.Options{Limits: codec.DefaultLimits()})
}

func decodeAllFrames(data []byte) (*codec.Image, error) {
	return decodeWith(data, codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames})
}

func vpxEncode(t testing.TB, img image.Image, o vpx.EncodeOptions) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := vpx.Encode(&buf, img, o); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func bitstreamChunks(t testing.TB, file []byte) [][]byte {
	t.Helper()
	var out [][]byte
	for position := riffHeaderSize; position+chunkHeaderSize <= len(file); {
		length := int(binary.LittleEndian.Uint32(file[position+4:]))
		next := position + chunkHeaderSize + length + length&1
		if next > len(file) {
			t.Fatalf("chunk at %d overruns the file", position)
		}
		if id := string(file[position : position+4]); id != "VP8X" {
			out = append(out, file[position:next])
		}
		position = next
	}
	return out
}

func chunkIDs(t testing.TB, file []byte) []string {
	t.Helper()
	var ids []string
	for position := riffHeaderSize; position+chunkHeaderSize <= len(file); {
		length := int(binary.LittleEndian.Uint32(file[position+4:]))
		ids = append(ids, string(file[position:position+4]))
		position += chunkHeaderSize + length + length&1
	}
	return ids
}

func losslessChunks(t testing.TB, img *image.NRGBA) [][]byte {
	t.Helper()
	return bitstreamChunks(t, vpxEncode(t, img, vpx.EncodeOptions{Lossless: true, Method: 1, Exact: true}))
}

func lossyChunks(t testing.TB, img *image.NRGBA, quality int) [][]byte {
	t.Helper()
	return bitstreamChunks(t, vpxEncode(t, img, vpx.EncodeOptions{Quality: quality, Method: 2}))
}

func smooth(width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			fx, fy := float64(x)/float64(width), float64(y)/float64(height)
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(40 + 170*fx),
				G: uint8(60 + 150*fy + 20*math.Sin(fx*9)),
				B: uint8(200 - 120*fx*fy),
				A: 255,
			})
		}
	}
	return img
}

func withAlpha(src *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(src.Bounds())
	copy(out.Pix, src.Pix)
	bounds := out.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := out.NRGBAAt(x, y)
			switch {
			case x < bounds.Min.X+bounds.Dx()/4:
				c.A = 0
			case x < bounds.Min.X+bounds.Dx()/2:
				c.A = uint8(255 * (y - bounds.Min.Y) / bounds.Dy())
			default:
				c.A = 255
			}
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}

func solid(width, height int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func psnr(a, b *image.NRGBA, minAlpha uint8) float64 {
	var sum float64
	var n int
	bounds := a.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pa, pb := a.NRGBAAt(x, y), b.NRGBAAt(x, y)
			if pa.A < minAlpha || pb.A < minAlpha {
				continue
			}
			for _, d := range [3]float64{float64(pa.R) - float64(pb.R), float64(pa.G) - float64(pb.G), float64(pa.B) - float64(pb.B)} {
				sum += d * d
				n++
			}
		}
	}
	if n == 0 || sum == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255/(sum/float64(n)))
}

func alphaIdentical(a, b *image.NRGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	bounds := a.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if a.NRGBAAt(x, y).A != b.NRGBAAt(x, y).A {
				return false
			}
		}
	}
	return true
}

func equalVisible(a, b *image.NRGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	bounds := a.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pa, pb := a.NRGBAAt(x, y), b.NRGBAAt(x, y)
			if pa.A == 0 && pb.A == 0 {
				continue
			}
			if pa != pb {
				return false
			}
		}
	}
	return true
}

type premultiplied4 [4]float64

func premultipliedAt(img *image.NRGBA, x, y int) premultiplied4 {
	c := img.NRGBAAt(x, y)
	a := float64(c.A) / 255
	return premultiplied4{float64(c.R) * a, float64(c.G) * a, float64(c.B) * a, float64(c.A)}
}

func maxPremultipliedDelta(a, b *image.NRGBA) float64 {
	if a.Bounds() != b.Bounds() {
		return math.Inf(1)
	}
	worst := 0.0
	bounds := a.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pa, pb := premultipliedAt(a, x, y), premultipliedAt(b, x, y)
			for i := range pa {
				worst = max(worst, math.Abs(pa[i]-pb[i]))
			}
		}
	}
	return worst
}

type frameSpec struct {
	x, y         int
	img          *image.NRGBA
	milliseconds int
	blend        bool
	dispose      bool
	lossy        bool
}

func buildAnimation(t testing.TB, width, height, loop int, specs []frameSpec, extra ...[]byte) []byte {
	t.Helper()
	chunks := [][]byte{vp8x(flagAnimation|flagAlpha, width, height), animChunk(loop)}
	chunks = append(chunks, extra...)
	for _, s := range specs {
		var flags byte
		if !s.blend {
			flags |= frameNoBlend
		}
		if s.dispose {
			flags |= frameDispose
		}
		var inner [][]byte
		if s.lossy {
			inner = lossyChunks(t, s.img, 90)
		} else {
			inner = losslessChunks(t, s.img)
		}
		chunks = append(chunks, frameChunk(s.x, s.y, s.img.Bounds().Dx(), s.img.Bounds().Dy(), s.milliseconds, flags, inner...))
	}
	return riff(chunks...)
}

func composeReference(width, height int, specs []frameSpec) []*image.NRGBA {
	canvas := make([]premultiplied4, width*height)
	var out []*image.NRGBA
	for i, s := range specs {
		if i > 0 && specs[i-1].dispose {
			p := specs[i-1]
			b := p.img.Bounds()
			for y := 0; y < b.Dy(); y++ {
				for x := 0; x < b.Dx(); x++ {
					canvas[(p.y+y)*width+p.x+x] = premultiplied4{}
				}
			}
		}
		b := s.img.Bounds()
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				src := premultipliedAt(s.img, b.Min.X+x, b.Min.Y+y)
				index := (s.y+y)*width + s.x + x
				if !s.blend {
					canvas[index] = src
					continue
				}
				keep := 1 - src[3]/255
				for k := range src {
					canvas[index][k] = src[k] + canvas[index][k]*keep
				}
			}
		}
		snapshot := image.NewNRGBA(image.Rect(0, 0, width, height))
		for index, p := range canvas {
			o := index * 4
			if p[3] > 0 {
				scale := 255 / p[3]
				snapshot.Pix[o] = uint8(math.Min(255, math.Round(p[0]*scale)))
				snapshot.Pix[o+1] = uint8(math.Min(255, math.Round(p[1]*scale)))
				snapshot.Pix[o+2] = uint8(math.Min(255, math.Round(p[2]*scale)))
				snapshot.Pix[o+3] = uint8(math.Round(p[3]))
			}
		}
		out = append(out, snapshot)
	}
	return out
}

type countingReader struct {
	data   []byte
	reads  atomic.Int64
	after  int64
	action func()
}

func (c *countingReader) ReadAt(p []byte, off int64) (int, error) {
	n := c.reads.Add(1)
	if c.action != nil && n == c.after {
		c.action()
	}
	if off >= int64(len(c.data)) {
		return 0, io.EOF
	}
	copied := copy(p, c.data[off:])
	if copied < len(p) {
		return copied, io.EOF
	}
	return copied, nil
}

func optionsLossless() vpx.EncodeOptions {
	return vpx.EncodeOptions{Lossless: true, Method: 1, Exact: true}
}

func vpxLossy(quality int) vpx.EncodeOptions {
	return vpx.EncodeOptions{Quality: quality, Method: 2}
}

var errBrokenDisk = errors.New("broken disk")

type brokenReader struct {
	data   []byte
	failAt int
	reads  int
}

func (b *brokenReader) ReadAt(p []byte, off int64) (int, error) {
	b.reads++
	if b.reads == b.failAt {
		return 0, errBrokenDisk
	}
	if off >= int64(len(b.data)) {
		return 0, io.EOF
	}
	copied := copy(p, b.data[off:])
	if copied < len(p) {
		return copied, io.EOF
	}
	return copied, nil
}
