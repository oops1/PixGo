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
	Deep    bool
}

type Frame struct {
	Pix   *image.NRGBA
	Deep  *image.NRGBA64
	CMYK  *image.CMYK
	Delay time.Duration
}

type Image struct {
	Info   Info
	Color  ColorInfo
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

type Metadata struct {
	EXIF []byte
	ICC  []byte
	XMP  []byte
}

func (m Metadata) Empty() bool { return len(m.EXIF) == 0 && len(m.ICC) == 0 && len(m.XMP) == 0 }

type EncodeOptions struct {
	Quality  int
	Lossless bool
	Metadata Metadata
}

type Encoder interface {
	ID() ID
	Name() string
	Extension() string
	Encode(w io.Writer, img *image.NRGBA, o EncodeOptions) error
}

type TileReader interface {
	Info() Info
	MinShift() int
	Overview(ctx context.Context, shift int) (*image.NRGBA, error)
	Region(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA, error)
	Close() error
}

type TiledDecoder interface {
	OpenTiled(ctx context.Context, r io.ReaderAt, size int64, lim Limits) (TileReader, error)
}

func LevelSize(w, h, shift int) (int, int) {
	if shift <= 0 {
		return w, h
	}
	d := 1 << uint(shift)
	return (w + d - 1) / d, (h + d - 1) / d
}

type Limits struct {
	MaxFileBytes   int64
	MaxPixels      int64
	MaxFrames      int
	MaxFramePixels int64
	MaxTiledPixels int64
	TiledAbove     int64
}

func DefaultLimits() Limits {
	return Limits{
		MaxFileBytes:   512 << 20,
		MaxPixels:      100_000_000,
		MaxFrames:      10_000,
		MaxFramePixels: 400_000_000,
		MaxTiledPixels: 1_000_000_000,
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
	if l.MaxTiledPixels <= 0 {
		l.MaxTiledPixels = d.MaxTiledPixels
	}
	if l.TiledAbove <= 0 || l.TiledAbove > l.MaxPixels {
		l.TiledAbove = l.MaxPixels
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

type CICP struct {
	Primaries uint8
	Transfer  uint8
	Matrix    uint8
	FullRange bool
}

type ColorInfo struct {
	ICC  []byte
	CICP *CICP
}

func (c ColorInfo) Empty() bool { return len(c.ICC) == 0 && c.CICP == nil }
