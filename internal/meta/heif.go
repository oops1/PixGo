package meta

import (
	"context"
	"strings"

	"github.com/oops1/PixGo/internal/isobmff"
)

const (
	heifXMPType      = "application/rdf+xml"
	heifICCHeader    = 4
	maxHEIFICCTiles  = 16
	heifSignatureLen = 12
)

func scanHEIF(ctx context.Context, in input, b *blocks) error {
	if in.size < heifSignatureLen {
		return nil
	}
	head, err := in.read(0, heifSignatureLen)
	if err != nil {
		return err
	}
	if string(head[4:8]) != "ftyp" {
		return nil
	}
	file, err := isobmff.Open(ctx, in.r, in.size)
	if err != nil {
		return containerErr(ctx, err)
	}
	primary := file.PrimaryItem()
	if primary != nil {
		if ispe := file.Property(primary, "ispe"); ispe != nil {
			b.setSize(int64(ispe.Width), int64(ispe.Height))
		}
		b.icc = heifICC(file, primary)
	}
	exifItem, xmpItem := heifMetaItems(file, primary)
	var scanErr error
	if exifItem != nil {
		data, err := file.ReadItem(exifItem, maxExifBlock+1)
		switch {
		case err != nil:
			scanErr = containerErr(ctx, err)
		case len(data) <= maxExifBlock:
			b.setTIFFBytes(isoExifTIFF(data))
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if xmpItem != nil {
		data, err := file.ReadItem(xmpItem, MaxXMPBytes+1)
		switch {
		case err != nil:
			if scanErr == nil {
				scanErr = containerErr(ctx, err)
			}
		case len(data) <= MaxXMPBytes:
			b.xmp = data
		}
	}
	return scanErr
}

func heifICC(file *isobmff.File, primary *isobmff.Item) []byte {
	candidates := []*isobmff.Item{primary}
	for i, id := range file.ReferencesFrom("dimg", primary.ID) {
		if i >= maxHEIFICCTiles {
			break
		}
		if tile := file.Items[id]; tile != nil {
			candidates = append(candidates, tile)
		}
	}
	for _, item := range candidates {
		for _, colr := range file.PropertiesOf(item, "colr") {
			payload := colr.Payload
			if len(payload) <= heifICCHeader {
				continue
			}
			if kind := string(payload[:heifICCHeader]); kind != "prof" && kind != "rICC" {
				continue
			}
			if profile := payload[heifICCHeader:]; len(profile) <= MaxICCBytes {
				return profile
			}
		}
	}
	return nil
}

func heifDescribing(file *isobmff.File, primary *isobmff.Item) map[uint32]bool {
	linked := map[uint32]bool{}
	if primary == nil {
		return linked
	}
	for _, ref := range file.References {
		if ref.Type != "cdsc" {
			continue
		}
		for _, to := range ref.To {
			if to == primary.ID {
				linked[ref.From] = true
				break
			}
		}
	}
	return linked
}

func heifMetaItems(file *isobmff.File, primary *isobmff.Item) (exifItem, xmpItem *isobmff.Item) {
	describing := heifDescribing(file, primary)
	var exifLinked, xmpLinked bool
	for _, id := range file.Order {
		item := file.Items[id]
		linked := describing[id]
		switch {
		case item.Type == "Exif":
			if exifItem == nil || (linked && !exifLinked) {
				exifItem, exifLinked = item, linked
			}
		case item.Type == "mime" && strings.EqualFold(item.ContentType, heifXMPType):
			if xmpItem == nil || (linked && !xmpLinked) {
				xmpItem, xmpLinked = item, linked
			}
		}
	}
	return exifItem, xmpItem
}
