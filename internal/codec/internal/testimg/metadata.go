package testimg

import (
	"encoding/binary"

	"github.com/oops1/PixGo/internal/codec"
)

const SampleXMP = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:xmp="http://ns.adobe.com/xap/1.0/" xmp:Rating="3"><dc:title><rdf:Alt><rdf:li xml:lang="x-default">Sample &amp; test</rdf:li></rdf:Alt></dc:title></rdf:Description></rdf:RDF></x:xmpmeta>
<?xpacket end="w"?>`

func SampleEXIF(width, height int) []byte {
	return SampleEXIFOriented(width, height, 1)
}

func SampleEXIFOriented(width, height int, orientation uint16) []byte {
	order := binary.LittleEndian
	make := []byte("PixGo\x00")
	buf := []byte{'I', 'I', 42, 0, 8, 0, 0, 0}
	entry := func(tag, typ uint16, count uint32, value uint32) {
		buf = order.AppendUint16(buf, tag)
		buf = order.AppendUint16(buf, typ)
		buf = order.AppendUint32(buf, count)
		buf = order.AppendUint32(buf, value)
	}
	buf = order.AppendUint16(buf, 3)
	ifd0End := 8 + 2 + 3*12 + 4
	entry(0x010f, 2, uint32(len(make)), uint32(ifd0End))
	entry(0x0112, 3, 1, uint32(orientation))
	entry(0x8769, 4, 1, uint32(ifd0End+len(make)+len(make)&1))
	buf = order.AppendUint32(buf, 0)
	buf = append(buf, make...)
	if len(buf)&1 == 1 {
		buf = append(buf, 0)
	}
	buf = order.AppendUint16(buf, 2)
	entry(0xa002, 4, 1, uint32(width))
	entry(0xa003, 4, 1, uint32(height))
	return order.AppendUint32(buf, 0)
}

func SampleICC() []byte {
	p := make([]byte, 128+4+12)
	binary.BigEndian.PutUint32(p[0:], uint32(len(p)))
	p[8], p[9] = 4, 0x30
	copy(p[12:], "mntr")
	copy(p[16:], "RGB ")
	copy(p[20:], "XYZ ")
	copy(p[36:], "acsp")
	binary.BigEndian.PutUint32(p[128:], 1)
	copy(p[132:], "wtpt")
	return p
}

func SampleMetadata(width, height int) codec.Metadata {
	return codec.Metadata{EXIF: SampleEXIF(width, height), ICC: SampleICC(), XMP: []byte(SampleXMP)}
}
