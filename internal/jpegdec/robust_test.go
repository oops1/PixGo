package jpegdec

import (
	"bytes"
	"context"
	"errors"
	stdjpeg "image/jpeg"
	"io"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func decodeAll(data []byte, o Options) (*Decoder, error) {
	d, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)), o)
	if err != nil {
		return nil, err
	}
	if _, err := d.Decode(context.Background(), 0); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func mixedFiles(t testing.TB) (map[string][]byte, []string) {
	files := corpus(t)
	for name, data := range syntheticFiles() {
		files["synthetic-"+name] = data
	}
	return files, sortedNames(files)
}

func TestMutatedFilesAreAcceptedExactlyLikeTheStandardLibrary(t *testing.T) {
	files, names := mixedFiles(t)
	rng := rand.New(rand.NewSource(99))
	var accepted, different int
	for iter := 0; iter < 6000; iter++ {
		name := names[rng.Intn(len(names))]
		mutated := append([]byte(nil), files[name]...)
		for k := 0; k < 1+rng.Intn(4); k++ {
			mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
		}
		if rng.Intn(5) == 0 {
			mutated = mutated[:rng.Intn(len(mutated))]
		}
		if config, err := stdjpeg.DecodeConfig(bytes.NewReader(mutated)); err == nil && config.Width*config.Height > 1<<22 {
			continue
		}
		want, wantErr := referenceNRGBA(t, mutated)
		d, err := Open(t.Context(), bytes.NewReader(mutated), int64(len(mutated)), Options{MaxPixels: 1 << 24})
		if err == nil {
			img, derr := d.Decode(t.Context(), 0)
			d.Close()
			err = derr
			if derr == nil && wantErr == nil {
				accepted++
				if !samePixels(img, want) {
					different++
				}
			}
		}
		if (err == nil) != (wantErr == nil) {
			t.Fatalf("%s iteration %d: mine %v, standard library %v", name, iter, err, wantErr)
		}
	}
	if accepted == 0 || different*100 > accepted {
		t.Fatalf("%d of %d accepted mutated files decode differently", different, accepted)
	}
}

func TestEveryTruncationBehavesLikeTheStandardLibrary(t *testing.T) {
	files := corpus(t)
	for _, name := range []string{"video-001.q50.420.jpeg", "video-001.restart2.jpeg", "video-005.gray.q50.jpeg", "video-001.q50.420.progressive.jpeg", "video-001.cmyk.jpeg"} {
		data := files[name]
		step := max(1, len(data)/700)
		for n := 0; n < len(data); n += step {
			prefix := data[:n]
			_, wantErr := referenceNRGBA(t, prefix)
			d, err := decodeAll(prefix, Options{})
			if d != nil {
				d.Close()
			}
			if (err == nil) != (wantErr == nil) {
				t.Fatalf("%s prefix %d/%d: mine %v, standard library %v", name, n, len(data), err, wantErr)
			}
		}
	}
}

func TestUnsupportedAndCorruptFilesAreClassified(t *testing.T) {
	good := corpus(t)["video-001.q50.420.jpeg"]
	patch := func(offset int, value byte) []byte {
		out := append([]byte(nil), good...)
		out[offset] = value
		return out
	}
	sof := bytes.Index(good, []byte{0xff, 0xc0})
	if sof < 0 {
		t.Fatal("no SOF0")
	}
	cases := []struct {
		name        string
		data        []byte
		unsupported bool
	}{
		{"arithmetic coding", patch(sof+1, 0xc9), true},
		{"lossless", patch(sof+1, 0xc3), true},
		{"twelve bit precision", patch(sof+4, 12), true},
		{"unusual component count", patch(sof+3, 0x0e), true},
		{"not a jpeg", []byte("GIF89a......"), false},
		{"empty", nil, false},
		{"only SOI", []byte{0xff, 0xd8}, false},
		{"duplicate SOF", append(append(append([]byte(nil), good[:sof+19]...), good[sof:sof+19]...), good[sof+19:]...), false},
	}
	for _, tc := range cases {
		_, err := Open(t.Context(), bytes.NewReader(tc.data), int64(len(tc.data)), Options{})
		if err == nil {
			t.Errorf("%s: accepted", tc.name)
			continue
		}
		var unsupported UnsupportedError
		if got := errors.As(err, &unsupported); got != tc.unsupported {
			t.Errorf("%s: unsupported=%v for %v", tc.name, got, err)
		}
	}
}

