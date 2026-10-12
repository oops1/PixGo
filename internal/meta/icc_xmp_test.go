package meta

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/oops1/PixGo/internal/codec"
)

func TestICCSplitOverSeveralAPP2Chunks(t *testing.T) {
	profiles := map[string][]byte{
		"v4 mluc": sRGBProfile(),
		"v2 desc": iccProfile(2, 0x10, "mntr", "RGB ", descASCII("sRGB IEC61966-2.1"), 450),
	}
	for name, profile := range profiles {
		for _, chunk := range []int{1, 7, 100, 301, len(profile), len(profile) + 50} {
			segs := jpegICCSegs(profile, chunk)
			if len(segs) > 255 {
				continue
			}
			m := mustRead(t, buildJPEG(10, 10, segs...), codec.JPEG)
			if m.ICC == nil || m.ICC.Description != "sRGB IEC61966-2.1" || m.ICC.Size != len(profile) {
				t.Fatalf("%s chunk %d: %+v", name, chunk, m.ICC)
			}
		}
	}
	m := mustRead(t, buildJPEG(10, 10, jpegICCSegs(profiles["v2 desc"], 100)...), codec.JPEG)
	if m.ICC.Version != "2.1" || m.ICC.Class != "Display" || m.ICC.ColorSpace != "RGB" {
		t.Fatalf("%+v", *m.ICC)
	}
}

func TestICCChunksInAnyOrder(t *testing.T) {
	profile := sRGBProfile()
	segs := jpegICCSegs(profile, 200)
	if len(segs) < 4 {
		t.Fatal("sample too small")
	}
	reordered := [][]byte{segs[2], segs[0], segs[3]}
	reordered = append(reordered, segs[1])
	reordered = append(reordered, segs[4:]...)
	m := mustRead(t, buildJPEG(10, 10, reordered...), codec.JPEG)
	checkICC(t, m.ICC, len(profile))
}

func TestICCChunkValidation(t *testing.T) {
	profile := sRGBProfile()
	segs := jpegICCSegs(profile, 200)
	patch := func(seg []byte, seq, count byte) []byte {
		out := append([]byte(nil), seg...)
		out[4+12], out[4+13] = seq, count
		return out
	}
	cases := map[string][][]byte{
		"missing chunk":    append(append([][]byte(nil), segs[:2]...), segs[3:]...),
		"duplicate seq":    append(append([][]byte(nil), segs...), segs[1]),
		"truncated set":    segs[:len(segs)-1],
		"count mismatch":   append([][]byte{patch(segs[0], 1, 9)}, segs[1:]...),
		"seq zero":         append([][]byte{patch(segs[0], 0, byte(len(segs)))}, segs[1:]...),
		"seq above count":  append([][]byte{patch(segs[0], byte(len(segs)+1), byte(len(segs)))}, segs[1:]...),
		"count zero":       {patch(segs[0], 1, 0)},
		"short app2":       {jpegSeg(0xe2, []byte("ICC_PROFILE\x00\x01"))},
		"foreign app2":     {jpegSeg(0xe2, []byte("MPF\x00....................."))},
		"profile too tiny": {jpegSeg(0xe2, append([]byte("ICC_PROFILE\x00\x01\x01"), 1, 2, 3))},
	}
	for name, input := range cases {
		m := mustRead(t, buildJPEG(10, 10, input...), codec.JPEG)
		if m.ICC != nil {
			t.Fatalf("%s: accepted %+v", name, *m.ICC)
		}
	}
}

func TestICCHeaderFields(t *testing.T) {
	desc := descASCII("Test")
	classes := map[string]string{"scnr": "Input", "mntr": "Display", "prtr": "Output", "link": "DeviceLink", "spac": "ColorSpace", "abst": "Abstract", "nmcl": "NamedColor", "wxyz": "wxyz"}
	for sig, want := range classes {
		if c := parseICC(iccProfile(4, 0, sig, "RGB ", desc, 0)); c == nil || c.Class != want {
			t.Fatalf("class %q: %+v", sig, c)
		}
	}
	spaces := map[string]string{"RGB ": "RGB", "GRAY": "GRAY", "CMYK": "CMYK", "Lab ": "Lab", "XYZ ": "XYZ", "YCbr": "YCbCr", "3CLR": "3CLR", "\x01\x02\x03\x04": ""}
	for sig, want := range spaces {
		if c := parseICC(iccProfile(4, 0, "mntr", sig, desc, 0)); c == nil || c.ColorSpace != want {
			t.Fatalf("space %q: %+v", sig, c)
		}
	}
	versions := map[[2]byte]string{{2, 0x10}: "2.1", {2, 0x40}: "2.4", {4, 0x30}: "4.3", {4, 0x31}: "4.3.1", {4, 0x20}: "4.2", {0, 0}: "0.0"}
	for in, want := range versions {
		if c := parseICC(iccProfile(in[0], in[1], "mntr", "RGB ", desc, 0)); c == nil || c.Version != want {
			t.Fatalf("version %v: %+v", in, c)
		}
	}
}

