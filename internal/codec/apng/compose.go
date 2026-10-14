package apng

import (
	"image"
)

type compositor struct {
	canvas   *image.NRGBA
	previous frame
	started  bool
	saved    []byte
}

func newCompositor(width, height int) *compositor {
	return &compositor{canvas: image.NewNRGBA(image.Rect(0, 0, width, height))}
}

func (c *compositor) apply(f frame, pix *image.NRGBA) {
	if c.started {
		c.dispose()
	}
	if f.dispose == disposePrevious {
		c.save(f)
	}
	c.draw(f, pix)
	c.previous, c.started = f, true
}

func (c *compositor) snapshot() *image.NRGBA {
	return &image.NRGBA{Pix: append([]byte(nil), c.canvas.Pix...), Stride: c.canvas.Stride, Rect: c.canvas.Rect}
}

func (c *compositor) rowOf(f frame, row int) []byte {
	offset := c.canvas.PixOffset(f.x, f.y+row)
	return c.canvas.Pix[offset : offset+f.width*4]
}

func (c *compositor) dispose() {
	f := c.previous
	switch f.dispose {
	case disposeBackground:
		for row := 0; row < f.height; row++ {
			clear(c.rowOf(f, row))
		}
	case disposePrevious:
		rowBytes := f.width * 4
		for row := 0; row < f.height; row++ {
			copy(c.rowOf(f, row), c.saved[row*rowBytes:(row+1)*rowBytes])
		}
	}
}

func (c *compositor) save(f frame) {
	rowBytes := f.width * 4
	c.saved = append(c.saved[:0], make([]byte, rowBytes*f.height)...)
	for row := 0; row < f.height; row++ {
		copy(c.saved[row*rowBytes:(row+1)*rowBytes], c.rowOf(f, row))
	}
}

func (c *compositor) draw(f frame, pix *image.NRGBA) {
	rowBytes := f.width * 4
	for row := 0; row < f.height; row++ {
		src := pix.Pix[row*pix.Stride : row*pix.Stride+rowBytes]
		dst := c.rowOf(f, row)
		if f.blend != blendOver {
			copy(dst, src)
			continue
		}
		for i := 0; i < rowBytes; i += 4 {
			blendPixel(dst[i:i+4:i+4], src[i:i+4:i+4])
		}
	}
}

func blendPixel(dst, src []byte) {
	sourceAlpha := uint32(src[3])
	switch {
	case sourceAlpha == 0:
		return
	case sourceAlpha == 255 || dst[3] == 0:
		copy(dst, src)
		return
	}
	destAlpha := uint32(dst[3])
	total := sourceAlpha*255 + destAlpha*(255-sourceAlpha)
	for k := 0; k < 3; k++ {
		weighted := uint32(src[k])*sourceAlpha*255 + uint32(dst[k])*destAlpha*(255-sourceAlpha)
		dst[k] = uint8((weighted + total/2) / total)
	}
	dst[3] = uint8((total + 127) / 255)
}
