package cad

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func cubeSTL(side float32) []byte {
	c := [8][3]float32{{0, 0, 0}, {side, 0, 0}, {side, side, 0}, {0, side, 0}, {0, 0, side}, {side, 0, side}, {side, side, side}, {0, side, side}}
	faces := [12][3]int{{0, 2, 1}, {0, 3, 2}, {4, 5, 6}, {4, 6, 7}, {0, 1, 5}, {0, 5, 4}, {1, 2, 6}, {1, 6, 5}, {2, 3, 7}, {2, 7, 6}, {3, 0, 4}, {3, 4, 7}}
	b := make([]byte, 84, 84+50*len(faces))
	copy(b, "solid binary header that lies")
	binary.LittleEndian.PutUint32(b[80:], uint32(len(faces)))
	for _, f := range faces {
		rec := make([]byte, 50)
		for k, i := range f {
			for a := 0; a < 3; a++ {
				binary.LittleEndian.PutUint32(rec[12+12*k+4*a:], math.Float32bits(c[i][a]))
			}
		}
		b = append(b, rec...)
	}
	return b
}

func boxSTEP(x, y, z float64) string {
	var b strings.Builder
	b.WriteString("ISO-10303-21;\nHEADER;\nFILE_DESCRIPTION((''),'2;1');\nFILE_NAME('box','',(''),(''),'','','');\nFILE_SCHEMA(('CONFIG_CONTROL_DESIGN'));\nENDSEC;\nDATA;\n")
	pts := [8][3]float64{{0, 0, 0}, {x, 0, 0}, {x, y, 0}, {0, y, 0}, {0, 0, z}, {x, 0, z}, {x, y, z}, {0, y, z}}
	for i, p := range pts {
		fmt.Fprintf(&b, "#%d=CARTESIAN_POINT('',(%g,%g,%g));\n", i+1, p[0], p[1], p[2])
	}
	quads := [6][4]int{{1, 4, 3, 2}, {5, 6, 7, 8}, {1, 2, 6, 5}, {2, 3, 7, 6}, {3, 4, 8, 7}, {4, 1, 5, 8}}
	var faces []string
	for i, q := range quads {
		loop, bound, face := 20+3*i, 21+3*i, 22+3*i
		fmt.Fprintf(&b, "#%d=POLY_LOOP('',(#%d,#%d,#%d,#%d));\n", loop, q[0], q[1], q[2], q[3])
		fmt.Fprintf(&b, "#%d=FACE_OUTER_BOUND('',#%d,.T.);\n", bound, loop)
		fmt.Fprintf(&b, "#%d=FACE('',(#%d));\n", face, bound)
		faces = append(faces, fmt.Sprintf("#%d", face))
	}
	fmt.Fprintf(&b, "#50=CLOSED_SHELL('',(%s));\n", strings.Join(faces, ","))
	b.WriteString("#51=FACETED_BREP('',#50);\n#52=FACETED_BREP_SHAPE_REPRESENTATION('',(#51),$);\nENDSEC;\nEND-ISO-10303-21;\n")
	return b.String()
}

func registry() *codec.Registry {
	r := codec.NewRegistry()
	r.RegisterDecoder(STLDecoder{})
	r.RegisterDecoder(STEPDecoder{})
	return r
}

func TestBinarySTLIsClaimedByItsExtension(t *testing.T) {
	data := cubeSTL(10)
	r := registry()
	d, ok := r.Detect(data[:min(len(data), codec.SniffLen)], "part.stl")
	if !ok || d.ID() != codec.STL {
		t.Fatalf("detect %v %v", d, ok)
	}
	if _, ok := r.Detect(data[:min(len(data), codec.SniffLen)], "part.bin"); ok {
		t.Fatal("no signature, no claim outside .stl")
	}
}

func TestSTLDecodesToATransparentPictureOfTheModel(t *testing.T) {
	data := cubeSTL(10)
	info, err := STLDecoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !info.Model || !info.Vector || !info.HasAlpha || info.Format != codec.STL || max(info.Width, info.Height) != 2048 {
		t.Fatalf("info %+v", info)
	}
	img, err := STLDecoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{VectorW: 300, VectorH: 200})
	if err != nil {
		t.Fatal(err)
	}
	pix := img.First()
	b := pix.Bounds()
	if b.Dx() > 300 || b.Dy() > 200 || (b.Dx() != 300 && b.Dy() != 200) {
		t.Fatalf("picture %v does not fit 300×200 snugly", b)
	}
	if pix.NRGBAAt(0, 0).A != 0 {
		t.Fatal("the corner is transparent")
	}
	if pix.NRGBAAt(b.Dx()/2, b.Dy()/2).A != 255 {
		t.Fatal("the middle shows the model")
	}
	if !img.Info.Model || img.Info.Width != b.Dx() || img.Info.Height != b.Dy() {
		t.Fatalf("decoded info %+v", img.Info)
	}
}

func TestSTEPBoxDecodes(t *testing.T) {
	data := []byte(boxSTEP(40, 20, 10))
	r := registry()
	d, ok := r.Detect(data, "box.step")
	if !ok || d.ID() != codec.STEP {
		t.Fatalf("detect %v %v", d, ok)
	}
	info, err := d.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !info.Model || info.Width <= info.Height {
		t.Fatalf("a long flat box gives a wide picture, info %+v", info)
	}
	img, err := d.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{VectorW: 256, VectorH: 256})
	if err != nil {
		t.Fatal(err)
	}
	pix := img.First()
	if pix.NRGBAAt(pix.Bounds().Dx()/2, pix.Bounds().Dy()/2).A != 255 {
		t.Fatal("the middle shows the box")
	}
}

func TestBrokenModelsFailWithACodecError(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty stl":    {},
		"short stl":    cubeSTL(10)[:120],
		"garbage step": []byte("ISO-10303-21;\nDATA;\n#1=FOO((((;\n"),
	} {
		var err error
		if strings.HasSuffix(name, "step") {
			_, err = STEPDecoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{})
		} else {
			_, err = STLDecoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{})
		}
		if err == nil || !(errors.Is(err, codec.ErrCorrupt) || errors.Is(err, codec.ErrUnsupported)) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCancelledDecodeStops(t *testing.T) {
	data := cubeSTL(10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (STLDecoder{}).Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}
