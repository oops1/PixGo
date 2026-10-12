package render

import "image"

func Shift(dst *image.RGBA, dx, dy int) {
	if dst == nil || (dx == 0 && dy == 0) {
		return
	}
	w, h := dst.Rect.Dx(), dst.Rect.Dy()
	if w <= 0 || h <= 0 || absInt(dx) >= w || absInt(dy) >= h {
		return
	}
	n := (w - absInt(dx)) * 4
	from, to := 0, 0
	if dx > 0 {
		to = dx * 4
	} else {
		from = -dx * 4
	}
	rows := h - absInt(dy)
	for k := 0; k < rows; k++ {
		var sy int
		if dy > 0 {
			sy = rows - 1 - k
		} else {
			sy = k - dy
		}
		ty := sy + dy
		so := sy*dst.Stride + from
		to2 := ty*dst.Stride + to
		copy(dst.Pix[to2:to2+n], dst.Pix[so:so+n])
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
