package meta

import (
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	iccHeaderSize   = 128
	iccMaxTags      = 4096
	iccMaxRecords   = 256
	iccMaxDescBytes = 512
)

var iccClasses = map[string]string{
	"scnr": "Input",
	"mntr": "Display",
	"prtr": "Output",
	"link": "DeviceLink",
	"spac": "ColorSpace",
	"abst": "Abstract",
	"nmcl": "NamedColor",
}

var iccSpaces = map[string]string{
	"YCbr": "YCbCr",
}

func signatureText(sig []byte) string {
	for _, c := range sig {
		if c < 0x20 || c > 0x7e {
			return ""
		}
	}
	return strings.TrimSpace(string(sig))
}

func parseICC(data []byte) *ICC {
	if len(data) < iccHeaderSize || string(data[36:40]) != "acsp" {
		return nil
	}
	icc := &ICC{Size: len(data)}
	icc.Version = fmt.Sprintf("%d.%d", data[8], data[9]>>4)
	if bugfix := data[9] & 0x0f; bugfix != 0 {
		icc.Version += fmt.Sprintf(".%d", bugfix)
	}
	class := string(data[12:16])
	if name, ok := iccClasses[class]; ok {
		icc.Class = name
	} else {
		icc.Class = signatureText(data[12:16])
	}
	space := string(data[16:20])
	if name, ok := iccSpaces[space]; ok {
		icc.ColorSpace = name
	} else {
		icc.ColorSpace = signatureText(data[16:20])
	}
	icc.Description = clipText(iccDescription(data), iccMaxDescBytes)
	return icc
}

func iccDescription(data []byte) string {
	if len(data) < iccHeaderSize+4 {
		return ""
	}
	count := uint64(binary.BigEndian.Uint32(data[iccHeaderSize : iccHeaderSize+4]))
	if count > iccMaxTags {
		count = iccMaxTags
	}
	table := data[iccHeaderSize+4:]
	if limit := uint64(len(table) / 12); count > limit {
		count = limit
	}
	for i := uint64(0); i < count; i++ {
		entry := table[i*12 : i*12+12]
		if string(entry[:4]) != "desc" {
			continue
		}
		offset := uint64(binary.BigEndian.Uint32(entry[4:8]))
		size := uint64(binary.BigEndian.Uint32(entry[8:12]))
		if offset > uint64(len(data)) || size > uint64(len(data))-offset || size < 12 {
			return ""
		}
		return iccTagText(data[offset : offset+size])
	}
	return ""
}

func iccTagText(tag []byte) string {
	switch string(tag[:4]) {
	case "text":
		return decodeText(tag[8:])
	case "desc":
		n := uint64(binary.BigEndian.Uint32(tag[8:12]))
		body := tag[12:]
		if n > uint64(len(body)) {
			n = uint64(len(body))
		}
		return decodeText(body[:n])
	case "mluc":
		return iccMultiLocalized(tag)
	}
	return ""
}

func iccMultiLocalized(tag []byte) string {
	if len(tag) < 16 {
		return ""
	}
	count := uint64(binary.BigEndian.Uint32(tag[8:12]))
	recordSize := uint64(binary.BigEndian.Uint32(tag[12:16]))
	if recordSize < 12 || count == 0 || count > iccMaxRecords || 16+count*recordSize > uint64(len(tag)) {
		return ""
	}
	first := ""
	for i := uint64(0); i < count; i++ {
		record := tag[16+i*recordSize : 16+i*recordSize+12]
		length := uint64(binary.BigEndian.Uint32(record[4:8]))
		offset := uint64(binary.BigEndian.Uint32(record[8:12]))
		if offset > uint64(len(tag)) || length > uint64(len(tag))-offset {
			continue
		}
		text := utf16BE(tag[offset : offset+length])
		if text == "" {
			continue
		}
		if record[0] == 'e' && record[1] == 'n' {
			return text
		}
		if first == "" {
			first = text
		}
	}
	return first
}
