package svg

import (
	"bytes"
	"compress/gzip"
	"context"
	"image/color"
	"io"
	"math"

	"github.com/oops1/headless-gui/v3/widget/svg"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

const (
	MaxFileBytes         = 32 << 20
	MaxDecompressedBytes = 64 << 20
	MinVectorLongSide    = 1024
	maxDimension         = 1<<31 - 1
	sniffInflateBytes    = 4096
)

var (
	byteOrderMark = []byte{0xef, 0xbb, 0xbf}
	gzipMagic     = []byte{0x1f, 0x8b}
)

type Decoder struct{}

func (Decoder) ID() codec.ID { return codec.SVG }

func (Decoder) Name() string { return "SVG" }

func (Decoder) Extensions() []string { return []string{".svg", ".svgz"} }

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

func hasPrefixFold(b []byte, prefix string) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != prefix[i] {
			return false
		}
	}
	return true
}

func containsFold(b []byte, word string) bool {
	for i := 0; i+len(word) <= len(b); i++ {
		if hasPrefixFold(b[i:], word) {
			return true
		}
	}
	return false
}

func skipDoctype(b []byte) (rest []byte, finished bool) {
	depth := 0
	var quote byte
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '[':
			depth++
		case c == ']':
			if depth > 0 {
				depth--
			}
		case c == '>' && depth == 0:
			return b[i+1:], true
		}
	}
	return b, false
}

func looksLikeSVG(head []byte) bool {
	b := bytes.TrimPrefix(head, byteOrderMark)
	for steps := 0; steps < 64; steps++ {
		for len(b) > 0 && isSpace(b[0]) {
			b = b[1:]
		}
		switch {
		case bytes.HasPrefix(b, []byte("<?")):
			end := bytes.Index(b, []byte("?>"))
			if end < 0 {
				return false
			}
			b = b[end+2:]
		case bytes.HasPrefix(b, []byte("<!--")):
			end := bytes.Index(b[4:], []byte("-->"))
			if end < 0 {
				return false
			}
			b = b[4+end+3:]
		case hasPrefixFold(b, "<!doctype"):
			rest, finished := skipDoctype(b)
			doctypeText := b
			if finished {
				doctypeText = b[:len(b)-len(rest)]
			}
			if !containsFold(doctypeText, "svg") {
				return false
			}
			if !finished {
				return true
			}
			b = rest
		case hasPrefixFold(b, "<svg"):
			return len(b) > 4 && (isSpace(b[4]) || b[4] == '>' || b[4] == '/' || b[4] == ':')
		default:
			return false
		}
	}
	return false
}

func (Decoder) Sniff(head []byte) bool {
	if bytes.HasPrefix(head, gzipMagic) {
		zr, err := gzip.NewReader(bytes.NewReader(head))
		if err != nil {
			return false
		}
		prefix, _ := io.ReadAll(io.LimitReader(zr, sniffInflateBytes))
		return looksLikeSVG(prefix)
	}
	return looksLikeSVG(head)
}

func loadSource(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) ([]byte, error) {
	if err := guard.CheckSize(size, lim); err != nil {
		return nil, err
	}
	if size > MaxFileBytes {
		return nil, guard.TooLarge("svg: file of %d bytes", size)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := guard.ReadAt(r, 0, size, size)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, gzipMagic) {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, guard.Corrupt("svg: %v", err)
		}
		inflated, err := io.ReadAll(io.LimitReader(zr, MaxDecompressedBytes+1))
		if err != nil {
			return nil, guard.Corrupt("svg: %v", err)
		}
		if len(inflated) > MaxDecompressedBytes {
			return nil, guard.TooLarge("svg: more than %d bytes after decompression", MaxDecompressedBytes)
		}
		data = inflated
	}
	return data, ctx.Err()
}

func parseDocument(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (*svg.Document, codec.Info, error) {
	data, err := loadSource(ctx, r, size, lim)
	if err != nil {
		return nil, codec.Info{}, err
	}
	if len(data) > svg.MaxFileBytes {
		return nil, codec.Info{}, guard.TooLarge("svg: %d bytes of markup exceed the renderer limit of %d", len(data), svg.MaxFileBytes)
	}
	if !looksLikeSVG(data) {
		return nil, codec.Info{}, guard.Corrupt("svg: the root element is not <svg>")
	}
	doc, err := svg.Parse(data)
	if err != nil {
		return nil, codec.Info{}, guard.Classify(ctx, err, nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, codec.Info{}, err
	}
	boxWidth, boxHeight := doc.ViewBox[2], doc.ViewBox[3]
	if !(boxWidth > 0) || !(boxHeight > 0) || math.IsInf(boxWidth, 0) || math.IsInf(boxHeight, 0) {
		return nil, codec.Info{}, guard.Corrupt("svg: empty or invalid view box %v", doc.ViewBox)
	}
	width, height := roundDimension(boxWidth), roundDimension(boxHeight)
	if err := lim.Check(width, height); err != nil {
		return nil, codec.Info{}, err
	}
	return doc, codec.Info{
		Format:      codec.SVG,
		FormatName:  "SVG",
		Width:       width,
		Height:      height,
		Frames:      1,
		HasAlpha:    true,
		Orientation: codec.OrientNormal,
		Vector:      true,
	}, nil
}

func roundDimension(v float64) int {
	v = math.Round(v)
	if v < 1 {
		return 1
	}
	if v > maxDimension {
		return maxDimension
	}
	return int(v)
}

func (Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	_, info, err := parseDocument(ctx, r, size, guard.Limits(lim))
	return info, err
}

func RasterSize(o codec.Options, boxWidth, boxHeight float64, lim codec.Limits) (int, int) {
	var width, height float64
	if o.VectorW > 0 && o.VectorH > 0 {
		width, height = float64(o.VectorW), float64(o.VectorH)
	} else {
		width, height = boxWidth, boxHeight
		if longSide := math.Max(width, height); longSide < MinVectorLongSide {
			factor := MinVectorLongSide / longSide
			width *= factor
			height *= factor
		}
	}
	if area := width * height; area > float64(lim.MaxPixels) {
		factor := math.Sqrt(float64(lim.MaxPixels) / area)
		width = math.Floor(width * factor)
		height = math.Floor(height * factor)
	}
	w, h := roundDimension(width), roundDimension(height)
	for int64(w)*int64(h) > lim.MaxPixels {
		if w >= h && w > 1 {
			w--
		} else if h > 1 {
			h--
		} else {
			break
		}
	}
	return w, h
}

func (Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	lim := guard.Limits(o.Limits)
	doc, info, err := parseDocument(ctx, r, size, lim)
	if err != nil {
		return nil, err
	}
	width, height := RasterSize(o, doc.ViewBox[2], doc.ViewBox[3], lim)
	rendered := doc.Rasterize(width, height, color.RGBA{A: 0xff}, false)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if rendered == nil {
		return nil, guard.Corrupt("svg: rasterisation failed")
	}
	pix := convert.NRGBA(rendered)
	return &codec.Image{Info: info, Frames: []codec.Frame{{Pix: pix}}, Scale: 1}, nil
}
