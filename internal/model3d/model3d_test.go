package model3d

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/mesh"
)

func cubeSTL(side float32, attr uint16) []byte {
	c := [8][3]float32{{0, 0, 0}, {side, 0, 0}, {side, side, 0}, {0, side, 0}, {0, 0, side}, {side, 0, side}, {side, side, side}, {0, side, side}}
	faces := [12][3]int{{0, 2, 1}, {0, 3, 2}, {4, 5, 6}, {4, 6, 7}, {0, 1, 5}, {0, 5, 4}, {1, 2, 6}, {1, 6, 5}, {2, 3, 7}, {2, 7, 6}, {3, 0, 4}, {3, 4, 7}}
	b := make([]byte, 84, 84+50*len(faces))
	binary.LittleEndian.PutUint32(b[80:], uint32(len(faces)))
	for _, f := range faces {
		rec := make([]byte, 50)
		for k, i := range f {
			for a := 0; a < 3; a++ {
				binary.LittleEndian.PutUint32(rec[12+12*k+4*a:], math.Float32bits(c[i][a]))
			}
		}
		binary.LittleEndian.PutUint16(rec[48:], attr)
		b = append(b, rec...)
	}
	return b
}

func TestFineReadsAreCachedByContent(t *testing.T) {
	Forget()
	data := cubeSTL(7, 0)
	a, err := Read(context.Background(), codec.STL, bytes.NewReader(data), int64(len(data)), Fine)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Read(context.Background(), codec.STL, bytes.NewReader(append([]byte(nil), data...)), int64(len(data)), Fine)
	if err != nil || a != b {
		t.Fatalf("the same bytes give the cached mesh, err %v", err)
	}
	c, _ := Read(context.Background(), codec.STL, bytes.NewReader(data), int64(len(data)), Coarse)
	if c == a {
		t.Fatal("coarse reads are not cached")
	}
	if v := a.Volume(); math.Abs(math.Abs(v)-343) > 1e-3 {
		t.Fatalf("volume %v", v)
	}
}

func TestAUniformColorIsDropped(t *testing.T) {
	Forget()
	data := cubeSTL(5, 0x8000|31<<10)
	m, err := Read(context.Background(), codec.STL, bytes.NewReader(data), int64(len(data)), Fine)
	if err != nil {
		t.Fatal(err)
	}
	if m.HasColors() {
		t.Fatal("one color on every face is ignored")
	}
	two := &mesh.Mesh{V: make([]float32, 18), C: []uint8{1, 2, 3, 4, 5, 6}}
	dropUniformColor(two)
	if !two.HasColors() {
		t.Fatal("different colors stay")
	}
}

func TestNotAModel(t *testing.T) {
	if _, err := Read(context.Background(), codec.PNG, bytes.NewReader(nil), 0, Fine); err != ErrNotModel {
		t.Fatalf("err %v", err)
	}
	if KindOfPath("a.STP") != codec.STEP || KindOfPath("a.Stl") != codec.STL || KindOfPath("a.png") != "" {
		t.Fatal("kinds by extension")
	}
}

func TestPictureSizeKeepsTheAspectInsideTheBox(t *testing.T) {
	for _, tc := range []struct {
		aspect float64
		o      codec.Options
		w, h   int
	}{
		{2, codec.Options{}, 2048, 1024},
		{0.5, codec.Options{}, 1024, 2048},
		{1, codec.Options{VectorW: 300, VectorH: 200}, 200, 200},
		{3, codec.Options{VectorW: 300, VectorH: 200}, 300, 100},
		{math.NaN(), codec.Options{}, 2048, 2048},
		{1e9, codec.Options{}, 2048, 128},
	} {
		w, h := PictureSize(tc.aspect, tc.o, codec.Limits{})
		if w != tc.w || h != tc.h {
			t.Errorf("aspect %v %+v: %d×%d, want %d×%d", tc.aspect, tc.o, w, h, tc.w, tc.h)
		}
	}
	if w, h := PictureSize(1, codec.Options{}, codec.Limits{MaxPixels: 1 << 20}); int64(w)*int64(h) > 1<<20 {
		t.Fatalf("pixel limit ignored: %d×%d", w, h)
	}
}

func TestBoundsAspectFollowsTheShape(t *testing.T) {
	wide := BoundsAspect([6]float32{0, 0, 0, 100, 100, 5})
	tall := BoundsAspect([6]float32{0, 0, 0, 10, 10, 200})
	if !(wide > 1 && tall < 1) {
		t.Fatalf("wide %v tall %v", wide, tall)
	}
}
