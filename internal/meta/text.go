package meta

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

const maxTextBytes = 1 << 12

var cp1252High = [32]rune{
	0x20ac, 0, 0x201a, 0x0192, 0x201e, 0x2026, 0x2020, 0x2021,
	0x02c6, 0x2030, 0x0160, 0x2039, 0x0152, 0, 0x017d, 0,
	0, 0x2018, 0x2019, 0x201c, 0x201d, 0x2022, 0x2013, 0x2014,
	0x02dc, 0x2122, 0x0161, 0x203a, 0x0153, 0, 0x017e, 0x0178,
}

func decodeText(raw []byte) string {
	if i := bytes.IndexByte(raw, 0); i >= 0 {
		raw = raw[:i]
	}
	if len(raw) > maxTextBytes {
		raw = raw[:maxTextBytes]
	}
	var text string
	if utf8.Valid(raw) {
		text = string(raw)
	} else {
		var sb strings.Builder
		for _, c := range raw {
			switch {
			case c >= 0x80 && c < 0xa0:
				if r := cp1252High[c-0x80]; r != 0 {
					sb.WriteRune(r)
				}
			default:
				sb.WriteRune(rune(c))
			}
		}
		text = sb.String()
	}
	return cleanText(text)
}

func cleanText(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return ' '
		case unicode.IsControl(r) || r == utf8.RuneError || r == 0xfeff:
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

func clipText(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func utf16BE(raw []byte) string {
	if len(raw)%2 == 1 {
		raw = raw[:len(raw)-1]
	}
	if len(raw) > 2*maxTextBytes {
		raw = raw[:2*maxTextBytes]
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[2*i])<<8 | uint16(raw[2*i+1])
	}
	for i, u := range units {
		if u == 0 {
			units = units[:i]
			break
		}
	}
	return cleanText(string(utf16.Decode(units)))
}
