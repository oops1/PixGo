package jpegdec

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type recordedMCU struct {
	offset int64
	bit    uint8
	pred   [maxComponents]int16
	coef   [maxComponents][]int16
}

func collectMCUs(t *testing.T, d *Decoder, g *geometry, s *stream, limit int) ([]recordedMCU, *streamScan) {
	t.Helper()
	ss, err := newStreamScan(d, g, s)
	if err != nil {
		t.Fatal(err)
	}
	band := newBandBuf(d.f, g, 1, g.mxx)
	strides := band.strides(d.f, g.mxx)
	var out []recordedMCU
	for my := 0; my < g.myy && len(out) < limit; my++ {
		band.clear(d.f)
		views := band.view(d.f, g, g.mxx, 0)
		for mx := 0; mx < g.mxx; mx++ {
			m := my*g.mxx + mx
			offset, bit, ok := ss.ent.position()
			rec := recordedMCU{offset: offset, bit: bit, pred: ss.pred}
			if !ok || ss.ent.eobRun != 0 {
				rec.offset = -1
			}
			if err := ss.decodeMCU(views, strides, mx, 0, false); err != nil {
				t.Fatal(err)
			}
			for c := 0; c < d.f.nComp; c++ {
				comp := &d.f.comp[c]
				for j := 0; j < comp.v; j++ {
					at := (j*strides[c] + comp.h*mx) * g.kk[c]
					rec.coef[c] = append(rec.coef[c], views[c][at:at+comp.h*g.kk[c]]...)
				}
			}
			if err := ss.afterMCU(m); err != nil {
				t.Fatal(err)
			}
			out = append(out, rec)
		}
	}
	return out, ss
}

func TestResumingAtAnyMCUReproducesTheSameCoefficients(t *testing.T) {
	files := syntheticFiles()
	for _, name := range []string{"ones-420", "ones-444-restart", "ones-gray", "noise-422-mixed", "420-restart-65x31"} {
		d := open(t, files[name], Options{})
		g := d.f.geometry(0, 0)
		s := newStream(t.Context(), bytes.NewReader(files[name]), int64(len(files[name])))
		s.seek(d.scanOffset)
		recorded, _ := collectMCUs(t, d, g, s, 1<<30)
		s.release()
		resumed := 0
		for index, rec := range recorded {
			if rec.offset < 0 {
				continue
			}
			s2 := newStream(t.Context(), bytes.NewReader(files[name]), int64(len(files[name])))
			ss, err := newStreamScan(d, g, s2)
			if err != nil {
				t.Fatal(err)
			}
			ss.ent.resume(rec.offset, rec.bit)
			ss.pred = rec.pred
			band := newBandBuf(d.f, g, 1, g.mxx)
			views := band.view(d.f, g, g.mxx, 0)
			strides := band.strides(d.f, g.mxx)
			mx := index % g.mxx
			if err := ss.decodeMCU(views, strides, mx, 0, false); err != nil {
				t.Fatalf("%s MCU %d: %v", name, index, err)
			}
			for c := 0; c < d.f.nComp; c++ {
				comp := &d.f.comp[c]
				var got []int16
				for j := 0; j < comp.v; j++ {
					at := (j*strides[c] + comp.h*mx) * g.kk[c]
					got = append(got, views[c][at:at+comp.h*g.kk[c]]...)
				}
				if !equalInt16(got, rec.coef[c]) {
					t.Fatalf("%s MCU %d component %d: resumed coefficients differ", name, index, c)
				}
			}
			s2.release()
			resumed++
		}
		if resumed < len(recorded)*3/4 {
			t.Fatalf("%s: only %d of %d positions could be resumed", name, resumed, len(recorded))
		}
		d.Close()
	}
}

func TestStuffedBytesAreCountedWhenRewinding(t *testing.T) {
	data := []byte{0xff, 0xd8, 0x12, 0xff, 0x00, 0xff, 0x00, 0x34, 0x56, 0xff, 0x00, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x11, 0x22}
	s := newStream(t.Context(), bytes.NewReader(data), int64(len(data)))
	defer s.release()
	s.seek(2)
	e := entropy{s: s}
	e.refill()
	for consumed := 0; consumed < 40; consumed += 3 {
		offset, bit, ok := e.position()
		if !ok {
			t.Fatal("no position")
		}
		unstuffed := 0
		for i := int64(2); i < offset; i++ {
			if data[i] == 0xff && data[i+1] == 0x00 {
				i++
			}
			unstuffed++
		}
		if consumed/8 != unstuffed || int(bit) != consumed%8 {
			t.Fatalf("after %d bits: offset %d bit %d (%d whole bytes behind)", consumed, offset, bit, unstuffed)
		}
		e.n -= 3
	}
}

type shortReader struct {
	data []byte
	max  int
}

func (r *shortReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p[:min(len(p), r.max)], r.data[off:])
	return n, nil
}

func TestShortReadsAreHandled(t *testing.T) {
	files := corpus(t)
	for _, name := range []string{"video-001.q50.420.jpeg", "video-001.restart2.jpeg", "video-001.q50.420.progressive.jpeg"} {
		want := decodeAt(t, files[name], 0, Options{})
		for _, max := range []int{1, 3, 7, 4096} {
			d, err := Open(t.Context(), &shortReader{data: files[name], max: max}, int64(len(files[name])), Options{Workers: 1})
			if err != nil {
				t.Fatalf("%s short %d: %v", name, max, err)
			}
			got, err := d.Decode(t.Context(), 0)
			if err != nil || !samePixels(got, want) {
				t.Fatalf("%s short %d: %v", name, max, err)
			}
			d.Close()
		}
	}
}

type failingReader struct {
	data  []byte
	after int64
	err   error
}

func (r *failingReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= r.after {
		return 0, r.err
	}
	n := copy(p, r.data[off:min(int64(len(r.data)), r.after)])
	return n, nil
}

func TestReaderFailuresSurfaceAsErrors(t *testing.T) {
	data := corpus(t)["video-001.q50.420.jpeg"]
	broken := errors.New("disk on fire")
	for _, after := range []int64{100, 400, int64(len(data)) - 300} {
		d, err := Open(t.Context(), &failingReader{data: data, after: after, err: broken}, int64(len(data)), Options{Workers: 1})
		if err == nil {
			_, err = d.Decode(t.Context(), 0)
			d.Close()
		}
		if !errors.Is(err, broken) {
			t.Fatalf("after %d bytes: %v", after, err)
		}
	}
}

func TestStreamSkipAndSeek(t *testing.T) {
	data := make([]byte, 100_000)
	for i := range data {
		data[i] = byte(i * 7)
	}
	s := newStream(context.Background(), bytes.NewReader(data), int64(len(data)))
	defer s.release()
	if b, err := s.readByte(); err != nil || b != data[0] {
		t.Fatal(b, err)
	}
	if err := s.skip(10); err != nil {
		t.Fatal(err)
	}
	if b, _ := s.readByte(); b != data[11] {
		t.Fatalf("after a short skip: %d", b)
	}
	if err := s.skip(50_000); err != nil {
		t.Fatal(err)
	}
	if s.offset() != 50_012 {
		t.Fatalf("offset %d", s.offset())
	}
	if b, _ := s.readByte(); b != data[50_012] {
		t.Fatalf("after a long skip: %d", b)
	}
	s.seek(5)
	if b, _ := s.readByte(); b != data[5] {
		t.Fatalf("after seeking backwards: %d", b)
	}
	if err := s.skip(200_000); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("skip past the end: %v", err)
	}
	s.seek(int64(len(data)))
	if _, err := s.readByte(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read at the end: %v", err)
	}
}
