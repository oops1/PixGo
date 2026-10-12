package svg

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

const redSquare = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" fill="#ff0000"/></svg>`

type virtualFile struct{ size int64 }

func (v virtualFile) ReadAt(p []byte, off int64) (int, error) {
	if off >= v.size {
		return 0, errors.New("past the end")
	}
	n := int64(len(p))
	if n > v.size-off {
		n = v.size - off
	}
	clear(p[:n])
	return int(n), nil
}

func gzipped(t testing.TB, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func probe(data string) (codec.Info, error) {
	return (Decoder{}).Probe(context.Background(), strings.NewReader(data), int64(len(data)), codec.DefaultLimits())
}

func decode(data []byte, o codec.Options) (*codec.Image, error) {
	if o.Limits == (codec.Limits{}) {
		o.Limits = codec.DefaultLimits()
	}
	return (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), o)
}

func TestSniffAcceptsPrologues(t *testing.T) {
	d := Decoder{}
	accepted := map[string]string{
		"plain":             `<svg xmlns="http://www.w3.org/2000/svg"></svg>`,
		"self closing":      `<svg/>`,
		"bom":               "\xef\xbb\xbf<svg width='1' height='1'/>",
		"whitespace":        "  \n\t\r\n<svg>",
		"xml declaration":   `<?xml version="1.0" encoding="UTF-8" standalone="no"?><svg>`,
		"comments":          "<?xml version=\"1.0\"?>\n<!-- Generator: Adobe -->\n<!-- another -->\n<svg>",
		"doctype":           `<?xml version="1.0"?><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg>`,
		"doctype subset":    `<!DOCTYPE svg [<!ENTITY ns_svg "http://www.w3.org/2000/svg"> <!ENTITY a ">">]><svg xmlns="&ns_svg;">`,
		"lower doctype":     `<!doctype svg><svg>`,
		"prefixed":          `<svg:svg xmlns:svg="http://www.w3.org/2000/svg">`,
		"upper case":        `<SVG>`,
		"truncated doctype": `<?xml version="1.0"?><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" [ <!ENTITY ns_extend "http://ns.adobe.com/Extensibility/1.0/"> <!ENTITY ns_ai "http://ns.adobe.com/AdobeIllustrator/10.0/">`,
	}
	for name, head := range accepted {
		if !d.Sniff([]byte(head)) {
			t.Fatalf("%s: not recognised", name)
		}
	}
	rejected := map[string]string{
		"empty":               "",
		"html":                `<html><svg></svg></html>`,
		"other xml":           `<?xml version="1.0"?><root><svg/></root>`,
		"svg prefix":          `<svgfoo>`,
		"just svg":            `<svg`,
		"text":                `svg`,
		"png":                 "\x89PNG\r\n\x1a\n",
		"unfinished comment":  `<!-- never closed <svg>`,
		"unfinished xml decl": `<?xml version="1.0" <svg>`,
		"truncated other":     `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd"`,
	}
	for name, head := range rejected {
		if d.Sniff([]byte(head)) {
			t.Fatalf("%s: wrongly recognised", name)
		}
	}
}

func TestSniffLooksInsideGzip(t *testing.T) {
	d := Decoder{}
	if !d.Sniff(gzipped(t, []byte(redSquare))) {
		t.Fatal("svgz not recognised")
	}
	big := redSquare + strings.Repeat(" ", 100000)
	zipped := gzipped(t, []byte(big))
	if !d.Sniff(zipped[:min(len(zipped), codec.SniffLen)]) {
		t.Fatal("svgz not recognised from the first 512 bytes")
	}
	if d.Sniff(gzipped(t, []byte("just some text"))) {
		t.Fatal("gzip of text recognised")
	}
	if d.Sniff([]byte{0x1f, 0x8b, 0x00}) || d.Sniff([]byte{0x1f, 0x8b}) {
		t.Fatal("bad gzip header recognised")
	}
	if d.ID() != codec.SVG || d.Name() != "SVG" {
		t.Fatal("identity")
	}
	if e := d.Extensions(); len(e) != 2 || e[0] != ".svg" || e[1] != ".svgz" {
		t.Fatalf("%v", e)
	}
}

