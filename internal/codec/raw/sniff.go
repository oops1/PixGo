package raw

import (
	"bytes"
	"encoding/binary"
	"strings"
)

const (
	tiffHeaderSize = 8
	ifdEntrySize   = 12
	maxSniffTags   = 256
	typeASCII      = 2
)

var cameraMakes = []string{
	"nikon", "sony", "pentax", "ricoh", "olympus", "om digital", "om system", "panasonic", "leica", "fujifilm",
}

type container uint8

const (
	containerNone container = iota
	containerTIFF
	containerCR3
	containerRAF
)

func orderOf(head []byte) binary.ByteOrder {
	switch {
	case head[0] == 'I' && head[1] == 'I':
		return binary.LittleEndian
	case head[0] == 'M' && head[1] == 'M':
		return binary.BigEndian
	}
	return nil
}

func cameraMagic(magic uint16) bool {
	switch magic {
	case 42, 0x4f52, 0x5352, 0x0055:
		return true
	}
	return false
}

func identify(head []byte) container {
	switch {
	case bytes.HasPrefix(head, []byte(rafSignature)):
		return containerRAF
	case len(head) >= 12 && string(head[4:8]) == "ftyp" && string(head[8:12]) == "crx ":
		return containerCR3
	case len(head) >= tiffHeaderSize:
		if order := orderOf(head); order != nil && cameraMagic(order.Uint16(head[2:4])) {
			return containerTIFF
		}
	}
	return containerNone
}

func isMake(name string) bool {
	name = strings.ToLower(name)
	for _, vendor := range cameraMakes {
		if strings.Contains(name, vendor) {
			return true
		}
	}
	return false
}

func sniffTIFF(head []byte) bool {
	order := orderOf(head)
	magic := order.Uint16(head[2:4])
	if magic != 42 {
		return true
	}
	if order == binary.LittleEndian && len(head) >= 10 && head[8] == 'C' && head[9] == 'R' {
		return true
	}
	first := int64(order.Uint32(head[4:8]))
	if first < tiffHeaderSize || first+2 > int64(len(head)) {
		return false
	}
	count := int(order.Uint16(head[first : first+2]))
	if count > maxSniffTags {
		return false
	}
	var maker string
	subIFDs := false
	for i := 0; i < count; i++ {
		at := first + 2 + int64(i)*ifdEntrySize
		if at+ifdEntrySize > int64(len(head)) {
			break
		}
		entry := head[at : at+ifdEntrySize]
		switch order.Uint16(entry[0:2]) {
		case tagDNGVersion:
			return true
		case tagSubIFDs:
			subIFDs = true
		case tagMake:
			maker = sniffASCII(head, order, entry)
		}
	}
	if maker != "" {
		return isMake(maker)
	}
	return subIFDs
}

func sniffASCII(head []byte, order binary.ByteOrder, entry []byte) string {
	if order.Uint16(entry[2:4]) != typeASCII {
		return ""
	}
	count := int64(order.Uint32(entry[4:8]))
	var text []byte
	switch {
	case count <= 4:
		text = entry[8 : 8+count]
	default:
		start := int64(order.Uint32(entry[8:12]))
		if start+count > int64(len(head)) {
			return ""
		}
		text = head[start : start+count]
	}
	if end := bytes.IndexByte(text, 0); end >= 0 {
		text = text[:end]
	}
	return string(text)
}
