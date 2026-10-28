package render3d

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/mesh"
	"github.com/oops1/PixGo/internal/par"
)

const (
	maxPitch  = 89.9 * math.Pi / 180
	minZoom   = 1e-4
	maxZoom   = 1e7
	maxSide   = 1 << 15
	maxPixels = 1 << 28
	fitMargin = 0.92
)

var bandSamples = 1 << 23

var maxTriangles = int64(math.MaxUint32 - 1)

var ErrBadSize = errors.New("render3d: invalid image size")

type Camera struct {
	Yaw, Pitch float64
	Zoom       float64
	PanX, PanY float64
	FOV        float64
	Fit        float64
}

func DefaultCamera() Camera {
	return Camera{Yaw: -35 * math.Pi / 180, Pitch: 25 * math.Pi / 180, Zoom: 1}
}

type Style struct {
	Background color.NRGBA
	Base       color.NRGBA
	Samples    int
	Edges      bool
}

func DefaultStyle() Style {
	return Style{Base: color.NRGBA{R: 182, G: 192, B: 206, A: 255}, Samples: 2, Edges: true}
}

type Scene struct {
	m      *mesh.Mesh
	tris   int
	center [3]float64
	radius float64
	smooth bool
	colors bool

	mu     sync.Mutex
	buf    []uint64
	extKey [2]extentKey
	extVal [2]extents
	extOK  [2]bool
}

func NewScene(m *mesh.Mesh) *Scene {
	s := &Scene{m: m, radius: 1}
	if m == nil {
		return s
	}
	s.tris = m.Triangles()
	if int64(s.tris) > maxTriangles {
		s.tris = int(maxTriangles)
	}
	if s.tris == 0 {
		return s
	}
	s.smooth = m.HasNormals()
	s.colors = m.HasColors()
	lo, hi := m.Min, m.Max
	if !boundsUsable(lo, hi) {
		lo, hi = s.scanBounds()
	}
	for k := 0; k < 3; k++ {
		s.center[k] = (float64(lo[k]) + float64(hi[k])) / 2
	}
	dx, dy, dz := float64(hi[0])-float64(lo[0]), float64(hi[1])-float64(lo[1]), float64(hi[2])-float64(lo[2])
	r := math.Sqrt(dx*dx+dy*dy+dz*dz) / 2
	if r > 0 && finite(r) {
		s.radius = r
	} else if c := max(math.Abs(s.center[0]), math.Abs(s.center[1]), math.Abs(s.center[2])); c > 0 && finite(c) {
		s.radius = c * 1e-6
	}
	return s
}

func boundsUsable(lo, hi [3]float32) bool {
	same := true
	for k := 0; k < 3; k++ {
		if !finite32(lo[k]) || !finite32(hi[k]) || hi[k] < lo[k] {
			return false
		}
		if hi[k] != lo[k] {
			same = false
		}
	}
	return !same
}

func finite32(v float32) bool { return v-v == 0 }

