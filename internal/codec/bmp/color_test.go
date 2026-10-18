package bmp

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func v5File(colorSpace uint32, profile []byte, profileOffset, profileSize uint32) []byte {
	const width, height = 3, 2
	rowBytes := (width*24 + 31) / 32 * 4
	pixelStart := uint32(14 + 124)
	pixels := make([]byte, rowBytes*height)
	for i := range pixels {
		pixels[i] = byte(i*11 + 3)
	}
	out := make([]byte, 14+124)
	out[0], out[1] = 'B', 'M'
	le := binary.LittleEndian
	le.PutUint32(out[10:], pixelStart)
	dib := out[14:]
	le.PutUint32(dib[0:], 124)
	le.PutUint32(dib[4:], width)
	le.PutUint32(dib[8:], height)
	le.PutUint16(dib[12:], 1)
	le.PutUint16(dib[14:], 24)
	le.PutUint32(dib[56:], colorSpace)
	le.PutUint32(dib[112:], profileOffset)
	le.PutUint32(dib[116:], profileSize)
	out = append(out, pixels...)
	le.PutUint32(out[2:], uint32(len(out)+len(profile)))
	return append(out, profile...)
}

func decodeBMP(t testing.TB, data []byte) *codec.Image {
	t.Helper()
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestVersion5HeaderReportsTheEmbeddedProfile(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 200)
	offset := uint32(124 + 24)
	data := v5File(0x4d424544, profile, offset, uint32(len(profile)))
	img := decodeBMP(t, data)
	if !bytes.Equal(img.Color.ICC, profile) || img.Color.CICP != nil {
		t.Fatalf("%d bytes", len(img.Color.ICC))
	}
}

func TestVersion5HeaderWithoutAnEmbeddedProfileHasNoColor(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 200)
	for _, space := range []uint32{0, 0x73524742, 0x57696e20, 0x4c494e4b} {
		data := v5File(space, profile, 124+24, uint32(len(profile)))
		if got := decodeBMP(t, data).Color; !got.Empty() {
			t.Fatalf("colour space %#x: %+v", space, got)
		}
	}
}

func TestBrokenProfileReferencesAreIgnored(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 200)
	size := uint32(len(profile))
	cases := map[string][]byte{
		"offset past the end": v5File(0x4d424544, profile, 1<<30, size),
		"size past the end":   v5File(0x4d424544, profile, 124+24, size+500),
		"zero size":           v5File(0x4d424544, profile, 124+24, 0),
		"huge size":           v5File(0x4d424544, profile, 124+24, 1<<31),
		"offset overflow":     v5File(0x4d424544, profile, 0xffffffff, size),
		"not a profile":       v5File(0x4d424544, bytes.Repeat([]byte{7}, 200), 124+24, 200),
		"cmyk profile":        v5File(0x4d424544, testimg.SampleICCSpace("CMYK", 20), 124+24, uint32(len(testimg.SampleICCSpace("CMYK", 20)))),
	}
	for name, data := range cases {
		if got := decodeBMP(t, data).Color; !got.Empty() {
			t.Fatalf("%s: %d bytes reported", name, len(got.ICC))
		}
	}
}

func TestOlderHeadersHaveNoColor(t *testing.T) {
	var buf bytes.Buffer
	if err := (Encoder{}).Encode(&buf, testimg.Photo(8, 8), codec.EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := decodeBMP(t, buf.Bytes()).Color; !got.Empty() {
		t.Fatalf("%+v", got)
	}
}
