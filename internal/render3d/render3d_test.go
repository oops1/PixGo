package render3d

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"

	"github.com/oops1/PixGo/internal/mesh"
	"github.com/oops1/PixGo/internal/par"
)

func boxMesh(lo, hi [3]float32) *mesh.Mesh {
	c := func(i int) [3]float32 {
		p := lo
		if i&1 != 0 {
			p[0] = hi[0]
		}
		if i&2 != 0 {
			p[1] = hi[1]
		}
		if i&4 != 0 {
			p[2] = hi[2]
		}
		return p
	}
	quads := [][4]int{{0, 2, 3, 1}, {4, 5, 7, 6}, {0, 1, 5, 4}, {2, 6, 7, 3}, {0, 4, 6, 2}, {1, 3, 7, 5}}
	m := &mesh.Mesh{}
	for _, q := range quads {
		for _, t := range [][3]int{{q[0], q[1], q[2]}, {q[0], q[2], q[3]}} {
			for _, i := range t {
				p := c(i)
				m.V = append(m.V, p[0], p[1], p[2])
			}
		}
	}
	m.UpdateBounds()
	return m
}

func cubeMesh() *mesh.Mesh { return boxMesh([3]float32{-1, -1, -1}, [3]float32{1, 1, 1}) }

type bbox struct {
	x0, y0, x1, y1 int
	full, partial  int
}

func alphaBox(img *image.NRGBA) bbox {
	b := bbox{x0: math.MaxInt, y0: math.MaxInt, x1: -1, y1: -1}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := img.Pix[y*img.Stride+x*4+3]
			if a == 0 {
				continue
			}
			if a == 255 {
				b.full++
			} else {
				b.partial++
			}
			b.x0, b.y0 = min(b.x0, x), min(b.y0, y)
			b.x1, b.y1 = max(b.x1, x+1), max(b.y1, y+1)
		}
	}
	return b
}

