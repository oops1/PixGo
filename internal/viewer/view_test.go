package viewer

import (
	"fmt"
	"image"
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/render"
)

const eps = 1e-6

func fr(w, h int, exif codec.Orientation, aw, ah int) Frame {
	return Frame{ImgW: w, ImgH: h, Exif: exif, AreaW: aw, AreaH: ah}
}

func allOrientations() []codec.Orientation {
	var out []codec.Orientation
	for o := codec.OrientNormal; o <= codec.OrientRotate270; o++ {
		out = append(out, o)
	}
	return out
}

func assertNear(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.IsNaN(got) || math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v (tol %v)", what, got, want, tol)
	}
}

func sourcePointAtAreaCentre(v *View, f Frame) (float64, float64) {
	u, w := v.AreaToImage(f, float64(f.AreaW)/2, float64(f.AreaH)/2)
	return v.Orient(f).ToSource(u, w, float64(f.ImgW), float64(f.ImgH))
}

func TestOrientAndOrientedSize(t *testing.T) {
	for _, exif := range allOrientations() {
		for _, user := range allOrientations() {
			v := View{User: user}
			f := fr(300, 200, exif, 1000, 800)
			if got, want := v.Orient(f), exif.Then(user); got != want {
				t.Errorf("exif %d user %d: orient %d, want %d", exif, user, got, want)
			}
			w, h := v.OrientedSize(f)
			if want := v.Orient(f).Transposes(); want != (w == 200 && h == 300) || (!want && (w != 300 || h != 200)) {
				t.Errorf("exif %d user %d: size %dx%d", exif, user, w, h)
			}
		}
	}
	var zero View
	if got := zero.Orient(fr(10, 10, 0, 100, 100)); got != codec.OrientNormal {
		t.Errorf("zero-value orientation = %d", got)
	}
}

func TestEffectiveZoom(t *testing.T) {
	cases := []struct {
		name string
		v    View
		f    Frame
		want float64
	}{
		{"fit shrinks wide image", View{Mode: ZoomFit}, fr(4000, 3000, 1, 1000, 800), 0.25},
		{"fit limited by height", View{Mode: ZoomFit}, fr(1000, 4000, 1, 1000, 800), 0.2},
		{"fit never upscales", View{Mode: ZoomFit}, fr(100, 50, 1, 1000, 800), 1},
		{"fit uses oriented size", View{Mode: ZoomFit}, fr(3000, 4000, codec.OrientRotate90, 1000, 800), 0.25},
		{"fit uses user rotation", View{Mode: ZoomFit, User: codec.OrientRotate90}, fr(4000, 2000, 1, 1000, 800), 0.2},
		{"fit width", View{Mode: ZoomFitWidth}, fr(4000, 3000, 1, 1000, 800), 0.25},
		{"fit width fills even when upscaling", View{Mode: ZoomFitWidth}, fr(100, 50, 1, 1000, 800), 10},
		{"fit width of rotated image", View{Mode: ZoomFitWidth}, fr(2000, 500, codec.OrientRotate270, 1000, 800), 2},
		{"fit width capped", View{Mode: ZoomFitWidth}, fr(1, 1, 1, 1000, 800), MaxZoom},
		{"actual", View{Mode: ZoomActual, Zoom: 7}, fr(4000, 3000, 1, 1000, 800), 1},
		{"custom", View{Mode: ZoomCustom, Zoom: 2.5}, fr(4000, 3000, 1, 1000, 800), 2.5},
		{"custom invalid zoom", View{Mode: ZoomCustom, Zoom: 0}, fr(4000, 3000, 1, 1000, 800), 1},
		{"custom negative zoom", View{Mode: ZoomCustom, Zoom: -3}, fr(4000, 3000, 1, 1000, 800), 1},
		{"custom NaN zoom", View{Mode: ZoomCustom, Zoom: math.NaN()}, fr(4000, 3000, 1, 1000, 800), 1},
		{"no image", View{Mode: ZoomFit}, fr(0, 0, 1, 1000, 800), 1},
		{"no area", View{Mode: ZoomFit}, fr(100, 100, 1, 0, 0), 1},
		{"no image fit width", View{Mode: ZoomFitWidth}, fr(0, 0, 1, 1000, 800), 1},
		{"unknown mode behaves like custom", View{Mode: ZoomMode(9), Zoom: 3}, fr(10, 10, 1, 100, 100), 3},
	}
	for _, c := range cases {
		assertNear(t, c.name, c.v.EffectiveZoom(c.f), c.want, 1e-12)
	}
}

func TestMappingFollowsRenderContract(t *testing.T) {
	for _, exif := range allOrientations() {
		for _, user := range []codec.Orientation{codec.OrientNormal, codec.OrientRotate90, codec.OrientFlipH, codec.OrientTranspose} {
			f := fr(3000, 2000, exif, 1000, 700)
			v := View{Mode: ZoomCustom, Zoom: 2, CenterU: 1200, CenterV: 900, User: user, Filter: render.FilterNearest}
			m := v.Mapping(f)
			if m.Scale != 2 || m.Orient != exif.Then(user) || m.Filter != render.FilterNearest {
				t.Fatalf("mapping fields: %+v", m)
			}
			assertNear(t, "origin x", m.OriginX, 1200-250, eps)
			assertNear(t, "origin y", m.OriginY, 900-175, eps)
			for _, px := range [][2]int{{0, 0}, {500, 350}, {999, 699}} {
				u := m.OriginX + (float64(px[0])+0.5)/m.Scale
				w := m.OriginY + (float64(px[1])+0.5)/m.Scale
				gu, gw := v.AreaToImage(f, float64(px[0])+0.5, float64(px[1])+0.5)
				assertNear(t, "u", gu, u, eps)
				assertNear(t, "v", gw, w, eps)
			}
			cu, cv := v.AreaToImage(f, 500, 350)
			assertNear(t, "centre u", cu, 1200, eps)
			assertNear(t, "centre v", cv, 900, eps)
		}
	}
}

func TestMappingIsPure(t *testing.T) {
	v := View{Mode: ZoomCustom, Zoom: 2, CenterU: 2990, CenterV: 5}
	f := fr(3000, 2000, 1, 1000, 700)
	before := v
	m1, m2 := v.Mapping(f), v.Mapping(f)
	if v != before || m1 != m2 {
		t.Fatal("Mapping must not change the view")
	}
	assertNear(t, "clamped origin x", m1.OriginX, 3000-1000/2.0, eps)
	assertNear(t, "clamped origin y", m1.OriginY, 0, eps)
	v.Clamp(f)
	assertNear(t, "stored centre u", v.CenterU, 2750, eps)
	assertNear(t, "stored centre v", v.CenterV, 175, eps)
}