func (s *Scene) scanBounds() ([3]float32, [3]float32) {
	v := s.m.V
	const chunk = 1 << 15
	tasks := (s.tris + chunk - 1) / chunk
	los := make([][3]float32, tasks)
	his := make([][3]float32, tasks)
	oks := make([]bool, tasks)
	par.Parallel(tasks, par.Count(), func(t int) {
		lo := [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
		hi := [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
		ok := false
		end := min((t+1)*chunk, s.tris)
		for i := t * chunk; i < end; i++ {
			p := v[i*9 : i*9+9 : i*9+9]
			if !finiteTri(p) {
				continue
			}
			ok = true
			for c := 0; c < 9; c++ {
				k := c % 3
				lo[k] = min(lo[k], p[c])
				hi[k] = max(hi[k], p[c])
			}
		}
		los[t], his[t], oks[t] = lo, hi, ok
	})
	lo := [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	hi := [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	any := false
	for t := range tasks {
		if !oks[t] {
			continue
		}
		any = true
		for k := 0; k < 3; k++ {
			lo[k] = min(lo[k], los[t][k])
			hi[k] = max(hi[k], his[t][k])
		}
	}
	if !any {
		return [3]float32{}, [3]float32{}
	}
	return lo, hi
}

func finiteTri(p []float32) bool {
	return p[0]-p[0] == 0 && p[1]-p[1] == 0 && p[2]-p[2] == 0 &&
		p[3]-p[3] == 0 && p[4]-p[4] == 0 && p[5]-p[5] == 0 &&
		p[6]-p[6] == 0 && p[7]-p[7] == 0 && p[8]-p[8] == 0
}

func (s *Scene) Triangles() int { return s.tris }

func (s *Scene) Center() [3]float64 { return s.center }

func (s *Scene) Radius() float64 { return s.radius }

func checkSize(w, h int) error {
	if w <= 0 || h <= 0 || w > maxSide || h > maxSide || w*h > maxPixels {
		return fmt.Errorf("render3d: %dx%d: %w: %w", w, h, ErrBadSize, codec.ErrTooLarge)
	}
	return nil
}

func samplesOf(st Style) int {
	switch {
	case st.Samples <= 0:
		return 2
	case st.Samples > 3:
		return 3
	}
	return st.Samples
}

func (s *Scene) Render(ctx context.Context, cam Camera, w, h int, st Style) (*image.NRGBA, error) {
	if err := checkSize(w, h); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	out := &target{nrgba: img, w: w, h: h}
	if err := s.render(ctx, cam, st, out); err != nil {
		return nil, err
	}
	return img, nil
}

func (s *Scene) RenderInto(ctx context.Context, cam Camera, st Style, dst *image.RGBA) error {
	if dst == nil {
		return fmt.Errorf("render3d: nil destination: %w", ErrBadSize)
	}
	b := dst.Bounds()
	if err := checkSize(b.Dx(), b.Dy()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	out := &target{rgba: dst, w: b.Dx(), h: b.Dy()}
	return s.render(ctx, cam, st, out)
}

func (s *Scene) render(ctx context.Context, cam Camera, st Style, out *target) error {
	sh := newShading(st)
	if s.tris == 0 {
		fillBackground(out, sh)
		return nil
	}
	S := samplesOf(st)
	v := s.newView(cam, out.w, out.h, S)
	ws := out.w * S
	bandRows := max(1, bandSamples/(ws*S))
	bandRows = min(bandRows, out.h)
	need := ws * (bandRows*S + 2)
	buf := s.takeBuf(need)
	defer s.putBuf(buf)
	for oy0 := 0; oy0 < out.h; oy0 += bandRows {
		oy1 := min(oy0+bandRows, out.h)
		ry0 := max(oy0*S-1, 0)
		ry1 := min(oy1*S+1, v.hs)
		b := buf[:ws*(ry1-ry0)]
		clearBuf(b)
		if err := s.rasterize(ctx, v, b, ry0, ry1); err != nil {
			return err
		}
		if err := s.shadeBand(ctx, v, sh, b, ry0, ry1, oy0, oy1, out); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scene) takeBuf(n int) []uint64 {
	s.mu.Lock()
	b := s.buf
	s.buf = nil
	s.mu.Unlock()
	if cap(b) >= n {
		return b[:n]
	}
	return make([]uint64, n)
}

func (s *Scene) putBuf(b []uint64) {
	s.mu.Lock()
	if cap(b) > cap(s.buf) {
		s.buf = b
	}
	s.mu.Unlock()
}

func (s *Scene) ReleaseBuffers() {
	s.mu.Lock()
	s.buf = nil
	s.mu.Unlock()
}

func clearBuf(b []uint64) {
	const chunk = 1 << 16
	tasks := (len(b) + chunk - 1) / chunk
	par.Parallel(tasks, par.Count(), func(t int) {
		p := b[t*chunk : min((t+1)*chunk, len(b))]
		for i := range p {
			p[i] = emptyKey
		}
	})
}

func (s *Scene) Aspect(cam Camera) float64 {
	if s.tris == 0 {
		return 1
	}
	cam = sanitize(cam)
	e := s.extentsFor(cam.Yaw, cam.Pitch)
	w, h := e.xhi-e.xlo, e.yhi-e.ylo
	if !e.ok || !(w > 0) || !(h > 0) {
		return 1
	}
	return math.Min(math.Max(w/h, 1.0/64), 64)
}

func (s *Scene) LockFit(cam Camera, w, h int) Camera {
	cam = sanitize(cam)
	if cam.Fit > 0 || w <= 0 || h <= 0 {
		return cam
	}
	return s.applyFit(cam, w, h)
}

func sanitize(c Camera) Camera {
	d := DefaultCamera()
	if !finite(c.Yaw) {
		c.Yaw = d.Yaw
	}
	if !finite(c.Pitch) {
		c.Pitch = d.Pitch
	}
	c.Pitch = math.Max(-math.Pi/2, math.Min(math.Pi/2, c.Pitch))
	if !finite(c.Zoom) || c.Zoom <= 0 {
		c.Zoom = 1
	}
	c.Zoom = math.Max(minZoom, math.Min(maxZoom, c.Zoom))
	if !finite(c.PanX) {
		c.PanX = 0
	}
	if !finite(c.PanY) {
		c.PanY = 0
	}
	c.PanX = math.Max(-1e9, math.Min(1e9, c.PanX))
	c.PanY = math.Max(-1e9, math.Min(1e9, c.PanY))
	if !finite(c.FOV) || c.FOV < 0 {
		c.FOV = 0
	}
	if c.FOV > 0 {
		c.FOV = math.Max(1*math.Pi/180, math.Min(150*math.Pi/180, c.FOV))
	}
	if !finite(c.Fit) || c.Fit < 0 {
		c.Fit = 0
	}
	return c
}

func finite(v float64) bool { return v-v == 0 }

func Orbit(cam Camera, dxPixels, dyPixels float64, viewH int) Camera {
	if viewH <= 0 || !finite(dxPixels) || !finite(dyPixels) {
		return cam
	}
	k := math.Pi / float64(viewH)
	cam.Yaw = math.Remainder(cam.Yaw+dxPixels*k, 2*math.Pi)
	cam.Pitch = math.Max(-maxPitch, math.Min(maxPitch, cam.Pitch+dyPixels*k))
	return cam
}

func PanBy(cam Camera, dxPixels, dyPixels float64, viewH int) Camera {
	if viewH <= 0 || !finite(dxPixels) || !finite(dyPixels) {
		return cam
	}
	z := cam.Zoom
	if !finite(z) || z <= 0 {
		z = 1
	}
	scale := 1 / (z * float64(viewH) / 2)
	cam.PanX += dxPixels * scale
	cam.PanY -= dyPixels * scale
	return cam
}

func (s *Scene) ZoomAt(cam Camera, w, h int, x, y, factor float64) Camera {
	if w <= 0 || h <= 0 || !finite(factor) || factor <= 0 || !finite(x) || !finite(y) {
		return cam
	}
	cam = sanitize(cam)
	a := (x - float64(w)/2) / (float64(h) / 2)
	b := (float64(h)/2 - y) / (float64(h) / 2)
	z0 := cam.Zoom
	z1 := math.Max(minZoom, math.Min(maxZoom, z0*factor))
	if cam.FOV == 0 {
		cam.PanX += a * (1/z1 - 1/z0)
		cam.PanY += b * (1/z1 - 1/z0)
		cam.Zoom = z1
		return cam
	}
	eff := s.applyFit(cam, w, h)
	H := eff.Fit
	offX, offY := eff.PanX-cam.PanX, eff.PanY-cam.PanY
	fr := frameOf(cam.Yaw, cam.Pitch)
	th := math.Tan(cam.FOV / 2)
	d0 := H / (z0 * th)
	tgt := s.targetOf(fr, eff, H)
	eye := sub(tgt, scale3(fr.f, d0))
	dir := norm3(add(fr.f, add(scale3(fr.r, a*th), scale3(fr.u, b*th))))
	var p [3]float64
	if t, ok := s.rayCast(eye, dir); ok {
		p = add(eye, scale3(dir, t))
	} else {
		p = add(tgt, add(scale3(fr.r, a*th*d0), scale3(fr.u, b*th*d0)))
	}
	rel := sub(p, s.center)
	pr, pu, pf := dot(rel, fr.r), dot(rel, fr.u), dot(rel, fr.f)
	d1 := H / (z1 * th)
	zmin := 0.002 * s.radius
	if pf+d1 < zmin {
		d1 = zmin - pf
		if d1 <= 0 {
			return cam
		}
		z1 = math.Max(minZoom, math.Min(maxZoom, H/(d1*th)))
		if (factor > 1 && z1 < z0) || (factor < 1 && z1 > z0) {
			return cam
		}
		d1 = H / (z1 * th)
	}
	zp := pf + d1
	cam.PanX = (a*th*zp-pr)/H - offX
	cam.PanY = (b*th*zp-pu)/H - offY
	cam.Zoom = z1
	return cam
}
