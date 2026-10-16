package jpegxform

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand"
	"runtime"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func fixtureJPEG(t testing.TB, mutate func(*buildSpec)) ([]byte, buildSpec) {
	t.Helper()
	s := samplingSpec(64, 48, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
	if mutate != nil {
		mutate(&s)
	}
	return buildJPEG(t, s, randomWall(rand.New(rand.NewSource(30)), s)), s
}

func patchSegment(t testing.TB, data []byte, marker byte, patch func(payload []byte) []byte) []byte {
	t.Helper()
	out := append([]byte{}, data[:2]...)
	pos := 2
	done := false
	for pos+4 <= len(data) && data[pos] == 0xFF {
		m := data[pos+1]
		n := int(data[pos+2])<<8 | int(data[pos+3])
		payload := data[pos+4 : pos+2+n]
		if m == marker && !done {
			done = true
			payload = patch(append([]byte{}, payload...))
			out = appendSegment(out, m, payload)
		} else {
			out = append(out, data[pos:pos+2+n]...)
		}
		pos += 2 + n
		if m == markerSOS {
			break
		}
	}
	if !done {
		t.Fatalf("no segment %#x", marker)
	}
	return append(out, data[pos:]...)
}

func wantErr(t *testing.T, data []byte, op Op, o Options, target error) {
	t.Helper()
	_, err := runTransform(data, op, o)
	if !errors.Is(err, target) {
		t.Fatalf("got %v, want %v", err, target)
	}
}

func TestUnsupportedCodingProcessesAskTheCallerToReencode(t *testing.T) {
	progressive := readCorpus(t, "video-001.q50.420.progressive.jpeg")
	wantErr(t, progressive, Rotate90, Options{}, codec.ErrUnsupported)
	if _, err := Perfect(bytes.NewReader(progressive), int64(len(progressive)), Rotate90); !errors.Is(err, codec.ErrUnsupported) {
		t.Fatalf("Perfect on a progressive file: %v", err)
	}
	base, _ := fixtureJPEG(t, nil)
	for _, marker := range []byte{0xC2, 0xC3, 0xC5, 0xC6, 0xC7, 0xC9, 0xCA, 0xCB, 0xCD, 0xCE, 0xCF} {
		data := bytes.Replace(base, []byte{0xFF, markerSOF0}, []byte{0xFF, marker}, 1)
		wantErr(t, data, FlipH, Options{}, codec.ErrUnsupported)
	}
	twelve := patchSegment(t, base, markerSOF0, func(p []byte) []byte { p[0] = 12; return p })
	wantErr(t, twelve, FlipH, Options{}, codec.ErrUnsupported)
	two := patchSegment(t, base, markerSOF0, func(p []byte) []byte { return append(p[:6:6], p[6:12]...) })
	two = patchSegment(t, two, markerSOF0, func(p []byte) []byte { p[5] = 2; return p })
	wantErr(t, two, FlipH, Options{}, codec.ErrUnsupported)
	dnl := patchSegment(t, base, markerSOF0, func(p []byte) []byte { p[1], p[2] = 0, 0; return p })
	wantErr(t, dnl, FlipH, Options{}, codec.ErrUnsupported)
	arithmeticTables := bytes.Replace(base, []byte{0xFF, markerDQT}, []byte{0xFF, markerDNL, 0, 2, 0xFF, markerDQT}, 1)
	wantErr(t, arithmeticTables, FlipH, Options{}, codec.ErrUnsupported)
}

func TestScanThatDoesNotCarryAllComponentsIsUnsupported(t *testing.T) {
	base, _ := fixtureJPEG(t, nil)
	one := patchSegment(t, base, markerSOS, func(p []byte) []byte {
		return []byte{1, p[1], p[2], 0, 63, 0}
	})
	wantErr(t, one, FlipH, Options{}, codec.ErrUnsupported)
	spectral := patchSegment(t, base, markerSOS, func(p []byte) []byte { p[len(p)-2] = 5; return p })
	wantErr(t, spectral, FlipH, Options{}, codec.ErrUnsupported)
	approx := patchSegment(t, base, markerSOS, func(p []byte) []byte { p[len(p)-1] = 1; return p })
	wantErr(t, approx, FlipH, Options{}, codec.ErrUnsupported)
}

func TestMalformedHeadersAreCorrupt(t *testing.T) {
	base, _ := fixtureJPEG(t, nil)
	cases := map[string][]byte{
		"empty":              {},
		"no SOI":             {0x12, 0x34, 0x56},
		"only SOI":           {0xFF, markerSOI},
		"second SOI":         bytes.Replace(base, []byte{0xFF, markerDQT}, []byte{0xFF, markerSOI, 0xFF, markerDQT}, 1),
		"EOI first":          {0xFF, markerSOI, 0xFF, markerEOI},
		"length below two":   {0xFF, markerSOI, 0xFF, markerCOM, 0, 1},
		"length past end":    {0xFF, markerSOI, 0xFF, markerCOM, 0xFF, 0xFF, 1, 2},
		"zero width":         patchSegment(t, base, markerSOF0, func(p []byte) []byte { p[3], p[4] = 0, 0; return p }),
		"short SOF":          patchSegment(t, base, markerSOF0, func(p []byte) []byte { return p[:5] }),
		"SOF length":         patchSegment(t, base, markerSOF0, func(p []byte) []byte { return p[:len(p)-1] }),
		"bad sampling":       patchSegment(t, base, markerSOF0, func(p []byte) []byte { p[7] = 0x50; return p }),
		"zero sampling":      patchSegment(t, base, markerSOF0, func(p []byte) []byte { p[7] = 0x01; return p }),
		"bad quant id":       patchSegment(t, base, markerSOF0, func(p []byte) []byte { p[8] = 7; return p }),
		"repeated id":        patchSegment(t, base, markerSOF0, func(p []byte) []byte { p[9] = p[6]; return p }),
		"too many blocks":    patchSegment(t, base, markerSOF0, func(p []byte) []byte { p[7], p[10], p[13] = 0x44, 0x44, 0x11; return p }),
		"second SOF":         bytes.Replace(base, []byte{0xFF, markerDQT}, append(append([]byte{0xFF, markerSOF0}, 0, 11, 8, 0, 8, 0, 8, 1, 1, 0x11, 0), 0xFF, markerDQT), 1),
		"short DQT":          patchSegment(t, base, markerDQT, func(p []byte) []byte { return p[:30] }),
		"bad DQT precision":  patchSegment(t, base, markerDQT, func(p []byte) []byte { p[0] = 0x20; return p }),
		"bad DQT id":         patchSegment(t, base, markerDQT, func(p []byte) []byte { p[0] = 0x07; return p }),
		"short DHT":          patchSegment(t, base, markerDHT, func(p []byte) []byte { return p[:10] }),
		"DHT cut symbols":    patchSegment(t, base, markerDHT, func(p []byte) []byte { return p[:len(p)-3] }),
		"bad DHT class":      patchSegment(t, base, markerDHT, func(p []byte) []byte { p[0] = 0x30; return p }),
		"bad DHT id":         patchSegment(t, base, markerDHT, func(p []byte) []byte { p[0] = 0x0A; return p }),
		"SOS short":          patchSegment(t, base, markerSOS, func(p []byte) []byte { return p[:3] }),
		"SOS empty":          patchSegment(t, base, markerSOS, func(p []byte) []byte { return nil }),
		"unknown scan comp":  patchSegment(t, base, markerSOS, func(p []byte) []byte { p[1] = 99; return p }),
		"repeated scan comp": patchSegment(t, base, markerSOS, func(p []byte) []byte { p[3] = p[1]; return p }),
		"scan table id":      patchSegment(t, base, markerSOS, func(p []byte) []byte { p[2] = 0x77; return p }),
		"DRI length":         bytes.Replace(base, []byte{0xFF, markerDQT}, []byte{0xFF, markerDRI, 0, 3, 1, 0xFF, markerDQT}, 1),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := runTransform(data, FlipH, Options{})
			if !errors.Is(err, codec.ErrCorrupt) {
				t.Fatalf("got %v", err)
			}
			if _, err := Perfect(bytes.NewReader(data), int64(len(data)), FlipH); err == nil && name != "second SOF" {
				t.Fatalf("Perfect accepted the header")
			}
		})
	}
}