func TestMappingClampsStaleCentreAfterResize(t *testing.T) {
	v := View{Mode: ZoomCustom, Zoom: 2, CenterU: 2900, CenterV: 1000}
	big := fr(3000, 2000, 1, 1000, 700)
	small := fr(3000, 2000, 1, 600, 400)
	for _, f := range []Frame{big, small} {
		r := v.ImageRect(f)
		if r.Min.X > 0 || r.Min.Y > 0 || r.Max.X < f.AreaW || r.Max.Y < f.AreaH {
			t.Errorf("area %dx%d: image rect %v leaves an empty band", f.AreaW, f.AreaH, r)
		}
	}
	if v.CenterU != 2900 || v.CenterV != 1000 {
		t.Fatal("resizing must not destroy the stored centre")
	}
}

func TestResetStartsCentred(t *testing.T) {
	v := View{Mode: ZoomCustom, Zoom: 3, CenterU: 10, CenterV: 20, User: codec.OrientRotate90, Filter: render.FilterNearest}
	v.Reset(ZoomActual)
	if v.Mode != ZoomActual || v.Zoom != 1 || v.User != codec.OrientNormal || v.Filter != render.FilterNearest {
		t.Fatalf("after reset: %+v", v)
	}
	f := fr(400, 300, 1, 1000, 800)
	if r := v.ImageRect(f); r != image.Rect(300, 250, 700, 550) {
		t.Fatalf("100%% must be centred, got %v", r)
	}
	big := fr(4000, 3000, 1, 1000, 800)
	u, w := v.AreaToImage(big, 500, 400)
	assertNear(t, "centre u", u, 2000, eps)
	assertNear(t, "centre v", w, 1500, eps)
	other := fr(100, 50, 1, 1000, 800)
	u, w = v.AreaToImage(other, 500, 400)
	assertNear(t, "centre u after the image changed", u, 50, eps)
	assertNear(t, "centre v after the image changed", w, 25, eps)
}

