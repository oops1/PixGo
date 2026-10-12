package imaging

import (
	"image"
	"math"

	"github.com/oops1/PixGo/internal/codec"
)

func Orient(src *image.NRGBA, o codec.Orientation) *image.NRGBA {
	if src == nil || !o.Valid() || o == codec.OrientNormal {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := o.Size(w, h)
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	fw, fh := float64(w), float64(h)
	x0, y0 := o.ToSource(.5, .5, fw, fh)
	xu, yu := o.ToSource(1.5, .5, fw, fh)
	xv, yv := o.ToSource(.5, 1.5, fw, fh)
	sx0, sy0 := int(math.Floor(x0)), int(math.Floor(y0))
	dxu, dyu := int(math.Round(xu-x0)), int(math.Round(yu-y0))
	dxv, dyv := int(math.Round(xv-x0)), int(math.Round(yv-y0))
	const rowsPerTask = 32
	tasks := (dh + rowsPerTask - 1) / rowsPerTask
	parallel(tasks, workerCount(), func(task int) {
		from := task * rowsPerTask
		to := min(dh, from+rowsPerTask)
		for v := from; v < to; v++ {
			sx, sy := sx0+v*dxv, sy0+v*dyv
			out := dst.Pix[v*dst.Stride : v*dst.Stride+dw*4]
			for u := 0; u < dw; u++ {
				i := (sy-b.Min.Y)*src.Stride + (sx-b.Min.X)*4
				copy(out[u*4:u*4+4], src.Pix[i:i+4])
				sx += dxu
				sy += dyu
			}
		}
	})
	return dst
}