func render(t *testing.T, s *Scene, cam Camera, w, h int, st Style) *image.NRGBA {
	t.Helper()
	img, err := s.Render(context.Background(), cam, w, h, st)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func transparent() Style {
	st := DefaultStyle()
	st.Background = color.NRGBA{}
	return st
}

func (v *view) toPixel(p [3]float64) (float64, float64) {
	S := float64(v.S)
	if v.ortho {
		x := v.ax[0]*p[0] + v.ax[1]*p[1] + v.ax[2]*p[2] + v.ax[3]
		y := v.ay[0]*p[0] + v.ay[1]*p[1] + v.ay[2]*p[2] + v.ay[3]
		return x / S, y / S
	}
	c := cvert{
		v.ax[0]*p[0] + v.ax[1]*p[1] + v.ax[2]*p[2] + v.ax[3],
		v.ay[0]*p[0] + v.ay[1]*p[1] + v.ay[2]*p[2] + v.ay[3],
		v.az[0]*p[0] + v.az[1]*p[1] + v.az[2]*p[2] + v.az[3],
	}
	sv := v.project(c)
	return sv.x / S, sv.y / S
}

func TestCubeFrontView(t *testing.T) {
	s := NewScene(cubeMesh())
	img := render(t, s, Camera{Zoom: 1}, 200, 200, transparent())
	b := alphaBox(img)
	if b.x0 != 8 || b.y0 != 8 || b.x1 != 192 || b.y1 != 192 {
		t.Fatalf("bbox %+v", b)
	}
	if b.full != 184*184 || b.partial != 0 {
		t.Fatalf("full %d partial %d", b.full, b.partial)
	}
}

func TestBoxTopView(t *testing.T) {
	s := NewScene(boxMesh([3]float32{0, 0, 0}, [3]float32{40, 20, 5}))
	cam := Camera{Pitch: math.Pi / 2, Zoom: 1}
	if a := s.Aspect(cam); math.Abs(a-2) > 1e-9 {
		t.Fatalf("aspect %v", a)
	}
	img := render(t, s, cam, 300, 150, transparent())
	b := alphaBox(img)
	if b.x0 != 12 || b.x1 != 288 || b.y0 != 6 || b.y1 != 144 {
		t.Fatalf("bbox %+v", b)
	}
	cam.Pitch = 0
	if a := s.Aspect(cam); math.Abs(a-8) > 1e-9 {
		t.Fatalf("front aspect %v", a)
	}
}

func TestDefaultViewProjection(t *testing.T) {
	m := boxMesh([3]float32{-10, -5, 0}, [3]float32{30, 15, 12})
	s := NewScene(m)
	cam := DefaultCamera()
	w, h := 320, 240
	v := s.newView(cam, w, h, 2)
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for i := 0; i < len(m.V); i += 3 {
		x, y := v.toPixel([3]float64{float64(m.V[i]), float64(m.V[i+1]), float64(m.V[i+2])})
		minX, maxX = min(minX, x), max(maxX, x)
		minY, maxY = min(minY, y), max(maxY, y)
	}
	img := render(t, s, cam, w, h, transparent())
	b := alphaBox(img)
	if math.Abs(float64(b.x0)-minX) > 1 || math.Abs(float64(b.x1)-maxX) > 1 || math.Abs(float64(b.y0)-minY) > 1 || math.Abs(float64(b.y1)-maxY) > 1 {
		t.Fatalf("bbox %+v projected %.2f %.2f %.2f %.2f", b, minX, minY, maxX, maxY)
	}
	cx, cy := (minX+maxX)/2, (minY+maxY)/2
	if math.Abs(cx-float64(w)/2) > 1e-6 || math.Abs(cy-float64(h)/2) > 1e-6 {
		t.Fatalf("not centred: %v %v", cx, cy)
	}
	if fillW, fillH := (maxX-minX)/float64(w), (maxY-minY)/float64(h); math.Abs(max(fillW, fillH)-fitMargin) > 1e-6 {
		t.Fatalf("fill %v %v", fillW, fillH)
	}
	asp := s.Aspect(cam)
	if math.Abs(asp-(maxX-minX)/(maxY-minY)) > 1e-6 {
		t.Fatalf("aspect %v vs %v", asp, (maxX-minX)/(maxY-minY))
	}
}

func TestLockFitSamePicture(t *testing.T) {
	s := NewScene(boxMesh([3]float32{-10, -5, 0}, [3]float32{30, 15, 12}))
	cam := DefaultCamera()
	a := render(t, s, cam, 200, 150, DefaultStyle())
	locked := s.LockFit(cam, 200, 150)
	if locked.Fit <= 0 {
		t.Fatal("fit not set")
	}
	b := render(t, s, locked, 200, 150, DefaultStyle())
	if d := diffPixels(a, b, 2); d > 0 {
		t.Fatalf("%d pixels differ", d)
	}
	o := Orbit(locked, 300, 0, 150)
	if o.Fit != locked.Fit {
		t.Fatal("orbit dropped fit")
	}
}

func diffPixels(a, b *image.NRGBA, tol int) int {
	n := 0
	for i := range a.Pix {
		d := int(a.Pix[i]) - int(b.Pix[i])
		if d > tol || d < -tol {
			n++
		}
	}
	return n
}

func TestDeterminism(t *testing.T) {
	m := sphereMesh(24, 48, 10, false)
	s := NewScene(m)
	st := DefaultStyle()
	st.Background = color.NRGBA{40, 40, 40, 255}
	a := render(t, s, DefaultCamera(), 160, 120, st)
	defer par.Set(0)
	par.Set(1)
	b := render(t, NewScene(m), DefaultCamera(), 160, 120, st)
	par.Set(7)
	c := render(t, s, DefaultCamera(), 160, 120, st)
	ha, hb, hc := hashImg(a), hashImg(b), hashImg(c)
	if ha != hb || ha != hc {
		t.Fatalf("not deterministic: %s %s %s", ha, hb, hc)
	}
	if runtime.GOARCH == "amd64" {
		const want = "85563a0747c6f3f0a3ac53dccf27db8062eb25c7984819b33769a0e2e83dbec8"
		if ha != want {
			t.Fatalf("hash %s", ha)
		}
	}
}

func hashImg(img *image.NRGBA) string {
	h := sha256.Sum256(img.Pix)
	return hex.EncodeToString(h[:])
}

func TestBandsMatchSinglePass(t *testing.T) {
	m := sphereMesh(30, 60, 10, true)
	s := NewScene(m)
	st := DefaultStyle()
	st.Background = color.NRGBA{250, 250, 250, 255}
	st.Samples = 3
	a := render(t, s, DefaultCamera(), 150, 100, st)
	old := bandSamples
	defer func() { bandSamples = old }()
	bandSamples = 450 * 9 * 7
	b := render(t, s, DefaultCamera(), 150, 100, st)
	if hashImg(a) != hashImg(b) {
		t.Fatalf("banded render differs: %d pixels", diffPixels(a, b, 0))
	}
}

func TestRenderIntoMatchesRender(t *testing.T) {
	s := NewScene(sphereMesh(12, 24, 3, false))
	st := DefaultStyle()
	st.Background = color.NRGBA{20, 30, 40, 255}
	a := render(t, s, DefaultCamera(), 64, 48, st)
	dst := image.NewRGBA(image.Rect(0, 0, 64, 48))
	if err := s.RenderInto(context.Background(), DefaultCamera(), st, dst); err != nil {
		t.Fatal(err)
	}
	for i := range a.Pix {
		if a.Pix[i] != dst.Pix[i] {
			t.Fatalf("byte %d: %d vs %d", i, a.Pix[i], dst.Pix[i])
		}
	}
	st.Background = color.NRGBA{}
	a = render(t, s, DefaultCamera(), 64, 48, st)
	if err := s.RenderInto(context.Background(), DefaultCamera(), st, dst); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(a.Pix); i += 4 {
		al := uint32(a.Pix[i+3])
		for k := 0; k < 3; k++ {
			want := uint8((uint32(a.Pix[i+k])*al + 127) / 255)
			if dst.Pix[i+k] != want || dst.Pix[i+3] != a.Pix[i+3] {
				t.Fatalf("pixel %d: premul mismatch", i/4)
			}
		}
	}
}

func TestStraightAlphaEdges(t *testing.T) {
	s := NewScene(cubeMesh())
	st := transparent()
	st.Edges = false
	st.Samples = 3
	cam := Camera{Yaw: 0.3, Zoom: 1}
	img := render(t, s, cam, 120, 120, st)
	b := alphaBox(img)
	if b.partial == 0 {
		t.Fatal("no anti-aliased pixels")
	}
	ref := map[[3]uint8]bool{}
	w := img.Rect.Dx()
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i+3] == 255 {
			ref[[3]uint8{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}] = true
		}
	}
	for i := 0; i < len(img.Pix); i += 4 {
		a := img.Pix[i+3]
		if a == 0 || a == 255 {
			continue
		}
		c := [3]uint8{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}
		best := 999
		for r := range ref {
			d := 0
			for k := 0; k < 3; k++ {
				d = max(d, absInt(int(c[k])-int(r[k])))
			}
			best = min(best, d)
		}
		if best > 40 {
			t.Fatalf("pixel %d,%d alpha %d colour %v is not a straight model colour", (i/4)%w, (i/4)/w, a, c)
		}
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestEmptyMesh(t *testing.T) {
	bg := color.NRGBA{10, 20, 30, 255}
	st := DefaultStyle()
	st.Background = bg
	for _, m := range []*mesh.Mesh{nil, {}, {V: []float32{1, 2}}} {
		s := NewScene(m)
		img := render(t, s, DefaultCamera(), 33, 17, st)
		for i := 0; i < len(img.Pix); i += 4 {
			if (color.NRGBA{img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]}) != bg {
				t.Fatal("not background")
			}
		}
		if a := s.Aspect(DefaultCamera()); a != 1 {
			t.Fatalf("aspect %v", a)
		}
		c := s.ZoomAt(DefaultCamera(), 33, 17, 5, 5, 2)
		if !finite(c.Zoom) || !finite(c.PanX) {
			t.Fatal("bad zoom")
		}
		dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
		if err := s.RenderInto(context.Background(), DefaultCamera(), st, dst); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDegenerateAndNonFinite(t *testing.T) {
	m := cubeMesh()
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	m.V = append(m.V,
		0, 0, 0, 0, 0, 0, 0, 0, 0,
		1, 1, 1, 2, 2, 2, 3, 3, 3,
		nan, 0, 0, 1, 0, 0, 0, 1, 0,
		0, 0, 0, inf, 0, 0, 0, 1, 0,
		0, 0, 0, 1, -inf, 0, 0, 1, nan,
	)
	m.UpdateBounds()
	m.Min, m.Max = [3]float32{nan, 0, 0}, [3]float32{inf, 1, 1}
	s := NewScene(m)
	if s.Radius() <= 0 || !finite(s.Radius()) {
		t.Fatalf("radius %v", s.Radius())
	}
	for _, cam := range []Camera{DefaultCamera(), {FOV: 0.8, Zoom: 3}, {Yaw: math.NaN(), Pitch: math.Inf(1), Zoom: math.NaN(), PanX: math.Inf(-1), FOV: math.NaN()}} {
		img := render(t, s, cam, 64, 64, DefaultStyle())
		if alphaBox(img).full == 0 {
			t.Fatalf("nothing drawn for %+v", cam)
		}
	}
	far := cubeMesh()
	far.V = append(far.V, 1e30, 0, 0, -1e30, 1, 0, 0, 1e30, 1, 3e38, 3e38, 3e38, -3e38, -3e38, -3e38, 0, 0, 1)
	far.UpdateBounds()
	for _, cam := range []Camera{DefaultCamera(), {FOV: 1, Zoom: 1e7}} {
		render(t, NewScene(far), cam, 32, 32, DefaultStyle())
	}
	clean := NewScene(boxMesh([3]float32{-1, -1, -1}, [3]float32{1, 1, 1}))
	m2 := cubeMesh()
	m2.V = append(m2.V, nan, nan, nan, 0, 0, 0, 1, 1, 1)
	m2.UpdateBounds()
	a := render(t, clean, DefaultCamera(), 80, 80, DefaultStyle())
	b := render(t, NewScene(m2), DefaultCamera(), 80, 80, DefaultStyle())
	if hashImg(a) != hashImg(b) {
		t.Fatal("NaN triangle changed the picture")
	}
}

func TestHugeZoom(t *testing.T) {
	s := NewScene(sphereMesh(40, 80, 5, true))
	for _, fov := range []float64{0, 0.9} {
		for _, z := range []float64{1e-3, 50, 1e4, 1e9} {
			cam := DefaultCamera()
			cam.FOV = fov
			cam.Zoom = z
			cam.PanX = 0.3
			img := render(t, s, cam, 96, 64, DefaultStyle())
			b := alphaBox(img)
			if z >= 50 && fov == 0 && b.full != 96*64 {
				t.Fatalf("fov %v zoom %v: expected full coverage, got %+v", fov, z, b)
			}
		}
	}
	cam := Camera{FOV: 1, Zoom: 1}
	inside := cam
	inside.Zoom = 1e6
	img := render(t, s, inside, 64, 64, transparent())
	if b := alphaBox(img); b.full != 64*64 {
		t.Fatalf("inside the sphere expected full coverage, got %+v", b)
	}
}

func TestPanMath(t *testing.T) {
	cam := DefaultCamera()
	cam.Zoom = 2.5
	p := PanBy(PanBy(cam, 37, -12, 480), -37, 12, 480)
	if math.Abs(p.PanX-cam.PanX) > 1e-12 || math.Abs(p.PanY-cam.PanY) > 1e-12 {
		t.Fatalf("round trip %+v", p)
	}
	s := NewScene(cubeMesh())
	base := Camera{Zoom: 0.7}
	base = s.LockFit(base, 200, 160)
	a := alphaBox(render(t, s, base, 200, 160, transparent()))
	moved := PanBy(base, 17, -9, 160)
	b := alphaBox(render(t, s, moved, 200, 160, transparent()))
	if b.x0-a.x0 != 17 || b.x1-a.x1 != 17 || b.y0-a.y0 != -9 || b.y1-a.y1 != -9 {
		t.Fatalf("pan moved %+v -> %+v", a, b)
	}
	if got := PanBy(cam, 1, 1, 0); got != cam {
		t.Fatal("zero view height must not change the camera")
	}
}

func TestOrbit(t *testing.T) {
	cam := DefaultCamera()
	up := Orbit(cam, 0, 1e6, 600)
	if math.Abs(up.Pitch-maxPitch) > 1e-12 {
		t.Fatalf("pitch %v", up.Pitch)
	}
	down := Orbit(cam, 0, -1e6, 600)
	if math.Abs(down.Pitch+maxPitch) > 1e-12 {
		t.Fatalf("pitch %v", down.Pitch)
	}
	r := Orbit(cam, 600, 0, 600)
	if math.Abs(math.Remainder(r.Yaw-cam.Yaw-math.Pi, 2*math.Pi)) > 1e-12 {
		t.Fatalf("yaw %v", r.Yaw)
	}
	if r.Yaw < -math.Pi || r.Yaw > math.Pi {
		t.Fatalf("yaw not wrapped %v", r.Yaw)
	}
}

func TestZoomAtKeepsPoint(t *testing.T) {
	m := boxMesh([3]float32{-10, -5, 0}, [3]float32{30, 15, 12})
	s := NewScene(m)
	w, h := 300, 200
	for _, fov := range []float64{0, 0.7} {
		for step, pt := range [][2]float64{{171.5, 93.5}, {120.25, 120.75}, {5.5, 7.5}, {150, 100}} {
			cam := DefaultCamera()
			cam.FOV = fov
			cam.PanX = 0.1
			for _, f := range []float64{1.6, 0.6, 3} {
				v := s.newView(cam, w, h, 1)
				o, d := pixelRay(v, pt[0], pt[1])
				var p [3]float64
				if tt, ok := s.rayCast(o, d); ok {
					p = add(o, scale3(d, tt))
				} else if fov == 0 {
					p = add(o, scale3(d, 4*v.R))
				} else {
					p = add(o, scale3(d, v.D/dot(d, v.fr.f)))
				}
				x0, y0 := v.toPixel(p)
				if math.Abs(x0-pt[0]) > 1e-6 || math.Abs(y0-pt[1]) > 1e-6 {
					t.Fatalf("setup: %v,%v vs %v", x0, y0, pt)
				}
				next := s.ZoomAt(cam, w, h, pt[0], pt[1], f)
				if next.Zoom == cam.Zoom {
					t.Fatalf("fov %v step %d: zoom unchanged", fov, step)
				}
				nv := s.newView(next, w, h, 1)
				x1, y1 := nv.toPixel(p)
				if math.Abs(x1-pt[0]) > 1 || math.Abs(y1-pt[1]) > 1 {
					t.Fatalf("fov %v step %d factor %v: point moved from %v to %.3f,%.3f", fov, step, f, pt, x1, y1)
				}
				cam = next
			}
		}
	}
}

func pixelRay(v *view, x, y float64) ([3]float64, [3]float64) {
	half := float64(v.hs) / 2
	xn := (x*float64(v.S) - v.cx) / half
	yn := (v.cy - y*float64(v.S)) / half
	if v.ortho {
		hw := half / v.k
		o := add(v.tgt, add(scale3(v.fr.r, xn*hw), scale3(v.fr.u, yn*hw)))
		return sub(o, scale3(v.fr.f, 4*v.R)), v.fr.f
	}
	d := norm3(add(v.fr.f, add(scale3(v.fr.r, xn*v.tanHalf), scale3(v.fr.u, yn*v.tanHalf))))
	return v.eye, d
}

func TestContextCancel(t *testing.T) {
	s := NewScene(sphereMesh(50, 100, 5, false))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Render(ctx, DefaultCamera(), 100, 100, DefaultStyle()); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	dst := image.NewRGBA(image.Rect(0, 0, 50, 50))
	if err := s.RenderInto(ctx, DefaultCamera(), DefaultStyle(), dst); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

func TestBadSize(t *testing.T) {
	s := NewScene(cubeMesh())
	for _, wh := range [][2]int{{0, 10}, {10, -1}, {1 << 16, 2}, {20000, 20000}} {
		if _, err := s.Render(context.Background(), DefaultCamera(), wh[0], wh[1], DefaultStyle()); !errors.Is(err, ErrBadSize) {
			t.Fatalf("%v: err %v", wh, err)
		}
	}
	if err := s.RenderInto(context.Background(), DefaultCamera(), DefaultStyle(), nil); err == nil {
		t.Fatal("nil dst accepted")
	}
}

func TestColorsAndNormals(t *testing.T) {
	m := cubeMesh()
	m.C = make([]uint8, m.Triangles()*3)
	for i := 0; i < m.Triangles(); i++ {
		m.C[i*3] = 220
		m.C[i*3+1] = 30
		m.C[i*3+2] = 30
	}
	img := render(t, NewScene(m), DefaultCamera(), 64, 64, DefaultStyle())
	c := img.Pix[(32*img.Stride)+32*4:]
	if !(c[0] > 2*c[1] && c[0] > 2*c[2]) {
		t.Fatalf("not red: %v", c[:4])
	}
	flat := render(t, NewScene(sphereMesh(60, 120, 1, false)), DefaultCamera(), 64, 64, DefaultStyle())
	smooth := render(t, NewScene(sphereMesh(60, 120, 1, true)), DefaultCamera(), 64, 64, DefaultStyle())
	if d := diffPixels(flat, smooth, 12); d > 64*64*4/50 {
		t.Fatalf("smooth and flat spheres differ too much: %d", d)
	}
	bad := sphereMesh(10, 20, 1, true)
	for i := range bad.N {
		bad.N[i] = 0
	}
	bad.N[5] = float32(math.NaN())
	render(t, NewScene(bad), Camera{FOV: 0.5, Zoom: 1}, 40, 40, DefaultStyle())
}

func TestInwardWindingLitTheSame(t *testing.T) {
	m := cubeMesh()
	flip := &mesh.Mesh{V: append([]float32(nil), m.V...)}
	for i := 0; i < len(flip.V); i += 9 {
		for k := 0; k < 3; k++ {
			flip.V[i+3+k], flip.V[i+6+k] = flip.V[i+6+k], flip.V[i+3+k]
		}
	}
	flip.UpdateBounds()
	a := render(t, NewScene(m), DefaultCamera(), 90, 90, DefaultStyle())
	b := render(t, NewScene(flip), DefaultCamera(), 90, 90, DefaultStyle())
	if d := diffPixels(a, b, 1); d > 0 {
		t.Fatalf("%d bytes differ", d)
	}
}

func TestWatertightRaster(t *testing.T) {
	m := sphereMesh(70, 140, 1, false)
	s := NewScene(m)
	st := transparent()
	st.Edges = false
	for _, cam := range []Camera{DefaultCamera(), {FOV: 0.6, Zoom: 1.2}, {Yaw: 1, Pitch: -0.4, Zoom: 1}} {
		img := render(t, s, cam, 101, 77, st)
		w, h := img.Rect.Dx(), img.Rect.Dy()
		alpha := func(x, y int) uint8 { return img.Pix[y*img.Stride+x*4+3] }
		for y := 1; y < h-1; y++ {
			for x := 1; x < w-1; x++ {
				if alpha(x, y) == 255 {
					continue
				}
				if alpha(x-1, y) == 255 && alpha(x+1, y) == 255 && alpha(x, y-1) == 255 && alpha(x, y+1) == 255 {
					t.Fatalf("hole at %d,%d (%+v)", x, y, cam)
				}
			}
		}
	}
}

func TestPerspectiveFit(t *testing.T) {
	m := boxMesh([3]float32{-10, -5, 0}, [3]float32{30, 15, 12})
	s := NewScene(m)
	for _, fov := range []float64{0.3, 0.8, 1.5} {
		cam := DefaultCamera()
		cam.FOV = fov
		w, h := 320, 200
		v := s.newView(cam, w, h, 1)
		minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for i := 0; i < len(m.V); i += 3 {
			x, y := v.toPixel([3]float64{float64(m.V[i]), float64(m.V[i+1]), float64(m.V[i+2])})
			minX, maxX = min(minX, x), max(maxX, x)
			minY, maxY = min(minY, y), max(maxY, y)
		}
		if minX < 0 || minY < 0 || maxX > float64(w) || maxY > float64(h) {
			t.Fatalf("fov %v: model outside the picture %.1f %.1f %.1f %.1f", fov, minX, minY, maxX, maxY)
		}
		fill := max((maxX-minX)/float64(w), (maxY-minY)/float64(h))
		if fill < 0.85 || fill > 0.95 {
			t.Fatalf("fov %v: fill %v", fov, fill)
		}
		if math.Abs((minX+maxX)/2-float64(w)/2) > 3 || math.Abs((minY+maxY)/2-float64(h)/2) > 3 {
			t.Fatalf("fov %v: not centred %.1f %.1f %.1f %.1f", fov, minX, minY, maxX, maxY)
		}
		img := render(t, s, cam, w, h, transparent())
		b := alphaBox(img)
		if math.Abs(float64(b.x0)-minX) > 1 || math.Abs(float64(b.x1)-maxX) > 1 || math.Abs(float64(b.y0)-minY) > 1 || math.Abs(float64(b.y1)-maxY) > 1 {
			t.Fatalf("fov %v: bbox %+v projected %.2f %.2f %.2f %.2f", fov, b, minX, minY, maxX, maxY)
		}
		locked := s.LockFit(cam, w, h)
		if d := diffPixels(img, render(t, s, locked, w, h, transparent()), 2); d > 0 {
			t.Fatalf("fov %v: locked fit differs in %d bytes", fov, d)
		}
	}
}

func TestConcurrentRenders(t *testing.T) {
	s := NewScene(sphereMesh(20, 40, 4, true))
	st := DefaultStyle()
	want := hashImg(render(t, s, DefaultCamera(), 70, 50, st))
	errs := make(chan string, 8)
	for g := 0; g < 8; g++ {
		go func() {
			img, err := s.Render(context.Background(), DefaultCamera(), 70, 50, st)
			switch {
			case err != nil:
				errs <- err.Error()
			case hashImg(img) != want:
				errs <- "different picture"
			default:
				errs <- ""
			}
		}()
	}
	for g := 0; g < 8; g++ {
		if e := <-errs; e != "" {
			t.Fatal(e)
		}
	}
}
