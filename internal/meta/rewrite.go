package meta

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/oops1/PixGo/internal/codec"
)

const (
	MaxJPEGExifBytes = 65533 - 6
	MaxJPEGXMPBytes  = 65533 - 29
	MaxJPEGICCBytes  = 255 * (65533 - 14)
)

var ErrDropped = errors.New("meta: some metadata was dropped")

type DroppedError struct {
	EXIF, ICC, XMP bool
	Reason         string
}

func (e *DroppedError) Error() string {
	var parts []string
	if e.EXIF {
		parts = append(parts, "EXIF")
	}
	if e.ICC {
		parts = append(parts, "ICC profile")
	}
	if e.XMP {
		parts = append(parts, "XMP")
	}
	return fmt.Sprintf("meta: dropped %s: %s", strings.Join(parts, ", "), e.Reason)
}

func (e *DroppedError) Unwrap() error { return ErrDropped }

type RewriteOptions struct {
	Width, Height  int
	StripGPS       bool
	KeepThumbnail  bool
	SwapResolution bool
}

func Rewrite(m codec.Metadata, o RewriteOptions) codec.Metadata {
	out, _ := rewrite(m, o)
	return out
}

func RewriteFor(m codec.Metadata, o RewriteOptions, id codec.ID) (codec.Metadata, error) {
	out, brokenEXIF := rewrite(m, o)
	fitted, err := Fit(out, id)
	var dropped *DroppedError
	if !errors.As(err, &dropped) {
		dropped = &DroppedError{}
	}
	if brokenEXIF {
		dropped.EXIF = true
		if dropped.Reason == "" {
			dropped.Reason = "the EXIF block could not be read"
		}
	}
	if dropped.EXIF || dropped.ICC || dropped.XMP {
		return fitted, dropped
	}
	return fitted, nil
}

func rewrite(m codec.Metadata, o RewriteOptions) (codec.Metadata, bool) {
	var out codec.Metadata
	broken := false
	if len(m.EXIF) > 0 {
		if data, ok := rewriteEXIF(m.EXIF, o); ok {
			out.EXIF = data
		} else {
			broken = true
		}
	}
	if len(m.ICC) > 0 {
		out.ICC = bytes.Clone(m.ICC)
	}
	if len(m.XMP) > 0 {
		out.XMP = rewriteXMP(m.XMP, o)
	}
	return out, broken
}

func stripExifHeader(raw []byte) []byte {
	if n := exifPrefixLen(raw); n > 0 {
		return raw[n:]
	}
	return raw
}

func rewriteEXIF(raw []byte, o RewriteOptions) ([]byte, bool) {
	doc, err := parseTIFF(stripExifHeader(raw))
	if err != nil {
		return nil, false
	}
	doc.setOrientation(doc.ifd0)
	if doc.ifd1 != nil {
		doc.setOrientation(doc.ifd1)
	}
	if o.Width > 0 && o.Height > 0 {
		doc.setSize(doc.ifd0, tagImageWidth, o.Width)
		doc.setSize(doc.ifd0, tagImageLength, o.Height)
		if doc.exif != nil {
			doc.setSize(doc.exif, tagPixelXDimension, o.Width)
			doc.setSize(doc.exif, tagPixelYDimension, o.Height)
		}
		if doc.interop != nil {
			doc.setSize(doc.interop, tagRelatedWidth, o.Width)
			doc.setSize(doc.interop, tagRelatedLength, o.Height)
		}
	}
	if o.SwapResolution {
		swapEntries(doc.ifd0, 0x011a, 0x011b)
		swapEntries(doc.exif, 0xa20e, 0xa20f)
		swapEntries(doc.ifd1, 0x011a, 0x011b)
	}
	if o.StripGPS {
		doc.gps = nil
	}
	if !o.KeepThumbnail {
		doc.ifd1, doc.thumb = nil, nil
	}
	data := doc.encode(true)
	if len(data) > MaxJPEGExifBytes && doc.exif != nil && doc.exif.index(tagMakerNote) >= 0 {
		doc.exif.remove(tagMakerNote)
		data = doc.encode(false)
	}
	return data, true
}

func Fit(m codec.Metadata, id codec.ID) (codec.Metadata, error) {
	out := m
	var dropped DroppedError
	reasons := []string{}
	if len(out.EXIF) > 0 && !validTIFFHeader(out.EXIF) {
		out.EXIF = nil
		dropped.EXIF = true
		reasons = append(reasons, "the EXIF block is not a TIFF structure")
	}
	if id == codec.JPEG {
		if len(out.EXIF) > MaxJPEGExifBytes {
			out.EXIF = nil
			dropped.EXIF = true
			reasons = append(reasons, "the EXIF block does not fit in a JPEG segment")
		}
		if len(out.XMP) > MaxJPEGXMPBytes {
			out.XMP = nil
			dropped.XMP = true
			reasons = append(reasons, "the XMP packet does not fit in a JPEG segment")
		}
		if len(out.ICC) > MaxJPEGICCBytes {
			out.ICC = nil
			dropped.ICC = true
			reasons = append(reasons, "the colour profile is too large for a JPEG")
		}
	}
	if len(out.ICC) > MaxICCBytes {
		out.ICC = nil
		dropped.ICC = true
		reasons = append(reasons, "the colour profile is too large")
	}
	if len(out.XMP) > MaxXMPBytes {
		out.XMP = nil
		dropped.XMP = true
		reasons = append(reasons, "the XMP packet is too large")
	}
	if len(out.EXIF) > maxExifBlock {
		out.EXIF = nil
		dropped.EXIF = true
		reasons = append(reasons, "the EXIF block is too large")
	}
	if dropped.EXIF || dropped.ICC || dropped.XMP {
		dropped.Reason = strings.Join(reasons, "; ")
		return out, &dropped
	}
	return out, nil
}

