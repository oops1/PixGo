package meta

import (
	"bytes"
	"context"
	"io"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/isobmff"
)

func profileOf(icc []byte, spaces ...string) []byte {
	if len(icc) < iccHeaderSize || len(icc) > MaxICCBytes || string(icc[36:40]) != "acsp" {
		return nil
	}
	for _, space := range spaces {
		if string(icc[16:20]) == space {
			return icc
		}
	}
	return nil
}

func ColorProfile(icc []byte) []byte { return profileOf(icc, "RGB ", "GRAY") }

func CMYKProfile(icc []byte) []byte { return profileOf(icc, "CMYK") }

func ReadCMYKColor(ctx context.Context, r io.ReaderAt, size int64) codec.ColorInfo {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil || size < 0 || ctx.Err() != nil {
		return codec.ColorInfo{}
	}
	in := input{r: ctxReaderAt{ctx: ctx, r: r}, size: size}
	found := blocks{colorOnly: true}
	scanJPEG(ctx, in, &found)
	if ctx.Err() != nil {
		return codec.ColorInfo{}
	}
	if profile := CMYKProfile(found.icc); profile != nil {
		return codec.ColorInfo{ICC: bytes.Clone(profile)}
	}
	return codec.ColorInfo{}
}

func NewColor(icc []byte, cicp *codec.CICP) codec.ColorInfo {
	var out codec.ColorInfo
	if profile := ColorProfile(icc); profile != nil {
		out.ICC = bytes.Clone(profile)
	}
	if cicp != nil {
		c := *cicp
		out.CICP = &c
	}
	return out
}

func ReadColor(ctx context.Context, r io.ReaderAt, size int64, id codec.ID) codec.ColorInfo {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil || size < 0 || ctx.Err() != nil {
		return codec.ColorInfo{}
	}
	in := input{r: ctxReaderAt{ctx: ctx, r: r}, size: size}
	found := blocks{colorOnly: true}
	switch id {
	case codec.JPEG:
		scanJPEG(ctx, in, &found)
	case codec.PNG, codec.APNG:
		scanPNG(ctx, in, &found)
	case codec.WebP:
		scanWebP(ctx, in, &found)
	default:
		return codec.ColorInfo{}
	}
	if ctx.Err() != nil {
		return codec.ColorInfo{}
	}
	return NewColor(found.icc, found.cicp)
}

type ctxReaderAt struct {
	ctx context.Context
	r   io.ReaderAt
}

func (c ctxReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.ReadAt(p, off)
}

func HEIFProfile(file *isobmff.File, primary *isobmff.Item) []byte {
	if file == nil || primary == nil {
		return nil
	}
	return ColorProfile(heifICC(file, primary))
}
