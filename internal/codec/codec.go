package codec

import (
	"context"
	"errors"
	"image"
	"io"
	"time"
)

type ID string

const (
	JPEG ID = "jpeg"
	PNG  ID = "png"
	APNG ID = "apng"
	GIF  ID = "gif"
	BMP  ID = "bmp"
	TIFF ID = "tiff"
	WebP ID = "webp"
	SVG  ID = "svg"
	ICO  ID = "ico"
	CUR  ID = "cur"
	AVIF ID = "avif"
	HEIC ID = "heic"
	JXL  ID = "jxl"
	RAW  ID = "raw"
)

var (
	ErrUnknownFormat = errors.New("codec: unknown format")
	ErrTooLarge      = errors.New("codec: image exceeds limits")
	ErrCorrupt       = errors.New("codec: corrupt or truncated data")
	ErrUnsupported   = errors.New("codec: unsupported variant")
)

type Info struct {
	Format      ID
	FormatName  string
	Width       int
	Height      int
	Frames      int
	Animated    bool
	HasAlpha    bool
	Orientation Orientation
	BitDepth    int
	Vector      bool
	Preview     bool
}

type FrameSel int

const (
	FirstFrame FrameSel = iota
	AllFrames
)

type Options struct {
	Limits  Limits
	Scale   int
	Frames  FrameSel
	Region  *image.Rectangle
	VectorW int
	VectorH int
}

type Frame struct {
	Pix   *image.NRGBA
	Delay time.Duration
}

type Image struct {
	Info   Info
	Frames []Frame
	Loop   int
	Scale  int
}

func (im *Image) First() *image.NRGBA {
	if im == nil || len(im.Frames) == 0 {
		return nil
	}
	return im.Frames[0].Pix
}

type Decoder interface {
	ID() ID
	Name() string
	Extensions() []string
	Sniff(head []byte) bool
	Probe(ctx context.Context, r io.ReaderAt, size int64, lim Limits) (Info, error)
	Decode(ctx context.Context, r io.ReaderAt, size int64, o Options) (*Image, error)
}

type EncodeOptions struct {
	Quality  int
	Lossless bool
}

type Encoder interface {
	ID() ID
	Name() string
	Extension() string
	Encode(w io.Writer, img *image.NRGBA, o EncodeOptions) error
}

type Limits struct {
	MaxFileBytes   int64
	MaxPixels      int64
	MaxFrames      int
	MaxFramePixels int64
}

func DefaultLimits() Limits {
	return Limits{
		MaxFileBytes:   512 << 20,
		MaxPixels:      100_000_000,
		MaxFrames:      10_000,
		MaxFramePixels: 400_000_000,
	}
}

func (l Limits) WithDefaults() Limits {
	d := DefaultLimits()
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = d.MaxFileBytes
	}
	if l.MaxPixels <= 0 {
		l.MaxPixels = d.MaxPixels
	}
	if l.MaxFrames <= 0 {
		l.MaxFrames = d.MaxFrames
	}
	if l.MaxFramePixels <= 0 {
		l.MaxFramePixels = d.MaxFramePixels
	}
	return l
}

func (l Limits) Check(w, h int) error {
	l = l.WithDefaults()
	if w <= 0 || h <= 0 {
		return ErrCorrupt
	}
	if int64(w)*int64(h) > l.MaxPixels {
		return ErrTooLarge
	}
	return nil
}

const SniffLen = 4096
