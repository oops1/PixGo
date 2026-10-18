package meta

import (
	"bytes"
	"context"
	"io"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta/exif"
)

func descriptiveTag(tag uint16) bool {
	switch tag {
	case 0x010e, 0x010f, 0x0110, tagOrientation, 0x011a, 0x011b, 0x0128, 0x0131, 0x0132, 0x013b, 0x013c, 0x0213, 0x8298:
		return true
	}
	return false
}

func notMakerNote(tag uint16) bool { return tag != tagMakerNote }

func docFromTree(tree exif.Tree) []byte {
	if tree.IFD0 == nil {
		return nil
	}
	var order tiffOrder = orderOf(nil)
	switch {
	case len(tree.IFD0.Entries) > 0:
		order = orderOf(tree.IFD0.Entries[0].ByteOrder())
	case tree.Exif != nil && len(tree.Exif.Entries) > 0:
		order = orderOf(tree.Exif.Entries[0].ByteOrder())
	}
	doc := &tiffDoc{order: order, ifd0: convertDir(tree.IFD0, order, descriptiveTag)}
	if tree.Exif != nil {
		doc.exif = convertDir(tree.Exif, order, notMakerNote)
	}
	if tree.GPS != nil {
		doc.gps = convertDir(tree.GPS, order, nil)
	}
	if len(doc.ifd0.entries) == 0 && doc.exif == nil && doc.gps == nil {
		return nil
	}
	return doc.encode(false)
}

func ReadBlocks(r io.ReaderAt, size int64, id codec.ID) (codec.Metadata, error) {
	return ReadBlocksContext(context.Background(), r, size, id)
}

func ReadBlocksContext(ctx context.Context, r io.ReaderAt, size int64, id codec.ID) (codec.Metadata, error) {
	var out codec.Metadata
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if r == nil || size < 0 {
		return out, corrupt("no input")
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
		return out, nil
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	icc, xmp := found.icc, found.xmp
	tree, haveTree := found.exifTree(in)
	if found.wholeTIFF && haveTree {
		_, _, tiffICC, tiffXMP := tiffPayloads(tree.IFD0)
		if icc == nil {
			icc = tiffICC
		}
		if xmp == nil {
			xmp = tiffXMP
		}
	}
	if len(icc) > 0 && len(icc) <= MaxICCBytes {
		out.ICC = bytes.Clone(icc)
	}
	if len(xmp) > 0 && len(xmp) <= MaxXMPBytes {
		out.XMP = bytes.Clone(xmp)
	}
	switch {
	case found.tree != nil || (found.wholeTIFF && haveTree):
		out.EXIF = docFromTree(tree)
	case found.tiffData != nil:
		out.EXIF = bytes.Clone(found.tiffData)
	case found.tiffLen > 0 && found.tiffLen <= maxExifBlock:
		if data, err := in.read(found.tiffBase, int(found.tiffLen)); err == nil {
			out.EXIF = data
		} else if scanErr == nil {
			scanErr = err
		}
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	return out, scanErr
}