func TestProbeSizes(t *testing.T) {
	cases := []struct {
		name          string
		svg           string
		width, height int
	}{
		{"square viewBox", redSquare, 24, 24},
		{"fractional", `<svg viewBox="0 0 10.4 3.6"><rect width="2" height="2"/></svg>`, 10, 4},
		{"tiny", `<svg viewBox="0 0 0.2 0.3"><rect width="0.1" height="0.1"/></svg>`, 1, 1},
		{"offset origin", `<svg viewBox="-50 -20 100 40"><rect x="-50" y="-20" width="10" height="10"/></svg>`, 100, 40},
		{"width and height only", `<svg width="64" height="48"><rect width="10" height="10"/></svg>`, 64, 48},
		{"wide", `<svg viewBox="0 0 2000 500"><rect width="10" height="10"/></svg>`, 2000, 500},
	}
	for _, c := range cases {
		info, err := probe(c.svg)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		want := codec.Info{Format: codec.SVG, FormatName: "SVG", Width: c.width, Height: c.height, Frames: 1, HasAlpha: true, Orientation: codec.OrientNormal, Vector: true}
		if info != want {
			t.Fatalf("%s: got %+v, want %+v", c.name, info, want)
		}
	}
}

func TestDecodeScalesSmallDocumentsUp(t *testing.T) {
	img, err := decode([]byte(redSquare), codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	pix := img.First()
	if pix.Bounds().Dx() != 1024 || pix.Bounds().Dy() != 1024 {
		t.Fatalf("raster %v", pix.Bounds())
	}
	if c := pix.NRGBAAt(512, 512); c != (color.NRGBA{R: 255, A: 255}) {
		t.Fatalf("centre = %v", c)
	}
	if img.Info.Width != 24 || img.Info.Height != 24 || !img.Info.Vector || img.Scale != 1 || len(img.Frames) != 1 {
		t.Fatalf("%+v", img.Info)
	}
	wide, err := decode([]byte(`<svg viewBox="0 0 100 25"><rect width="100" height="25" fill="#0000ff"/></svg>`), codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if b := wide.First().Bounds(); b.Dx() != 1024 || b.Dy() != 256 {
		t.Fatalf("wide raster %v", b)
	}
}

func TestDecodeKeepsLargeIntrinsicSize(t *testing.T) {
	img, err := decode([]byte(`<svg viewBox="0 0 2000 500"><rect width="2000" height="500" fill="#00ff00"/></svg>`), codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if b := img.First().Bounds(); b.Dx() != 2000 || b.Dy() != 500 {
		t.Fatalf("raster %v", b)
	}
}

func TestDecodeAtRequestedSize(t *testing.T) {
	img, err := decode([]byte(redSquare), codec.Options{VectorW: 100, VectorH: 100})
	if err != nil {
		t.Fatal(err)
	}
	if b := img.First().Bounds(); b.Dx() != 100 || b.Dy() != 100 {
		t.Fatalf("raster %v", b)
	}
	letterbox, err := decode([]byte(redSquare), codec.Options{VectorW: 200, VectorH: 100})
	if err != nil {
		t.Fatal(err)
	}
	pix := letterbox.First()
	if b := pix.Bounds(); b.Dx() != 200 || b.Dy() != 100 {
		t.Fatalf("raster %v", b)
	}
	if pix.NRGBAAt(100, 50) != (color.NRGBA{R: 255, A: 255}) || pix.NRGBAAt(5, 50).A != 0 {
		t.Fatalf("content must be centred: centre %v left %v", pix.NRGBAAt(100, 50), pix.NRGBAAt(5, 50))
	}
	onlyOne, err := decode([]byte(redSquare), codec.Options{VectorW: 50})
	if err != nil || onlyOne.First().Bounds().Dx() != 1024 {
		t.Fatalf("a single dimension must be ignored: %v", err)
	}
}

func TestRasterSizeStaysWithinPixelLimit(t *testing.T) {
	lim := codec.DefaultLimits()
	lim.MaxPixels = 1_000_000
	for _, o := range []codec.Options{
		{VectorW: 10000, VectorH: 10000},
		{VectorW: 7, VectorH: 3_000_000},
		{VectorW: 1 << 30, VectorH: 1 << 30},
		{},
	} {
		w, h := RasterSize(o, 24, 24, lim)
		if w < 1 || h < 1 || int64(w)*int64(h) > lim.MaxPixels {
			t.Fatalf("%+v -> %dx%d", o, w, h)
		}
	}
	w, h := RasterSize(codec.Options{}, 100000, 100000, lim)
	if int64(w)*int64(h) > lim.MaxPixels || w < 900 {
		t.Fatalf("large intrinsic -> %dx%d", w, h)
	}
	tiny := codec.DefaultLimits()
	tiny.MaxPixels = 1
	if w, h := RasterSize(codec.Options{}, 24, 24, tiny); w != 1 || h != 1 {
		t.Fatalf("one pixel budget -> %dx%d", w, h)
	}
	img, err := decode([]byte(redSquare), codec.Options{Limits: lim, VectorW: 5000, VectorH: 5000})
	if err != nil || int64(img.First().Bounds().Dx())*int64(img.First().Bounds().Dy()) > lim.MaxPixels {
		t.Fatalf("decode with limit: %v", err)
	}
}

func TestStraightAlphaOutput(t *testing.T) {
	img, err := decode([]byte(`<svg viewBox="0 0 8 8"><rect width="8" height="8" fill="#ff0000" fill-opacity="0.5"/></svg>`), codec.Options{VectorW: 8, VectorH: 8})
	if err != nil {
		t.Fatal(err)
	}
	c := img.First().NRGBAAt(4, 4)
	if c.A < 126 || c.A > 130 || c.R < 250 || c.G != 0 || c.B != 0 {
		t.Fatalf("half transparent red became %v", c)
	}
	if empty := img.First().NRGBAAt(0, 0); empty.A == 0 {
		t.Fatalf("covered corner %v", empty)
	}
	blank, err := decode([]byte(`<svg viewBox="0 0 8 8"><rect width="2" height="2"/></svg>`), codec.Options{VectorW: 8, VectorH: 8})
	if err != nil || blank.First().NRGBAAt(7, 7) != (color.NRGBA{}) {
		t.Fatalf("uncovered pixels must be transparent: %v", blank.First().NRGBAAt(7, 7))
	}
}

func TestGzippedDocuments(t *testing.T) {
	data := gzipped(t, []byte(redSquare))
	info, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil || info.Width != 24 {
		t.Fatalf("%+v %v", info, err)
	}
	img, err := decode(data, codec.Options{VectorW: 16, VectorH: 16})
	if err != nil || img.First().NRGBAAt(8, 8) != (color.NRGBA{R: 255, A: 255}) {
		t.Fatalf("svgz decode: %v", err)
	}
	if _, err := decode(data[:len(data)-8], codec.Options{}); err == nil {
		t.Fatal("truncated gzip accepted")
	} else {
		testimg.RequireCodecError(t, err)
	}
	if _, err := decode([]byte{0x1f, 0x8b, 8, 0, 0, 0}, codec.Options{}); err == nil {
		t.Fatal("broken gzip header accepted")
	} else {
		testimg.RequireCodecError(t, err)
	}
}

func TestSizeCaps(t *testing.T) {
	huge := gzipped(t, []byte(redSquare+strings.Repeat(" ", MaxDecompressedBytes+1024)))
	testimg.Within(t, 30*time.Second, func() {
		if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(huge), int64(len(huge)), codec.DefaultLimits()); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("decompression bomb: %v", err)
		}
	})
	large := gzipped(t, []byte(redSquare+strings.Repeat(" ", 20<<20)))
	testimg.Within(t, 30*time.Second, func() {
		if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(large), int64(len(large)), codec.DefaultLimits()); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("markup above the renderer limit: %v", err)
		}
	})
	if _, err := (Decoder{}).Probe(context.Background(), (virtualFile{size: MaxFileBytes + 1}), MaxFileBytes+1, codec.DefaultLimits()); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("file above 32 MB: %v", err)
	}
	lim := codec.DefaultLimits()
	lim.MaxFileBytes = 10
	if _, err := (Decoder{}).Probe(context.Background(), strings.NewReader(redSquare), int64(len(redSquare)), lim); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("configured file limit: %v", err)
	}
}

