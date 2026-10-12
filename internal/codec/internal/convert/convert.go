package convert

import (
	"image"
	"image/color"
	"runtime"
	"sync"
	"sync/atomic"
)

const (
	parallelThresholdPixels = 1 << 20
	bandsPerWorker          = 4
	minBandRows             = 8
)

func Bands(width, height int, work func(y0, y1 int)) {
	if height <= 0 || width <= 0 {
		return
	}
	workers := runtime.NumCPU()
	if int64(width)*int64(height) < parallelThresholdPixels || workers < 2 || height < 2*minBandRows {
		work(0, height)
		return
	}
	bandRows := height / (workers * bandsPerWorker)
	if bandRows < minBandRows {
		bandRows = minBandRows
	}
	bandCount := (height + bandRows - 1) / bandRows
	if workers > bandCount {
		workers = bandCount
	}
	var (
		next      atomic.Int64
		wg        sync.WaitGroup
		panicOnce sync.Once
		panicked  any
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if v := recover(); v != nil {
					panicOnce.Do(func() { panicked = v })
				}
			}()
			for {
				band := int(next.Add(1)) - 1
				if band >= bandCount {
					return
				}
				y0 := band * bandRows
				y1 := y0 + bandRows
				if y1 > height {
					y1 = height
				}
				work(y0, y1)
			}
		}()
	}
	wg.Wait()
	if panicked != nil {
		panic(panicked)
	}
}

func NRGBA(src image.Image) *image.NRGBA {
	if src == nil {
		return image.NewNRGBA(image.Rectangle{})
	}
	if s, ok := src.(*image.NRGBA); ok && s.Rect.Min == (image.Point{}) {
		return s
	}
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	if width == 0 || height == 0 {
		return dst
	}
	var rows func(y0, y1 int)
	switch s := src.(type) {
	case *image.YCbCr:
		if s.Rect.Min.X >= 0 && s.Rect.Min.Y >= 0 && ycbcrBacked(s) {
			rows = func(y0, y1 int) { ycbcrRows(dst, s, y0, y1) }
		}
	case *image.Gray:
		rows = func(y0, y1 int) { grayRows(dst, s, y0, y1) }
	case *image.Gray16:
		rows = func(y0, y1 int) { gray16Rows(dst, s, y0, y1) }
	case *image.CMYK:
		rows = func(y0, y1 int) { cmykRows(dst, s, y0, y1) }
	case *image.Paletted:
		table := paletteTable(s.Palette)
		rows = func(y0, y1 int) { palettedRows(dst, s, table, y0, y1) }
	case *image.RGBA:
		unpremultiplyTableOnce.Do(buildUnpremultiplyTable)
		rows = func(y0, y1 int) { rgbaRows(dst, s, y0, y1) }
	case *image.NRGBA:
		rows = func(y0, y1 int) { nrgbaRows(dst, s, y0, y1) }
	case *image.NRGBA64:
		rows = func(y0, y1 int) { nrgba64Rows(dst, s, y0, y1) }
	case *image.RGBA64:
		rows = func(y0, y1 int) { rgba64Rows(dst, s, y0, y1) }
	}
	if rows == nil {
		rows = func(y0, y1 int) { genericRows(dst, src, y0, y1) }
	}
	Bands(width, height, rows)
	return dst
}

func ycbcrBacked(src *image.YCbCr) bool {
	width, height := src.Rect.Dx(), src.Rect.Dy()
	if width <= 0 || height <= 0 || src.YStride < width || src.CStride <= 0 {
		return false
	}
	if len(src.Y) < (height-1)*src.YStride+width {
		return false
	}
	var cxShift, cyShift uint
	switch src.SubsampleRatio {
	case image.YCbCrSubsampleRatio422:
		cxShift = 1
	case image.YCbCrSubsampleRatio420:
		cxShift, cyShift = 1, 1
	case image.YCbCrSubsampleRatio440:
		cyShift = 1
	case image.YCbCrSubsampleRatio411:
		cxShift = 2
	case image.YCbCrSubsampleRatio410:
		cxShift, cyShift = 2, 1
	}
	lastX := src.Rect.Max.X - 1
	lastY := src.Rect.Max.Y - 1
	last := ((lastY>>cyShift)-(src.Rect.Min.Y>>cyShift))*src.CStride + (lastX >> cxShift) - (src.Rect.Min.X >> cxShift)
	return last >= 0 && last < len(src.Cb) && last < len(src.Cr)
}