func TestICCDescriptionVariants(t *testing.T) {
	cases := []struct {
		name string
		desc []byte
		want string
	}{
		{"ascii", descASCII("Adobe RGB (1998)"), "Adobe RGB (1998)"},
		{"ascii latin1", append(append(append([]byte("desc\x00\x00\x00\x00\x00\x00\x00\x05"), "Caf\xe9"...), 0), make([]byte, 78)...), "Café"},
		{"mluc en", descMLUC(mlucRecord{"fr", "FR", "Profil"}, mlucRecord{"en", "US", "Display P3"}), "Display P3"},
		{"mluc no en", descMLUC(mlucRecord{"fr", "FR", "Profil P3"}, mlucRecord{"de", "DE", "Anzeige"}), "Profil P3"},
		{"mluc empty en", descMLUC(mlucRecord{"en", "US", ""}, mlucRecord{"ru", "RU", "Профиль"}), "Профиль"},
		{"mluc cyrillic", descMLUC(mlucRecord{"ru", "RU", "Цветовой профиль"}), "Цветовой профиль"},
		{"mluc emoji", descMLUC(mlucRecord{"en", "US", "Wide \U0001F3A8 gamut"}), "Wide \U0001F3A8 gamut"},
		{"text type", append([]byte("text\x00\x00\x00\x00"), "Plain text profile\x00"...), "Plain text profile"},
		{"unknown type", append([]byte("XYZ \x00\x00\x00\x00"), make([]byte, 20)...), ""},
	}
	for _, c := range cases {
		icc := parseICC(iccProfile(4, 0x30, "mntr", "RGB ", c.desc, 0))
		if icc == nil || icc.Description != c.want {
			t.Fatalf("%s: %+v", c.name, icc)
		}
	}
}

func TestICCDescriptionIsClipped(t *testing.T) {
	icc := parseICC(iccProfile(4, 0, "mntr", "RGB ", descMLUC(mlucRecord{"en", "US", strings.Repeat("ж", 5000)}), 0))
	if icc == nil || len(icc.Description) == 0 || len(icc.Description) > iccMaxDescBytes {
		t.Fatalf("%d bytes", len(icc.Description))
	}
}

func TestICCHostileHeaders(t *testing.T) {
	good := iccProfile(4, 0x30, "mntr", "RGB ", descMLUC(mlucRecord{"en", "US", "OK"}), 64)
	if c := parseICC(good); c == nil || c.Description != "OK" {
		t.Fatalf("control: %+v", c)
	}
	patched := func(f func(b []byte)) []byte {
		b := append([]byte(nil), good...)
		f(b)
		return b
	}
	cases := map[string][]byte{
		"tag count huge":     patched(func(b []byte) { binary.BigEndian.PutUint32(b[128:], 0xffffffff) }),
		"tag offset huge":    patched(func(b []byte) { binary.BigEndian.PutUint32(b[136:], 0xfffffff0) }),
		"tag size huge":      patched(func(b []byte) { binary.BigEndian.PutUint32(b[140:], 0xffffffff) }),
		"tag size tiny":      patched(func(b []byte) { binary.BigEndian.PutUint32(b[140:], 3) }),
		"mluc count huge":    patched(func(b []byte) { binary.BigEndian.PutUint32(b[144+8:], 0xffffffff) }),
		"mluc rec size zero": patched(func(b []byte) { binary.BigEndian.PutUint32(b[144+12:], 0) }),
		"mluc rec size huge": patched(func(b []byte) { binary.BigEndian.PutUint32(b[144+12:], 0xffffffff) }),
		"mluc str offset":    patched(func(b []byte) { binary.BigEndian.PutUint32(b[144+16+8:], 0xfffffff0) }),
		"mluc str length":    patched(func(b []byte) { binary.BigEndian.PutUint32(b[144+16+4:], 0xfffffff0) }),
		"mluc odd length":    patched(func(b []byte) { binary.BigEndian.PutUint32(b[144+16+4:], 3) }),
	}
	for name, data := range cases {
		c := parseICC(data)
		if c == nil {
			t.Fatalf("%s: header is still valid", name)
		}
		if len(c.Description) > iccMaxDescBytes {
			t.Fatalf("%s: description %d bytes", name, len(c.Description))
		}
	}
	if parseICC(good[:127]) != nil || parseICC(nil) != nil {
		t.Fatal("short profile accepted")
	}
	noSig := append([]byte(nil), good...)
	copy(noSig[36:], "xxxx")
	if parseICC(noSig) != nil {
		t.Fatal("profile without acsp accepted")
	}
	for n := 0; n <= len(good); n++ {
		parseICC(good[:n])
	}
}

