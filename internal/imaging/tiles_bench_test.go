package imaging

import (
	"context"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/render"
)

func benchRig(b *testing.B, budget int64) *tileRig {
	b.Helper()
	pic := bigPic
	rd := newFakeReader(pic)
	ov, err := rd.Overview(context.Background(), 3)
	if err != nil {
		b.Fatal(err)
	}
	r := &tileRig{pic: pic, rd: rd, closer: &countingCloser{}, base: 3}
	r.m = NewTileManager(rd, BuildPyramid(ov, 0), 3, pic.w, pic.h, TileOptions{Budget: budget, Workers: 3}, r.closer)
	b.Cleanup(func() { r.m.Close() })
	return r
}

func (r *tileRig) warm(b *testing.B, shift int, need image.Rectangle) {
	b.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		r.m.Window(shift, need)
		if r.idle() {
			return
		}
		if time.Now().After(deadline) {
			b.Fatal("tiles did not settle")
		}
		time.Sleep(time.Millisecond)
	}
}

var (
	viewA = image.Rect(2000, 2000, 2000+1920, 2000+1080)
	viewB = image.Rect(9000, 7000, 9000+1920, 7000+1080)
)

func BenchmarkWindowAssembleColdPlaceholders(b *testing.B) {
	r := benchRig(b, DefaultTileBudget)
	r.rd.setHook(func(ctx context.Context, shift int, rc image.Rectangle) error {
		<-ctx.Done()
		return ctx.Err()
	})
	r.m.Window(0, viewA)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := viewA
		if i%2 == 1 {
			v = viewB
		}
		if w := r.m.Window(0, v); w.Img == nil {
			b.Fatal("no window")
		}
	}
}

func BenchmarkWindowAssembleWarmTiles(b *testing.B) {
	r := benchRig(b, 512<<20)
	r.warm(b, 0, viewA)
	r.warm(b, 0, viewB)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := viewA
		if i%2 == 1 {
			v = viewB
		}
		if w := r.m.Window(0, v); w.Img == nil {
			b.Fatal("no window")
		}
	}
	b.StopTimer()
	if st := r.m.Stats(); st.WindowBuilds < uint64(b.N) {
		b.Fatalf("expected a rebuild per iteration: %+v", st)
	}
}

func BenchmarkWindowHit(b *testing.B) {
	r := benchRig(b, 512<<20)
	r.warm(b, 0, viewA)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := viewA.Add(image.Pt(i%64, i%32))
		if w := r.m.Window(0, v); w.Img == nil {
			b.Fatal("no window")
		}
	}
}

func BenchmarkPanFrame1080p(b *testing.B) {
	r := benchRig(b, 512<<20)
	strip := image.Rect(1500, 1500, 1500+5600, 1500+1700)
	for x := strip.Min.X; x < strip.Max.X; x += 900 {
		r.warm(b, 0, image.Rect(x, strip.Min.Y, x+1920, strip.Min.Y+1080))
	}
	src := render.Source{Levels: r.m.levels, Width: bigPic.w, Height: bigPic.h, Shift: 3, Detail: r.m}
	dst := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	bg := render.Background{Canvas: color.RGBA{20, 20, 20, 255}}
	const span = 3600
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		off := (i * 3) % (2 * span)
		if off > span {
			off = 2*span - off
		}
		m := render.Mapping{Scale: 1, OriginX: float64(1700 + off), OriginY: 1600}
		render.Render(dst, src, m, bg)
	}
	b.StopTimer()
	st := r.m.Stats()
	b.ReportMetric(float64(st.WindowBuilds)/float64(b.N), "builds/frame")
}

func BenchmarkPanFrame1080pRotated(b *testing.B) {
	r := benchRig(b, 512<<20)
	for y := 7600; y < 13500; y += 900 {
		r.warm(b, 0, image.Rect(1500, y, 1500+1920, y+1920))
	}
	src := render.Source{Levels: r.m.levels, Width: bigPic.w, Height: bigPic.h, Shift: 3, Detail: r.m}
	dst := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	bg := render.Background{Canvas: color.RGBA{20, 20, 20, 255}}
	const span = 3600
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		off := (i * 3) % (2 * span)
		if off > span {
			off = 2*span - off
		}
		m := render.Mapping{Scale: 1, Orient: 6, OriginX: float64(1700 + off), OriginY: 1600}
		render.Render(dst, src, m, bg)
	}
}

func BenchmarkPanFrame1080pBilinear(b *testing.B) {
	r := benchRig(b, 512<<20)
	for x := 1500; x < 1500+5600; x += 900 {
		r.warm(b, 0, image.Rect(x, 1500, x+2200, 1500+1500))
	}
	src := render.Source{Levels: r.m.levels, Width: bigPic.w, Height: bigPic.h, Shift: 3, Detail: r.m}
	dst := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	bg := render.Background{Canvas: color.RGBA{20, 20, 20, 255}}
	const span = 3000
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		off := float64((i*3)%(2*span)) * 0.97
		if off > span {
			off = 2*span - off
		}
		m := render.Mapping{Scale: 0.9, Filter: render.FilterBilinear, OriginX: 1700 + off + 0.37, OriginY: 1600.41}
		render.Render(dst, src, m, bg)
	}
	b.StopTimer()
	b.ReportMetric(float64(r.m.Stats().WindowBuilds)/float64(b.N), "builds/frame")
}
