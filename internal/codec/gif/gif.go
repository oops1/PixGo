package gif

import (
	"bytes"
	"context"
	"image"
	"image/color"
	stdgif "image/gif"
	"io"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

const LoopForever = 0

const (
	MinFrameDelay     = 20 * time.Millisecond
	DefaultFrameDelay = 100 * time.Millisecond
	delayUnit         = 10 * time.Millisecond
)

const (
	disposalBackground = 2
	disposalPrevious   = 3
)

const (
	headerSize          = 13
	windowSize          = 64 << 10
	maxBlocks           = 1 << 20
	extensionIntroducer = 0x21
	imageSeparator      = 0x2c
	trailer             = 0x3b
	graphicControlLabel = 0xf9
	applicationLabel    = 0xff
	appIdentifierSize   = 11
)

func PlayCount(loopCount int) int {
	switch {
	case loopCount == 0:
		return LoopForever
	case loopCount < 0:
		return 1
	}
	return loopCount + 1
}

func FrameDelay(centiseconds int) time.Duration {
	delay := time.Duration(centiseconds) * delayUnit
	if delay < MinFrameDelay {
		return DefaultFrameDelay
	}
	return delay
}

type Decoder struct{}

func (Decoder) ID() codec.ID { return codec.GIF }

func (Decoder) Name() string { return "GIF" }

func (Decoder) Extensions() []string { return []string{".gif"} }

func (Decoder) Sniff(head []byte) bool {
	return len(head) >= 6 && (bytes.Equal(head[:6], []byte("GIF87a")) || bytes.Equal(head[:6], []byte("GIF89a")))
}

type scan struct {
	width        int
	height       int
	frames       int
	transparent  bool
	loopCount    int
	firstDelayCS int
}

type window struct {
	ctx   context.Context
	r     io.ReaderAt
	size  int64
	start int64
	buf   []byte
}

func (w *window) ensure(position int64, n int) error {
	if position >= w.start && position+int64(n) <= w.start+int64(len(w.buf)) {
		return nil
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if position < 0 || position+int64(n) > w.size {
		return guard.Corrupt("gif: read of %d bytes at %d outside %d byte file", n, position, w.size)
	}
	want := int64(windowSize)
	if want > w.size-position {
		want = w.size - position
	}
	if cap(w.buf) < int(want) {
		w.buf = make([]byte, want)
	}
	w.buf = w.buf[:want]
	if err := guard.ReadAtInto(w.r, w.buf, position, w.size); err != nil {
		w.buf = w.buf[:0]
		return err
	}
	w.start = position
	return nil
}

func (w *window) at(position int64, n int) ([]byte, error) {
	if err := w.ensure(position, n); err != nil {
		return nil, err
	}
	offset := position - w.start
	return w.buf[offset : offset+int64(n)], nil
}

func (w *window) skipSubBlocks(position int64) (int64, error) {
	for {
		length, err := w.at(position, 1)
		if err != nil {
			return 0, err
		}
		position++
		if length[0] == 0 {
			return position, nil
		}
		position += int64(length[0])
		if position > w.size {
			return 0, guard.Corrupt("gif: sub-block runs past the end of the file")
		}
	}
}

func scanBlocks(ctx context.Context, r io.ReaderAt, size int64) (scan, error) {
	result := scan{loopCount: -1}
	w := &window{ctx: ctx, r: r, size: size}
	head, err := w.at(0, headerSize)
	if err != nil {
		return result, err
	}
	if string(head[:3]) != "GIF" || (string(head[3:6]) != "87a" && string(head[3:6]) != "89a") {
		return result, guard.Corrupt("gif: bad signature")
	}
	result.width = int(head[6]) | int(head[7])<<8
	result.height = int(head[8]) | int(head[9])<<8
	position := int64(headerSize)
	if head[10]&0x80 != 0 {
		position += 3 << ((head[10] & 7) + 1)
	}
	pendingDelayCS := 0
	for blocks := 0; ; blocks++ {
		if blocks >= maxBlocks {
			return result, guard.Corrupt("gif: more than %d blocks", maxBlocks)
		}
		introducer, err := w.at(position, 1)
		if err != nil {
			if result.frames > 0 {
				return result, nil
			}
			return result, err
		}
		position++
		switch introducer[0] {
		case trailer:
			if result.frames == 0 {
				return result, guard.Corrupt("gif: no image data")
			}
			return result, nil
		case extensionIntroducer:
			label, err := w.at(position, 1)
			if err != nil {
				return result, err
			}
			position++
			switch label[0] {
			case graphicControlLabel:
				body, err := w.at(position, 5)
				if err != nil {
					return result, err
				}
				if body[0] != 4 {
					return result, guard.Corrupt("gif: graphic control block of %d bytes", body[0])
				}
				if body[1]&1 != 0 {
					result.transparent = true
				}
				pendingDelayCS = int(body[2]) | int(body[3])<<8
			case applicationLabel:
				length, err := w.at(position, 1)
				if err != nil {
					return result, err
				}
				if int(length[0]) >= appIdentifierSize {
					identifier, err := w.at(position+1, appIdentifierSize)
					if err != nil {
						return result, err
					}
					if string(identifier) == "NETSCAPE2.0" || string(identifier) == "ANIMEXTS1.0" {
						next := position + 1 + int64(length[0])
						sub, err := w.at(next, 1)
						if err == nil && sub[0] >= 3 {
							if body, err := w.at(next+1, 3); err == nil && body[0] == 1 {
								result.loopCount = int(body[1]) | int(body[2])<<8
							}
						}
					}
				}
			}
			position, err = w.skipSubBlocks(position)
			if err != nil {
				return result, err
			}
		case imageSeparator:
			descriptor, err := w.at(position, 9)
			if err != nil {
				return result, err
			}
			result.frames++
			if result.frames == 1 {
				result.firstDelayCS = pendingDelayCS
			}
			pendingDelayCS = 0
			position += 9
			if descriptor[8]&0x80 != 0 {
				position += 3 << ((descriptor[8] & 7) + 1)
			}
			if position >= size {
				return result, guard.Corrupt("gif: image data missing")
			}
			position++
			position, err = w.skipSubBlocks(position)
			if err != nil {
				return result, err
			}
		default:
			return result, guard.Corrupt("gif: unknown block type 0x%02x", introducer[0])
		}
	}
}

func (Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	info, _, err := probeScan(ctx, r, size, lim)
	return info, err
}

func probeScan(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, scan, error) {
	lim = guard.Limits(lim)
	if err := guard.CheckSize(size, lim); err != nil {
		return codec.Info{}, scan{}, err
	}
	if err := ctx.Err(); err != nil {
		return codec.Info{}, scan{}, err
	}
	config, err := stdgif.DecodeConfig(guard.Stream(ctx, r, size))
	if err != nil {
		return codec.Info{}, scan{}, guard.Classify(ctx, err, nil)
	}
	if err := lim.Check(config.Width, config.Height); err != nil {
		return codec.Info{}, scan{}, err
	}
	blocks, err := scanBlocks(ctx, r, size)
	if err != nil {
		return codec.Info{}, scan{}, guard.Classify(ctx, err, nil)
	}
	return codec.Info{
		Format:      codec.GIF,
		FormatName:  "GIF",
		Width:       config.Width,
		Height:      config.Height,
		Frames:      blocks.frames,
		Animated:    blocks.frames > 1,
		HasAlpha:    blocks.transparent,
		Orientation: codec.OrientNormal,
		BitDepth:    8,
	}, blocks, nil
}

type paletteTable [256]color.NRGBA

func newPaletteTable(palette color.Palette) *paletteTable {
	var table paletteTable
	for i := 0; i < len(palette) && i < len(table); i++ {
		if palette[i] != nil {
			table[i] = color.NRGBAModel.Convert(palette[i]).(color.NRGBA)
		}
	}
	return &table
}

func drawFrame(canvas *image.NRGBA, frame *image.Paletted) {
	area := frame.Rect.Intersect(canvas.Rect)
	if area.Empty() {
		return
	}
	table := newPaletteTable(frame.Palette)
	for y := area.Min.Y; y < area.Max.Y; y++ {
		in := frame.Pix[frame.PixOffset(area.Min.X, y):]
		in = in[:area.Dx()]
		out := canvas.Pix[canvas.PixOffset(area.Min.X, y):]
		for i, index := range in {
			c := table[index]
			if c.A == 0 {
				continue
			}
			o := i * 4
			out[o], out[o+1], out[o+2], out[o+3] = c.R, c.G, c.B, c.A
		}
	}
}

func clearArea(canvas *image.NRGBA, area image.Rectangle) {
	area = area.Intersect(canvas.Rect)
	for y := area.Min.Y; y < area.Max.Y; y++ {
		row := canvas.Pix[canvas.PixOffset(area.Min.X, y):]
		clear(row[:area.Dx()*4])
	}
}

func saveArea(canvas *image.NRGBA, area image.Rectangle) []byte {
	area = area.Intersect(canvas.Rect)
	if area.Empty() {
		return nil
	}
	saved := make([]byte, 0, area.Dx()*area.Dy()*4)
	for y := area.Min.Y; y < area.Max.Y; y++ {
		offset := canvas.PixOffset(area.Min.X, y)
		saved = append(saved, canvas.Pix[offset:offset+area.Dx()*4]...)
	}
	return saved
}

func restoreArea(canvas *image.NRGBA, area image.Rectangle, saved []byte) {
	area = area.Intersect(canvas.Rect)
	if area.Empty() || len(saved) != area.Dx()*area.Dy()*4 {
		return
	}
	rowBytes := area.Dx() * 4
	for i, y := 0, area.Min.Y; y < area.Max.Y; i, y = i+1, y+1 {
		offset := canvas.PixOffset(area.Min.X, y)
		copy(canvas.Pix[offset:offset+rowBytes], saved[i*rowBytes:(i+1)*rowBytes])
	}
}

func cloneImage(src *image.NRGBA) *image.NRGBA {
	return &image.NRGBA{Pix: append([]uint8(nil), src.Pix...), Stride: src.Stride, Rect: src.Rect}
}

func composeFrames(ctx context.Context, animation *stdgif.GIF, width, height int) ([]codec.Frame, error) {
	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))
	frames := make([]codec.Frame, 0, len(animation.Image))
	var previousDisposal byte
	var previousArea image.Rectangle
	var saved []byte
	for i, frame := range animation.Image {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		switch previousDisposal {
		case disposalBackground:
			clearArea(canvas, previousArea)
		case disposalPrevious:
			restoreArea(canvas, previousArea, saved)
		}
		var disposal byte
		if i < len(animation.Disposal) {
			disposal = animation.Disposal[i]
		}
		if disposal == disposalPrevious {
			saved = saveArea(canvas, frame.Rect)
		}
		drawFrame(canvas, frame)
		var delay time.Duration
		if i < len(animation.Delay) {
			delay = FrameDelay(animation.Delay[i])
		} else {
			delay = DefaultFrameDelay
		}
		frames = append(frames, codec.Frame{Pix: cloneImage(canvas), Delay: delay})
		previousDisposal, previousArea = disposal, frame.Rect
	}
	return frames, nil
}

