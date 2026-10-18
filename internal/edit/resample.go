package edit

import (
	"context"
	"image"
	"math"
	"sync"

	"github.com/oops1/PixGo/internal/par"
)

const (
	weightBits   = 14
	weightOne    = 1 << weightBits
	weightHalf   = weightOne / 2
	lanczosLobes = 3
	boxFrom      = 16
	boxDivisor   = 8
	premultMax   = 255 * 255
	maxPixels    = 1 << 29
	maxBandRows  = 64
	bandsPerCore = 4
)

type axis struct {
	box   int
	count int
	first []int
	end   []int
	wts   []int32
}

func lanczos3(x float64) float64 {
	if x < 0 {
		x = -x
	}
	if x >= lanczosLobes {
		return 0
	}
	if x < 1e-9 {
		return 1
	}
	px := float64(math.Pi * x)
	num := float64(lanczosLobes * math.Sin(px))
	num = float64(num * math.Sin(float64(px/lanczosLobes)))
	return float64(num / float64(px*px))
}

func buildAxis(srcN, dstN int) *axis {
	s := float64(srcN) / float64(dstN)
	scale := math.Max(1, s)
	box := 1
	if s >= boxFrom {
		box = int(s / boxDivisor)
	}
	count := (srcN + box - 1) / box
	a := &axis{
		box:   box,
		count: count,
		first: make([]int, dstN),
		end:   make([]int, dstN+1),
	}
	support := lanczosLobes * scale
	var raw []float64
	for d := 0; d < dstN; d++ {
		c := float64(float64(d)+0.5) * s
		lo := int(math.Floor(float64(c-support) / float64(box)))
		hi := int(math.Ceil(float64(c+support) / float64(box)))
		lo = max(lo, 0)
		hi = min(hi, count-1)
		if hi < lo {
			lo = min(max(int(c/float64(box)), 0), count-1)
			hi = lo
		}
		raw = raw[:0]
		sum := 0.0
		for j := lo; j <= hi; j++ {
			from := j * box
			to := min(srcN, from+box)
			pos := float64(from+to) / 2
			k := lanczos3(float64(pos-c) / scale)
			raw = append(raw, k)
			sum += k
		}
		a.first[d] = lo
		base := len(a.wts)
		if sum <= 0 {
			nearest := 0
			for i := range raw {
				raw[i] = 0
			}
			best := math.Inf(1)
			for j := lo; j <= hi; j++ {
				from := j * box
				to := min(srcN, from+box)
				if dist := math.Abs(float64(from+to)/2 - c); dist < best {
					best, nearest = dist, j-lo
				}
			}
			for i := range raw {
				w := int32(0)
				if i == nearest {
					w = weightOne
				}
				a.wts = append(a.wts, w)
			}
		} else {
			total := int32(0)
			big := 0
			for i, k := range raw {
				w := int32(math.Round(float64(k / sum * weightOne)))
				a.wts = append(a.wts, w)
				total += w
				if abs32(w) > abs32(a.wts[base+big]) {
					big = i
				}
			}
			a.wts[base+big] += weightOne - total
		}
		a.end[d+1] = len(a.wts)
	}
	return a
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func opaque(src *image.NRGBA) bool {
	b := src.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		o := src.PixOffset(b.Min.X, y)
		row := src.Pix[o : o+b.Dx()*4]
		for i := 3; i < len(row); i += 4 {
			if row[i] != 255 {
				return false
			}
		}
	}
	return true
}

func clamp16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > premultMax {
		return premultMax
	}
	return uint16(v)
}

func clamp8(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

type job struct {
	src    *image.NRGBA
	dst    *image.NRGBA
	ax, ay *axis
	sw, sh int
	w, h   int
	ch     int
}

type scratch struct {
	pm  []uint16
	row []uint16
	red []uint16
	sum []int32
	acc []int
}

var scratchPool = sync.Pool{New: func() any { return new(scratch) }}

func grow[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n)
	}
	return s[:n]
}