func TestICCAtAndAboveTheCap(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates 16 MB profiles")
	}
	base := len(iccProfile(4, 0x30, "mntr", "RGB ", descMLUC(mlucRecord{"en", "US", "Big"}), 0))
	atCap := iccProfile(4, 0x30, "mntr", "RGB ", descMLUC(mlucRecord{"en", "US", "Big"}), MaxICCBytes-base)
	if len(atCap) != MaxICCBytes {
		t.Fatalf("sample is %d bytes", len(atCap))
	}
	over := append(append([]byte(nil), atCap...), 0)

	png := func(profile []byte) *Metadata {
		return mustRead(t, buildPNG(t, 1, 1, [][]byte{iCCPChunk(t, profile)}, nil), codec.PNG)
	}
	if m := png(atCap); m.ICC == nil || m.ICC.Description != "Big" || m.ICC.Size != MaxICCBytes {
		t.Fatalf("PNG at cap: %+v", m.ICC)
	}
	if m := png(over); m.ICC != nil {
		t.Fatal("PNG over cap accepted")
	}
	webp := func(profile []byte) *Metadata {
		return mustRead(t, buildWebP(vp8xChunk(0x20, 1, 1), riffChunk("ICCP", profile), vp8lChunk(1, 1)), codec.WebP)
	}
	if m := webp(atCap); m.ICC == nil || m.ICC.Size != MaxICCBytes {
		t.Fatalf("WebP at cap: %+v", m.ICC)
	}
	if m := webp(over); m.ICC != nil {
		t.Fatal("WebP over cap accepted")
	}
}

func TestICCJPEGChunkLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates 16 MB")
	}
	chunk := bytes.Repeat([]byte{0xa5}, 65519)
	var segs [][]byte
	for i := 0; i < 255; i++ {
		payload := append([]byte("ICC_PROFILE\x00"), byte(i+1), 255)
		segs = append(segs, jpegSeg(0xe2, append(payload, chunk...)))
	}
	first := iccProfile(4, 0x30, "mntr", "RGB ", descMLUC(mlucRecord{"en", "US", "Huge"}), 0)
	segs[0] = jpegSeg(0xe2, append(append([]byte("ICC_PROFILE\x00"), 1, 255), append(first, chunk[:65519-len(first)]...)...))
	m := mustRead(t, buildJPEG(1, 1, segs...), codec.JPEG)
	if m.ICC == nil || m.ICC.Description != "Huge" || m.ICC.Size != 255*65519 {
		t.Fatalf("255 chunks: %+v", m.ICC)
	}

	var many [][]byte
	for i := 0; i < 300; i++ {
		payload := append([]byte("ICC_PROFILE\x00"), 1, 1)
		many = append(many, jpegSeg(0xe2, append(payload, chunk...)))
	}
	if m := mustRead(t, buildJPEG(1, 1, many...), codec.JPEG); m.ICC != nil || m.Width != 1 {
		t.Fatalf("300 big chunks: %+v", m.ICC)
	}
	var tiny [][]byte
	for i := 0; i < 2000; i++ {
		tiny = append(tiny, jpegSeg(0xe2, append([]byte("ICC_PROFILE\x00"), 1, 1, 7)))
	}
	if m := mustRead(t, buildJPEG(1, 1, tiny...), codec.JPEG); m.ICC != nil || m.Width != 1 {
		t.Fatalf("2000 tiny chunks: %+v", m.ICC)
	}
}

