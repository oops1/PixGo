package meta

import (
	"bytes"
	"context"

	"github.com/oops1/PixGo/internal/isobmff"
)

var jxlSignature = []byte("\x00\x00\x00\x0cJXL \x0d\x0a\x87\x0a")

func scanJXL(ctx context.Context, in input, b *blocks) error {
	signature := int64(len(jxlSignature))
	if in.size < signature {
		return nil
	}
	head, err := in.read(0, len(jxlSignature))
	if err != nil {
		return err
	}
	if !bytes.Equal(head, jxlSignature) {
		return nil
	}
	var (
		scanErr  error
		haveExif bool
		haveXMP  bool
	)
	fail := func(err error) {
		if scanErr == nil {
			scanErr = err
		}
	}
	err = isobmff.Walk(ctx, in.r, in.size, signature, in.size, maxContainerOps, func(s isobmff.Span) error {
		switch s.Type {
		case "Exif":
			if haveExif {
				return nil
			}
			haveExif = true
			data, ok, err := in.readBlock(s.Payload, s.Len(), maxExifBlock)
			if err != nil {
				fail(err)
			} else if ok {
				b.setTIFFBytes(isoExifTIFF(data))
			}
		case "xml ":
			if haveXMP {
				return nil
			}
			haveXMP = true
			data, ok, err := in.readBlock(s.Payload, s.Len(), MaxXMPBytes)
			if err != nil {
				fail(err)
			} else if ok {
				b.xmp = data
			}
		}
		return nil
	})
	if err != nil {
		fail(containerErr(ctx, err))
	}
	return scanErr
}
