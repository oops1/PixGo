package jpegdec

func referenceIDCT(b *block) {
	referenceRows(b)
	referenceCols(b)
}

func referenceRows(b *block) {
	for i := range 8 {
		x := b[8*i : 8*i+8 : 8*i+8]
		x0 := x[0]
		x7 := x[1]
		x2 := x[2]
		x5 := x[3]
		x1 := x[4]
		x6 := x[5]
		x3 := x[6]
		x4 := x[7]

		x0 <<= 17
		x1 <<= 17
		x0, x1 = x0+x1, x0-x1

		x2, x3 = dctBox(x2, x3, c(sqrt2inv_cos6, 18), -c(sqrt2inv_sin6, 18))
		x1, x2 = x1+x2, x1-x2
		x0, x3 = x0+x3, x0-x3

		x4 <<= 7
		x7 <<= 7
		x7, x4 = x7+x4, x7-x4

		x6 = x6 * c(sqrt2inv, 8)
		x5 = x5 * c(sqrt2inv, 8)

		x7, x5 = x7+x5, x7-x5
		x4, x6 = x4+x6, x4-x6

		x4, x7 = dctBox(x4>>2, x7>>2, c(cos3, 12), -c(sin3, 12))
		x5, x6 = dctBox(x5>>2, x6>>2, c(cos1, 12), -c(sin1, 12))

		x0, x7 = x0+x7, x0-x7
		x1, x6 = x1+x6, x1-x6
		x2, x5 = x2+x5, x2-x5
		x3, x4 = x3+x4, x3-x4

		x[0] = x0
		x[1] = x1
		x[2] = x2
		x[3] = x3
		x[4] = x4
		x[5] = x5
		x[6] = x6
		x[7] = x7
	}
}

func referenceCols(b *block) {
	for i := range 8 {
		x0 := b[0*8+i]
		x7 := b[1*8+i]
		x2 := b[2*8+i]
		x5 := b[3*8+i]
		x1 := b[4*8+i]
		x6 := b[5*8+i]
		x3 := b[6*8+i]
		x4 := b[7*8+i]

		x0 += 1 << 19

		x0, x1 = (x0+x1)>>2, (x0-x1)>>2
		x2, x3 = dctBox(x2>>13, x3>>13, c(sqrt2inv_cos6, 12), -c(sqrt2inv_sin6, 12))

		x1, x2 = x1+x2, x1-x2
		x0, x3 = x0+x3, x0-x3

		x7, x4 = x7+x4, x7-x4

		x5 = (x5 >> 13) * c(sqrt2inv, 14)
		x6 = (x6 >> 13) * c(sqrt2inv, 14)

		x7, x5 = x7+x5, x7-x5
		x4, x6 = x4+x6, x4-x6

		x4, x7 = dctBox(x4>>14, x7>>14, c(cos3, 12), -c(sin3, 12))
		x5, x6 = dctBox(x5>>14, x6>>14, c(cos1, 12), -c(sin1, 12))

		x0, x7 = x0+x7, x0-x7
		x1, x6 = x1+x6, x1-x6
		x2, x5 = x2+x5, x2-x5
		x3, x4 = x3+x4, x3-x4

		x0 >>= 18
		x1 >>= 18
		x2 >>= 18
		x3 >>= 18
		x4 >>= 18
		x5 >>= 18
		x6 >>= 18
		x7 >>= 18

		b[0*8+i] = x0
		b[1*8+i] = x1
		b[2*8+i] = x2
		b[3*8+i] = x3
		b[4*8+i] = x4
		b[5*8+i] = x5
		b[6*8+i] = x6
		b[7*8+i] = x7
	}
}