func TestDimensionLimits(t *testing.T) {
	big := `<svg viewBox="0 0 100000 100000"><rect width="1" height="1"/></svg>`
	if _, err := probe(big); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("100000x100000: %v", err)
	}
	if _, err := probe(`<svg viewBox="0 0 1e30 1e30"><rect width="1" height="1"/></svg>`); err == nil {
		t.Fatal("absurd view box accepted")
	} else {
		testimg.RequireCodecError(t, err)
	}
	lim := codec.DefaultLimits()
	lim.MaxPixels = 100
	if _, err := (Decoder{}).Decode(context.Background(), strings.NewReader(redSquare), int64(len(redSquare)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("intrinsic size over the limit: %v", err)
	}
}

func TestMalformedDocuments(t *testing.T) {
	cases := map[string]string{
		"empty":        "",
		"not xml":      "this is not xml at all",
		"unclosed":     `<svg viewBox="0 0 10 10"><rect`,
		"not svg root": `<html><body/></html>`,
		"deep nesting": `<svg viewBox="0 0 10 10">` + strings.Repeat("<g>", 5000) + strings.Repeat("</g>", 5000) + `</svg>`,
		"binary":       "\x00\x01\x02\x03",
	}
	for name, data := range cases {
		testimg.Within(t, 20*time.Second, func() {
			_, probeErr := probe(data)
			_, decodeErr := decode([]byte(data), codec.Options{})
			if probeErr == nil || decodeErr == nil {
				t.Errorf("%s: probe=%v decode=%v", name, probeErr, decodeErr)
				return
			}
			testimg.RequireCodecError(t, probeErr)
			testimg.RequireCodecError(t, decodeErr)
		})
	}
}