func ycbcrRows(dst *image.NRGBA, src *image.YCbCr, y0, y1 int) {
	bounds := src.Rect
	width := bounds.Dx()
	var cxShift, cyShift uint
	switch src.SubsampleRatio {
	case image.YCbCrSubsampleRatio422:
		cxShift = 1
	case image.YCbCrSubsampleRatio420:
		cxShift, cyShift = 1, 1
	case image.YCbCrSubsampleRatio440:
		cyShift = 1
	case image.YCbCrSubsampleRatio411:
		cxShift = 2
	case image.YCbCrSubsampleRatio410:
		cxShift, cyShift = 2, 1
	}
	minChromaX := bounds.Min.X >> cxShift
	minChromaY := bounds.Min.Y >> cyShift
	for row := y0; row < y1; row++ {
		y := bounds.Min.Y + row
		luma := src.Y[row*src.YStride : row*src.YStride+width]
		chromaRow := ((y >> cyShift) - minChromaY) * src.CStride
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*4]
		lastChroma := -1
		var addR, addG, addB int32
		for i := 0; i < width; i++ {
			chroma := chromaRow + ((bounds.Min.X + i) >> cxShift) - minChromaX
			if chroma != lastChroma {
				lastChroma = chroma
				cb := int32(src.Cb[chroma]) - 128
				cr := int32(src.Cr[chroma]) - 128
				addR = 91881 * cr
				addG = -22554*cb - 46802*cr
				addB = 116130 * cb
			}
			yy := int32(luma[i]) * 0x10101
			r := yy + addR
			if uint32(r)&0xff000000 == 0 {
				r >>= 16
			} else {
				r = ^(r >> 31)
			}
			g := yy + addG
			if uint32(g)&0xff000000 == 0 {
				g >>= 16
			} else {
				g = ^(g >> 31)
			}
			b := yy + addB
			if uint32(b)&0xff000000 == 0 {
				b >>= 16
			} else {
				b = ^(b >> 31)
			}
			o := i * 4
			out[o] = uint8(r)
			out[o+1] = uint8(g)
			out[o+2] = uint8(b)
			out[o+3] = 0xff
		}
	}
}

func grayRows(dst *image.NRGBA, src *image.Gray, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		in := src.Pix[src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row):]
		in = in[:width]
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*4]
		for i, v := range in {
			o := i * 4
			out[o] = v
			out[o+1] = v
			out[o+2] = v
			out[o+3] = 0xff
		}
	}
}

func gray16Rows(dst *image.NRGBA, src *image.Gray16, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		in := src.Pix[src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row):]
		in = in[:width*2]
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*4]
		for i := 0; i < width; i++ {
			v := in[i*2]
			o := i * 4
			out[o] = v
			out[o+1] = v
			out[o+2] = v
			out[o+3] = 0xff
		}
	}
}

func cmykRows(dst *image.NRGBA, src *image.CMYK, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		in := src.Pix[src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row):]
		in = in[:width*4]
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*4]
		for i := 0; i < width; i++ {
			o := i * 4
			w := 0xffff - uint32(in[o+3])*0x101
			out[o] = uint8(((0xffff - uint32(in[o])*0x101) * w / 0xffff) >> 8)
			out[o+1] = uint8(((0xffff - uint32(in[o+1])*0x101) * w / 0xffff) >> 8)
			out[o+2] = uint8(((0xffff - uint32(in[o+2])*0x101) * w / 0xffff) >> 8)
			out[o+3] = 0xff
		}
	}
}

func paletteTable(palette color.Palette) *[256]color.NRGBA {
	var table [256]color.NRGBA
	for i := 0; i < len(palette) && i < 256; i++ {
		if palette[i] == nil {
			continue
		}
		table[i] = color.NRGBAModel.Convert(palette[i]).(color.NRGBA)
	}
	return &table
}

func palettedRows(dst *image.NRGBA, src *image.Paletted, table *[256]color.NRGBA, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		in := src.Pix[src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row):]
		in = in[:width]
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*4]
		for i, index := range in {
			c := table[index]
			o := i * 4
			out[o] = c.R
			out[o+1] = c.G
			out[o+2] = c.B
			out[o+3] = c.A
		}
	}
}

var (
	unpremultiplyTable     [256 * 256]uint8
	unpremultiplyTableOnce sync.Once
)

func buildUnpremultiplyTable() {
	for a := 1; a < 256; a++ {
		for v := 0; v < 256; v++ {
			unpremultiplyTable[a<<8|v] = uint8((uint32(v) * 0xffff / uint32(a)) >> 8)
		}
	}
}

