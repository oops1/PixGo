package thumbs

import (
	"bytes"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/all"
)

func TestAllCodecsSmoke(t *testing.T) {
	reg := codec.NewRegistry()
	all.Register(reg)
	hn := newHarness(t, reg, Options{})
	src := quadrantImage(300, 200)
	files := map[string][]byte{}
	for _, id := range []codec.ID{codec.PNG, codec.BMP, codec.JPEG} {
		enc, ok := reg.Encoder(id)
		if !ok {
			t.Fatalf("no encoder for %s", id)
		}
		var buf bytes.Buffer
		if err := enc.Encode(&buf, src, codec.EncodeOptions{Quality: 90}); err != nil {
			t.Fatal(err)
		}
		files["s"+enc.Extension()] = buf.Bytes()
	}
	for name, data := range files {
		p := hn.write(name, data)
		th := hn.get(p, 128)
		if th.Err != nil {
			t.Errorf("%s: %v", name, th.Err)
			continue
		}
		if th.Img.Bounds().Dx() != 128 || th.Img.Bounds().Dy() != 85 {
			t.Errorf("%s: size %v", name, th.Img.Bounds())
		}
		if got := pixelAt(th.Img, 20, 20); !colorNear(got, colRed, 30) {
			t.Errorf("%s: top-left %v", name, got)
		}
	}
}