func TestPNGICCBombIsRejected(t *testing.T) {
	bomb := zlibBytes(t, make([]byte, 40<<20))
	chunk := pngChunk("iCCP", append([]byte("bomb\x00\x00"), bomb...))
	m := mustRead(t, buildPNG(t, 1, 1, [][]byte{chunk}, nil), codec.PNG)
	if m.ICC != nil || m.Width != 1 {
		t.Fatalf("%+v", m.ICC)
	}
	garbage := pngChunk("iCCP", append([]byte("junk\x00\x00"), 1, 2, 3, 4, 5))
	badName := pngChunk("iCCP", append([]byte("\x00\x00"), bomb...))
	badMethod := pngChunk("iCCP", append([]byte("sRGB\x00\x07"), bomb...))
	for name, c := range map[string][]byte{"garbage": garbage, "no name": badName, "bad method": badMethod} {
		if m := mustRead(t, buildPNG(t, 1, 1, [][]byte{c}, nil), codec.PNG); m.ICC != nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestPNGXMPCaps(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates 4 MB documents")
	}
	pad := func(n int) string {
		head := `<x:xmpmeta xmlns:x="adobe:ns:meta/" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:xmp="http://ns.adobe.com/xap/1.0/"><rdf:RDF><rdf:Description xmp:Rating="3"/></rdf:RDF></x:xmpmeta>`
		return head + strings.Repeat(" ", n-len(head))
	}
	for _, compress := range []bool{false, true} {
		atCap := pad(MaxXMPBytes)
		m := mustRead(t, buildPNG(t, 1, 1, [][]byte{pngChunk("iTXt", iTXtXMP(t, atCap, compress))}, nil), codec.PNG)
		if m.XMP == nil || m.XMP.Rating != 3 {
			t.Fatalf("compress=%v at cap: %+v", compress, m.XMP)
		}
		over := pad(MaxXMPBytes + 1)
		m = mustRead(t, buildPNG(t, 1, 1, [][]byte{pngChunk("iTXt", iTXtXMP(t, over, compress))}, nil), codec.PNG)
		if m.XMP != nil {
			t.Fatalf("compress=%v over cap accepted", compress)
		}
	}
	bomb := iTXtXMP(t, strings.Repeat("a", 24<<20), true)
	if m := mustRead(t, buildPNG(t, 1, 1, [][]byte{pngChunk("iTXt", bomb)}, nil), codec.PNG); m.XMP != nil {
		t.Fatal("XMP bomb accepted")
	}
	huge := make([]byte, MaxXMPBytes+2000)
	copy(huge, "XML:com.adobe.xmp\x00\x00\x00\x00\x00")
	if m := mustRead(t, buildPNG(t, 1, 1, [][]byte{pngChunk("iTXt", huge)}, nil), codec.PNG); m.XMP != nil {
		t.Fatal("oversized iTXt accepted")
	}
	webp := buildWebP(vp8lChunk(1, 1), riffChunk("XMP ", []byte(pad(MaxXMPBytes+1))))
	if m := mustRead(t, webp, codec.WebP); m.XMP != nil {
		t.Fatal("WebP oversized XMP accepted")
	}
	webp = buildWebP(vp8lChunk(1, 1), riffChunk("XMP ", []byte(pad(MaxXMPBytes))))
	if m := mustRead(t, webp, codec.WebP); m.XMP == nil || m.XMP.Rating != 3 {
		t.Fatal("WebP XMP at cap rejected")
	}
}

func TestXMPSample(t *testing.T) {
	checkXMPSample(t, parseXMP([]byte(xmpSample)))
}

func TestXMPFlavours(t *testing.T) {
	const head = `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:xmp="http://ns.adobe.com/xap/1.0/" xmlns:tiff="http://ns.adobe.com/tiff/1.0/"><rdf:Description rdf:about="">`
	const tail = `</rdf:Description></rdf:RDF></x:xmpmeta>`
	cases := []struct {
		name string
		body string
		want XMP
	}{
		{"element rating", `<xmp:Rating>5</xmp:Rating>`, XMP{Rating: 5}},
		{"rejected rating", `<xmp:Rating>-1</xmp:Rating>`, XMP{Rating: -1}},
		{"float rating", `<xmp:Rating>3.0</xmp:Rating>`, XMP{Rating: 3}},
		{"zero rating", `<xmp:Rating>0</xmp:Rating><dc:title>T</dc:title>`, XMP{Title: "T"}},
		{"plain title", `<dc:title>Plain</dc:title>`, XMP{Title: "Plain"}},
		{"alt without default", `<dc:title><rdf:Alt><rdf:li xml:lang="fr">Bonjour</rdf:li><rdf:li xml:lang="de">Hallo</rdf:li></rdf:Alt></dc:title>`, XMP{Title: "Bonjour"}},
		{"empty default", `<dc:title><rdf:Alt><rdf:li xml:lang="x-default"> </rdf:li><rdf:li xml:lang="en">Hello</rdf:li></rdf:Alt></dc:title>`, XMP{Title: "Hello"}},
		{"default after others", `<dc:title><rdf:Alt><rdf:li xml:lang="en">Hello</rdf:li><rdf:li xml:lang="x-default">Default</rdf:li></rdf:Alt></dc:title>`, XMP{Title: "Default"}},
		{"solo creator", `<dc:creator>Solo</dc:creator>`, XMP{Creator: []string{"Solo"}}},
		{"creator bag", `<dc:creator><rdf:Bag><rdf:li>A</rdf:li><rdf:li></rdf:li><rdf:li> B </rdf:li></rdf:Bag></dc:creator>`, XMP{Creator: []string{"A", "B"}}},
		{"image description fallback", `<tiff:ImageDescription><rdf:Alt><rdf:li xml:lang="x-default">From TIFF</rdf:li></rdf:Alt></tiff:ImageDescription>`, XMP{Description: "From TIFF"}},
		{"dc description wins", `<tiff:ImageDescription>tiff</tiff:ImageDescription><dc:description>dc</dc:description>`, XMP{Description: "dc"}},
		{"multi line", "<dc:description>line one\r\nline two\ttabbed</dc:description>", XMP{Description: "line one\nline two tabbed"}},
		{"entities", `<dc:title>A &amp; B &lt;C&gt; &#1044; &bogus;</dc:title>`, XMP{Title: "A & B <C> Д &bogus;"}},
		{"foreign namespace", `<foo:title xmlns:foo="http://example.com/">no</foo:title><dc:title xmlns:dc="http://example.com/other">no</dc:title>`, XMP{}},
	}
	for _, c := range cases {
		got := parseXMP([]byte(head + c.body + tail))
		if c.want.Title == "" && c.want.Description == "" && c.want.Rating == 0 && len(c.want.Creator) == 0 {
			if got != nil {
				t.Fatalf("%s: %+v", c.name, *got)
			}
			continue
		}
		if got == nil || got.Title != c.want.Title || got.Description != c.want.Description || got.Rating != c.want.Rating || strings.Join(got.Creator, "|") != strings.Join(c.want.Creator, "|") {
			t.Fatalf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
}

func TestXMPAttributeFormAndMissingNamespaces(t *testing.T) {
	attr := `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:xmp="http://ns.adobe.com/xap/1.0/" xmp:Rating="2" dc:title="Attr title" dc:description="Attr desc"/></rdf:RDF>`
	got := parseXMP([]byte(attr))
	if got == nil || got.Rating != 2 || got.Title != "Attr title" || got.Description != "Attr desc" {
		t.Fatalf("%+v", got)
	}
	bare := `<rdf:RDF><rdf:Description><dc:title><rdf:Alt><rdf:li xml:lang="x-default">No decl</rdf:li></rdf:Alt></dc:title><xmp:Rating>4</xmp:Rating></rdf:Description></rdf:RDF>`
	got = parseXMP([]byte(bare))
	if got == nil || got.Title != "No decl" || got.Rating != 4 {
		t.Fatalf("%+v", got)
	}
}

func TestXMPRatingRange(t *testing.T) {
	for in, want := range map[string]int{"0": 0, "1": 1, "5": 5, "-1": -1, "4.4": 4, "4.6": 5} {
		got := parseXMP([]byte(`<xmp:Rating xmlns:xmp="http://ns.adobe.com/xap/1.0/">` + in + `</xmp:Rating>`))
		if got == nil || got.Rating != want {
			t.Fatalf("%q: %+v", in, got)
		}
	}
	for _, in := range []string{"6", "99", "-2", "abc", "", "NaN", "Inf", "1e9"} {
		if got := parseXMP([]byte(`<xmp:Rating xmlns:xmp="http://ns.adobe.com/xap/1.0/">` + in + `</xmp:Rating>`)); got != nil {
			t.Fatalf("%q accepted: %+v", in, got)
		}
	}
}

func TestXMPEncodings(t *testing.T) {
	doc := `<dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">Привет, мир</dc:title>`
	utf16Of := func(bom []byte, big bool) []byte {
		out := append([]byte(nil), bom...)
		for _, u := range utf16.Encode([]rune(doc)) {
			if big {
				out = append(out, byte(u>>8), byte(u))
			} else {
				out = append(out, byte(u), byte(u>>8))
			}
		}
		return out
	}
	inputs := map[string][]byte{
		"utf8":          []byte(doc),
		"utf8 bom":      append([]byte{0xef, 0xbb, 0xbf}, doc...),
		"utf16 be":      utf16Of([]byte{0xfe, 0xff}, true),
		"utf16 le":      utf16Of([]byte{0xff, 0xfe}, false),
		"nul padded":    append([]byte(doc), make([]byte, 100)...),
		"declared utf8": []byte(`<?xml version="1.0" encoding="UTF-8"?>` + doc),
		"declared 16":   []byte(`<?xml version="1.0" encoding="UTF-16"?>` + doc),
		"declared 1251": []byte(`<?xml version="1.0" encoding="windows-1251"?>` + doc),
	}
	for name, data := range inputs {
		got := parseXMP(data)
		if got == nil || got.Title != "Привет, мир" {
			t.Fatalf("%s: %+v", name, got)
		}
	}
	dirty := []byte("<dc:title xmlns:dc=\"http://purl.org/dc/elements/1.1/\">ab\xff\xfe\x01\x02cd</dc:title>")
	got := parseXMP(dirty)
	if got == nil || !strings.HasPrefix(got.Title, "ab") || !strings.HasSuffix(got.Title, "cd") {
		t.Fatalf("dirty bytes: %+v", got)
	}
}

func TestXMPTruncationAndGarbage(t *testing.T) {
	data := []byte(xmpSample)
	for n := 0; n <= len(data); n += 7 {
		parseXMP(data[:n])
	}
	cut := parseXMP(data[:bytes.Index(data, []byte("</dc:title>"))+len("</dc:title>")])
	if cut == nil || cut.Title != "Sunset over Moscow" || cut.Rating != 4 {
		t.Fatalf("partial document: %+v", cut)
	}
	for name, in := range map[string]string{
		"empty":       "",
		"text":        "just some text",
		"binary":      "\x00\x01\x02\x03\xff\xfe",
		"no props":    `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"/></x:xmpmeta>`,
		"unclosed":    `<dc:title xmlns:dc="http://purl.org/dc/elements/1.1/"><rdf:Alt><rdf:li>`,
		"angle mess":  `<<<<>>>><dc:title`,
		"only header": `<?xpacket begin="x" id="W5M0MpCehiHzreSzNTczkc9d"?>`,
	} {
		if got := parseXMP([]byte(in)); got != nil && name != "unclosed" {
			t.Fatalf("%s: %+v", name, got)
		}
	}
}

func TestXMPNestingAndExpansionAreHarmless(t *testing.T) {
	deep := strings.Repeat("<a>", 200000) + "x" + strings.Repeat("</a>", 200000)
	start := time.Now()
	parseXMP([]byte(deep))
	laughs := `<?xml version="1.0"?><!DOCTYPE lolz [<!ENTITY lol "lol"><!ENTITY lol2 "&lol;&lol;&lol;&lol;&lol;&lol;&lol;&lol;&lol;&lol;"><!ENTITY lol3 "&lol2;&lol2;&lol2;&lol2;&lol2;&lol2;&lol2;&lol2;&lol2;&lol2;">]><dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">&lol3;</dc:title>`
	got := parseXMP([]byte(laughs))
	if got != nil && len(got.Title) > xmpMaxText {
		t.Fatalf("title %d bytes", len(got.Title))
	}
	nested := `<dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">` + strings.Repeat("<b>", 5000) + "deep" + strings.Repeat("</b>", 5000) + `</dc:title><xmp:Rating xmlns:xmp="http://ns.adobe.com/xap/1.0/">3</xmp:Rating>`
	parseXMP([]byte(nested))
	big := `<dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">` + strings.Repeat("я", 100000) + `</dc:title>`
	got = parseXMP([]byte(big))
	if got == nil || len(got.Title) == 0 || len(got.Title) > xmpMaxText {
		t.Fatalf("long title %v", got)
	}
	creators := `<dc:creator xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Seq>` + strings.Repeat("<rdf:li>n</rdf:li>", 500) + `</rdf:Seq></dc:creator>`
	got = parseXMP([]byte(creators))
	if got == nil || len(got.Creator) != xmpMaxCreators {
		t.Fatalf("creators %v", got)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("hostile XMP took too long")
	}
}