func TestMissingTablesAreCorrupt(t *testing.T) {
	base, _ := fixtureJPEG(t, nil)
	noDHT := bytes.Join(splitOut(t, base, markerDHT), nil)
	wantErr(t, noDHT, FlipH, Options{}, codec.ErrCorrupt)
	noDQT := bytes.Join(splitOut(t, base, markerDQT), nil)
	wantErr(t, noDQT, FlipH, Options{}, codec.ErrCorrupt)
}

func splitOut(t testing.TB, data []byte, marker byte) [][]byte {
	t.Helper()
	var parts [][]byte
	parts = append(parts, data[:2])
	pos := 2
	for pos+4 <= len(data) {
		m := data[pos+1]
		n := int(data[pos+2])<<8 | int(data[pos+3])
		if m != marker {
			parts = append(parts, data[pos:pos+2+n])
		}
		pos += 2 + n
		if m == markerSOS {
			break
		}
	}
	return append(parts, data[pos:])
}

func TestMarkersBetweenSegmentsAreTolerated(t *testing.T) {
	base, _ := fixtureJPEG(t, nil)
	padded := bytes.Replace(base, []byte{0xFF, markerDQT}, []byte{0xFF, 0xFF, 0xFF, markerTEM, 0xFF, markerRST0, 'j', 'u', 'n', 'k', 0xFF, markerDQT}, 1)
	padded = bytes.Replace(padded, []byte{0xFF, 0xC4}, []byte{0xFF, 0xF3, 0, 4, 9, 9, 0xFF, 0xC4}, 1)

	out := mustTransform(t, padded, FlipH, Options{})

	_, again := parseCoefficients(t, out)
	_, wantGrids := parseCoefficients(t, base)
	if len(again) != len(wantGrids) {
		t.Fatal("components differ")
	}
}