func (j *job) premultRow(y int, pm []uint16) {
	b := j.src.Bounds()
	o := j.src.PixOffset(b.Min.X, b.Min.Y+y)
	px := j.src.Pix[o : o+j.sw*4]
	box := j.ax.box
	if j.ch == 3 {
		if box == 1 {
			for x := 0; x < j.sw; x++ {
				pm[x*3] = uint16(px[x*4]) * 255
				pm[x*3+1] = uint16(px[x*4+1]) * 255
				pm[x*3+2] = uint16(px[x*4+2]) * 255
			}
			return
		}
		for g := 0; g < j.ax.count; g++ {
			from := g * box
			to := min(j.sw, from+box)
			var r, gr, bl int
			for x := from; x < to; x++ {
				r += int(px[x*4])
				gr += int(px[x*4+1])
				bl += int(px[x*4+2])
			}
			n := to - from
			pm[g*3] = uint16((r*255 + n/2) / n)
			pm[g*3+1] = uint16((gr*255 + n/2) / n)
			pm[g*3+2] = uint16((bl*255 + n/2) / n)
		}
		return
	}
	if box == 1 {
		for x := 0; x < j.sw; x++ {
			a := uint16(px[x*4+3])
			pm[x*4] = uint16(px[x*4]) * a
			pm[x*4+1] = uint16(px[x*4+1]) * a
			pm[x*4+2] = uint16(px[x*4+2]) * a
			pm[x*4+3] = a * 255
		}
		return
	}
	for g := 0; g < j.ax.count; g++ {
		from := g * box
		to := min(j.sw, from+box)
		var r, gr, bl, al int
		for x := from; x < to; x++ {
			a := int(px[x*4+3])
			r += int(px[x*4]) * a
			gr += int(px[x*4+1]) * a
			bl += int(px[x*4+2]) * a
			al += a
		}
		n := to - from
		pm[g*4] = uint16((r + n/2) / n)
		pm[g*4+1] = uint16((gr + n/2) / n)
		pm[g*4+2] = uint16((bl + n/2) / n)
		pm[g*4+3] = uint16((al*255 + n/2) / n)
	}
}

func (j *job) horizontal(y int, s *scratch, out []uint16) {
	j.premultRow(y, s.pm)
	ax := j.ax
	pm := s.pm
	if j.ch == 3 {
		for x := 0; x < j.w; x++ {
			first := ax.first[x]
			ws := ax.wts[ax.end[x]:ax.end[x+1]]
			var r, g, b int
			p := pm[first*3:]
			for k, wt := range ws {
				v := int(wt)
				r += int(p[k*3]) * v
				g += int(p[k*3+1]) * v
				b += int(p[k*3+2]) * v
			}
			out[x*3] = clamp16((r + weightHalf) >> weightBits)
			out[x*3+1] = clamp16((g + weightHalf) >> weightBits)
			out[x*3+2] = clamp16((b + weightHalf) >> weightBits)
		}
		return
	}
	for x := 0; x < j.w; x++ {
		first := ax.first[x]
		ws := ax.wts[ax.end[x]:ax.end[x+1]]
		var r, g, b, a int
		p := pm[first*4:]
		for k, wt := range ws {
			v := int(wt)
			r += int(p[k*4]) * v
			g += int(p[k*4+1]) * v
			b += int(p[k*4+2]) * v
			a += int(p[k*4+3]) * v
		}
		out[x*4] = clamp16((r + weightHalf) >> weightBits)
		out[x*4+1] = clamp16((g + weightHalf) >> weightBits)
		out[x*4+2] = clamp16((b + weightHalf) >> weightBits)
		out[x*4+3] = clamp16((a + weightHalf) >> weightBits)
	}
}

func (j *job) reducedRow(r int, s *scratch, out []uint16) {
	box := j.ay.box
	from := r * box
	to := min(j.sh, from+box)
	if to-from == 1 {
		j.horizontal(from, s, out)
		return
	}
	for i := range s.sum {
		s.sum[i] = 0
	}
	for y := from; y < to; y++ {
		j.horizontal(y, s, s.row)
		for i, v := range s.row {
			s.sum[i] += int32(v)
		}
	}
	n := int32(to - from)
	for i, v := range s.sum {
		out[i] = uint16((v + n/2) / n)
	}
}