func TestImageRect(t *testing.T) {
	cases := []struct {
		name string
		v    View
		f    Frame
		want image.Rectangle
	}{
		{"fit centres", View{Mode: ZoomFit}, fr(4000, 3000, 1, 1000, 800), image.Rect(0, 25, 1000, 775)},
		{"fit small image at 100%", View{Mode: ZoomFit}, fr(400, 300, 1, 1000, 800), image.Rect(300, 250, 700, 550)},
		{"odd sizes round outward", View{Mode: ZoomActual}, fr(401, 301, 1, 1000, 800), image.Rect(299, 249, 701, 551)},
		{"rotated fit", View{Mode: ZoomFit, User: codec.OrientRotate90}, fr(4000, 2000, 1, 1000, 800), image.Rect(300, 0, 700, 800)},
		{"exif rotated fit", View{Mode: ZoomFit}, fr(2000, 4000, codec.OrientRotate90, 1000, 800), image.Rect(0, 150, 1000, 650)},
		{"fit width tall image starts centred", View{Mode: ZoomFitWidth}, fr(500, 2000, 1, 1000, 800), image.Rect(0, -1600, 1000, 2400)},
		{"custom zoomed in", View{Mode: ZoomCustom, Zoom: 2}, fr(1000, 800, 1, 1000, 800), image.Rect(-500, -400, 1500, 1200)},
		{"custom zoomed out", View{Mode: ZoomCustom, Zoom: 0.5}, fr(1000, 800, 1, 1000, 800), image.Rect(250, 200, 750, 600)},
	}
	for _, c := range cases {
		if got := c.v.ImageRect(c.f); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestImageRectAgreesWithMapping(t *testing.T) {
	frames := []Frame{
		fr(4000, 3000, 1, 1000, 800),
		fr(317, 211, 1, 900, 600),
		fr(3000, 400, codec.OrientRotate90, 777, 555),
		fr(64, 64, codec.OrientTransverse, 640, 480),
	}
	views := []View{
		{Mode: ZoomFit},
		{Mode: ZoomActual},
		{Mode: ZoomFitWidth},
		{Mode: ZoomCustom, Zoom: 0.37},
		{Mode: ZoomCustom, Zoom: 1.7, CenterU: 300, CenterV: 120},
		{Mode: ZoomCustom, Zoom: 13, CenterU: 20, CenterV: 30, User: codec.OrientFlipV},
	}
	for fi, f := range frames {
		for vi, v := range views {
			rect := v.ImageRect(f)
			m := v.Mapping(f)
			w, h := v.OrientedSize(f)
			inside := func(x, y int) bool {
				u := m.OriginX + (float64(x)+0.5)/m.Scale
				vv := m.OriginY + (float64(y)+0.5)/m.Scale
				return u >= 0 && u < float64(w) && vv >= 0 && vv < float64(h)
			}
			clipped := rect.Intersect(image.Rect(0, 0, f.AreaW, f.AreaH))
			for y := 0; y < f.AreaH; y++ {
				for x := 0; x < f.AreaW; x++ {
					if inside(x, y) && !image.Pt(x, y).In(clipped) {
						t.Fatalf("frame %d view %d: pixel (%d,%d) shows the image but is outside %v", fi, vi, x, y, rect)
					}
				}
			}
			if !clipped.Empty() {
				for _, edge := range []image.Point{{clipped.Min.X, clipped.Min.Y}, {clipped.Max.X - 1, clipped.Max.Y - 1}} {
					px, py := float64(edge.X), float64(edge.Y)
					u, vv := v.AreaToImage(f, px+1, py+1)
					u2, vv2 := v.AreaToImage(f, px, py)
					covered := (u > 0 && u2 < float64(w)) && (vv > 0 && vv2 < float64(h))
					if !covered {
						t.Fatalf("frame %d view %d: rect edge pixel %v does not touch the image", fi, vi, edge)
					}
				}
			}
			x0, y0 := v.ImageToArea(f, 0, 0)
			x1, y1 := v.ImageToArea(f, float64(w), float64(h))
			if math.Floor(x0+eps) != float64(rect.Min.X) || math.Floor(y0+eps) != float64(rect.Min.Y) ||
				math.Ceil(x1-eps) != float64(rect.Max.X) || math.Ceil(y1-eps) != float64(rect.Max.Y) {
				t.Fatalf("frame %d view %d: rect %v vs corners (%v,%v)-(%v,%v)", fi, vi, rect, x0, y0, x1, y1)
			}
		}
	}
}

func TestAreaImageRoundTrip(t *testing.T) {
	for _, exif := range allOrientations() {
		f := fr(3000, 2000, exif, 1000, 700)
		v := View{Mode: ZoomCustom, Zoom: 1.3, CenterU: 800, CenterV: 900}
		for _, p := range [][2]float64{{0, 0}, {123.5, 456.25}, {999, 699}, {-50, 1200}} {
			u, w := v.AreaToImage(f, p[0], p[1])
			px, py := v.ImageToArea(f, u, w)
			assertNear(t, "px", px, p[0], eps)
			assertNear(t, "py", py, p[1], eps)
		}
	}
}

func TestClampEdgeCases(t *testing.T) {
	f := fr(4000, 3000, 1, 1000, 800)
	t.Run("smaller than area is centred on both axes", func(t *testing.T) {
		v := View{Mode: ZoomCustom, Zoom: 0.1, CenterU: 10, CenterV: 10}
		v.Clamp(f)
		if r := v.ImageRect(f); r != image.Rect(300, 250, 700, 550) {
			t.Fatalf("got %v", r)
		}
		assertNear(t, "centre u", v.CenterU, 2000, eps)
		assertNear(t, "centre v", v.CenterV, 1500, eps)
	})
	t.Run("larger than area never shows a band", func(t *testing.T) {
		for _, z := range []float64{0.3, 0.5, 1, 3.7, 20} {
			for _, pan := range [][2]float64{{1e9, 1e9}, {-1e9, -1e9}, {1e9, -1e9}, {-1e9, 1e9}, {0, 0}, {17, -23}} {
				v := View{Mode: ZoomCustom, Zoom: z}
				v.Pan(f, pan[0], pan[1])
				r := v.ImageRect(f)
				if r.Min.X > 0 || r.Min.Y > 0 || r.Max.X < f.AreaW || r.Max.Y < f.AreaH {
					t.Fatalf("zoom %v pan %v: rect %v", z, pan, r)
				}
			}
		}
	})
	t.Run("pan limit touches the area edges exactly", func(t *testing.T) {
		v := View{Mode: ZoomCustom, Zoom: 1}
		v.Pan(f, 1e9, 1e9)
		if r := v.ImageRect(f); r.Min != image.Pt(0, 0) {
			t.Fatalf("top-left limit: %v", r)
		}
		v.Pan(f, -1e9, -1e9)
		if r := v.ImageRect(f); r.Max != image.Pt(1000, 800) {
			t.Fatalf("bottom-right limit: %v", r)
		}
	})
	t.Run("wide panorama pans on one axis only", func(t *testing.T) {
		pano := fr(8000, 400, 1, 1000, 800)
		v := View{Mode: ZoomCustom, Zoom: 1}
		v.Pan(pano, 1e9, 1e9)
		r := v.ImageRect(pano)
		if r.Min.X != 0 || r.Min.Y != 200 || r.Max.Y != 600 {
			t.Fatalf("got %v", r)
		}
	})
	t.Run("fit is always centred", func(t *testing.T) {
		v := View{Mode: ZoomFit, CenterU: 5, CenterV: 5}
		v.Clamp(f)
		if v.CenterU != 0 || v.CenterV != 0 {
			t.Fatalf("fit must store the implicit centre, got %v,%v", v.CenterU, v.CenterV)
		}
		v.Pan(f, 300, 300)
		if r := v.ImageRect(f); r != image.Rect(0, 25, 1000, 775) {
			t.Fatalf("pan in fit moved the image: %v", r)
		}
	})
	t.Run("image exactly as large as the area", func(t *testing.T) {
		exact := fr(1000, 800, 1, 1000, 800)
		v := View{Mode: ZoomActual}
		v.Pan(exact, 100, 100)
		if r := v.ImageRect(exact); r != image.Rect(0, 0, 1000, 800) {
			t.Fatalf("got %v", r)
		}
		if v.CanPan(exact) {
			t.Fatal("nothing to pan")
		}
	})
	t.Run("degenerate frames leave the view alone", func(t *testing.T) {
		v := View{Mode: ZoomCustom, Zoom: 2, CenterU: 7, CenterV: 9}
		v.Clamp(fr(0, 0, 1, 0, 0))
		v.Clamp(fr(100, 100, 1, 0, 0))
		v.Clamp(fr(0, 0, 1, 100, 100))
		if v.CenterU != 7 || v.CenterV != 9 {
			t.Fatalf("got %v,%v", v.CenterU, v.CenterV)
		}
	})
	t.Run("non-finite centre falls back to the middle", func(t *testing.T) {
		v := View{Mode: ZoomCustom, Zoom: 2, CenterU: math.NaN(), CenterV: math.Inf(1)}
		v.Clamp(f)
		assertNear(t, "u", v.CenterU, 2000, eps)
		assertNear(t, "v", v.CenterV, 1500, eps)
	})
}

func TestPanDirection(t *testing.T) {
	f := fr(4000, 3000, 1, 1000, 800)
	v := View{Mode: ZoomCustom, Zoom: 1}
	before := v.ImageRect(f)
	v.Pan(f, 100, 40)
	after := v.ImageRect(f)
	if after.Min.X-before.Min.X != 100 || after.Min.Y-before.Min.Y != 40 {
		t.Fatalf("dragging right/down must move the image right/down: %v -> %v", before, after)
	}
	v.Pan(f, -250, -90)
	again := v.ImageRect(f)
	if again.Min.X-after.Min.X != -250 || again.Min.Y-after.Min.Y != -90 {
		t.Fatalf("%v -> %v", after, again)
	}
	zoomed := View{Mode: ZoomCustom, Zoom: 4}
	zoomed.Pan(f, 40, 0)
	r0 := (&View{Mode: ZoomCustom, Zoom: 4}).ImageRect(f)
	if zoomed.ImageRect(f).Min.X-r0.Min.X != 40 {
		t.Fatal("pan distance is in screen pixels regardless of zoom")
	}
}

func TestPanKeepsModes(t *testing.T) {
	f := fr(4000, 3000, 1, 1000, 800)
	for _, mode := range []ZoomMode{ZoomActual, ZoomFitWidth, ZoomCustom} {
		v := View{Mode: mode, Zoom: 1}
		v.Pan(f, -500, -100)
		if v.Mode != mode {
			t.Errorf("pan changed mode %d to %d", mode, v.Mode)
		}
	}
	fw := View{Mode: ZoomFitWidth}
	tall := fr(500, 3000, 1, 1000, 800)
	fw.Pan(tall, 0, -1e9)
	if r := fw.ImageRect(tall); r.Max.Y != 800 || r.Min.X != 0 || r.Max.X != 1000 {
		t.Fatalf("fit width pans vertically only: %v", r)
	}
}

func TestCanPan(t *testing.T) {
	cases := []struct {
		name string
		v    View
		f    Frame
		want bool
	}{
		{"fit", View{Mode: ZoomFit}, fr(4000, 3000, 1, 1000, 800), false},
		{"fit small", View{Mode: ZoomFit}, fr(100, 100, 1, 1000, 800), false},
		{"fit awkward fraction", View{Mode: ZoomFit}, fr(3333, 2222, 1, 1001, 777), false},
		{"actual large", View{Mode: ZoomActual}, fr(4000, 3000, 1, 1000, 800), true},
		{"actual small", View{Mode: ZoomActual}, fr(400, 300, 1, 1000, 800), false},
		{"actual wide only", View{Mode: ZoomActual}, fr(2000, 300, 1, 1000, 800), true},
		{"fit width tall", View{Mode: ZoomFitWidth}, fr(500, 2000, 1, 1000, 800), true},
		{"fit width short", View{Mode: ZoomFitWidth}, fr(2000, 500, 1, 1000, 800), false},
		{"custom zoomed in", View{Mode: ZoomCustom, Zoom: 3}, fr(400, 300, 1, 1000, 800), true},
		{"custom zoomed out", View{Mode: ZoomCustom, Zoom: 0.1}, fr(4000, 3000, 1, 1000, 800), false},
		{"no image", View{Mode: ZoomActual}, fr(0, 0, 1, 1000, 800), false},
	}
	for _, c := range cases {
		if got := c.v.CanPan(c.f); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestZoomAtKeepsPointFixedForAllOrientations(t *testing.T) {
	points := [][2]float64{{500, 350}, {300, 200}, {850, 620}, {12.5, 700}}
	factors := []float64{1.25, 0.8, 3, 0.4, 1.0000001}
	pans := [][2]float64{{0, 0}, {123, -57}, {-300, 200}}
	for _, exif := range allOrientations() {
		for _, user := range []codec.Orientation{codec.OrientNormal, codec.OrientRotate90, codec.OrientFlipH, codec.OrientTransverse} {
			f := fr(3000, 2000, exif, 1000, 700)
			for _, pan := range pans {
				for _, p := range points {
					for _, k := range factors {
						v := View{Mode: ZoomCustom, Zoom: 2, User: user}
						v.Pan(f, pan[0], pan[1])
						u0, w0 := v.AreaToImage(f, p[0], p[1])
						v.ZoomAt(f, p[0], p[1], k)
						u1, w1 := v.AreaToImage(f, p[0], p[1])
						assertNear(t, "u", u1, u0, eps)
						assertNear(t, "v", w1, w0, eps)
						if want := math.Max(2*k, 1); k != 1.0000001 {
							assertNear(t, "zoom", v.EffectiveZoom(f), want, 1e-9)
						}
						if v.Mode != ZoomCustom {
							t.Fatalf("mode %d", v.Mode)
						}
					}
				}
			}
		}
	}
}

func TestZoomAtFromFitAndActualKeepsPointOnLargeAxis(t *testing.T) {
	f := fr(6000, 4000, 1, 1000, 700)
	v := View{Mode: ZoomActual}
	u0, w0 := v.AreaToImage(f, 640, 210)
	v.ZoomAt(f, 640, 210, 1.25)
	u1, w1 := v.AreaToImage(f, 640, 210)
	assertNear(t, "u", u1, u0, eps)
	assertNear(t, "v", w1, w0, eps)
	if v.Mode != ZoomCustom {
		t.Fatalf("mode %d", v.Mode)
	}
	fw := View{Mode: ZoomFitWidth}
	u0, w0 = fw.AreaToImage(f, 100, 600)
	fw.ZoomAt(f, 100, 600, 2)
	u1, w1 = fw.AreaToImage(f, 100, 600)
	assertNear(t, "fit width u", u1, u0, eps)
	assertNear(t, "fit width v", w1, w0, eps)
}

func TestZoomAtClampsToLimits(t *testing.T) {
	f := fr(4000, 3000, 1, 1000, 800)
	v := View{Mode: ZoomCustom, Zoom: 50}
	v.ZoomAt(f, 500, 400, 10)
	assertNear(t, "max", v.EffectiveZoom(f), MaxZoom, 1e-12)
	before := v
	v.ZoomAt(f, 500, 400, 2)
	if v != before {
		t.Fatal("zooming past the maximum must change nothing")
	}
	v = View{Mode: ZoomCustom, Zoom: 0.1}
	v.ZoomAt(f, 500, 400, 0.1)
	assertNear(t, "min", v.EffectiveZoom(f), MinZoom, 1e-12)
	before = v
	v.ZoomAt(f, 500, 400, 0.5)
	if v != before {
		t.Fatal("zooming below the minimum must change nothing")
	}
	for _, bad := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		v2 := View{Mode: ZoomCustom, Zoom: 2}
		v2.ZoomAt(f, 1, 1, bad)
		if v2.Zoom != 2 {
			t.Errorf("factor %v changed the zoom", bad)
		}
	}
}

func TestZoomAtHugeImageCanStillBeShownWhole(t *testing.T) {
	f := fr(20000, 20000, 1, 500, 500)
	v := View{Mode: ZoomFit}
	fit := v.FitZoom(f)
	if fit >= MinZoom {
		t.Fatalf("scenario needs a fit zoom below the minimum, got %v", fit)
	}
	v.StepZoom(f, 3, 250, 250)
	for i := 0; i < 40; i++ {
		v.StepZoom(f, -1, 250, 250)
	}
	assertNear(t, "zoom", v.EffectiveZoom(f), fit, 1e-12)
	if v.Mode != ZoomFit {
		t.Fatalf("zooming out to the fit zoom must select Fit, mode %d", v.Mode)
	}
	before := v
	v.StepZoom(f, -1, 250, 250)
	if v != before {
		t.Fatal("cannot zoom out past the fit zoom of a huge image")
	}
}

func zoomTrail(v *View, f Frame, steps int, px, py float64) []float64 {
	out := []float64{v.EffectiveZoom(f)}
	dir := 1
	if steps < 0 {
		dir, steps = -1, -steps
	}
	for i := 0; i < steps; i++ {
		v.StepZoom(f, dir, px, py)
		out = append(out, v.EffectiveZoom(f))
	}
	return out
}

func TestStepZoomSnapsTo100AndFit(t *testing.T) {
	f := fr(4000, 3000, 1, 1000, 800)
	fit := 0.25
	t.Run("out from fit past 100 percent", func(t *testing.T) {
		v := View{Mode: ZoomFit}
		trail := zoomTrail(&v, f, 12, 500, 400)
		idx := -1
		for i, z := range trail {
			if z == 1 {
				idx = i
			}
			if i > 0 && trail[i-1] < 1 && z > 1 {
				t.Fatalf("step %d jumped over 100%%: %v", i, trail)
			}
		}
		if idx < 0 {
			t.Fatalf("never stopped on exactly 100%%: %v", trail)
		}
		if next := trail[idx+1]; next != 1.25 {
			t.Fatalf("step after 100%% = %v", next)
		}
	})
	t.Run("in from above", func(t *testing.T) {
		v := View{Mode: ZoomCustom, Zoom: 3}
		trail := zoomTrail(&v, f, -6, 500, 400)
		hit := false
		for i, z := range trail {
			if z == 1 {
				hit = true
				if trail[i+1] != 0.8 {
					t.Fatalf("step below 100%% = %v", trail[i+1])
				}
				break
			}
			if i > 0 && trail[i-1] > 1 && z < 1 {
				t.Fatalf("jumped below 100%%: %v", trail)
			}
		}
		if !hit {
			t.Fatalf("never stopped on 100%%: %v", trail)
		}
	})
	t.Run("one step from either side lands on 100", func(t *testing.T) {
		for _, start := range []float64{0.7, 0.9, 0.99, 1.01, 1.2, 1.25, 1.1} {
			v := View{Mode: ZoomCustom, Zoom: start}
			dir := 1
			if start > 1 {
				dir = -1
			}
			v.StepZoom(f, dir, 500, 400)
			z := v.EffectiveZoom(f)
			want := start * math.Pow(ZoomStepFactor, float64(dir))
			if (dir > 0 && want >= 1) || (dir < 0 && want <= 1) {
				want = 1
			}
			assertNear(t, fmt.Sprint("zoom from ", start), z, want, 1e-12)
		}
	})
	t.Run("in to fit from below selects fit mode", func(t *testing.T) {
		v := View{Mode: ZoomCustom, Zoom: 0.21}
		v.StepZoom(f, 1, 500, 400)
		if v.Mode != ZoomFit || v.EffectiveZoom(f) != fit {
			t.Fatalf("mode %d zoom %v", v.Mode, v.EffectiveZoom(f))
		}
	})
	t.Run("out to fit from above selects fit mode", func(t *testing.T) {
		v := View{Mode: ZoomCustom, Zoom: 0.27}
		v.StepZoom(f, -1, 500, 400)
		if v.Mode != ZoomFit || v.EffectiveZoom(f) != fit {
			t.Fatalf("mode %d zoom %v", v.Mode, v.EffectiveZoom(f))
		}
		if r := v.ImageRect(f); r != image.Rect(0, 25, 1000, 775) {
			t.Fatalf("fit rect %v", r)
		}
	})
	t.Run("out from 100 stops on fit then continues to the minimum", func(t *testing.T) {
		v := View{Mode: ZoomActual}
		trail := zoomTrail(&v, f, -20, 500, 400)
		fitAt := -1
		for i, z := range trail {
			if z == fit {
				fitAt = i
			}
			if i > 0 && z > trail[i-1] {
				t.Fatalf("zoom went up while zooming out: %v", trail)
			}
			if i > 0 && trail[i-1] > fit && z < fit {
				t.Fatalf("jumped over the fit zoom: %v", trail)
			}
		}
		if fitAt < 0 {
			t.Fatalf("never stopped on the fit zoom: %v", trail)
		}
		if last := trail[len(trail)-1]; last != MinZoom || v.Mode != ZoomCustom {
			t.Fatalf("ended at %v mode %d: %v", last, v.Mode, trail)
		}
		stop := View{Mode: ZoomActual}
		zoomTrail(&stop, f, -(fitAt), 500, 400)
		if stop.Mode != ZoomFit {
			t.Fatalf("landing on the fit zoom must select Fit, mode %d", stop.Mode)
		}
	})
	t.Run("from fit and back is stable", func(t *testing.T) {
		v := View{Mode: ZoomFit}
		v.StepZoom(f, 1, 700, 100)
		v.StepZoom(f, -1, 700, 100)
		if v.Mode != ZoomFit || v.EffectiveZoom(f) != fit {
			t.Fatalf("mode %d zoom %v", v.Mode, v.EffectiveZoom(f))
		}
	})
}

func TestStepZoomSmallImage(t *testing.T) {
	f := fr(300, 200, 1, 1000, 800)
	v := View{Mode: ZoomFit}
	v.StepZoom(f, 1, 500, 400)
	if v.Mode != ZoomCustom || v.EffectiveZoom(f) != 1.25 {
		t.Fatalf("mode %d zoom %v", v.Mode, v.EffectiveZoom(f))
	}
	v.StepZoom(f, -1, 500, 400)
	if v.Mode != ZoomFit || v.EffectiveZoom(f) != 1 {
		t.Fatalf("back at fit: mode %d zoom %v", v.Mode, v.EffectiveZoom(f))
	}
	v.StepZoom(f, -1, 500, 400)
	assertNear(t, "below 100%", v.EffectiveZoom(f), 0.8, 1e-12)
	v.StepZoom(f, 1, 500, 400)
	if v.Mode != ZoomFit || v.EffectiveZoom(f) != 1 {
		t.Fatalf("mode %d zoom %v", v.Mode, v.EffectiveZoom(f))
	}
}

func TestStepZoomMultipleStepsAndLimits(t *testing.T) {
	f := fr(4000, 3000, 1, 1000, 800)
	v := View{Mode: ZoomCustom, Zoom: 1}
	v.StepZoom(f, 3, 500, 400)
	assertNear(t, "three steps in", v.EffectiveZoom(f), math.Pow(1.25, 3), 1e-12)
	v.StepZoom(f, -3, 500, 400)
	assertNear(t, "three steps out", v.EffectiveZoom(f), 1, 1e-12)
	v.StepZoom(f, 0, 500, 400)
	assertNear(t, "zero steps", v.EffectiveZoom(f), 1, 1e-12)
	v.StepZoom(f, 1_000_000, 500, 400)
	assertNear(t, "huge step count", v.EffectiveZoom(f), MaxZoom, 1e-12)
	v.StepZoom(f, -1_000_000, 500, 400)
	assertNear(t, "huge negative step count", v.EffectiveZoom(f), MinZoom, 1e-12)
	if v.Mode != ZoomCustom {
		t.Fatalf("mode %d", v.Mode)
	}
}

func TestStepZoomKeepsPointFixed(t *testing.T) {
	for _, exif := range allOrientations() {
		f := fr(3000, 2000, exif, 1000, 700)
		v := View{Mode: ZoomCustom, Zoom: 3}
		v.Pan(f, 40, -90)
		u0, w0 := v.AreaToImage(f, 222, 111)
		v.StepZoom(f, 2, 222, 111)
		u1, w1 := v.AreaToImage(f, 222, 111)
		assertNear(t, "u in", u1, u0, eps)
		assertNear(t, "v in", w1, w0, eps)
		v.StepZoom(f, -3, 222, 111)
		u2, w2 := v.AreaToImage(f, 222, 111)
		assertNear(t, "u out", u2, u0, eps)
		assertNear(t, "v out", w2, w0, eps)
	}
}

func TestZoomTo(t *testing.T) {
	f := fr(4000, 3000, 1, 1000, 800)
	v := View{Mode: ZoomFit}
	u0, w0 := v.AreaToImage(f, 600, 300)
	v.ZoomTo(f, 2, 600, 300)
	u1, w1 := v.AreaToImage(f, 600, 300)
	assertNear(t, "u", u1, u0, eps)
	assertNear(t, "v", w1, w0, eps)
	if v.Mode != ZoomCustom || v.EffectiveZoom(f) != 2 {
		t.Fatalf("mode %d zoom %v", v.Mode, v.EffectiveZoom(f))
	}
	v.ZoomTo(f, 0.25, 500, 400)
	if v.Mode != ZoomFit {
		t.Fatalf("zoom equal to fit must select Fit, mode %d", v.Mode)
	}
	v.ZoomTo(f, 1000, 0, 0)
	assertNear(t, "clamped high", v.EffectiveZoom(f), MaxZoom, 1e-12)
	v.ZoomTo(f, 0.0001, 0, 0)
	assertNear(t, "clamped low", v.EffectiveZoom(f), MinZoom, 1e-12)
	same := View{Mode: ZoomActual}
	same.ZoomTo(f, 1, 10, 10)
	if same.Mode != ZoomActual {
		t.Fatal("ZoomTo the current zoom must change nothing")
	}
	for _, bad := range []float64{0, -2, math.NaN(), math.Inf(1)} {
		v3 := View{Mode: ZoomCustom, Zoom: 2}
		v3.ZoomTo(f, bad, 1, 1)
		if v3.Zoom != 2 {
			t.Errorf("ZoomTo(%v) changed the zoom", bad)
		}
	}
}

func TestSetMode(t *testing.T) {
	f := fr(4000, 3000, 1, 1000, 800)
	t.Run("actual keeps the area centre point", func(t *testing.T) {
		v := View{Mode: ZoomCustom, Zoom: 2}
		v.Pan(f, -300, 120)
		cu, cv := v.AreaToImage(f, 500, 400)
		v.SetMode(f, ZoomActual)
		u, w := v.AreaToImage(f, 500, 400)
		assertNear(t, "u", u, cu, eps)
		assertNear(t, "v", w, cv, eps)
		if v.Mode != ZoomActual || v.EffectiveZoom(f) != 1 {
			t.Fatalf("mode %d zoom %v", v.Mode, v.EffectiveZoom(f))
		}
	})
	t.Run("fit recentres", func(t *testing.T) {
		v := View{Mode: ZoomCustom, Zoom: 2}
		v.Pan(f, -300, 120)
		v.SetMode(f, ZoomFit)
		if r := v.ImageRect(f); r != image.Rect(0, 25, 1000, 775) {
			t.Fatalf("got %v", r)
		}
	})
	t.Run("fit width", func(t *testing.T) {
		v := View{Mode: ZoomFit}
		v.SetMode(f, ZoomFitWidth)
		if v.Mode != ZoomFitWidth || v.EffectiveZoom(f) != 0.25 {
			t.Fatalf("mode %d zoom %v", v.Mode, v.EffectiveZoom(f))
		}
		tall := fr(500, 3000, 1, 1000, 800)
		v.SetMode(tall, ZoomFitWidth)
		if r := v.ImageRect(tall); r.Min.X != 0 || r.Max.X != 1000 {
			t.Fatalf("got %v", r)
		}
	})
	t.Run("custom freezes the current zoom", func(t *testing.T) {
		v := View{Mode: ZoomFit}
		v.SetMode(f, ZoomCustom)
		if v.Mode != ZoomCustom || v.Zoom != 0.25 {
			t.Fatalf("mode %d zoom %v", v.Mode, v.Zoom)
		}
		f2 := fr(4000, 3000, 1, 500, 400)
		if v.EffectiveZoom(f2) != 0.25 {
			t.Fatal("custom zoom must survive a resize")
		}
	})
	t.Run("unknown mode is ignored", func(t *testing.T) {
		v := View{Mode: ZoomCustom, Zoom: 2}
		before := v
		v.SetMode(f, ZoomMode(77))
		if v != before {
			t.Fatal("changed")
		}
	})
	t.Run("before the image is known", func(t *testing.T) {
		var v View
		v.SetMode(Frame{AreaW: 1000, AreaH: 800}, ZoomActual)
		if v.CenterU != 0 || v.CenterV != 0 {
			t.Fatalf("centre must stay implicit, got %v,%v", v.CenterU, v.CenterV)
		}
		u, w := v.AreaToImage(f, 500, 400)
		assertNear(t, "u", u, 2000, eps)
		assertNear(t, "v", w, 1500, eps)
	})
}

func TestToggleFitActualRoundTrip(t *testing.T) {
	f := fr(4000, 3000, 1, 1000, 800)
	for _, p := range [][2]float64{{500, 400}, {640, 210}, {120, 700}, {900, 100}} {
		var v View
		v.Reset(ZoomFit)
		start := v
		startMap := v.Mapping(f)
		u0, w0 := v.AreaToImage(f, p[0], p[1])
		v.ToggleFitActual(f, p[0], p[1])
		if v.Mode != ZoomActual || v.EffectiveZoom(f) != 1 {
			t.Fatalf("to 100%%: mode %d zoom %v", v.Mode, v.EffectiveZoom(f))
		}
		u1, w1 := v.AreaToImage(f, p[0], p[1])
		assertNear(t, "u", u1, u0, eps)
		assertNear(t, "v", w1, w0, eps)
		r := v.ImageRect(f)
		if r.Min.X > 0 || r.Min.Y > 0 || r.Max.X < f.AreaW || r.Max.Y < f.AreaH {
			t.Fatalf("100%% leaves a band: %v", r)
		}
		v.ToggleFitActual(f, p[0], p[1])
		if v != start || v.Mapping(f) != startMap {
			t.Fatalf("round trip changed the view: %+v vs %+v", v, start)
		}
	}
}

func TestToggleFitActualFromOtherStates(t *testing.T) {
	f := fr(4000, 3000, 1, 1000, 800)
	custom := View{Mode: ZoomCustom, Zoom: 3}
	custom.ToggleFitActual(f, 500, 400)
	if custom.Mode != ZoomFit {
		t.Fatalf("custom zoom must go to fit, mode %d", custom.Mode)
	}
	actual := View{Mode: ZoomActual}
	actual.ToggleFitActual(f, 500, 400)
	if actual.Mode != ZoomFit {
		t.Fatalf("100%% must go to fit, mode %d", actual.Mode)
	}
	asFit := View{Mode: ZoomCustom, Zoom: 0.25}
	asFit.ToggleFitActual(f, 500, 400)
	if asFit.Mode != ZoomActual || asFit.EffectiveZoom(f) != 1 {
		t.Fatalf("custom zoom equal to fit counts as fit: mode %d", asFit.Mode)
	}
	fitWidth := View{Mode: ZoomFitWidth}
	fitWidth.ToggleFitActual(f, 500, 400)
	if fitWidth.Mode != ZoomActual {
		t.Fatalf("fit width equal to fit counts as fit: mode %d", fitWidth.Mode)
	}
	tall := fr(500, 3000, 1, 1000, 800)
	tallWidth := View{Mode: ZoomFitWidth}
	tallWidth.ToggleFitActual(tall, 500, 400)
	if tallWidth.Mode != ZoomFit {
		t.Fatalf("fit width that differs from fit goes to fit: mode %d", tallWidth.Mode)
	}
	unready := View{Mode: ZoomFit}
	unready.ToggleFitActual(Frame{}, 1, 1)
	if unready.Mode != ZoomFit {
		t.Fatal("no image: nothing to toggle")
	}
}

func TestToggleFitActualSmallImageStillAlternates(t *testing.T) {
	f := fr(300, 200, 1, 1000, 800)
	var v View
	v.Reset(ZoomFit)
	v.ToggleFitActual(f, 500, 400)
	if v.Mode != ZoomActual {
		t.Fatalf("mode %d", v.Mode)
	}
	v.ToggleFitActual(f, 500, 400)
	if v.Mode != ZoomFit {
		t.Fatalf("mode %d", v.Mode)
	}
	if r := v.ImageRect(f); r != image.Rect(350, 300, 650, 500) {
		t.Fatalf("got %v", r)
	}
}

func TestToggleAtImageEdgeStaysCovered(t *testing.T) {
	f := fr(4000, 3000, 1, 1000, 800)
	var v View
	v.Reset(ZoomFit)
	v.ToggleFitActual(f, 0, 25)
	r := v.ImageRect(f)
	if r.Min.X != 0 || r.Min.Y != 0 {
		t.Fatalf("zooming at the top-left corner of the image: %v", r)
	}
	var w View
	w.Reset(ZoomFit)
	w.ToggleFitActual(f, 1000, 775)
	r = w.ImageRect(f)
	if r.Max.X != 1000 || r.Max.Y != 800 {
		t.Fatalf("zooming at the bottom-right corner of the image: %v", r)
	}
}

func TestRotateKeepsContentCentred(t *testing.T) {
	type op struct {
		name string
		do   func(v *View, f Frame)
	}
	ops := []op{
		{"cw", func(v *View, f Frame) { v.Rotate(f, true) }},
		{"ccw", func(v *View, f Frame) { v.Rotate(f, false) }},
		{"flipH", func(v *View, f Frame) { v.FlipH(f) }},
		{"flipV", func(v *View, f Frame) { v.FlipV(f) }},
	}
	for _, exif := range allOrientations() {
		for _, o := range ops {
			for _, mode := range []ZoomMode{ZoomCustom, ZoomActual} {
				f := fr(3000, 2000, exif, 1000, 700)
				v := View{Mode: mode, Zoom: 2}
				v.Pan(f, 150, -80)
				x0, y0 := sourcePointAtAreaCentre(&v, f)
				zoom := v.EffectiveZoom(f)
				o.do(&v, f)
				x1, y1 := sourcePointAtAreaCentre(&v, f)
				assertNear(t, o.name+" source x", x1, x0, eps)
				assertNear(t, o.name+" source y", y1, y0, eps)
				assertNear(t, o.name+" zoom", v.EffectiveZoom(f), zoom, 1e-12)
				if v.Mode != mode {
					t.Fatalf("%s changed mode to %d", o.name, v.Mode)
				}
			}
		}
	}
}

func TestRotateChangesOrientation(t *testing.T) {
	f := fr(3000, 2000, 1, 1000, 800)
	var v View
	v.Reset(ZoomFit)
	v.Rotate(f, true)
	if v.Orient(f) != codec.OrientRotate90 {
		t.Fatalf("orient %d", v.Orient(f))
	}
	if w, h := v.OrientedSize(f); w != 2000 || h != 3000 {
		t.Fatalf("size %dx%d", w, h)
	}
	v.Rotate(f, false)
	v.Rotate(f, false)
	if v.Orient(f) != codec.OrientRotate270 {
		t.Fatalf("orient %d", v.Orient(f))
	}
	v.FlipH(f)
	if v.Orient(f) != codec.OrientRotate270.FlipH() {
		t.Fatalf("orient %d", v.Orient(f))
	}
	exifRotated := fr(3000, 2000, codec.OrientRotate90, 1000, 800)
	var w View
	w.Reset(ZoomFit)
	w.Rotate(exifRotated, true)
	if w.Orient(exifRotated) != codec.OrientRotate180 {
		t.Fatalf("exif and user rotations must compose, got %d", w.Orient(exifRotated))
	}
}

func TestRotateInFitModesRefits(t *testing.T) {
	f := fr(4000, 2000, 1, 1000, 800)
	var fit View
	fit.Reset(ZoomFit)
	fit.Rotate(f, true)
	if fit.Mode != ZoomFit {
		t.Fatalf("mode %d", fit.Mode)
	}
	if r := fit.ImageRect(f); r != image.Rect(300, 0, 700, 800) {
		t.Fatalf("got %v", r)
	}
	fit.Rotate(f, false)
	if r := fit.ImageRect(f); r != image.Rect(0, 150, 1000, 650) {
		t.Fatalf("got %v", r)
	}
	width := View{Mode: ZoomFitWidth}
	tall := fr(500, 3000, 1, 1000, 800)
	width.Rotate(tall, true)
	if width.Mode != ZoomFitWidth {
		t.Fatalf("mode %d", width.Mode)
	}
	assertNear(t, "fit width after rotation", width.EffectiveZoom(tall), 1000.0/3000, 1e-12)
	if r := width.ImageRect(tall); r.Min.X != 0 || r.Max.X != 1000 {
		t.Fatalf("got %v", r)
	}
}

func TestRotationCyclesReturnToStart(t *testing.T) {
	f := fr(3000, 2000, codec.OrientTranspose, 1000, 700)
	v := View{Mode: ZoomCustom, Zoom: 2}
	v.Pan(f, 150, -80)
	start := v.Mapping(f)
	for i := 0; i < 4; i++ {
		v.Rotate(f, true)
	}
	got := v.Mapping(f)
	assertNear(t, "origin x", got.OriginX, start.OriginX, eps)
	assertNear(t, "origin y", got.OriginY, start.OriginY, eps)
	if got.Orient != start.Orient {
		t.Fatalf("orient %d vs %d", got.Orient, start.Orient)
	}
	v.Rotate(f, true)
	v.Rotate(f, false)
	v.FlipH(f)
	v.FlipH(f)
	v.FlipV(f)
	v.FlipV(f)
	again := v.Mapping(f)
	assertNear(t, "origin x", again.OriginX, start.OriginX, eps)
	assertNear(t, "origin y", again.OriginY, start.OriginY, eps)
}

func TestRotateNearEdgeStaysCovered(t *testing.T) {
	f := fr(3000, 2000, 1, 1000, 700)
	v := View{Mode: ZoomCustom, Zoom: 1}
	v.Pan(f, 1e9, 1e9)
	for i := 0; i < 8; i++ {
		v.Rotate(f, true)
		v.FlipH(f)
		r := v.ImageRect(f)
		if r.Min.X > 0 || r.Min.Y > 0 || r.Max.X < f.AreaW || r.Max.Y < f.AreaH {
			t.Fatalf("step %d: %v", i, r)
		}
	}
}

func TestFromSourceInvertsToSource(t *testing.T) {
	for _, o := range allOrientations() {
		for _, dims := range [][2]float64{{300, 200}, {7, 7}, {1, 90}} {
			w, h := dims[0], dims[1]
			ow, oh := o.Size(int(w), int(h))
			for _, p := range [][2]float64{{0, 0}, {float64(ow) / 2, float64(oh) / 3}, {float64(ow), float64(oh)}, {0.25, 0.75}} {
				x, y := o.ToSource(p[0], p[1], w, h)
				u, v := fromSource(o, x, y, w, h)
				assertNear(t, "u", u, p[0], eps)
				assertNear(t, "v", v, p[1], eps)
			}
		}
	}
}

func TestZoomPercent(t *testing.T) {
	f := fr(4000, 3000, 1, 1000, 800)
	cases := []struct {
		v    View
		want int
	}{
		{View{Mode: ZoomFit}, 25},
		{View{Mode: ZoomActual}, 100},
		{View{Mode: ZoomCustom, Zoom: 1.2551}, 126},
		{View{Mode: ZoomCustom, Zoom: 1.2549}, 125},
		{View{Mode: ZoomCustom, Zoom: 0.0504}, 5},
		{View{Mode: ZoomCustom, Zoom: 64}, 6400},
		{View{Mode: ZoomCustom, Zoom: 0.333333}, 33},
	}
	for _, c := range cases {
		if got := c.v.ZoomPercent(f); got != c.want {
			t.Errorf("%+v: %d%%, want %d%%", c.v, got, c.want)
		}
	}
}

func TestFitZoom(t *testing.T) {
	var v View
	if got := v.FitZoom(fr(4000, 3000, 1, 1000, 800)); got != 0.25 {
		t.Errorf("got %v", got)
	}
	if got := v.FitZoom(fr(10, 10, 1, 1000, 800)); got != 1 {
		t.Errorf("got %v", got)
	}
	custom := View{Mode: ZoomCustom, Zoom: 9}
	if got := custom.FitZoom(fr(4000, 3000, 1, 1000, 800)); got != 0.25 {
		t.Errorf("fit zoom must not depend on the mode, got %v", got)
	}
}

func TestDegenerateFramesNeverProduceNaN(t *testing.T) {
	frames := []Frame{{}, {AreaW: 100, AreaH: 100}, {ImgW: 100, ImgH: 100}, fr(0, 5, 1, 10, 10), fr(5, 5, 1, 0, 10)}
	finite := func(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
	for _, mode := range []ZoomMode{ZoomFit, ZoomFitWidth, ZoomActual, ZoomCustom} {
		for _, f := range frames {
			var v View
			v.Reset(mode)
			v.ZoomAt(f, 5, 5, 1.25)
			v.StepZoom(f, 2, 5, 5)
			v.ZoomTo(f, 2, 5, 5)
			v.Pan(f, 10, 10)
			v.Rotate(f, true)
			v.FlipH(f)
			v.FlipV(f)
			v.ToggleFitActual(f, 1, 1)
			v.SetMode(f, ZoomActual)
			v.Clamp(f)
			m := v.Mapping(f)
			x, y := v.AreaToImage(f, 3, 3)
			px, py := v.ImageToArea(f, 3, 3)
			rect := v.ImageRect(f)
			_ = rect
			for _, val := range []float64{m.Scale, m.OriginX, m.OriginY, x, y, px, py, v.CenterU, v.CenterV, v.Zoom, v.EffectiveZoom(f)} {
				if !finite(val) {
					t.Fatalf("mode %d frame %+v: non-finite value in %+v / %+v", mode, f, v, m)
				}
			}
			if m.Scale <= 0 {
				t.Fatalf("scale %v", m.Scale)
			}
			v.CanPan(f)
			v.ZoomPercent(f)
		}
	}
}

func TestViewIsComparable(t *testing.T) {
	a := View{Mode: ZoomCustom, Zoom: 2, CenterU: 5, CenterV: 6, User: codec.OrientFlipH, Filter: render.FilterBilinear}
	b := a
	if a != b {
		t.Fatal("equal views must compare equal")
	}
	b.Zoom = 3
	if a == b {
		t.Fatal("different views must differ")
	}
}

func TestRotate180TurnsTheOrientationHalfWay(t *testing.T) {
	f := fr(3000, 2000, 1, 1000, 800)
	var v View
	v.Reset(ZoomFit)
	v.Rotate180(f)
	if v.Orient(f) != codec.OrientRotate180 {
		t.Fatalf("orient %d", v.Orient(f))
	}
	if w, h := v.OrientedSize(f); w != 3000 || h != 2000 {
		t.Fatalf("size %dx%d", w, h)
	}
	v.Rotate(f, true)
	if v.Orient(f) != codec.OrientRotate270 {
		t.Fatalf("orient after a further turn %d", v.Orient(f))
	}
	v.Rotate180(f)
	if v.Orient(f) != codec.OrientRotate90 {
		t.Fatalf("orient after the second half turn %d", v.Orient(f))
	}
}