func TestEveryTruncationIsRejectedOrProducesAValidFile(t *testing.T) {
	for _, name := range []string{"video-001.restart2.jpeg", "video-005.gray.q50.2x2.jpeg", "video-001.q50.420.jpeg"} {
		data := readCorpus(t, name)
		for cut := 0; cut < len(data); cut += 1 + len(data)/400 {
			out, err := runTransform(data[:cut], Rotate90, Options{Trim: true})
			if err != nil {
				if !errors.Is(err, codec.ErrCorrupt) && !errors.Is(err, codec.ErrUnsupported) {
					t.Fatalf("%s cut at %d: %v", name, cut, err)
				}
				continue
			}
			if _, err := decodeStdErr(out); err != nil {
				t.Fatalf("%s cut at %d: success with an invalid file: %v", name, cut, err)
			}
		}
	}
}

func TestRandomCorruptionNeverPanicsAndSuccessDecodes(t *testing.T) {
	rng := rand.New(rand.NewSource(31))
	for _, name := range supportedCorpus(t) {
		data := readCorpus(t, name)
		for i := 0; i < 40; i++ {
			mutated := append([]byte{}, data...)
			for k := 0; k < 1+rng.Intn(6); k++ {
				mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
			}
			out, err := runTransform(mutated, Op(rng.Intn(8)), Options{Trim: true})
			if err != nil {
				continue
			}
			if _, err := decodeStdErr(out); err != nil {
				t.Fatalf("%s: a successful run produced a file that does not decode: %v", name, err)
			}
		}
	}
}

func TestRandomScanDataNeverPanics(t *testing.T) {
	rng := rand.New(rand.NewSource(32))
	for _, style := range []tableStyle{flatTables, skewedTables} {
		base, _ := fixtureJPEG(t, func(s *buildSpec) { s.style = style })
		scanAt := bytes.LastIndex(base, []byte{0xFF, markerSOS})
		scanAt += 2 + 2 + int(base[scanAt+2])<<8 + int(base[scanAt+3]) - 2
		for i := 0; i < 200; i++ {
			data := append([]byte{}, base[:scanAt]...)
			noise := make([]byte, rng.Intn(600))
			rng.Read(noise)
			data = append(data, noise...)
			if rng.Intn(2) == 0 {
				data = append(data, 0xFF, markerEOI)
			}
			out, err := runTransform(data, Op(rng.Intn(8)), Options{Trim: true})
			if err == nil {
				if _, err := decodeStdErr(out); err != nil {
					t.Fatalf("success with an invalid file: %v", err)
				}
			}
		}
	}
}

func scanOffset(t testing.TB, data []byte) int {
	t.Helper()
	pos := 2
	for pos+4 <= len(data) {
		m := data[pos+1]
		n := int(data[pos+2])<<8 | int(data[pos+3])
		pos += 2 + n
		if m == markerSOS {
			return pos
		}
	}
	t.Fatal("no scan")
	return 0
}

