package thumbs

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/jpeg"
)

type oversizeCodec struct {
	mu      sync.Mutex
	decodes []codec.Options
	opens   int
	readers []*overviewReader
}

type spec struct {
	w, h  int
	flags map[string]bool
}

func (c *oversizeCodec) ID() codec.ID           { return "ovr" }
func (c *oversizeCodec) Name() string           { return "Oversize" }
func (c *oversizeCodec) Extensions() []string   { return []string{".ovr"} }
func (c *oversizeCodec) Sniff(head []byte) bool { return bytes.HasPrefix(head, []byte("OVR ")) }

func (c *oversizeCodec) parse(r io.ReaderAt, size int64) spec {
	buf := make([]byte, size)
	r.ReadAt(buf, 0)
	fields := strings.Fields(strings.TrimPrefix(string(buf), "OVR "))
	dims := strings.SplitN(fields[0], "x", 2)
	s := spec{flags: map[string]bool{}}
	s.w, _ = strconv.Atoi(dims[0])
	s.h, _ = strconv.Atoi(dims[1])
	for _, f := range fields[1:] {
		s.flags[f] = true
	}
	return s
}

func (c *oversizeCodec) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	s := c.parse(r, size)
	info := codec.Info{Format: "ovr", Width: s.w, Height: s.h, Frames: 1, Orientation: codec.OrientNormal}
	if s.flags["bare"] {
		return codec.Info{}, codec.ErrTooLarge
	}
	if int64(s.w)*int64(s.h) > lim.MaxPixels {
		return info, codec.ErrTooLarge
	}
	return info, nil
}

func (c *oversizeCodec) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	s := c.parse(r, size)
	c.mu.Lock()
	c.decodes = append(c.decodes, o)
	c.mu.Unlock()
	info := codec.Info{Format: "ovr", Width: s.w, Height: s.h, Frames: 1, Orientation: codec.OrientNormal}
	if s.flags["scale"] && o.Scale > 1 {
		shift := 0
		for 1<<uint(shift) < o.Scale {
			shift++
		}
		w, h := codec.LevelSize(s.w, s.h, shift)
		if int64(w)*int64(h) > o.Limits.MaxPixels {
			return nil, codec.ErrTooLarge
		}
		return &codec.Image{Info: info, Frames: []codec.Frame{{Pix: solid(w, h, colRed)}}, Scale: o.Scale}, nil
	}
	if s.w*s.h > 1<<24 {
		return nil, codec.ErrTooLarge
	}
	return &codec.Image{Info: info, Frames: []codec.Frame{{Pix: solid(s.w, s.h, colBlue)}}, Scale: 1}, nil
}

func (c *oversizeCodec) decodeCalls() []codec.Options {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]codec.Options(nil), c.decodes...)
}

type tiledOversize struct {
	*oversizeCodec
	minShift int
}

type overviewReader struct {
	info      codec.Info
	minShift  int
	overviews []int
	closed    atomic.Int32
	fail      error
	orient    codec.Orientation
}

func (r *overviewReader) Info() codec.Info { return r.info }
func (r *overviewReader) MinShift() int    { return r.minShift }
func (r *overviewReader) Close() error {
	r.closed.Add(1)
	return nil
}

func (r *overviewReader) Region(ctx context.Context, shift int, rc image.Rectangle) (*image.NRGBA, error) {
	return nil, errors.New("not used by thumbnails")
}

func (r *overviewReader) Overview(ctx context.Context, shift int) (*image.NRGBA, error) {
	r.overviews = append(r.overviews, shift)
	if r.fail != nil {
		return nil, r.fail
	}
	w, h := codec.LevelSize(r.info.Width, r.info.Height, shift)
	return solid(w, h, colGreen), nil
}

func (t *tiledOversize) OpenTiled(ctx context.Context, rd io.ReaderAt, size int64, lim codec.Limits) (codec.TileReader, error) {
	s := t.parse(rd, size)
	t.mu.Lock()
	t.opens++
	t.mu.Unlock()
	if s.flags["opentilefail"] {
		return nil, codec.ErrUnsupported
	}
	r := &overviewReader{info: codec.Info{Format: "ovr", Width: s.w, Height: s.h, Frames: 1, Orientation: codec.OrientNormal}, minShift: t.minShift}
	if s.flags["ovfail"] {
		r.fail = codec.ErrCorrupt
	}
	if s.flags["rotated"] {
		r.info.Orientation = codec.OrientRotate90
	}
	t.mu.Lock()
	t.readers = append(t.readers, r)
	t.mu.Unlock()
	return r, nil
}

