package render

func bil(a, b, c, d, wx0, wx1, wy0, wy1 uint32) uint32 {
	t := (a*wx0 + b*wx1) >> 8
	u := (c*wx0 + d*wx1) >> 8
	return (t*wy0 + u*wy1 + 1<<23) >> 24
}

func lerpH(a, b, wx0, wx1 uint32) uint32 {
	return (a*wx0 + b*wx1 + 1<<15) >> 16
}

func spanOpaqueNearest(out, row []byte, off []int) {
	out = out[:len(off)*4]
	for k, o := range off {
		le.PutUint32(out[k*4:], le.Uint32(row[o:])|opaqueAlpha)
	}
}

func spanOpaqueBilinearH(out, row []byte, off0, off1 []int, frac []uint32) {
	n := len(off0)
	out = out[:n*4]
	off1 = off1[:n]
	frac = frac[:n]
	for k, a := range off0 {
		wx1 := frac[k]
		wx0 := fixedOne - wx1
		p0 := le.Uint32(row[a:])
		p1 := le.Uint32(row[off1[k]:])
		r := lerpH(p0&0xFF, p1&0xFF, wx0, wx1)
		g := lerpH((p0>>8)&0xFF, (p1>>8)&0xFF, wx0, wx1)
		b := lerpH((p0>>16)&0xFF, (p1>>16)&0xFF, wx0, wx1)
		le.PutUint32(out[k*4:], r|g<<8|b<<16|opaqueAlpha)
	}
}

func spanOpaqueBilinear(out, row0, row1 []byte, wy uint32, off0, off1 []int, frac []uint32) {
	n := len(off0)
	out = out[:n*4]
	off1 = off1[:n]
	frac = frac[:n]
	wy1 := wy
	wy0 := fixedOne - wy
	for k, a := range off0 {
		b := off1[k]
		wx1 := frac[k]
		wx0 := fixedOne - wx1
		p00 := le.Uint32(row0[a:])
		p01 := le.Uint32(row0[b:])
		p10 := le.Uint32(row1[a:])
		p11 := le.Uint32(row1[b:])
		r := bil(p00&0xFF, p01&0xFF, p10&0xFF, p11&0xFF, wx0, wx1, wy0, wy1)
		g := bil((p00>>8)&0xFF, (p01>>8)&0xFF, (p10>>8)&0xFF, (p11>>8)&0xFF, wx0, wx1, wy0, wy1)
		bl := bil((p00>>16)&0xFF, (p01>>16)&0xFF, (p10>>16)&0xFF, (p11>>16)&0xFF, wx0, wx1, wy0, wy1)
		le.PutUint32(out[k*4:], r|g<<8|bl<<16|opaqueAlpha)
	}
}

func mulW(a, b uint32) uint32 {
	return uint32((uint64(a) * uint64(b)) >> 16)
}

func overBg(c, a, bg uint32) uint32 {
	return (c*a + bg*(255-a) + 127) / 255
}

func (p *plan) spanAlphaNearest(out, row []byte, off []int, par []uint8, rowPar uint8) {
	n := len(off)
	out = out[:n*4]
	par = par[:n]
	for k, o := range off {
		px := le.Uint32(row[o:])
		a := px >> 24
		switch a {
		case 255:
			le.PutUint32(out[k*4:], px)
		case 0:
			le.PutUint32(out[k*4:], p.pal[(par[k]^rowPar)&1].px)
		default:
			bg := &p.pal[(par[k]^rowPar)&1]
			r := overBg(px&0xFF, a, bg.r)
			g := overBg((px>>8)&0xFF, a, bg.g)
			b := overBg((px>>16)&0xFF, a, bg.b)
			le.PutUint32(out[k*4:], r|g<<8|b<<16|opaqueAlpha)
		}
	}
}

func (p *plan) spanAlphaBilinear(out, row0, row1 []byte, wy uint32, off0, off1 []int, frac []uint32, par []uint8, rowPar uint8) {
	n := len(off0)
	out = out[:n*4]
	off1 = off1[:n]
	frac = frac[:n]
	par = par[:n]
	wy1 := wy
	wy0 := fixedOne - wy
	for k, a := range off0 {
		b := off1[k]
		wx1 := frac[k]
		wx0 := fixedOne - wx1
		p00 := le.Uint32(row0[a:])
		p01 := le.Uint32(row0[b:])
		p10 := le.Uint32(row1[a:])
		p11 := le.Uint32(row1[b:])
		a00, a01, a10, a11 := p00>>24, p01>>24, p10>>24, p11>>24
		bg := &p.pal[(par[k]^rowPar)&1]
		if a00&a01&a10&a11 == 255 {
			r := bil(p00&0xFF, p01&0xFF, p10&0xFF, p11&0xFF, wx0, wx1, wy0, wy1)
			g := bil((p00>>8)&0xFF, (p01>>8)&0xFF, (p10>>8)&0xFF, (p11>>8)&0xFF, wx0, wx1, wy0, wy1)
			bl := bil((p00>>16)&0xFF, (p01>>16)&0xFF, (p10>>16)&0xFF, (p11>>16)&0xFF, wx0, wx1, wy0, wy1)
			le.PutUint32(out[k*4:], r|g<<8|bl<<16|opaqueAlpha)
			continue
		}
		if a00|a01|a10|a11 == 0 {
			le.PutUint32(out[k*4:], bg.px)
			continue
		}
		w00 := mulW(wx0, wy0)
		w01 := mulW(wx1, wy0)
		w10 := mulW(wx0, wy1)
		w11 := mulW(wx1, wy1)
		m00, m01, m10, m11 := w00*a00, w01*a01, w10*a10, w11*a11
		cover := m00 + m01 + m10 + m11
		rest := alphaDivisor - cover
		r := (m00*(p00&0xFF) + m01*(p01&0xFF) + m10*(p10&0xFF) + m11*(p11&0xFF) + bg.r*rest + alphaDivisor/2) / alphaDivisor
		g := (m00*((p00>>8)&0xFF) + m01*((p01>>8)&0xFF) + m10*((p10>>8)&0xFF) + m11*((p11>>8)&0xFF) + bg.g*rest + alphaDivisor/2) / alphaDivisor
		bl := (m00*((p00>>16)&0xFF) + m01*((p01>>16)&0xFF) + m10*((p10>>16)&0xFF) + m11*((p11>>16)&0xFF) + bg.b*rest + alphaDivisor/2) / alphaDivisor
		le.PutUint32(out[k*4:], r|g<<8|bl<<16|opaqueAlpha)
	}
}