func validTIFFHeader(b []byte) bool {
	return len(b) >= 8 && (b[0] == 'I' && b[1] == 'I' && b[2] == 42 && b[3] == 0 || b[0] == 'M' && b[1] == 'M' && b[2] == 0 && b[3] == 42)
}

var (
	xmpOrientationAttr = regexp.MustCompile(`(tiff:Orientation\s*=\s*)(["'])\s*[0-9]+\s*(["'])`)
	xmpOrientationElem = regexp.MustCompile(`(<tiff:Orientation\s*>\s*)[0-9]+(\s*</tiff:Orientation\s*>)`)
	xmpGPSAttr         = regexp.MustCompile(`\s+exif:GPS[A-Za-z0-9]*\s*=\s*(?:"[^"]*"|'[^']*')`)
)

type xmpSizeEdit struct {
	attr, elem *regexp.Regexp
	height     bool
}

func newSizeEdit(name string, height bool) xmpSizeEdit {
	q := regexp.QuoteMeta(name)
	return xmpSizeEdit{
		attr:   regexp.MustCompile(`(` + q + `s*=s*)(["'])s*[0-9]+s*(["'])`),
		elem:   regexp.MustCompile(`(<` + q + `s*>s*)[0-9]+(s*</` + q + `s*>)`),
		height: height,
	}
}

var xmpSizeEdits = []xmpSizeEdit{
	newSizeEdit("exif:PixelXDimension", false),
	newSizeEdit("exif:PixelYDimension", true),
	newSizeEdit("tiff:ImageWidth", false),
	newSizeEdit("tiff:ImageLength", true),
}

func rewriteXMP(xmp []byte, o RewriteOptions) []byte {
	text := xmp
	if bytes.HasPrefix(text, []byte{0xfe, 0xff}) {
		text = decodeUTF16(text[2:], true)
	} else if bytes.HasPrefix(text, []byte{0xff, 0xfe}) {
		text = decodeUTF16(text[2:], false)
	} else if bytes.HasPrefix(text, []byte{0xef, 0xbb, 0xbf}) {
		text = text[3:]
	}
	s := string(text)
	s = xmpOrientationAttr.ReplaceAllString(s, `${1}${2}1${3}`)
	s = xmpOrientationElem.ReplaceAllString(s, `${1}1${2}`)
	if o.Width > 0 && o.Height > 0 {
		for _, e := range xmpSizeEdits {
			v := o.Width
			if e.height {
				v = o.Height
			}
			value := strconv.Itoa(v)
			s = e.attr.ReplaceAllString(s, `${1}${2}`+value+`${3}`)
			s = e.elem.ReplaceAllString(s, `${1}`+value+`${2}`)
		}
	}
	if o.StripGPS {
		s = xmpGPSAttr.ReplaceAllString(s, "")
		s = stripGPSElements(s)
	}
	return []byte(s)
}

func stripGPSElements(s string) string {
	const open = "<exif:GPS"
	var out strings.Builder
	for {
		i := strings.Index(s, open)
		if i < 0 {
			out.WriteString(s)
			return out.String()
		}
		nameEnd := i + len(open)
		for nameEnd < len(s) && isXMLNameByte(s[nameEnd]) {
			nameEnd++
		}
		name := s[i+1 : nameEnd]
		tagEnd := strings.IndexByte(s[nameEnd:], '>')
		if tagEnd < 0 {
			out.WriteString(s[:i])
			return out.String()
		}
		tagEnd += nameEnd
		end := tagEnd + 1
		if s[tagEnd-1] != '/' {
			closing := "</" + name
			j := strings.Index(s[end:], closing)
			if j < 0 {
				out.WriteString(s[:i])
				s = s[end:]
				continue
			}
			k := strings.IndexByte(s[end+j:], '>')
			if k < 0 {
				out.WriteString(s[:i])
				s = s[end:]
				continue
			}
			end = end + j + k + 1
		}
		start := i
		for start > 0 && (s[start-1] == ' ' || s[start-1] == '\t') {
			start--
		}
		if start > 0 && s[start-1] == '\n' && end < len(s) && s[end] == '\n' {
			end++
		}
		out.WriteString(s[:start])
		s = s[end:]
	}
}

func isXMLNameByte(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == ':' || c == '.'
}