func TestHostileButBoundedDocuments(t *testing.T) {
	entities := `<?xml version="1.0"?><!DOCTYPE svg [<!ENTITY a "aaaaaaaaaa"><!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;"><!ENTITY c "&b;&b;&b;&b;&b;&b;&b;&b;&b;&b;">]><svg viewBox="0 0 10 10"><text>&c;&c;&c;&c;</text></svg>`
	useBomb := `<svg xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 10 10"><defs><g id="a"><rect width="1" height="1"/></g>` +
		`<g id="b"><use xlink:href="#a"/><use xlink:href="#a"/><use xlink:href="#a"/><use xlink:href="#a"/><use xlink:href="#a"/><use xlink:href="#a"/><use xlink:href="#a"/><use xlink:href="#a"/></g>` +
		`<g id="c"><use xlink:href="#b"/><use xlink:href="#b"/><use xlink:href="#b"/><use xlink:href="#b"/><use xlink:href="#b"/><use xlink:href="#b"/><use xlink:href="#b"/><use xlink:href="#b"/></g>` +
		`<g id="d"><use xlink:href="#c"/><use xlink:href="#c"/><use xlink:href="#c"/><use xlink:href="#c"/><use xlink:href="#c"/><use xlink:href="#c"/><use xlink:href="#c"/><use xlink:href="#c"/></g></defs><use xlink:href="#d"/></svg>`
	for _, data := range []string{entities, useBomb} {
		testimg.Within(t, 30*time.Second, func() {
			_, err := decode([]byte(data), codec.Options{VectorW: 32, VectorH: 32})
			if err != nil {
				testimg.RequireCodecError(t, err)
			}
		})
	}
}

func TestEveryTruncatedPrefix(t *testing.T) {
	data := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle cx="5" cy="5" r="4" fill="blue"/></svg>`)
	for n := 0; n <= len(data); n++ {
		prefix := data[:n]
		if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(prefix), int64(n), codec.DefaultLimits()); err != nil {
			testimg.RequireCodecError(t, err)
		}
		if _, err := decode(prefix, codec.Options{VectorW: 8, VectorH: 8}); err != nil {
			testimg.RequireCodecError(t, err)
		}
		if n < len(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle`) {
			if _, err := decode(prefix, codec.Options{VectorW: 8, VectorH: 8}); err == nil {
				t.Fatalf("prefix of %d bytes decoded", n)
			}
		}
	}
}

func TestCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Decoder{}).Decode(ctx, strings.NewReader(redSquare), int64(len(redSquare)), codec.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("decode: %v", err)
	}
	if _, err := (Decoder{}).Probe(ctx, strings.NewReader(redSquare), int64(len(redSquare)), codec.Limits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("probe: %v", err)
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte(redSquare))
	f.Add(gzipped(f, []byte(redSquare)))
	f.Add([]byte(`<?xml version="1.0"?><!DOCTYPE svg [<!ENTITY a "5">]><svg width="&a;" height="5"><circle r="2" cx="2" cy="2"/></svg>`))
	f.Add([]byte(`<svg viewBox="0 0 4 4"><defs><linearGradient id="g"><stop offset="0" stop-color="red"/></linearGradient></defs><rect width="4" height="4" fill="url(#g)"/></svg>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := testimg.SmallLimits()
		(Decoder{}).Sniff(data)
		(Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
		img, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim, VectorW: 16, VectorH: 16})
		if err == nil && (img == nil || img.First() == nil) {
			t.Fatal("success without pixels")
		}
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
	})
}

func TestBlankDocumentsRenderTransparent(t *testing.T) {
	for _, data := range []string{`<svg/>`, `<svg viewBox="0 0 0 0"/>`, `<svg xmlns="http://www.w3.org/2000/svg"></svg>`} {
		img, err := decode([]byte(data), codec.Options{VectorW: 8, VectorH: 8})
		if err != nil {
			t.Fatalf("%s: %v", data, err)
		}
		if img.Info.Width != 1 || img.Info.Height != 1 || img.First().NRGBAAt(4, 4) != (color.NRGBA{}) {
			t.Fatalf("%s: %+v", data, img.Info)
		}
	}
}
