package imaging

import (
	"context"
	"encoding/binary"
	"errors"
	"image"
	"sync/atomic"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

type scriptedReader struct {
	pic      picture
	script   []byte
	cursor   atomic.Int64
	closed   atomic.Int32
	overview *image.NRGBA
}

func (s *scriptedReader) Info() codec.Info { return s.pic.info() }
func (s *scriptedReader) MinShift() int    { return s.pic.minShift }
func (s *scriptedReader) Close() error {
	s.closed.Add(1)
	return nil
}

func (s *scriptedReader) Overview(ctx context.Context, shift int) (*image.NRGBA, error) {
	return s.overview, nil
}

func (s *scriptedReader) next() byte {
	if len(s.script) == 0 {
		return 0
	}
	i := int(s.cursor.Add(1)-1) % len(s.script)
	return s.script[i]
}

func (s *scriptedReader) Region(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA, error) {
	switch s.next() % 8 {
	case 0:
		return nil, errors.New("scripted failure")
	case 1:
		panic("scripted panic")
	case 2:
		return nil, nil
	case 3:
		return image.NewNRGBA(image.Rect(0, 0, r.Dx()+1, r.Dy())), nil
	case 4:
		return image.NewNRGBA(image.Rect(0, 0, 0, 0)), nil
	case 5:
		return nil, ctx.Err()
	case 6:
		img := image.NewNRGBA(image.Rect(5, 7, 5+r.Dx(), 7+r.Dy()))
		return img, nil
	}
	return image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy())), nil
}

func FuzzTileManager(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 7, 7, 7, 7})
	f.Add([]byte{255, 255, 255, 255, 255, 255, 255, 255, 1, 2, 3, 4, 5, 6})
	f.Add(binary.LittleEndian.AppendUint64([]byte{9, 9, 9, 9, 9, 9, 9, 9}, 0x8000000000000001))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 12 {
			return
		}
		w := 1 + int(binary.LittleEndian.Uint16(data[0:2]))%30000
		h := 1 + int(binary.LittleEndian.Uint16(data[2:4]))%30000
		base := 1 + int(data[4])%4
		for {
			lw, lh := codec.LevelSize(w, h, base)
			if int64(lw)*int64(lh) <= 1<<20 {
				break
			}
			base++
		}
		lw, lh := codec.LevelSize(w, h, base)
		rd := &scriptedReader{pic: picture{w: w, h: h, minShift: int(data[5]) % 3}, script: data[8:]}
		rd.overview = image.NewNRGBA(image.Rect(0, 0, lw, lh))
		budget := int64(1+int(data[6])%8) * TileSide * TileSide * 4
		m := NewTileManager(rd, BuildPyramid(rd.overview, 0), base, w, h, TileOptions{Budget: budget, Workers: 1 + int(data[7])%3}, nil)
		rest := data[8:]
		for len(rest) >= 9 {
			shift := int(int8(rest[0])) % 8
			x0 := int(int16(binary.LittleEndian.Uint16(rest[1:3]))) * 3
			y0 := int(int16(binary.LittleEndian.Uint16(rest[3:5]))) * 3
			dx := int(binary.LittleEndian.Uint16(rest[5:7])) % 6000
			dy := int(binary.LittleEndian.Uint16(rest[7:9])) % 4000
			rest = rest[9:]
			need := image.Rect(x0, y0, x0+dx, y0+dy)
			win := m.Window(shift, need)
			if win.Img == nil {
				continue
			}
			vw, vh := codec.LevelSize(w, h, shift)
			clipped := need.Intersect(image.Rect(0, 0, vw, vh))
			if win.Shift != shift || !clipped.In(win.Rect()) || !win.Rect().In(image.Rect(0, 0, vw, vh)) {
				t.Fatalf("window %v shift %d for need %v at shift %d", win.Rect(), win.Shift, need, shift)
			}
			area := win.Rect().Dx() * win.Rect().Dy()
			if win.Img.Rect.Dx() != win.Rect().Dx() || win.Img.Rect.Dy() != win.Rect().Dy() || len(win.Img.Pix) != area*4 || area > maxWindowPixels {
				t.Fatalf("image %v with %d bytes does not match window %v", win.Img.Rect, len(win.Img.Pix), win.Rect())
			}
			if st := m.Stats(); st.TileBytes > budget {
				t.Fatalf("tile cache %d over budget %d", st.TileBytes, budget)
			}
		}
		m.Close()
		if rd.closed.Load() != 1 {
			t.Fatalf("reader closed %d times", rd.closed.Load())
		}
	})
}
