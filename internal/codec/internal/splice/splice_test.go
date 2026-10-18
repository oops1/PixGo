package splice

import (
	"bytes"
	"errors"
	"testing"
)

type failing struct{ after int }

func (f *failing) Write(p []byte) (int, error) {
	if f.after <= 0 {
		return 0, errors.New("disk full")
	}
	f.after -= len(p)
	return len(p), nil
}

func TestInsertsAtTheOffsetWhateverTheWriteBoundaries(t *testing.T) {
	src := []byte("0123456789abcdef")
	for at := 0; at <= len(src); at++ {
		for chunk := 1; chunk <= len(src); chunk++ {
			var out bytes.Buffer
			w := New(&out, at, []byte("<<X>>"))
			for off := 0; off < len(src); off += chunk {
				end := min(len(src), off+chunk)
				n, err := w.Write(src[off:end])
				if err != nil || n != end-off {
					t.Fatalf("at %d chunk %d: n=%d err=%v", at, chunk, n, err)
				}
			}
			if err := w.Flush(); err != nil {
				t.Fatal(err)
			}
			want := string(src[:at]) + "<<X>>" + string(src[at:])
			if out.String() != want {
				t.Fatalf("at %d chunk %d: %q, want %q", at, chunk, out.String(), want)
			}
		}
	}
}

func TestNothingToInsertPassesThrough(t *testing.T) {
	var out bytes.Buffer
	w := New(&out, 3, nil)
	w.Write([]byte("abcdef"))
	w.Flush()
	if out.String() != "abcdef" {
		t.Fatal(out.String())
	}
}

func TestShortStreamStillGetsTheDataOnFlush(t *testing.T) {
	var out bytes.Buffer
	w := New(&out, 10, []byte("!"))
	w.Write([]byte("abc"))
	w.Flush()
	if out.String() != "abc!" {
		t.Fatal(out.String())
	}
}

func TestWriteErrorsSurface(t *testing.T) {
	w := New(&failing{after: 4}, 2, []byte("XXXXXXXX"))
	if _, err := w.Write([]byte("abcdef")); err == nil {
		t.Fatal("expected an error")
	}
}