func TestRestartMarkersMustBeThere(t *testing.T) {
	base, _ := fixtureJPEG(t, func(s *buildSpec) { s.restart = 2 })
	at := bytes.Index(base[scanOffset(t, base):], []byte{0xFF, markerRST0})
	if at < 0 {
		t.Fatal("fixture has no restart markers")
	}
	at += scanOffset(t, base)

	wrong := append([]byte{}, base...)
	wrong[at+1] = markerEOI
	wantErr(t, wrong, FlipH, Options{}, codec.ErrCorrupt)

	missing := append(append([]byte{}, base[:at]...), base[at+2:]...)
	if _, err := runTransform(missing, FlipH, Options{}); err == nil {
		t.Fatal("a missing restart marker went unnoticed")
	}

	outOfSequence := append([]byte{}, base...)
	outOfSequence[at+1] = markerRST0 + 5
	mustTransform(t, outOfSequence, FlipH, Options{})

	junk := append(append(append([]byte{}, base[:at]...), 0x12, 0x34), base[at:]...)
	mustTransform(t, junk, FlipH, Options{})
}

type failingReaderAt struct {
	data  []byte
	after int
	err   error
}

func (f failingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if int(off) >= f.after {
		return 0, f.err
	}
	end := min(int(off)+len(p), f.after, len(f.data))
	n := copy(p, f.data[off:end])
	if n < len(p) {
		return n, f.err
	}
	return n, nil
}

func TestReaderErrorsAreReturnedAsTheyAre(t *testing.T) {
	data := readCorpus(t, "video-001.jpeg")
	boom := errors.New("disk on fire")
	for _, after := range []int{0, 1, 100, 300, len(data) / 2, len(data) - 40} {
		var out bytes.Buffer
		err := Transform(&out, failingReaderAt{data: data, after: after, err: boom}, int64(len(data)), Rotate90, Options{Trim: true})
		if !errors.Is(err, boom) {
			t.Errorf("after %d bytes: %v", after, err)
		}
	}
	_, err := Perfect(failingReaderAt{data: data, after: 50, err: boom}, int64(len(data)), FlipH)
	if err != nil && !errors.Is(err, boom) && !errors.Is(err, codec.ErrCorrupt) {
		t.Errorf("Perfect: %v", err)
	}
}

func TestSizeLargerThanTheDataIsCorrupt(t *testing.T) {
	data := readCorpus(t, "video-001.jpeg")
	var out bytes.Buffer
	err := Transform(&out, bytes.NewReader(data[:len(data)/2]), int64(len(data)), Rotate90, Options{Trim: true})
	if !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
}

type limitedWriter struct {
	left int
	err  error
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if len(p) > w.left {
		return w.left, w.err
	}
	w.left -= len(p)
	return len(p), nil
}

func TestWriterErrorsAreReturned(t *testing.T) {
	data := readCorpus(t, "video-001.jpeg")
	whole, err := runTransform(data, FlipH, Options{Trim: true})
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("disk full")
	for _, room := range []int{0, 10, 200, len(whole) / 2, len(whole) - 3, len(whole) - 1} {
		err := Transform(&limitedWriter{left: room, err: boom}, bytes.NewReader(data), int64(len(data)), FlipH, Options{Trim: true})
		if !errors.Is(err, boom) {
			t.Errorf("room %d of %d: %v", room, len(whole), err)
		}
	}
}

type cancelOnRead struct {
	r      io.ReaderAt
	cancel context.CancelFunc
	after  int
	reads  int
}

func (c *cancelOnRead) ReadAt(p []byte, off int64) (int, error) {
	c.reads++
	if c.reads > c.after {
		c.cancel()
	}
	return c.r.ReadAt(p, off)
}

