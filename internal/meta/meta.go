package meta

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	MaxICCBytes = 16 << 20
	MaxXMPBytes = 4 << 20

	maxDimension    = 1<<31 - 1
	memoryWindow    = 1 << 20
	maxContainerOps = 1 << 18
	maxJPEGSegments = 1 << 14
	maxExifBlock    = 8 << 20
	ctxCheckEvery   = 64
)

type Metadata struct {
	Format        codec.ID
	Width, Height int
	Exif          *Exif
	GPS           *GPS
	ICC           *ICC
	XMP           *XMP
}

type FlashMode uint8

const (
	FlashModeUnknown FlashMode = iota
	FlashModeCompulsoryOn
	FlashModeCompulsoryOff
	FlashModeAuto
)

type FlashReturn uint8

const (
	FlashReturnNone FlashReturn = iota
	FlashReturnNotDetected
	FlashReturnDetected
)

type Flash struct {
	Code       int
	Fired      bool
	Mode       FlashMode
	Return     FlashReturn
	RedEye     bool
	NoFunction bool
}

type Exif struct {
	Make, Model, Lens, Software string
	Exposure                    Rational
	FNumber                     Rational
	ISO                         int
	FocalLength                 Rational
	FocalLength35               int
	Flash                       *Flash
	TakenAt                     time.Time
	HasTakenZone                bool
	Orientation                 codec.Orientation
	ExposureProgram             int
	MeteringMode                int
	WhiteBalance                int
	ColorSpace                  int
	PixelWidth, PixelHeight     int
}

type GPS struct {
	Lat, Lon, Alt float64
	HasAlt        bool
	Time          time.Time
}

type ICC struct {
	Description string
	Size        int
	ColorSpace  string
	Class       string
	Version     string
}

type XMP struct {
	Title       string
	Description string
	Rating      int
	Creator     []string
}

func corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: meta: %s", codec.ErrCorrupt, fmt.Sprintf(format, args...))
}

type input struct {
	r    io.ReaderAt
	size int64
}

func (in input) read(offset int64, n int) ([]byte, error) {
	if offset < 0 || n < 0 || offset > in.size || int64(n) > in.size-offset {
		return nil, corrupt("range %d+%d outside %d bytes", offset, n, in.size)
	}
	buf := make([]byte, n)
	if n == 0 {
		return buf, nil
	}
	got, err := in.r.ReadAt(buf, offset)
	if got == n {
		return buf, nil
	}
	if err == nil || err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return nil, fmt.Errorf("%w: meta: %v", codec.ErrCorrupt, err)
}

func (in input) readUpTo(offset, limit int64) ([]byte, error) {
	if offset < 0 || offset > in.size {
		return nil, corrupt("offset %d outside %d bytes", offset, in.size)
	}
	n := in.size - offset
	if n > limit {
		n = limit
	}
	return in.read(offset, int(n))
}

type blocks struct {
	width, height int
	tiffBase      int64
	tiffLen       int64
	tiffData      []byte
	wholeTIFF     bool
	camera        bool
	noSize        bool
	tree          *exif.Tree
	icc           []byte
	xmp           []byte
	cicp          *codec.CICP
	colorOnly     bool
}

func (b *blocks) setSize(w, h int64) {
	if b.width != 0 || b.height != 0 {
		return
	}
	if w <= 0 || h <= 0 || w > maxDimension || h > maxDimension {
		return
	}
	b.width, b.height = int(w), int(h)
}

func (b *blocks) setTIFF(base, length int64) {
	if b.tiffLen != 0 || length < 8 {
		return
	}
	b.tiffBase, b.tiffLen = base, length
}

func (b *blocks) setTIFFBytes(data []byte) {
	if b.tiffLen != 0 || len(data) < 8 {
		return
	}
	b.tiffData, b.tiffLen = data, int64(len(data))
}

func (b *blocks) exifTree(in input) (exif.Tree, bool) {
	if b.tree != nil {
		return *b.tree, true
	}
	if b.tiffLen == 0 {
		return exif.Tree{}, false
	}
	var reader *exif.Reader
	var err error
	if b.tiffData != nil {
		reader, err = exif.NewReader(b.tiffData)
	} else {
		reader, err = openTIFF(in, b.tiffBase, b.tiffLen, b.camera)
	}
	if err != nil {
		return exif.Tree{}, false
	}
	tree, err := reader.Tree()
	return tree, err == nil
}

func Read(ctx context.Context, r io.ReaderAt, size int64, id codec.ID) (*Metadata, error) {
	m := &Metadata{Format: id}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return m, err
	}
	if r == nil || size < 0 {
		return m, corrupt("no input")
	}
	in := input{r: r, size: size}
	var found blocks
	var scanErr error
	switch id {
	case codec.JPEG:
		scanErr = scanJPEG(ctx, in, &found)
	case codec.PNG, codec.APNG:
		scanErr = scanPNG(ctx, in, &found)
	case codec.WebP:
		scanErr = scanWebP(ctx, in, &found)
	case codec.TIFF:
		scanErr = scanTIFF(in, &found)
	case codec.HEIC, codec.AVIF:
		scanErr = scanHEIF(ctx, in, &found)
	case codec.JXL:
		scanErr = scanJXL(ctx, in, &found)
	case codec.RAW:
		scanErr = scanRAW(ctx, in, &found)
	default:
		return m, nil
	}
	if err := ctx.Err(); err != nil {
		return m, err
	}
	finish(in, &found, m)
	if err := ctx.Err(); err != nil {
		return m, err
	}
	return m, scanErr
}

func finish(in input, b *blocks, m *Metadata) {
	icc, xmp := b.icc, b.xmp
	width, height := b.width, b.height
	if tree, ok := b.exifTree(in); ok {
		m.Exif, m.GPS = extractTags(tree)
		if b.wholeTIFF {
			w, h, tiffICC, tiffXMP := tiffPayloads(tree.IFD0)
			if width == 0 && height == 0 && !b.noSize {
				width, height = w, h
			}
			if icc == nil {
				icc = tiffICC
			}
			if xmp == nil {
				xmp = tiffXMP
			}
		}
	}
	m.Width, m.Height = width, height
	if len(icc) > 0 {
		m.ICC = parseICC(icc)
	}
	if len(xmp) > 0 {
		m.XMP = parseXMP(xmp)
	}
}

func openTIFF(in input, base, length int64, camera bool) (*exif.Reader, error) {
	if base < 0 || length < 8 || base > in.size || length > in.size-base {
		return nil, corrupt("TIFF window %d+%d outside %d bytes", base, length, in.size)
	}
	if camera {
		return exif.NewReaderAtCamera(in.r, base, length)
	}
	if length <= memoryWindow {
		buf, err := in.read(base, int(length))
		if err != nil {
			return nil, err
		}
		return exif.NewReader(buf)
	}
	return exif.NewReaderAt(in.r, base, length)
}

func exifPrefixLen(head []byte) int64 {
	if len(head) >= 6 && string(head[:5]) == "Exif\x00" && (head[5] == 0 || head[5] == 0xff) {
		return 6
	}
	return 0
}