func TestPixelLimitIsCheckedBeforeAnyAllocation(t *testing.T) {
	header := []byte{
		0xff, 0xd8,
		0xff, 0xc0, 0x00, 0x11, 0x08, 0xff, 0xff, 0xff, 0xff,
		0x03, 0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01,
		0xff, 0xda, 0x00, 0x0c, 0x03, 0x01, 0x00, 0x02, 0x11, 0x03, 0x11, 0x00, 0x3f, 0x00,
		0xff, 0xd9,
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for _, limit := range []int64{0, 1 << 20, 1 << 40} {
		_, err := Open(t.Context(), bytes.NewReader(header), int64(len(header)), Options{MaxPixels: limit})
		if err == nil {
			t.Fatalf("limit %d: accepted a 65535x65535 header in a %d byte file", limit, len(header))
		}
		if limit != 1<<40 && !errors.Is(err, ErrTooLarge) {
			t.Fatalf("limit %d: %v", limit, err)
		}
	}
	runtime.ReadMemStats(&after)
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 8<<20 {
		t.Fatalf("rejecting a hostile header allocated %d bytes", grown)
	}
}

func TestWholeFileSmallerThanItsBlocksIsRejectedEarly(t *testing.T) {
	data := corpus(t)["video-001.q50.420.jpeg"]
	sof := bytes.Index(data, []byte{0xff, 0xc0})
	patched := append([]byte(nil), data...)
	patched[sof+5], patched[sof+6] = 0x7f, 0xff
	patched[sof+7], patched[sof+8] = 0x7f, 0xff
	_, err := Open(t.Context(), bytes.NewReader(patched), int64(len(patched)), Options{MaxPixels: 1 << 31})
	if err == nil || errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
}

func TestCancelledContextStopsEveryEntryPoint(t *testing.T) {
	data := corpus(t)["video-001.q50.420.jpeg"]
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(ctx, bytes.NewReader(data), int64(len(data)), Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("open: %v", err)
	}
	if _, err := ReadHeader(ctx, bytes.NewReader(data), int64(len(data))); !errors.Is(err, context.Canceled) {
		t.Fatalf("header: %v", err)
	}
	d := open(t, data, Options{})
	defer d.Close()
	if _, err := d.Decode(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("decode: %v", err)
	}
	if _, err := d.Region(ctx, 0, rect(0, 0, 10, 10)); !errors.Is(err, context.Canceled) {
		t.Fatalf("region: %v", err)
	}
	if err := d.BuildIndex(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("index: %v", err)
	}
}

type slowReader struct {
	data  []byte
	reads chan struct{}
	once  sync.Once
}

func (s *slowReader) ReadAt(p []byte, off int64) (int, error) {
	s.once.Do(func() { close(s.reads) })
	if off >= int64(len(s.data)) {
		return 0, io.EOF
	}
	time.Sleep(time.Millisecond)
	n := copy(p, s.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestDecodeStopsPromptlyWhenCancelled(t *testing.T) {
	data := mediumJPEG(t)
	for _, workers := range []int{1, 4} {
		ctx, cancel := context.WithCancel(context.Background())
		reader := &slowReader{data: data, reads: make(chan struct{})}
		d, err := Open(ctx, reader, int64(len(data)), Options{Workers: workers})
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			<-reader.reads
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()
		start := time.Now()
		_, err = d.Decode(ctx, 0)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("workers %d: %v", workers, err)
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Fatalf("workers %d: cancellation took %v", workers, elapsed)
		}
		d.Close()
	}
}

func TestClosedDecoderRefusesWork(t *testing.T) {
	data := corpus(t)["video-001.q50.420.jpeg"]
	d := open(t, data, Options{})
	d.Close()
	d.Close()
	if _, err := d.Decode(t.Context(), 0); !errors.Is(err, ErrClosed) {
		t.Fatalf("decode: %v", err)
	}
	if _, err := d.Region(t.Context(), 0, rect(0, 0, 4, 4)); !errors.Is(err, ErrClosed) {
		t.Fatalf("region: %v", err)
	}
	if err := d.BuildIndex(t.Context()); !errors.Is(err, ErrClosed) {
		t.Fatalf("index: %v", err)
	}
}

func TestShiftOutOfRangeIsRejected(t *testing.T) {
	d := open(t, corpus(t)["video-001.q50.420.jpeg"], Options{})
	defer d.Close()
	for _, shift := range []int{-1, 4, 9} {
		if _, err := d.Decode(t.Context(), shift); err == nil {
			t.Fatalf("shift %d accepted", shift)
		}
		if _, err := d.Region(t.Context(), shift, rect(0, 0, 4, 4)); err == nil {
			t.Fatalf("region at shift %d accepted", shift)
		}
	}
}

func TestHeaderMatchesTheFile(t *testing.T) {
	files := corpus(t)
	for _, name := range sortedNames(files) {
		data := files[name]
		h, err := ReadHeader(t.Context(), bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if h.Width != 150 || h.Height != 103 {
			t.Fatalf("%s: %+v", name, h)
		}
		wantComponents := 3
		switch {
		case strings.Contains(name, "gray"):
			wantComponents = 1
		case strings.Contains(name, "cmyk"):
			wantComponents = 4
		}
		if h.Components != wantComponents || h.Progressive != strings.Contains(name, "progressive") {
			t.Fatalf("%s: %+v", name, h)
		}
	}
}

func TestSpecificMalformedHeaders(t *testing.T) {
	cases := map[string][]byte{
		"short segment length":   {0xff, 0xd8, 0xff, 0xe0, 0x00, 0x01},
		"truncated length":       {0xff, 0xd8, 0xff, 0xe0, 0x00},
		"marker without payload": {0xff, 0xd8, 0xff},
		"unknown marker":         {0xff, 0xd8, 0xff, 0x01, 0x00, 0x02},
		"missing SOF":            {0xff, 0xd8, 0xff, 0xda, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3f, 0x00},
		"segment past the end":   {0xff, 0xd8, 0xff, 0xfe, 0x7f, 0xff, 0x01},
		"restart marker only":    {0xff, 0xd8, 0xff, 0xd0, 0xff, 0xd9},
	}
	for name, data := range cases {
		if _, err := ReadHeader(t.Context(), bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("%s: header accepted", name)
		}
		if _, err := Open(t.Context(), bytes.NewReader(data), int64(len(data)), Options{}); err == nil {
			t.Errorf("%s: open accepted", name)
		}
	}
}

func TestWorkersDoNotLeakAfterSuccessErrorOrCancellation(t *testing.T) {
	data := mediumJPEG(t)
	before := runtime.NumGoroutine()
	for i := 0; i < 6; i++ {
		d := open(t, data, Options{Workers: 4})
		if _, err := d.Decode(t.Context(), i%4); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Region(t.Context(), 0, rect(10, 10, 1500, 900)); err != nil {
			t.Fatal(err)
		}
		d.Close()
		broken := data[:len(data)/2]
		if d, err := Open(t.Context(), bytes.NewReader(broken), int64(len(broken)), Options{Workers: 4}); err == nil {
			if _, err := d.Decode(t.Context(), 0); err == nil {
				t.Fatal("truncated file decoded")
			}
			d.Close()
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if d, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)), Options{Workers: 4}); err == nil {
			d.Decode(ctx, 0)
			d.Close()
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutines grew from %d to %d", before, after)
	}
}