func TestCancellationStopsTheWork(t *testing.T) {
	data := readCorpus(t, "video-001.jpeg")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := TransformContext(ctx, &out, bytes.NewReader(data), int64(len(data)), FlipH, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("already cancelled: %v", err)
	}
	if out.Len() != 0 {
		t.Fatal("output written after cancellation")
	}
	big, _ := fixtureJPEG(t, func(s *buildSpec) { s.width, s.height = 1024, 768 })
	for _, op := range []Op{Identity, Rotate90} {
		ctx, cancel = context.WithCancel(context.Background())
		src := &cancelOnRead{r: bytes.NewReader(big), cancel: cancel, after: 0}
		err := TransformContext(ctx, io.Discard, src, int64(len(big)), op, Options{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("%v cancelled while reading: %v", op, err)
		}
	}
}

func TestHeaderClaimingAHugePictureIsRefusedBeforeAllocating(t *testing.T) {
	base, _ := fixtureJPEG(t, nil)
	huge := patchSegment(t, base, markerSOF0, func(p []byte) []byte {
		p[1], p[2], p[3], p[4] = 0xFF, 0xFF, 0xFF, 0xFF
		return p
	})
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	wantErr(t, huge, FlipH, Options{}, codec.ErrTooLarge)
	_, err := runTransform(huge, FlipH, Options{MaxPixels: 1 << 33, Trim: true})

	runtime.ReadMemStats(&after)
	if !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("huge claim with no data: %v", err)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 64<<20 {
		t.Fatalf("%d MB allocated for a %d byte file", grew>>20, len(huge))
	}
}

func TestLowerPixelLimitIsHonoured(t *testing.T) {
	base, _ := fixtureJPEG(t, nil)
	wantErr(t, base, FlipH, Options{MaxPixels: 64*48 - 1}, codec.ErrTooLarge)
	mustTransform(t, base, FlipH, Options{MaxPixels: 64 * 48})
}

func TestInvalidOperationIsAnError(t *testing.T) {
	base, _ := fixtureJPEG(t, nil)
	if _, err := runTransform(base, Op(9), Options{}); err == nil {
		t.Fatal("Op(9) was accepted")
	}
	if _, err := Perfect(bytes.NewReader(base), int64(len(base)), Op(9)); err == nil {
		t.Fatal("Perfect accepted Op(9)")
	}
	if Op(9).String() != "invalid" || Rotate90.String() != "rotate-90" || Op(9).Transposes() {
		t.Fatal("Op helpers misbehave on invalid values")
	}
}

func TestExcessiveMetadataIsUnsupportedNotAMemoryHog(t *testing.T) {
	spec := samplingSpec(16, 16, [2]int{1, 1})
	chunk := bytes.Repeat([]byte{1}, 60000)
	for i := 0; i < 600; i++ {
		spec.segments = append(spec.segments, segment{marker: markerAPP0 + 5, payload: chunk})
	}
	data := buildJPEG(t, spec, randomWall(rand.New(rand.NewSource(33)), spec))

	wantErr(t, data, FlipH, Options{}, codec.ErrUnsupported)
	if ok, err := Perfect(bytes.NewReader(data), int64(len(data)), FlipH); err != nil || !ok {
		t.Fatalf("Perfect must not read metadata: %v %v", ok, err)
	}
}

func TestOutOfRangeDCAndRunsAreCorrupt(t *testing.T) {
	spec := samplingSpec(8, 8, [2]int{1, 1})
	spec.dcTables = []int{0}
	grids := newWall(spec)
	grids[0].block(0, 0)[0] = 20000
	data := buildJPEG(t, spec, grids)
	wantErr(t, data, FlipH, Options{}, codec.ErrCorrupt)
}

type junkReader struct{ head []byte }

func (j junkReader) ReadAt(p []byte, off int64) (int, error) {
	for i := range p {
		pos := off + int64(i)
		if pos < int64(len(j.head)) {
			p[i] = j.head[pos]
		} else {
			p[i] = 'x'
		}
	}
	return len(p), nil
}

func TestCancellationReachesTheHeaderScan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	src := &cancelOnRead{r: junkReader{head: []byte{0xFF, markerSOI}}, cancel: cancel, after: 2}

	err := TransformContext(ctx, io.Discard, src, 900<<20, FlipH, Options{})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if src.reads > 6 {
		t.Fatalf("%d reads happened after the cancellation", src.reads)
	}
}

func TestInputBeyondAGigabyteIsRefused(t *testing.T) {
	err := Transform(io.Discard, junkReader{}, 2<<30, FlipH, Options{})

	if !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
}

func TestJunkWithoutMarkersIsCorruptNotAHang(t *testing.T) {
	err := Transform(io.Discard, junkReader{head: []byte{0xFF, markerSOI}}, 3<<20, FlipH, Options{})

	if !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
}