func rgbaRows(dst *image.NRGBA, src *image.RGBA, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		in := src.Pix[src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row):]
		in = in[:width*4]
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*4]
		for o := 0; o < width*4; o += 4 {
			a := in[o+3]
			switch a {
			case 0xff:
				out[o] = in[o]
				out[o+1] = in[o+1]
				out[o+2] = in[o+2]
				out[o+3] = 0xff
			case 0:
				out[o] = 0
				out[o+1] = 0
				out[o+2] = 0
				out[o+3] = 0
			default:
				base := int(a) << 8
				out[o] = unpremultiplyTable[base|int(in[o])]
				out[o+1] = unpremultiplyTable[base|int(in[o+1])]
				out[o+2] = unpremultiplyTable[base|int(in[o+2])]
				out[o+3] = a
			}
		}
	}
}

func nrgbaRows(dst *image.NRGBA, src *image.NRGBA, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		start := src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row)
		copy(dst.Pix[row*dst.Stride:row*dst.Stride+width*4], src.Pix[start:start+width*4])
	}
}

func nrgba64Rows(dst *image.NRGBA, src *image.NRGBA64, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		in := src.Pix[src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row):]
		in = in[:width*8]
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*4]
		for i := 0; i < width; i++ {
			out[i*4] = in[i*8]
			out[i*4+1] = in[i*8+2]
			out[i*4+2] = in[i*8+4]
			out[i*4+3] = in[i*8+6]
		}
	}
}

func rgba64Rows(dst *image.NRGBA, src *image.RGBA64, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		in := src.Pix[src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row):]
		in = in[:width*8]
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*4]
		for i := 0; i < width; i++ {
			p := in[i*8 : i*8+8]
			r := uint32(p[0])<<8 | uint32(p[1])
			g := uint32(p[2])<<8 | uint32(p[3])
			b := uint32(p[4])<<8 | uint32(p[5])
			a := uint32(p[6])<<8 | uint32(p[7])
			o := i * 4
			switch a {
			case 0xffff:
				out[o] = uint8(r >> 8)
				out[o+1] = uint8(g >> 8)
				out[o+2] = uint8(b >> 8)
				out[o+3] = 0xff
			case 0:
				out[o] = 0
				out[o+1] = 0
				out[o+2] = 0
				out[o+3] = 0
			default:
				out[o] = uint8((r * 0xffff / a) >> 8)
				out[o+1] = uint8((g * 0xffff / a) >> 8)
				out[o+2] = uint8((b * 0xffff / a) >> 8)
				out[o+3] = uint8(a >> 8)
			}
		}
	}
}

func genericRows(dst *image.NRGBA, src image.Image, y0, y1 int) {
	bounds := src.Bounds()
	width := bounds.Dx()
	for row := y0; row < y1; row++ {
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*4]
		for i := 0; i < width; i++ {
			c := color.NRGBAModel.Convert(src.At(bounds.Min.X+i, bounds.Min.Y+row)).(color.NRGBA)
			o := i * 4
			out[o] = c.R
			out[o+1] = c.G
			out[o+2] = c.B
			out[o+3] = c.A
		}
	}
}

func OverBackground(src *image.NRGBA, background color.RGBA) *image.RGBA {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	if width == 0 || height == 0 {
		return dst
	}
	bg := [3]uint32{uint32(background.R), uint32(background.G), uint32(background.B)}
	Bands(width, height, func(y0, y1 int) {
		for row := y0; row < y1; row++ {
			start := src.PixOffset(bounds.Min.X, bounds.Min.Y+row)
			in := src.Pix[start : start+width*4]
			out := dst.Pix[row*dst.Stride : row*dst.Stride+width*4]
			for o := 0; o < width*4; o += 4 {
				a := uint32(in[o+3])
				if a == 0xff {
					out[o] = in[o]
					out[o+1] = in[o+1]
					out[o+2] = in[o+2]
				} else {
					inverse := 0xff - a
					out[o] = uint8((uint32(in[o])*a + bg[0]*inverse + 127) / 255)
					out[o+1] = uint8((uint32(in[o+1])*a + bg[1]*inverse + 127) / 255)
					out[o+2] = uint8((uint32(in[o+2])*a + bg[2]*inverse + 127) / 255)
				}
				out[o+3] = 0xff
			}
		}
	})
	return dst
}

func AlphaAllZero(img *image.NRGBA) bool {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	for row := 0; row < height; row++ {
		start := img.PixOffset(bounds.Min.X, bounds.Min.Y+row)
		for o := start + 3; o < start+width*4; o += 4 {
			if img.Pix[o] != 0 {
				return false
			}
		}
	}
	return true
}

func SetOpaque(img *image.NRGBA) {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	Bands(width, height, func(y0, y1 int) {
		for row := y0; row < y1; row++ {
			start := img.PixOffset(bounds.Min.X, bounds.Min.Y+row)
			for o := start + 3; o < start+width*4; o += 4 {
				img.Pix[o] = 0xff
			}
		}
	})
}
