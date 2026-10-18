package jpegxform

import (
	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta"
)

const maxSegmentPayload = 65533

func rewriteSegment(s segment, pl *plan, o Options, change exifChange) ([]byte, bool) {
	if s.marker != markerAPP0+1 {
		return s.payload, true
	}
	ro := meta.RewriteOptions{Width: pl.outW, Height: pl.outH, StripGPS: o.StripGPS, SwapResolution: change.swapResolution}
	switch {
	case isExif(s.payload):
		res := meta.Rewrite(codec.Metadata{EXIF: s.payload[exifPrefix:]}, ro).EXIF
		if len(res) > 0 && exifPrefix+len(res) <= maxSegmentPayload {
			return append(append([]byte{}, s.payload[:exifPrefix]...), res...), true
		}
	case isXMP(s.payload):
		res := meta.Rewrite(codec.Metadata{XMP: s.payload[len(xmpPrefix):]}, ro).XMP
		if len(res) > 0 && len(xmpPrefix)+len(res) <= maxSegmentPayload {
			return append(append([]byte{}, s.payload[:len(xmpPrefix)]...), res...), true
		}
	default:
		return s.payload, true
	}
	if o.StripGPS {
		return nil, false
	}
	fixExifSegment(s.payload, change)
	fixXMPSegment(s.payload)
	return s.payload, true
}
