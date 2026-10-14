package raw

import (
	"bytes"
	"encoding/binary"
	"testing"

	xtiff "golang.org/x/image/tiff"

	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func plainTIFF(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := xtiff.Encode(&buf, testimg.Noise(40, 30, 1, false), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tiffWithMake(order byteOrder, name string, extra ...entry) []byte {
	f := standardTIFF(order)
	entries := append([]entry{longEntry(0x0100, 64)}, extra...)
	if name != "" {
		entries = append(entries, f.asciiEntry(tagMake, name))
	}
	ifd0, _ := f.ifd(entries...)
	f.setFirst(ifd0)
	return f.data()
}

func lateMake(extra ...entry) []byte {
	f := standardTIFF(little)
	entries := append([]entry{{tag: tagMake, typ: typeASCIIType, count: 11, value: 1000}}, extra...)
	ifd0, _ := f.ifd(entries...)
	f.setFirst(ifd0)
	return f.data()
}

func withBytes(data []byte, at int, patch ...byte) []byte {
	out := append([]byte(nil), data...)
	copy(out[at:], patch)
	return out
}

func TestSniffRecognisesEveryContainer(t *testing.T) {
	p := makePreviews(t)
	orf := orfFile(p, "new")
	cases := []struct {
		name string
		data []byte
	}{
		{"CR2", cr2File(p, 0)},
		{"NEF little endian", nefFile(little, p, 0)},
		{"NEF big endian", nefFile(big, p, 0)},
		{"ARW", arwFile(p)},
		{"DNG little endian", dngFile(little, p, 0)},
		{"DNG big endian", dngFile(big, p, 0)},
		{"RW2", rw2File(p)},
		{"ORF IIRO", orf},
		{"ORF IIRS", withBytes(orf, 2, 'R', 'S')},
		{"ORF MMOR", withBytes(orf, 0, 'M', 'M', 'O', 'R')},
		{"PEF", tiffWithMake(big, "PENTAX Corporation", entry{tag: tagSubIFDs, typ: typeLong, count: 1, value: 8})},
		{"Ricoh PEF", tiffWithMake(little, "RICOH IMAGING COMPANY, LTD.")},
		{"Olympus OM", tiffWithMake(little, "OM Digital Solutions")},
		{"Leica DNG", tiffWithMake(little, "", dngVersion())},
		{"Fujifilm DNG", tiffWithMake(big, "FUJIFILM")},
		{"CR3", cr3File(p, 0)},
		{"RAF", rafFile(p.large)},
		{"SubIFDs with a make beyond the sniff window", lateMake(entry{tag: tagSubIFDs, typ: typeLong, count: 1, value: 8})},
	}
	for _, c := range cases {
		if !(Decoder{}).Sniff(head(c.data)) {
			t.Errorf("%s: not recognised", c.name)
		}
	}
}

func TestSniffDoesNotClaimOrdinaryFiles(t *testing.T) {
	p := makePreviews(t)
	manyEntries := binary.LittleEndian.AppendUint16([]byte{'I', 'I', 42, 0, 8, 0, 0, 0}, 300)
	manyEntries = append(manyEntries, make([]byte, 600)...)
	cases := []struct {
		name string
		data []byte
	}{
		{"plain TIFF", plainTIFF(t)},
		{"TIFF with an empty make", tiffWithMake(little, "")},
		{"TIFF from Photoshop", tiffWithMake(big, "Adobe Photoshop")},
		{"scanner TIFF", tiffWithMake(little, "EPSON Perfection V850")},
		{"make shorter than five bytes", tiffWithMake(little, "XYZ", entry{tag: tagSubIFDs, typ: typeLong, count: 1, value: 8})},
		{"canon scanner TIFF", tiffWithMake(little, "Canon")},
		{"foreign make with SubIFDs", tiffWithMake(little, "Microsoft", entry{tag: tagSubIFDs, typ: typeLong, count: 1, value: 8})},
		{"make of the wrong type", tiffWithMake(little, "", entry{tag: tagMake, typ: typeShort, count: 1, value: 7})},
		{"BigTIFF", []byte{'I', 'I', 43, 0, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"IFD beyond the sniff window", withBytes(plainTIFF(t), 4, 0x00, 0x70, 0, 0)},
		{"IFD offset inside the header", withBytes(plainTIFF(t), 4, 2, 0, 0, 0)},
		{"IFD with too many entries", manyEntries},
		{"JPEG", p.large},
		{"PNG", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")},
		{"GIF", []byte("GIF89a\x01\x00\x01\x00")},
		{"AVIF", append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, make([]byte, 12)...)},
		{"HEIC", append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c'}, make([]byte, 12)...)},
		{"MP4", append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, make([]byte, 12)...)},
		{"crx without the space", append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'c', 'r', 'x', 'x'}, make([]byte, 12)...)},
		{"almost RAF", []byte("FUJIFILMCCD-RAX 0201")},
		{"short", []byte{'I', 'I', 42}},
		{"empty", nil},
		{"zeros", make([]byte, 600)},
	}
	for _, c := range cases {
		if (Decoder{}).Sniff(head(c.data)) {
			t.Errorf("%s: claimed as RAW", c.name)
		}
	}
}

func TestSniffNeverPanicsOnTruncatedHeads(t *testing.T) {
	for _, s := range allSamples(t) {
		for n := 0; n <= len(head(s.data)); n++ {
			(Decoder{}).Sniff(s.data[:n])
		}
	}
}

func TestMakeMatching(t *testing.T) {
	for name, want := range map[string]bool{
		"NIKON CORPORATION": true, "SONY": true, "Sony Corporation": true, "PENTAX Corporation": true,
		"RICOH IMAGING COMPANY, LTD.": true, "OLYMPUS IMAGING CORP.": true, "OM Digital Solutions": true,
		"Panasonic": true, "LEICA CAMERA AG": true, "FUJIFILM": true,
		"": false, "Adobe": false, "Canon": false, "EPSON": false,
	} {
		if isMake(name) != want {
			t.Errorf("isMake(%q) = %v", name, !want)
		}
	}
}