func ovrHarness(t *testing.T, dec codec.Decoder, lim codec.Limits) *harness {
	t.Helper()
	reg := codec.NewRegistry()
	reg.RegisterDecoder(dec)
	return newHarness(t, reg, Options{Limits: func() codec.Limits { return lim }})
}

func (h *harness) writeOvr(name, body string) string {
	h.t.Helper()
	return h.write(name+".ovr", []byte("OVR "+body))
}

func TestOversizeFileIsThumbnailedFromAScaledDecode(t *testing.T) {
	c := &oversizeCodec{}
	hn := ovrHarness(t, c, codec.DefaultLimits())
	p := hn.writeOvr("huge", "20000x15000 scale")
	th := hn.get(p, 256)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if th.Img.Bounds().Dx() != 256 || th.Img.Bounds().Dy() != 192 {
		t.Fatalf("thumbnail %v", th.Img.Bounds())
	}
	if th.SrcW != 20000 || th.SrcH != 15000 {
		t.Fatalf("source size %dx%d", th.SrcW, th.SrcH)
	}
	calls := c.decodeCalls()
	if len(calls) != 1 || calls[0].Scale != 8 {
		t.Fatalf("decode options %+v", calls)
	}
	if got := pixelAt(th.Img, 100, 100); !colorNear(got, colRed, 3) {
		t.Fatalf("pixel %v is not from the reduced decode", got)
	}
}

func TestOversizeFileThatCannotBeReducedStillFails(t *testing.T) {
	c := &oversizeCodec{}
	hn := ovrHarness(t, c, codec.DefaultLimits())
	p := hn.writeOvr("huge", "20000x15000 noscale")
	th := hn.get(p, 256)
	if !errors.Is(th.Err, codec.ErrTooLarge) {
		t.Fatalf("err %v", th.Err)
	}
	if th.SrcW != 20000 || th.SrcH != 15000 {
		t.Fatalf("failure must keep the source size: %dx%d", th.SrcW, th.SrcH)
	}
}

func TestOversizeWithoutFilledInfoFails(t *testing.T) {
	c := &oversizeCodec{}
	hn := ovrHarness(t, c, codec.DefaultLimits())
	p := hn.writeOvr("bare", "20000x15000 bare scale")
	th := hn.get(p, 256)
	if !errors.Is(th.Err, codec.ErrTooLarge) || th.SrcW != 0 {
		t.Fatalf("%+v", th)
	}
	if len(c.decodeCalls()) != 0 {
		t.Fatal("must not decode without a probed size")
	}
}

func TestOversizeScaledDecodeRespectsOutputPixelLimit(t *testing.T) {
	c := &oversizeCodec{}
	lim := codec.DefaultLimits()
	lim.MaxPixels = 1_000_000
	hn := ovrHarness(t, c, lim)
	p := hn.writeOvr("huge", "20000x15000 scale")
	th := hn.get(p, 256)
	if !errors.Is(th.Err, codec.ErrTooLarge) {
		t.Fatalf("the output pixel limit must still apply: %v", th.Err)
	}
}

func TestOversizeFileUsesTheOverviewOfATiledDecoder(t *testing.T) {
	c := &tiledOversize{oversizeCodec: &oversizeCodec{}}
	hn := ovrHarness(t, c, codec.DefaultLimits())
	p := hn.writeOvr("huge", "20000x15000")
	th := hn.get(p, 256)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if len(c.decodeCalls()) != 0 {
		t.Fatal("tiled decoders must not be decoded as a whole")
	}
	r := c.readers[0]
	if len(r.overviews) != 1 || r.overviews[0] != 3 || r.closed.Load() != 1 {
		t.Fatalf("overviews %v, closed %d", r.overviews, r.closed.Load())
	}
	if th.Img.Bounds().Dx() != 256 || th.Img.Bounds().Dy() != 192 || th.SrcW != 20000 {
		t.Fatalf("thumbnail %v from %dx%d", th.Img.Bounds(), th.SrcW, th.SrcH)
	}
	if got := pixelAt(th.Img, 50, 50); !colorNear(got, colGreen, 3) {
		t.Fatalf("pixel %v is not from the overview", got)
	}
}