func (Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	lim := guard.Limits(o.Limits)
	info, blocks, err := probeScan(ctx, r, size, lim)
	if err != nil {
		return nil, err
	}
	if o.Frames == codec.FirstFrame {
		first, err := stdgif.Decode(guard.Stream(ctx, r, size))
		if err != nil {
			return nil, guard.Classify(ctx, err, nil)
		}
		paletted, ok := first.(*image.Paletted)
		if !ok {
			return nil, guard.Corrupt("gif: unexpected first frame type %T", first)
		}
		canvas := image.NewNRGBA(image.Rect(0, 0, info.Width, info.Height))
		drawFrame(canvas, paletted)
		return &codec.Image{
			Info:   info,
			Frames: []codec.Frame{{Pix: canvas, Delay: FrameDelay(blocks.firstDelayCS)}},
			Loop:   PlayCount(blocks.loopCount),
			Scale:  1,
		}, nil
	}
	if err := guard.CheckFrames(lim, blocks.frames, info.Width, info.Height); err != nil {
		return nil, err
	}
	animation, err := stdgif.DecodeAll(guard.Stream(ctx, r, size))
	if err != nil {
		return nil, guard.Classify(ctx, err, nil)
	}
	if len(animation.Image) == 0 {
		return nil, guard.Corrupt("gif: no frames")
	}
	if err := guard.CheckFrames(lim, len(animation.Image), info.Width, info.Height); err != nil {
		return nil, err
	}
	frames, err := composeFrames(ctx, animation, info.Width, info.Height)
	if err != nil {
		return nil, err
	}
	info.Frames = len(frames)
	info.Animated = len(frames) > 1
	return &codec.Image{
		Info:   info,
		Frames: frames,
		Loop:   PlayCount(animation.LoopCount),
		Scale:  1,
	}, nil
}
