//go:build !noapng

package all

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	stdpng "image/png"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func pngChunk(kind string, body []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(body)))
	start := len(out)
	out = append(out, kind...)
	out = append(out, body...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[start:]))
}

func animatedPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := stdpng.Encode(&buf, testimg.Noise(8, 6, 3, true)); err != nil {
		t.Fatal(err)
	}
	plain := buf.Bytes()
	const afterHeader = 8 + 25
	control := binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint32(nil, 2), 0)
	frame := make([]byte, 26)
	binary.BigEndian.PutUint32(frame[4:], 8)
	binary.BigEndian.PutUint32(frame[8:], 6)
	binary.BigEndian.PutUint16(frame[20:], 1)
	binary.BigEndian.PutUint16(frame[22:], 10)
	idat := plain[afterHeader : len(plain)-12]
	out := append([]byte(nil), plain[:afterHeader]...)
	out = append(out, pngChunk("acTL", control)...)
	out = append(out, pngChunk("fcTL", frame)...)
	out = append(out, idat...)
	binary.BigEndian.PutUint32(frame[:4], 1)
	out = append(out, pngChunk("fcTL", frame)...)
	data := idat[8 : len(idat)-4]
	out = append(out, pngChunk("fdAT", append(binary.BigEndian.AppendUint32(nil, 2), data...))...)
	return append(out, plain[len(plain)-12:]...)
}

func TestAnimatedPNGIsClaimedByAPNGWhateverItsName(t *testing.T) {
	r := newRegistry()
	data := animatedPNG(t)
	plain := samples(t)[1].data
	for _, name := range []string{"a.png", "a.apng", "a.PNG", "a.dat", "a", "a.jpg"} {
		d, ok := r.Detect(head(data), name)
		if !ok || d.ID() != codec.APNG {
			t.Fatalf("%s: animated PNG detected as %v", name, d)
		}
		d, ok = r.Detect(head(plain), name)
		if !ok || (name != "a.jpg" && d.ID() != codec.PNG) {
			t.Fatalf("%s: plain PNG detected as %v", name, d)
		}
	}
	if !r.Supports("movie.apng") {
		t.Fatal(".apng is not supported")
	}
}

func TestAnimatedPNGThroughTheRegistry(t *testing.T) {
	r := newRegistry()
	data := animatedPNG(t)
	src := &codec.Source{Path: "x.png", R: bytes.NewReader(data), Size: int64(len(data)), Head: head(data)}
	d, info, err := r.Probe(context.Background(), src, codec.DefaultLimits())
	if err != nil || d.ID() != codec.APNG || info.Frames != 2 || !info.Animated {
		t.Fatalf("%v %+v %v", d, info, err)
	}
	img, err := r.Decode(context.Background(), d, src, codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames})
	if err != nil || len(img.Frames) != 2 {
		t.Fatalf("%v", err)
	}
}
