package model3d

import (
	"context"
	"errors"
	"hash/fnv"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/mesh"
	"github.com/oops1/PixGo/internal/render3d"
	"github.com/oops1/PixGo/internal/step"
	"github.com/oops1/PixGo/internal/stl"
)

type Quality uint8

const (
	Fine Quality = iota
	Coarse
)

const (
	fingerprintSpan = 64 << 10
	cacheEntries    = 2
	cacheBudget     = 1 << 30
	coarseDeflect   = 0.004
)

var ErrNotModel = errors.New("model3d: not a 3D model")

func IsModel(id codec.ID) bool { return id == codec.STL || id == codec.STEP }

func KindOfPath(path string) codec.ID {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".stl":
		return codec.STL
	case ".step", ".stp":
		return codec.STEP
	}
	return ""
}

func Detect(head []byte, size int64, path string) codec.ID {
	switch {
	case step.Sniff(head):
		return codec.STEP
	case stl.Sniff(head):
		return codec.STL
	case stl.IsBinary(head, size):
		return codec.STL
	}
	if KindOfPath(path) == codec.STL {
		return codec.STL
	}
	return ""
}

type key struct {
	id    codec.ID
	size  int64
	print uint64
}

type entry struct {
	key  key
	mesh *mesh.Mesh
}

var cache struct {
	sync.Mutex
	list []entry
}

func Fingerprint(r io.ReaderAt, size int64) (uint64, error) {
	h := fnv.New64a()
	span := int64(fingerprintSpan)
	head := make([]byte, min(span, size))
	if _, err := r.ReadAt(head, 0); err != nil && err != io.EOF {
		return 0, err
	}
	h.Write(head)
	if size > span {
		tail := make([]byte, min(span, size-span))
		if _, err := r.ReadAt(tail, size-int64(len(tail))); err != nil && err != io.EOF {
			return 0, err
		}
		h.Write(tail)
	}
	return h.Sum64(), nil
}

func lookup(k key) *mesh.Mesh {
	cache.Lock()
	defer cache.Unlock()
	for i, e := range cache.list {
		if e.key == k {
			if i > 0 {
				copy(cache.list[1:i+1], cache.list[:i])
				cache.list[0] = e
			}
			return e.mesh
		}
	}
	return nil
}

func remember(k key, m *mesh.Mesh) {
	if m.Bytes() > cacheBudget/2 {
		return
	}
	cache.Lock()
	defer cache.Unlock()
	for _, e := range cache.list {
		if e.key == k {
			return
		}
	}
	cache.list = append([]entry{{key: k, mesh: m}}, cache.list...)
	var total int64
	for i, e := range cache.list {
		total += e.mesh.Bytes()
		if i >= cacheEntries || total > cacheBudget {
			cache.list = cache.list[:i]
			break
		}
	}
}

func Forget() {
	cache.Lock()
	cache.list = nil
	cache.Unlock()
}

func Read(ctx context.Context, id codec.ID, r io.ReaderAt, size int64, q Quality) (*mesh.Mesh, error) {
	if !IsModel(id) {
		return nil, ErrNotModel
	}
	var k key
	if q == Fine {
		fp, err := Fingerprint(r, size)
		if err != nil {
			return nil, err
		}
		k = key{id: id, size: size, print: fp}
		if m := lookup(k); m != nil {
			return m, nil
		}
	}
	var m *mesh.Mesh
	var err error
	switch id {
	case codec.STL:
		m, _, err = stl.Read(ctx, r, size, stl.Options{})
	case codec.STEP:
		o := step.Options{}
		if q == Coarse {
			o.Deflection = coarseDeflect
		}
		m, _, err = step.Read(ctx, r, size, o)
	}
	if err != nil {
		return nil, err
	}
	if m.Empty() {
		return nil, codec.ErrUnsupported
	}
	dropUniformColor(m)
	if q == Fine {
		remember(k, m)
	}
	return m, nil
}

func Load(ctx context.Context, path string) (*mesh.Mesh, codec.ID, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	head := make([]byte, codec.SniffLen)
	n, _ := f.ReadAt(head, 0)
	id := Detect(head[:n], st.Size(), path)
	if id == "" {
		return nil, "", ErrNotModel
	}
	m, err := Read(ctx, id, f, st.Size(), Fine)
	return m, id, err
}

const (
	PictureLongSide = 2048
	minPictureSide  = 16
)

func boxMesh(b [6]float32) *mesh.Mesh {
	x0, y0, z0, x1, y1, z1 := b[0], b[1], b[2], b[3], b[4], b[5]
	c := [8][3]float32{{x0, y0, z0}, {x1, y0, z0}, {x1, y1, z0}, {x0, y1, z0}, {x0, y0, z1}, {x1, y0, z1}, {x1, y1, z1}, {x0, y1, z1}}
	faces := [12][3]int{{0, 2, 1}, {0, 3, 2}, {4, 5, 6}, {4, 6, 7}, {0, 1, 5}, {0, 5, 4}, {1, 2, 6}, {1, 6, 5}, {2, 3, 7}, {2, 7, 6}, {3, 0, 4}, {3, 4, 7}}
	m := &mesh.Mesh{V: make([]float32, 0, 12*9)}
	for _, f := range faces {
		for _, i := range f {
			m.V = append(m.V, c[i][0], c[i][1], c[i][2])
		}
	}
	m.UpdateBounds()
	return m
}

func BoundsAspect(b [6]float32) float64 {
	a := render3d.NewScene(boxMesh(b)).Aspect(render3d.DefaultCamera())
	if !(a > 0) || math.IsInf(a, 0) {
		return 1
	}
	return a
}

func PictureSize(aspect float64, o codec.Options, lim codec.Limits) (int, int) {
	if !(aspect > 0) || math.IsInf(aspect, 0) {
		aspect = 1
	}
	aspect = math.Min(16, math.Max(1.0/16, aspect))
	boxW, boxH := float64(PictureLongSide), float64(PictureLongSide)
	if o.VectorW > 0 && o.VectorH > 0 {
		boxW, boxH = float64(o.VectorW), float64(o.VectorH)
	}
	w, h := boxW, boxW/aspect
	if h > boxH {
		w, h = boxH*aspect, boxH
	}
	iw, ih := max(minPictureSide, int(math.Round(w))), max(minPictureSide, int(math.Round(h)))
	lim = lim.WithDefaults()
	for int64(iw)*int64(ih) > lim.MaxPixels && iw > minPictureSide && ih > minPictureSide {
		iw, ih = iw*3/4, ih*3/4
	}
	return iw, ih
}

func Picture(ctx context.Context, m *mesh.Mesh, o codec.Options) (*image.NRGBA, error) {
	sc := render3d.NewScene(m)
	cam := render3d.DefaultCamera()
	w, h := PictureSize(sc.Aspect(cam), o, o.Limits)
	st := render3d.DefaultStyle()
	st.Background = color.NRGBA{}
	return sc.Render(ctx, cam, w, h, st)
}

func dropUniformColor(m *mesh.Mesh) {
	if !m.HasColors() {
		return
	}
	r, g, b := m.C[0], m.C[1], m.C[2]
	for i := 3; i+2 < len(m.C); i += 3 {
		if m.C[i] != r || m.C[i+1] != g || m.C[i+2] != b {
			return
		}
	}
	m.C = nil
}
