package jpegdec

type block [blockSize]int32

const blockSize = 8 * 8

const (
	cos1          = 1130768441178740757
	sin1          = 224923827593068887
	cos3          = 958619196450722178
	sin3          = 640528868967736374
	sqrt2inv      = 815238614083298888
	sqrt2inv_cos6 = 311978311033955632
	sqrt2inv_sin6 = 753182269664427492
)

func c(x uint64, bits int) int32 {
	return int32((x + (1 << (59 - bits))) >> (60 - bits))
}

func dctBox(x0, x1, kcos, ksin int32) (y0, y1 int32) {
	ksum := kcos * (x0 + x1)
	y0 = ksum + (ksin-kcos)*x1
	y1 = ksum - (kcos+ksin)*x0
	return y0, y1
}

func idctRowsFrom(b *block, coef []int16, stride int, q *[blockSize]int32) {
	for i := range 8 {
		row := coef[i*stride : i*stride+8 : i*stride+8]
		x := b[8*i : 8*i+8 : 8*i+8]
		if row[1]|row[2]|row[3]|row[4]|row[5]|row[6]|row[7] == 0 {
			v := (int32(row[0]) * q[8*i]) << 17
			x[0], x[1], x[2], x[3], x[4], x[5], x[6], x[7] = v, v, v, v, v, v, v, v
			continue
		}
		qr := q[8*i : 8*i+8 : 8*i+8]
		x0 := int32(row[0]) * qr[0]
		x7 := int32(row[1]) * qr[1]
		x2 := int32(row[2]) * qr[2]
		x5 := int32(row[3]) * qr[3]
		x1 := int32(row[4]) * qr[4]
		x6 := int32(row[5]) * qr[5]
		x3 := int32(row[6]) * qr[6]
		x4 := int32(row[7]) * qr[7]

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

func idctColsTo(b *block, dst []uint8, stride int) {
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

		dst[0*stride+i] = clampSample((x0 >> 18) + 128)
		dst[1*stride+i] = clampSample((x1 >> 18) + 128)
		dst[2*stride+i] = clampSample((x2 >> 18) + 128)
		dst[3*stride+i] = clampSample((x3 >> 18) + 128)
		dst[4*stride+i] = clampSample((x4 >> 18) + 128)
		dst[5*stride+i] = clampSample((x5 >> 18) + 128)
		dst[6*stride+i] = clampSample((x6 >> 18) + 128)
		dst[7*stride+i] = clampSample((x7 >> 18) + 128)
	}
}
