package meta

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	nsDC   = "http://purl.org/dc/elements/1.1/"
	nsXMP  = "http://ns.adobe.com/xap/1.0/"
	nsTIFF = "http://ns.adobe.com/tiff/1.0/"
	nsRDF  = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	nsXML  = "http://www.w3.org/XML/1998/namespace"

	xmpMaxDepth     = 64
	xmpMaxPropDepth = 16
	xmpMaxTokens    = 1 << 20
	xmpMaxCreators  = 32
	xmpMaxText      = 4096
	xmpMaxItemBytes = 1 << 14
)

var errXMPTooDeep = errors.New("xmp: nesting too deep")

type propKind uint8

const (
	propNone propKind = iota
	propTitle
	propDescription
	propCreator
	propRating
	propImageDescription
)

type liItem struct{ lang, text string }

func inNS(space, uri, prefix string) bool { return space == uri || space == prefix }

func classify(name xml.Name) propKind {
	switch {
	case inNS(name.Space, nsDC, "dc"):
		switch name.Local {
		case "title":
			return propTitle
		case "description":
			return propDescription
		case "creator":
			return propCreator
		}
	case inNS(name.Space, nsXMP, "xmp"):
		if name.Local == "Rating" {
			return propRating
		}
	case inNS(name.Space, nsTIFF, "tiff"):
		if name.Local == "ImageDescription" {
			return propImageDescription
		}
	}
	return propNone
}

func xmpText(s string) string {
	s = clipText(s, xmpMaxText)
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t':
			return ' '
		case unicode.IsControl(r) || r == utf8.RuneError || r == 0xfeff:
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

func prepareXMP(data []byte) []byte {
	switch {
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		data = decodeUTF16(data[2:], true)
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		data = decodeUTF16(data[2:], false)
	case bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}):
		data = data[3:]
	}
	data = bytes.TrimRight(data, "\x00")
	out := []byte(strings.ToValidUTF8(string(data), "�"))
	for i, c := range out {
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			out[i] = ' '
		}
	}
	return out
}

func decodeUTF16(raw []byte, bigEndian bool) []byte {
	if len(raw)%2 == 1 {
		raw = raw[:len(raw)-1]
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		if bigEndian {
			units[i] = uint16(raw[2*i])<<8 | uint16(raw[2*i+1])
		} else {
			units[i] = uint16(raw[2*i+1])<<8 | uint16(raw[2*i])
		}
	}
	return []byte(string(utf16.Decode(units)))
}

func langOf(attrs []xml.Attr) string {
	for _, a := range attrs {
		if a.Name.Local == "lang" && (a.Name.Space == "xml" || a.Name.Space == nsXML) {
			return a.Value
		}
	}
	return ""
}

func readProperty(dec *xml.Decoder) ([]liItem, string, error) {
	var (
		items  []liItem
		direct strings.Builder
		item   strings.Builder
		lang   string
		inItem int
		depth  = 1
	)
	for {
		tok, err := dec.Token()
		if err != nil {
			return items, direct.String(), err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth > xmpMaxPropDepth {
				return items, direct.String(), errXMPTooDeep
			}
			if t.Name.Local == "li" && inNS(t.Name.Space, nsRDF, "rdf") && inItem == 0 {
				inItem = depth
				lang = langOf(t.Attr)
				item.Reset()
			}
		case xml.CharData:
			switch {
			case inItem > 0:
				if item.Len() < xmpMaxItemBytes {
					item.Write(t)
				}
			case depth == 1:
				if direct.Len() < xmpMaxItemBytes {
					direct.Write(t)
				}
			}
		case xml.EndElement:
			if inItem == depth {
				items = append(items, liItem{lang: lang, text: item.String()})
				inItem = 0
			}
			depth--
			if depth == 0 {
				return items, direct.String(), nil
			}
		}
	}
}

type xmpParser struct {
	title, description, imageDescription string
	creators                             []string
	rating                               int
	hasRating                            bool
}

func pickLang(items []liItem, direct string) string {
	for _, it := range items {
		if it.lang == "x-default" {
			if text := xmpText(it.text); text != "" {
				return text
			}
		}
	}
	for _, it := range items {
		if text := xmpText(it.text); text != "" {
			return text
		}
	}
	return xmpText(direct)
}

func (p *xmpParser) setRating(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || v < -1 || v > 5 {
		return
	}
	p.rating, p.hasRating = int(math.Round(v)), true
}

func (p *xmpParser) store(kind propKind, items []liItem, direct string) {
	switch kind {
	case propTitle:
		if p.title == "" {
			p.title = pickLang(items, direct)
		}
	case propDescription:
		if p.description == "" {
			p.description = pickLang(items, direct)
		}
	case propImageDescription:
		if p.imageDescription == "" {
			p.imageDescription = pickLang(items, direct)
		}
	case propRating:
		p.setRating(direct)
	case propCreator:
		if len(p.creators) > 0 {
			return
		}
		for _, it := range items {
			if text := xmpText(it.text); text != "" && len(p.creators) < xmpMaxCreators {
				p.creators = append(p.creators, text)
			}
		}
		if len(p.creators) == 0 {
			if text := xmpText(direct); text != "" {
				p.creators = append(p.creators, text)
			}
		}
	}
}

func (p *xmpParser) attribute(name xml.Name, value string) {
	switch kind := classify(name); kind {
	case propTitle, propDescription, propImageDescription:
		p.store(kind, []liItem{{lang: "x-default", text: value}}, "")
	case propRating:
		p.setRating(value)
	case propCreator:
		p.store(kind, nil, value)
	}
}

func parseXMP(data []byte) *XMP {
	dec := xml.NewDecoder(bytes.NewReader(prepareXMP(data)))
	dec.Strict = false
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	var p xmpParser
	depth := 0
walk:
	for tokens := 0; tokens < xmpMaxTokens; tokens++ {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth > xmpMaxDepth {
				break walk
			}
			for _, a := range t.Attr {
				p.attribute(a.Name, a.Value)
			}
			if kind := classify(t.Name); kind != propNone {
				items, direct, err := readProperty(dec)
				depth--
				p.store(kind, items, direct)
				if err != nil {
					break walk
				}
			}
		case xml.EndElement:
			depth--
		}
	}
	description := p.description
	if description == "" {
		description = p.imageDescription
	}
	if p.title == "" && description == "" && len(p.creators) == 0 && !p.hasRating {
		return nil
	}
	return &XMP{Title: p.title, Description: description, Rating: p.rating, Creator: p.creators}
}
