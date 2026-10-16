package imaging

import (
	"bytes"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/jpeg"
)

func photoLike(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := uint8(x * 255 / w)
			g := uint8(y * 255 / h)
			b := uint8((x*3 + y*5) / 16)
			if (x/97+y/89)%2 == 0 {
				r, g = r/2+40, g/2+40
			}
			if (x*y)%1031 < 3 {
				b = 255 - b
			}
			o := img.PixOffset(x, y)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = r, g, b, 255
		}
	}
	return img
}

func writeJPEG(t *testing.T, path string, img image.Image) {
	t.Helper()
	var buf bytes.Buffer
	if err := stdjpeg.Encode(&buf, img, &stdjpeg.Options{Quality: 88}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

type jpegRig struct {
	t      *testing.T
	ui     *uiQueue
	cache  *Cache
	loader *Loader
	lim    codec.Limits
	states []State
}

func newJPEGRig(t *testing.T, lim codec.Limits) *jpegRig {
	t.Helper()
	reg := codec.NewRegistry()
	reg.RegisterDecoder(jpeg.Decoder{})
	r := &jpegRig{t: t, ui: &uiQueue{}, cache: NewCache(1 << 30), lim: lim}
	r.loader = NewLoader(reg, r.cache, r.ui.Post, 2, func() codec.Limits { return r.lim })
	r.loader.Subscribe(func(p string) {
		if e, ok := r.loader.Entry(p); ok {
			r.states = append(r.states, e.State)
		}
	})
	t.Cleanup(func() {
		r.loader.Close()
		r.cache.Clear()
	})
	return r
}

func (r *jpegRig) wait(path string, st State) Entry {
	r.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		r.ui.Drain()
		if e, ok := r.loader.Entry(path); ok {
			if e.State == StateFailed {
				r.t.Fatalf("load failed: %v", e.Err)
			}
			if e.State == st {
				return e
			}
		}
		if time.Now().After(deadline) {
			r.t.Fatalf("timeout waiting for state %d", st)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestLoaderServesTilesFromTheRealJPEGReader(t *testing.T) {
	const w, h = 5200, 3600
	src := photoLike(w, h)
	path := filepath.Join(t.TempDir(), "big.jpg")
	writeJPEG(t, path, src)
	lim := codec.DefaultLimits()
	lim.MaxPixels = 4_000_000
	r := newJPEGRig(t, lim)
	r.loader.Want([]Request{{Path: path, Prio: PrioCurrent}})
	d := r.wait(path, StateReady).Img
	if d.Shift != 1 || d.Detail == nil || d.Reduced {
		t.Fatalf("shift %d detail %v", d.Shift, d.Detail)
	}
	s := d.Source()
	if s.Width != w || s.Height != h || s.Levels[0].Bounds().Dx() != 2600 || s.Levels[0].Bounds().Dy() != 1800 {
		t.Fatalf("source %dx%d level %v", s.Width, s.Height, s.Levels[0].Bounds())
	}
	var full image.Image
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	full, err = stdjpeg.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	mgr := d.Detail.(*TileManager)
	need := image.Rect(1900, 1300, 3300, 2200)
	win := d.Detail.Window(0, need)
	deadline := time.Now().Add(60 * time.Second)
	for {
		win = d.Detail.Window(0, need)
		if st := mgr.Stats(); st.Pending == 0 && st.Inflight == 0 {
			win = d.Detail.Window(0, need)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tiles did not arrive: %+v", mgr.Stats())
		}
		time.Sleep(time.Millisecond)
	}
	if st := mgr.Stats(); st.Failed != 0 || st.Decoded == 0 {
		t.Fatalf("stats %+v", st)
	}
	worst := 0
	for y := 0; y < win.Img.Rect.Dy(); y += 3 {
		for x := 0; x < win.Img.Rect.Dx(); x += 3 {
			px := win.Img.NRGBAAt(x, y)
			rr, gg, bb, _ := color.NRGBAModel.Convert(full.At(win.X+x, win.Y+y)).RGBA()
			for _, d := range []int{int(px.R) - int(rr>>8), int(px.G) - int(gg>>8), int(px.B) - int(bb>>8)} {
				worst = max(worst, abs(d))
			}
		}
	}
	if worst > 2 {
		t.Fatalf("tiles differ from the standard decoder by up to %d levels", worst)
	}
}

func TestLoaderPreviewStageWithTheRealJPEGDecoder(t *testing.T) {
	const w, h = 3000, 2000
	src := photoLike(w, h)
	path := filepath.Join(t.TempDir(), "photo.jpg")
	writeJPEG(t, path, src)
	r := newJPEGRig(t, codec.DefaultLimits())
	r.loader.SetViewportHint(800, 600)
	r.loader.Want([]Request{{Path: path, Prio: PrioCurrent}})
	final := r.wait(path, StateReady).Img
	if final.Shift != 0 || final.Reduced || final.Levels[0].Bounds().Dx() != w {
		t.Fatalf("final %+v", final)
	}
	if got := r.states; len(got) != 3 || got[0] != StateProbed || got[1] != StatePreview || got[2] != StateReady {
		t.Fatalf("states %v, want Probed, Preview, Ready", got)
	}
}

func TestLoaderPreviewMatchesTheFullDecodeOfTheRealJPEGDecoder(t *testing.T) {
	const w, h = 3000, 2000
	src := photoLike(w, h)
	path := filepath.Join(t.TempDir(), "photo.jpg")
	writeJPEG(t, path, src)
	r := newJPEGRig(t, codec.DefaultLimits())
	r.loader.SetViewportHint(800, 600)
	preview := make(chan *Decoded, 1)
	r.loader.Subscribe(func(p string) {
		if e, ok := r.loader.Entry(p); ok && e.State == StatePreview {
			select {
			case preview <- e.Img:
			default:
			}
		}
	})
	r.loader.Want([]Request{{Path: path, Prio: PrioCurrent}})
	final := r.wait(path, StateReady).Img
	var pv *Decoded
	select {
	case pv = <-preview:
	default:
		t.Fatal("no preview was published")
	}
	if !pv.Reduced || pv.Shift != 1 {
		t.Fatalf("preview %+v", pv)
	}
	ps := pv.Source()
	if ps.Width != w || ps.Height != h || ps.Levels[0].Bounds().Dx() != 1500 || ps.Levels[0].Bounds().Dy() != 1000 {
		t.Fatalf("preview source %dx%d level %v", ps.Width, ps.Height, ps.Levels[0].Bounds())
	}
	half := BuildPyramid(final.Levels[0], 0)[1]
	var sum, n int
	for y := 0; y < 1000; y += 2 {
		for x := 0; x < 1500; x += 2 {
			a, b := pv.Levels[0].NRGBAAt(x, y), half.NRGBAAt(x, y)
			sum += abs(int(a.R)-int(b.R)) + abs(int(a.G)-int(b.G)) + abs(int(a.B)-int(b.B))
			n += 3
		}
	}
	if mean := float64(sum) / float64(n); mean > 3 {
		t.Fatalf("the preview differs from the reduced full decode by %.2f levels on average", mean)
	}
}

func BenchmarkOpenJPEGWithPreview(b *testing.B) {
	const w, h = 6000, 4000
	path := filepath.Join(b.TempDir(), "photo24.jpg")
	var buf bytes.Buffer
	if err := stdjpeg.Encode(&buf, photoLike(w, h), &stdjpeg.Options{Quality: 88}); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		b.Fatal(err)
	}
	var toPreview, toReady time.Duration
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reg := codec.NewRegistry()
		reg.RegisterDecoder(jpeg.Decoder{})
		ui := &uiQueue{}
		cache := NewCache(1 << 30)
		loader := NewLoader(reg, cache, ui.Post, 8, nil)
		loader.SetViewportHint(1920, 1080)
		start := time.Now()
		loader.Want([]Request{{Path: path, Prio: PrioCurrent}})
		var previewAt time.Duration
		for {
			ui.Drain()
			e, ok := loader.Entry(path)
			if ok && e.State == StatePreview && previewAt == 0 {
				previewAt = time.Since(start)
			}
			if ok && e.State == StateReady {
				break
			}
			if ok && e.State == StateFailed {
				b.Fatal(e.Err)
			}
			time.Sleep(200 * time.Microsecond)
		}
		toReady += time.Since(start)
		toPreview += previewAt
		loader.Close()
		cache.Clear()
	}
	b.ReportMetric(float64(toPreview.Milliseconds())/float64(b.N), "ms-to-preview")
	b.ReportMetric(float64(toReady.Milliseconds())/float64(b.N), "ms-to-ready")
}

func BenchmarkOpenJPEGTiledOverview(b *testing.B) {
	const w, h = 6000, 4000
	path := filepath.Join(b.TempDir(), "photo24.jpg")
	var buf bytes.Buffer
	if err := stdjpeg.Encode(&buf, photoLike(w, h), &stdjpeg.Options{Quality: 88}); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		b.Fatal(err)
	}
	lim := codec.DefaultLimits()
	lim.TiledAbove = 8_000_000
	var toReady time.Duration
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reg := codec.NewRegistry()
		reg.RegisterDecoder(jpeg.Decoder{})
		ui := &uiQueue{}
		cache := NewCache(1 << 30)
		loader := NewLoader(reg, cache, ui.Post, 8, func() codec.Limits { return lim })
		start := time.Now()
		loader.Want([]Request{{Path: path, Prio: PrioCurrent}})
		for {
			ui.Drain()
			e, ok := loader.Entry(path)
			if ok && e.State == StateReady {
				break
			}
			if ok && e.State == StateFailed {
				b.Fatal(e.Err)
			}
			time.Sleep(200 * time.Microsecond)
		}
		toReady += time.Since(start)
		loader.Close()
		cache.Clear()
	}
	b.ReportMetric(float64(toReady.Milliseconds())/float64(b.N), "ms-to-ready")
}
