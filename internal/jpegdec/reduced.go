package jpegdec

import "math"

const (
	dcFastLimit = 1 << 14
	cosine1Q12  = 2676
	cosine3Q12  = 1108
)

func clampSample(v int32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func onlyDC(coef []int16, stride, side int) bool {
	for y := 0; y < side; y++ {
		row := coef[y*stride : y*stride+side]
		start := 0
		if y == 0 {
			start = 1
		}
		for _, v := range row[start:] {
			if v != 0 {
				return false
			}
		}
	}
	return true
}

func fillSquare(dst []uint8, stride, side int, v uint8) {
	for y := 0; y < side; y++ {
		row := dst[y*stride : y*stride+side]
		for x := range row {
			row[x] = v
		}
	}
}

func (sc *scratch) block8(coef []int16, stride int, q *[blockSize]int32, dst []uint8, outStride int) {
	dc := int32(coef[0]) * q[0]
	if dc >= -dcFastLimit && dc < dcFastLimit && onlyDC(coef, stride, 8) {
		fillSquare(dst, outStride, 8, clampSample(((dc+4)>>3)+128))
		return
	}
	b := &sc.blk
	idctRowsFrom(b, coef, stride, q)
	idctColsTo(b, dst, outStride)
}

func idct4(x0, x1, x2, x3 int32) (y0, y1, y2, y3 int32) {
	e0 := (x0 + x2) << 11
	e1 := (x0 - x2) << 11
	o0 := cosine1Q12*x1 + cosine3Q12*x3
	o1 := cosine3Q12*x1 - cosine1Q12*x3
	return e0 + o0, e1 + o1, e1 - o1, e0 - o0
}

func (sc *scratch) block4(coef []int16, stride int, q *[blockSize]int32, dst []uint8, outStride int) {
	dc := int32(coef[0]) * q[0]
	if dc >= -dcFastLimit && dc < dcFastLimit && onlyDC(coef, stride, 4) {
		fillSquare(dst, outStride, 4, clampSample(((dc+4)>>3)+128))
		return
	}
	var mid [4][4]int32
	for y := 0; y < 4; y++ {
		row := coef[y*stride : y*stride+4 : y*stride+4]
		qrow := q[y*8 : y*8+4 : y*8+4]
		a, b, c, d := idct4(int32(row[0])*qrow[0], int32(row[1])*qrow[1], int32(row[2])*qrow[2], int32(row[3])*qrow[3])
		mid[y][0] = (a + 32) >> 6
		mid[y][1] = (b + 32) >> 6
		mid[y][2] = (c + 32) >> 6
		mid[y][3] = (d + 32) >> 6
	}
	for x := 0; x < 4; x++ {
		a, b, c, d := idct4(mid[0][x], mid[1][x], mid[2][x], mid[3][x])
		dst[x] = clampSample(((a + 1<<18) >> 19) + 128)
		dst[outStride+x] = clampSample(((b + 1<<18) >> 19) + 128)
		dst[2*outStride+x] = clampSample(((c + 1<<18) >> 19) + 128)
		dst[3*outStride+x] = clampSample(((d + 1<<18) >> 19) + 128)
	}
}

func (sc *scratch) block2(coef []int16, stride int, q *[blockSize]int32, dst []uint8, outStride int) {
	x00 := int32(coef[0]) * q[0]
	x01 := int32(coef[1]) * q[1]
	x10 := int32(coef[stride]) * q[8]
	x11 := int32(coef[stride+1]) * q[9]
	s0, s1 := x00+x01, x00-x01
	t0, t1 := x10+x11, x10-x11
	dst[0] = clampSample(((s0 + t0 + 4) >> 3) + 128)
	dst[1] = clampSample(((s1 + t1 + 4) >> 3) + 128)
	dst[outStride] = clampSample(((s0 - t0 + 4) >> 3) + 128)
	dst[outStride+1] = clampSample(((s1 - t1 + 4) >> 3) + 128)
}

func (sc *scratch) block1(coef []int16, stride int, q *[blockSize]int32, dst []uint8, outStride int) {
	dst[0] = clampSample(((int32(coef[0])*q[0] + 4) >> 3) + 128)
}

type basisTable [9][8][8]int32

var reducedBasis = func() (table basisTable) {
	for _, n := range []int{1, 2, 4, 8} {
		for x := 0; x < n; x++ {
			for u := 0; u < n; u++ {
				alpha := math.Sqrt(2 / float64(n))
				if u == 0 {
					alpha = math.Sqrt(1 / float64(n))
				}
				value := alpha * math.Sqrt(float64(n)/8) * math.Cos(float64(2*x+1)*float64(u)*math.Pi/float64(2*n))
				table[n][x][u] = int32(math.Round(value * 8192))
			}
		}
	}
	return table
}()

func rectangularBlock(kx, ky int) blockFunc {
	bx := &reducedBasis[kx]
	by := &reducedBasis[ky]
	return func(sc *scratch, coef []int16, stride int, q *[blockSize]int32, dst []uint8, outStride int) {
		var mid [8][8]int32
		for v := 0; v < ky; v++ {
			for x := 0; x < kx; x++ {
				var sum int32
				for u := 0; u < kx; u++ {
					sum += bx[x][u] * (int32(coef[v*stride+u]) * q[v*8+u])
				}
				mid[v][x] = (sum + 64) >> 7
			}
		}
		for y := 0; y < ky; y++ {
			for x := 0; x < kx; x++ {
				var sum int64
				for v := 0; v < ky; v++ {
					sum += int64(by[y][v]) * int64(mid[v][x])
				}
				dst[y*outStride+x] = clampSample(int32((sum+1<<18)>>19) + 128)
			}
		}
	}
}
