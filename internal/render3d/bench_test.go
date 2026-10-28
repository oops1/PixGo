package render3d

import (
	"context"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/mesh"
)

func sphereMesh(rings, segs int, r float32, normals bool) *mesh.Mesh {
	m := &mesh.Mesh{V: make([]float32, 0, rings*segs*18)}
	if normals {
		m.N = make([]float32, 0, rings*segs*18)
	}
	pt := func(i, j int) [3]float32 {
		th := math.Pi * float64(i) / float64(rings)
		ph := 2 * math.Pi * float64(j) / float64(segs)
		return [3]float32{
			float32(math.Sin(th) * math.Cos(ph)),
			float32(math.Sin(th) * math.Sin(ph)),
			float32(math.Cos(th)),
		}
	}
	put := func(ps ...[3]float32) {
		for _, p := range ps {
			m.V = append(m.V, p[0]*r, p[1]*r, p[2]*r)
			if normals {
				m.N = append(m.N, p[0], p[1], p[2])
			}
		}
	}
	for i := 0; i < rings; i++ {
		for j := 0; j < segs; j++ {
			a, b, c, d := pt(i, j), pt(i+1, j), pt(i+1, j+1), pt(i, j+1)
			put(a, b, c)
			put(a, c, d)
		}
	}
	m.UpdateBounds()
	return m
}

func benchRender(b *testing.B, m *mesh.Mesh, w, h, samples int, edges bool) {
	s := NewScene(m)
	st := DefaultStyle()
	st.Background = color.NRGBA{32, 32, 32, 255}
	st.Samples = samples
	st.Edges = edges
	cam := s.LockFit(DefaultCamera(), w, h)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	ctx := context.Background()
	if err := s.RenderInto(ctx, cam, st, dst); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.RenderInto(ctx, cam, st, dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSphere1M_1080p_S1(b *testing.B) {
	benchRender(b, sphereMesh(500, 1000, 50, false), 1920, 1080, 1, true)
}

func BenchmarkSphere1M_1080p_S2(b *testing.B) {
	benchRender(b, sphereMesh(500, 1000, 50, false), 1920, 1080, 2, true)
}

func BenchmarkSphere1M_1080p_S2_Smooth(b *testing.B) {
	benchRender(b, sphereMesh(500, 1000, 50, true), 1920, 1080, 2, true)
}

func BenchmarkSphere1M_1080p_S1_NoEdges(b *testing.B) {
	benchRender(b, sphereMesh(500, 1000, 50, false), 1920, 1080, 1, false)
}

func BenchmarkSmall100_256(b *testing.B) {
	benchRender(b, sphereMesh(5, 10, 50, false), 256, 256, 2, true)
}

func BenchmarkSmall100_1080p_S2(b *testing.B) {
	benchRender(b, sphereMesh(5, 10, 50, false), 1920, 1080, 2, true)
}

func plateMesh(nx, ny int, sx, sy float32) *mesh.Mesh {
	m := &mesh.Mesh{V: make([]float32, 0, nx*ny*18)}
	z := func(i, j int) float32 {
		x, y := float64(i)/float64(nx), float64(j)/float64(ny)
		return float32(3*math.Sin(x*17)*math.Cos(y*11) + 2*math.Sin(x*y*40))
	}
	pt := func(i, j int) [3]float32 {
		return [3]float32{sx * float32(i) / float32(nx), sy * float32(j) / float32(ny), z(i, j)}
	}
	for j := 0; j < ny; j++ {
		for i := 0; i < nx; i++ {
			a, b, c, d := pt(i, j), pt(i+1, j), pt(i+1, j+1), pt(i, j+1)
			for _, p := range [][3]float32{a, b, c, a, c, d} {
				m.V = append(m.V, p[0], p[1], p[2])
			}
		}
	}
	m.UpdateBounds()
	return m
}

func BenchmarkPlate1M_1080p_S1(b *testing.B) {
	benchRender(b, plateMesh(1000, 500, 160, 90), 1920, 1080, 1, true)
}

func BenchmarkPlate1M_1080p_S2(b *testing.B) {
	benchRender(b, plateMesh(1000, 500, 160, 90), 1920, 1080, 2, true)
}

func BenchmarkPlate1M_1080p_S3(b *testing.B) {
	benchRender(b, plateMesh(1000, 500, 160, 90), 1920, 1080, 3, true)
}
