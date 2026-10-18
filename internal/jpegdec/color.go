package jpegdec

import "encoding/binary"

type colorMode uint8

const (
	colorGray colorMode = iota
	colorYCbCr
	colorRGB
	colorCMYK
	colorYCCK
	colorInkCMYK
	colorInkYCCK
)

func ycbcrToRGB(y, cb, cr uint8) (uint8, uint8, uint8) {
	yy := int32(y) * 0x10101
	cbv := int32(cb) - 128
	crv := int32(cr) - 128
	return clampShift16(yy + 91881*crv), clampShift16(yy - 22554*cbv - 46802*crv), clampShift16(yy + 116130*cbv)
}

func clampShift16(v int32) uint8 {
	if uint32(v)&0xff000000 == 0 {
		return uint8(v >> 16)
	}
	return uint8(^(v >> 31))
}

func cmykToRGB(c, m, y, k uint8) (uint8, uint8, uint8) {
	w := 0xffff - uint32(k)*0x101
	return uint8(((0xffff - uint32(c)*0x101) * w / 0xffff) >> 8),
		uint8(((0xffff - uint32(m)*0x101) * w / 0xffff) >> 8),
		uint8(((0xffff - uint32(y)*0x101) * w / 0xffff) >> 8)
}

type rowSet struct {
	rows  [maxComponents][]uint8
	shift [maxComponents]uint8
}

func convertGray(dst []byte, rs *rowSet, first, n int) {
	src := rs.rows[0][first : first+n]
	dst = dst[:4*n]
	for i, v := range src {
		o := i * 4
		dst[o] = v
		dst[o+1] = v
		dst[o+2] = v
		dst[o+3] = 0xff
	}
}

func convertYCbCr(dst []byte, rs *rowSet, first, n int) {
	if rs.shift[0] == 0 && rs.shift[1] == rs.shift[2] {
		convertYCbCrShared(dst, rs.rows[0], rs.rows[1], rs.rows[2], rs.shift[1], first, n)
		return
	}
	for i := 0; i < n; i++ {
		x := first + i
		r, g, b := ycbcrToRGB(rs.rows[0][x>>rs.shift[0]], rs.rows[1][x>>rs.shift[1]], rs.rows[2][x>>rs.shift[2]])
		o := i * 4
		dst[o] = r
		dst[o+1] = g
		dst[o+2] = b
		dst[o+3] = 0xff
	}
}

func packRGB(r, g, b uint8) uint32 {
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16 | 0xff000000
}

func convertYCbCrShared(dst []byte, luma, cbRow, crRow []uint8, shift uint8, first, n int) {
	switch shift {
	case 0:
		convertYCbCr444(dst, luma, cbRow, crRow, first, n)
	case 1:
		convertYCbCr420(dst, luma, cbRow, crRow, first, n)
	default:
		convertYCbCrRuns(dst, luma, cbRow, crRow, shift, first, n)
	}
}

func convertYCbCr444(dst []byte, luma, cbRow, crRow []uint8, first, n int) {
	ys := luma[first : first+n]
	cbs := cbRow[first : first+n]
	crs := crRow[first : first+n]
	dst = dst[:4*n]
	for i, yv := range ys {
		yy := int32(yv) * 0x10101
		cb := int32(cbs[i]) - 128
		cr := int32(crs[i]) - 128
		binary.LittleEndian.PutUint32(dst[i*4:], packRGB(clampShift16(yy+91881*cr), clampShift16(yy-22554*cb-46802*cr), clampShift16(yy+116130*cb)))
	}
}

func convertYCbCr420(dst []byte, luma, cbRow, crRow []uint8, first, n int) {
	i := 0
	x := first
	if x&1 == 1 && n > 0 {
		r, g, b := ycbcrToRGB(luma[x], cbRow[x>>1], crRow[x>>1])
		binary.LittleEndian.PutUint32(dst, packRGB(r, g, b))
		i++
		x++
	}
	for ; i+2 <= n; i += 2 {
		ci := x >> 1
		cb := int32(cbRow[ci]) - 128
		cr := int32(crRow[ci]) - 128
		addR := 91881 * cr
		addG := -22554*cb - 46802*cr
		addB := 116130 * cb
		pair := luma[x : x+2 : x+2]
		out := dst[i*4 : i*4+8 : i*4+8]
		y0 := int32(pair[0]) * 0x10101
		y1 := int32(pair[1]) * 0x10101
		binary.LittleEndian.PutUint32(out[0:], packRGB(clampShift16(y0+addR), clampShift16(y0+addG), clampShift16(y0+addB)))
		binary.LittleEndian.PutUint32(out[4:], packRGB(clampShift16(y1+addR), clampShift16(y1+addG), clampShift16(y1+addB)))
		x += 2
	}
	if i < n {
		r, g, b := ycbcrToRGB(luma[x], cbRow[x>>1], crRow[x>>1])
		binary.LittleEndian.PutUint32(dst[i*4:], packRGB(r, g, b))
	}
}