func (j *job) band(ctx context.Context, y0, y1 int) {
	if ctx.Err() != nil {
		return
	}
	rowLen := j.w * j.ch
	ay := j.ay
	r0 := ay.first[y0]
	r1 := r0
	for y := y0; y < y1; y++ {
		r1 = max(r1, ay.first[y]+ay.end[y+1]-ay.end[y])
	}
	s := scratchPool.Get().(*scratch)
	defer scratchPool.Put(s)
	s.pm = grow(s.pm, j.ax.count*j.ch)
	s.row = grow(s.row, rowLen)
	s.sum = grow(s.sum, rowLen)
	s.acc = grow(s.acc, rowLen)
	s.red = grow(s.red, (r1-r0)*rowLen)
	red := s.red
	for r := r0; r < r1; r++ {
		if ctx.Err() != nil {
			return
		}
		j.reducedRow(r, s, red[(r-r0)*rowLen:(r-r0+1)*rowLen])
	}
	for y := y0; y < y1; y++ {
		first := ay.first[y]
		ws := ay.wts[ay.end[y]:ay.end[y+1]]
		acc := s.acc
		for i := range acc {
			acc[i] = 0
		}
		for k, wt := range ws {
			row := red[(first+k-r0)*rowLen : (first+k-r0+1)*rowLen]
			v := int(wt)
			for i, p := range row {
				acc[i] += int(p) * v
			}
		}
		out := j.dst.Pix[y*j.dst.Stride : y*j.dst.Stride+j.w*4]
		if j.ch == 3 {
			for x := 0; x < j.w; x++ {
				out[x*4] = clamp8((((acc[x*3] + weightHalf) >> weightBits) + 127) / 255)
				out[x*4+1] = clamp8((((acc[x*3+1] + weightHalf) >> weightBits) + 127) / 255)
				out[x*4+2] = clamp8((((acc[x*3+2] + weightHalf) >> weightBits) + 127) / 255)
				out[x*4+3] = 255
			}
			continue
		}
		for x := 0; x < j.w; x++ {
			pr := int(clamp16((acc[x*4] + weightHalf) >> weightBits))
			pg := int(clamp16((acc[x*4+1] + weightHalf) >> weightBits))
			pb := int(clamp16((acc[x*4+2] + weightHalf) >> weightBits))
			a := int(clamp16((acc[x*4+3] + weightHalf) >> weightBits))
			if a == 0 {
				out[x*4], out[x*4+1], out[x*4+2], out[x*4+3] = 0, 0, 0, 0
				continue
			}
			half := a / 2
			out[x*4] = clamp8((pr*255 + half) / a)
			out[x*4+1] = clamp8((pg*255 + half) / a)
			out[x*4+2] = clamp8((pb*255 + half) / a)
			out[x*4+3] = uint8((a + 127) / 255)
		}
	}
}

func Resample(ctx context.Context, src *image.NRGBA, w, h int) (*image.NRGBA, error) {
	if src == nil || src.Bounds().Empty() {
		return nil, ErrEmptyPicture
	}
	if w < 1 || h < 1 || int64(w)*int64(h) > maxPixels {
		return nil, ErrSize
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b := src.Bounds()
	if b.Dx() == w && b.Dy() == h {
		return clone(src), nil
	}
	j := &job{
		src: src, dst: image.NewNRGBA(image.Rect(0, 0, w, h)),
		sw: b.Dx(), sh: b.Dy(), w: w, h: h, ch: 4,
		ax: buildAxis(b.Dx(), w), ay: buildAxis(b.Dy(), h),
	}
	if opaque(src) {
		j.ch = 3
	}
	workers := par.Count()
	rows := min(maxBandRows, max(1, (h+workers*bandsPerCore-1)/(workers*bandsPerCore)))
	tasks := (h + rows - 1) / rows
	par.Parallel(tasks, workers, func(t int) {
		y0 := t * rows
		j.band(ctx, y0, min(h, y0+rows))
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return j.dst, nil
}