func TestOversizeTiledOverviewPicksSmallestSufficientLevel(t *testing.T) {
	cases := []struct {
		side  int
		shift int
	}{{256, 3}, {1000, 3}, {1100, 2}, {2000, 2}, {2100, 1}}
	for _, tc := range cases {
		c := &tiledOversize{oversizeCodec: &oversizeCodec{}}
		lim := codec.DefaultLimits()
		lim.MaxPixels = 20_000_000
		hn := ovrHarness(t, c, lim)
		p := hn.writeOvr("big", "8000x6000")
		th := hn.get(p, tc.side)
		if th.Err != nil {
			t.Fatalf("side %d: %v", tc.side, th.Err)
		}
		if got := c.readers[0].overviews; len(got) != 1 || got[0] != tc.shift {
			t.Fatalf("side %d: overview shifts %v, want %d", tc.side, got, tc.shift)
		}
	}
}

func TestOversizeTiledOverviewHonoursMinShiftAndLimit(t *testing.T) {
	c := &tiledOversize{oversizeCodec: &oversizeCodec{}, minShift: 3}
	hn := ovrHarness(t, c, codec.DefaultLimits())
	p := hn.writeOvr("huge", "20000x15000")
	th := hn.get(p, 4096)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if got := c.readers[0].overviews; len(got) != 1 || got[0] != 3 {
		t.Fatalf("min shift ignored: %v", got)
	}
	c2 := &tiledOversize{oversizeCodec: &oversizeCodec{}}
	lim := codec.DefaultLimits()
	lim.MaxPixels = 1_000_000
	hn2 := ovrHarness(t, c2, lim)
	p2 := hn2.writeOvr("huge", "20000x15000")
	th = hn2.get(p2, 256)
	if !errors.Is(th.Err, codec.ErrTooLarge) {
		t.Fatalf("overview above the pixel limit: %v", th.Err)
	}
	if c2.readers[0].closed.Load() != 1 || len(c2.readers[0].overviews) != 0 {
		t.Fatalf("reader closed %d, overviews %v", c2.readers[0].closed.Load(), c2.readers[0].overviews)
	}
}

func TestOversizeTiledFailuresAreReported(t *testing.T) {
	c := &tiledOversize{oversizeCodec: &oversizeCodec{}}
	hn := ovrHarness(t, c, codec.DefaultLimits())
	open := hn.writeOvr("open", "20000x15000 opentilefail")
	th := hn.get(open, 256)
	if !errors.Is(th.Err, codec.ErrUnsupported) || th.SrcW != 20000 {
		t.Fatalf("open failure: %+v", th)
	}
	bad := hn.writeOvr("bad", "20000x15000 ovfail")
	th = hn.get(bad, 256)
	if !errors.Is(th.Err, codec.ErrCorrupt) {
		t.Fatalf("overview failure: %v", th.Err)
	}
	if c.readers[0].closed.Load() != 1 {
		t.Fatalf("reader closed %d times after a failed overview", c.readers[0].closed.Load())
	}
}

func TestOversizeTiledOrientationComesFromTheReader(t *testing.T) {
	c := &tiledOversize{oversizeCodec: &oversizeCodec{}}
	hn := ovrHarness(t, c, codec.DefaultLimits())
	p := hn.writeOvr("huge", "20000x15000 rotated")
	th := hn.get(p, 256)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if th.Orientation != codec.OrientRotate90 || th.Img.Bounds().Dx() != 192 || th.Img.Bounds().Dy() != 256 {
		t.Fatalf("orientation %d size %v", th.Orientation, th.Img.Bounds())
	}
}

type oversizeJPEG struct {
	countingJPEG
}

func (o *oversizeJPEG) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	info, err := o.countingJPEG.Probe(ctx, r, size, codec.DefaultLimits())
	if err != nil {
		return info, err
	}
	if int64(info.Width)*int64(info.Height) > lim.MaxPixels {
		return info, codec.ErrTooLarge
	}
	return info, nil
}

func TestOversizeJPEGStillUsesTheEXIFThumbnail(t *testing.T) {
	oj := &oversizeJPEG{countingJPEG{Decoder: jpeg.Decoder{}}}
	lim := codec.DefaultLimits()
	lim.MaxPixels = 1000
	hn := ovrHarness(t, oj, lim)
	path := hn.write("t.jpg", exifThumbFixture(t, 600, 400, 160, 107, 1))
	th := hn.get(path, 128)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if oj.decodes.Load() != 0 {
		t.Fatal("the EXIF fast path must work for oversize files too")
	}
	if th.SrcW != 600 || th.SrcH != 400 {
		t.Fatalf("source size %dx%d", th.SrcW, th.SrcH)
	}
	if got := pixelAt(th.Img, 64, 42); !colorNear(got, colRed, 12) {
		t.Fatalf("centre = %v", got)
	}
}
