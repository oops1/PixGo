package apng

import "image"

type compositor64 struct {
	canvas   *image.NRGBA64
	previous frame
	started  bool
	saved    []byte
}

func newCompositor64(width, height int) *compositor64 {
	return &compositor64{canvas: image.NewNRGBA64(image.Rect(0, 0, width, height))}
}

func (c *compositor64) apply(f frame, pix *image.NRGBA64) {
	if c.started {
		c.dispose()
	}
	if f.dispose == disposePrevious {
		c.save(f)
	}
	c.draw(f, pix)
	c.previous, c.started = f, true
}

func (c *compositor64) snapshot() *image.NRGBA64 {
	return &image.NRGBA64{Pix: append([]byte(nil), c.canvas.Pix...), Stride: c.canvas.Stride, Rect: c.canvas.Rect}
}

func (c *compositor64) rowOf(f frame, row int) []byte {
	offset := c.canvas.PixOffset(f.x, f.y+row)
	return c.canvas.Pix[offset : offset+f.width*8]
}

func (c *compositor64) dispose() {
	f := c.previous
	switch f.dispose {
	case disposeBackground:
		for row := 0; row < f.height; row++ {
			clear(c.rowOf(f, row))
		}
	case disposePrevious:
		rowBytes := f.width * 8
		for row := 0; row < f.height; row++ {
			copy(c.rowOf(f, row), c.saved[row*rowBytes:(row+1)*rowBytes])
		}
	}
}

func (c *compositor64) save(f frame) {
	rowBytes := f.width * 8
	c.saved = append(c.saved[:0], make([]byte, rowBytes*f.height)...)
	for row := 0; row < f.height; row++ {
		copy(c.saved[row*rowBytes:(row+1)*rowBytes], c.rowOf(f, row))
	}
}

func (c *compositor64) draw(f frame, pix *image.NRGBA64) {
	rowBytes := f.width * 8
	for row := 0; row < f.height; row++ {
		src := pix.Pix[row*pix.Stride : row*pix.Stride+rowBytes]
		dst := c.rowOf(f, row)
		if f.blend != blendOver {
			copy(dst, src)
			continue
		}
		for i := 0; i < rowBytes; i += 8 {
			blendPixel64(dst[i:i+8:i+8], src[i:i+8:i+8])
		}
	}
}

func word(b []byte) uint64 { return uint64(b[0])<<8 | uint64(b[1]) }

func blendPixel64(dst, src []byte) {
	sourceAlpha := word(src[6:])
	switch {
	case sourceAlpha == 0:
		return
	case sourceAlpha == 0xffff || word(dst[6:]) == 0:
		copy(dst, src)
		return
	}
	destAlpha := word(dst[6:])
	total := sourceAlpha*0xffff + destAlpha*(0xffff-sourceAlpha)
	for k := 0; k < 6; k += 2 {
		weighted := word(src[k:])*sourceAlpha*0xffff + word(dst[k:])*destAlpha*(0xffff-sourceAlpha)
		v := (weighted + total/2) / total
		dst[k], dst[k+1] = byte(v>>8), byte(v)
	}
	a := (total + 0x7fff) / 0xffff
	dst[6], dst[7] = byte(a>>8), byte(a)
}