func convertYCbCrRuns(dst []byte, luma, cbRow, crRow []uint8, shift uint8, first, n int) {
	for i := 0; i < n; {
		x := first + i
		ci := x >> shift
		run := ((ci + 1) << shift) - x
		if run > n-i {
			run = n - i
		}
		cb := int32(cbRow[ci]) - 128
		cr := int32(crRow[ci]) - 128
		addR := 91881 * cr
		addG := -22554*cb - 46802*cr
		addB := 116130 * cb
		ys := luma[x : x+run]
		out := dst[i*4 : (i+run)*4]
		for j, yv := range ys {
			yy := int32(yv) * 0x10101
			binary.LittleEndian.PutUint32(out[j*4:], packRGB(clampShift16(yy+addR), clampShift16(yy+addG), clampShift16(yy+addB)))
		}
		i += run
	}
}

func convertRGB(dst []byte, rs *rowSet, first, n int) {
	for i := 0; i < n; i++ {
		x := first + i
		o := i * 4
		dst[o] = rs.rows[0][x>>rs.shift[0]]
		dst[o+1] = rs.rows[1][x>>rs.shift[1]]
		dst[o+2] = rs.rows[2][x>>rs.shift[2]]
		dst[o+3] = 0xff
	}
}

func convertCMYK(dst []byte, rs *rowSet, first, n int) {
	for i := 0; i < n; i++ {
		x := first + i
		c := 255 - rs.rows[0][x>>rs.shift[0]]
		m := 255 - rs.rows[1][x>>rs.shift[1]]
		y := 255 - rs.rows[2][x>>rs.shift[2]]
		k := 255 - rs.rows[3][x>>rs.shift[3]]
		o := i * 4
		dst[o], dst[o+1], dst[o+2] = cmykToRGB(c, m, y, k)
		dst[o+3] = 0xff
	}
}

func convertYCCK(dst []byte, rs *rowSet, first, n int) {
	for i := 0; i < n; i++ {
		x := first + i
		c, m, y := ycbcrToRGB(rs.rows[0][x>>rs.shift[0]], rs.rows[1][x>>rs.shift[1]], rs.rows[2][x>>rs.shift[2]])
		k := 255 - rs.rows[3][x>>rs.shift[3]]
		o := i * 4
		dst[o], dst[o+1], dst[o+2] = cmykToRGB(c, m, y, k)
		dst[o+3] = 0xff
	}
}

func convertInkCMYK(dst []byte, rs *rowSet, first, n int) {
	for i := 0; i < n; i++ {
		x := first + i
		o := i * 4
		dst[o] = 255 - rs.rows[0][x>>rs.shift[0]]
		dst[o+1] = 255 - rs.rows[1][x>>rs.shift[1]]
		dst[o+2] = 255 - rs.rows[2][x>>rs.shift[2]]
		dst[o+3] = 255 - rs.rows[3][x>>rs.shift[3]]
	}
}

func convertInkYCCK(dst []byte, rs *rowSet, first, n int) {
	for i := 0; i < n; i++ {
		x := first + i
		o := i * 4
		dst[o], dst[o+1], dst[o+2] = ycbcrToRGB(rs.rows[0][x>>rs.shift[0]], rs.rows[1][x>>rs.shift[1]], rs.rows[2][x>>rs.shift[2]])
		dst[o+3] = 255 - rs.rows[3][x>>rs.shift[3]]
	}
}

func inkModeOf(mode colorMode) (colorMode, bool) {
	switch mode {
	case colorCMYK:
		return colorInkCMYK, true
	case colorYCCK:
		return colorInkYCCK, true
	}
	return mode, false
}

func convertRow(mode colorMode, dst []byte, rs *rowSet, first, n int) {
	switch mode {
	case colorInkCMYK:
		convertInkCMYK(dst, rs, first, n)
	case colorInkYCCK:
		convertInkYCCK(dst, rs, first, n)
	case colorGray:
		convertGray(dst, rs, first, n)
	case colorYCbCr:
		convertYCbCr(dst, rs, first, n)
	case colorRGB:
		convertRGB(dst, rs, first, n)
	case colorCMYK:
		convertCMYK(dst, rs, first, n)
	case colorYCCK:
		convertYCCK(dst, rs, first, n)
	}
}
