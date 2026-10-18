package colorm

import (
	"errors"
	"fmt"
	"image"
	"math"
	"sync"

	"github.com/oops1/PixGo/internal/par"
)

type Intent uint8

const (
	RelativeColorimetric Intent = iota
	Perceptual
)

type Options struct {
	Intent   Intent
	NoBPC    bool
	SoftClip bool
	NoDither bool
	HDR      HDRConfig
}

type HDRConfig struct {
	White      float64
	Peak       float64
	SourcePeak float64
}

const (
	DefaultWhiteNits      = 203
	DefaultDisplayNits    = 300
	DefaultSourcePeakNits = 1000
)

func (c HDRConfig) withDefaults() HDRConfig {
	if c.White <= 0 {
		c.White = DefaultWhiteNits
	}
	if c.Peak < c.White {
		c.Peak = max(c.White, DefaultDisplayNits)
		if c.Peak < c.White {
			c.Peak = c.White
		}
	}
	if c.SourcePeak <= 0 {
		c.SourcePeak = DefaultSourcePeakNits
	}
	return c
}

type mode uint8

const (
	modeIdentity mode = iota
	modeMatrix
	modeLUT3
	modeCMYK
	modeHDR
)

var ErrChannels = errors.New("colorm: wrong channel layout for this transform")

type Transform struct {
	src, dst *Profile
	opt      Options
	mode     mode
	mp       *matrixPath
	lp       *lutPath
	hp       *hdrPath
	enc      *encoder
	dstLuma  [3]float32
	dstMat   Mat3
	dstInv   Mat3
	bpc      bpcParams
	pixelFn  func(r, g, b float64) (float64, float64, float64)
}

func (t *Transform) Identity() bool { return t.mode == modeIdentity }

func (t *Transform) HDR() bool { return t.mode == modeHDR }

func (t *Transform) Source() *Profile { return t.src }

func (t *Transform) Destination() *Profile { return t.dst }

type cacheKey struct {
	src, dst [32]byte
	opt      Options
}

var (
	cacheMu    sync.Mutex
	cacheItems = map[cacheKey]*Transform{}
	cacheOrder []cacheKey
)

const cacheMax = 24

func NewTransform(src, dst *Profile, opt Options) (*Transform, error) {
	if src == nil || dst == nil {
		return nil, ErrProfile
	}
	opt.HDR = opt.HDR.withDefaults()
	key := cacheKey{src.Hash, dst.Hash, opt}
	cacheMu.Lock()
	if t, ok := cacheItems[key]; ok {
		cacheMu.Unlock()
		return t, nil
	}
	cacheMu.Unlock()
	t, err := buildTransform(src, dst, opt)
	if err != nil {
		return nil, err
	}
	cacheMu.Lock()
	if _, ok := cacheItems[key]; !ok {
		if len(cacheOrder) >= cacheMax {
			delete(cacheItems, cacheOrder[0])
			cacheOrder = cacheOrder[1:]
		}
		cacheItems[key] = t
		cacheOrder = append(cacheOrder, key)
	}
	cacheMu.Unlock()
	return t, nil
}

func (p *Profile) a2bFor(in Intent) *lutPipe {
	order := [...]int{0, 1, 2}
	if in == RelativeColorimetric {
		order = [...]int{1, 0, 2}
	}
	if in == RelativeColorimetric && p.kind == kindMatrix {
		return nil
	}
	for _, i := range order {
		if p.a2b[i] != nil {
			return p.a2b[i]
		}
	}
	return nil
}

func (p *Profile) b2aFor(in Intent) *lutPipe {
	order := [...]int{0, 1, 2}
	if in == RelativeColorimetric {
		order = [...]int{1, 0, 2}
	}
	if in == RelativeColorimetric && p.kind == kindMatrix {
		return nil
	}
	for _, i := range order {
		if p.b2a[i] != nil {
			return p.b2a[i]
		}
	}
	return nil
}

func (p *Profile) usesLUT(in Intent) bool {
	if p.kind == kindLUT {
		return true
	}
	return in == Perceptual && p.a2bFor(in) != nil && !p.gray
}

func (p *Profile) toPCS(in Intent) func(dev [4]float64) Vec3 {
	if lp := p.a2bFor(in); lp != nil && p.usesLUT(in) {
		return lp.toPCS
	}
	m := p.colorants
	trc := p.trc
	return func(dev [4]float64) Vec3 {
		return m.Apply(Vec3{trc[0].Eval(dev[0]), trc[1].Eval(dev[1]), trc[2].Eval(dev[2])})
	}
}

func buildTransform(src, dst *Profile, opt Options) (*Transform, error) {
	if dst.Space != SpaceRGB {
		return nil, ErrUnsupported
	}
	t := &Transform{src: src, dst: dst, opt: opt}
	dstLUT := dst.kind == kindLUT || opt.Intent == Perceptual && dst.b2aFor(opt.Intent) != nil
	if !dstLUT {
		inv, ok := dst.colorants.Inverse()
		if !ok {
			return nil, ErrProfile
		}
		t.dstInv, t.dstMat = inv, dst.colorants
		t.dstLuma = [3]float32{float32(dst.colorants[3]), float32(dst.colorants[4]), float32(dst.colorants[5])}
	} else if dst.b2aFor(opt.Intent) == nil {
		return nil, ErrUnsupported
	}
	if src.Space == SpaceCMYK {
		return buildCMYK(t)
	}
	if src.hdr != hdrNone {
		if dstLUT {
			return nil, ErrUnsupported
		}
		return buildHDR(t)
	}
	srcLUT := src.usesLUT(opt.Intent)
	if !srcLUT && !dstLUT {
		return buildMatrix(t)
	}
	return buildLUT3(t, srcLUT, dstLUT)
}

type bpcParams struct {
	on     bool
	ax, bx [3]float64
}

func (t *Transform) setupBPC(srcLUT bool) {
	if t.opt.NoBPC || !srcLUT {
		return
	}
	if t.opt.Intent == Perceptual && t.src.Major >= 4 && t.dst.Major >= 4 {
		return
	}
	bs := t.src.blackPoint(t.opt.Intent)
	var bd Vec3
	if t.dst.kind == kindLUT || t.opt.Intent == Perceptual && t.dst.b2aFor(t.opt.Intent) != nil {
		bd = t.dst.blackPoint(t.opt.Intent)
	}
	var p bpcParams
	for i := 0; i < 3; i++ {
		den := d50[i] - bs[i]
		if den <= 1e-6 {
			return
		}
		p.ax[i] = (d50[i] - bd[i]) / den
		p.bx[i] = d50[i] * (bd[i] - bs[i]) / den
	}
	p.on = true
	t.bpc = p
}

func (p bpcParams) apply(v Vec3) Vec3 {
	if !p.on {
		return v
	}
	return Vec3{p.ax[0]*v[0] + p.bx[0], p.ax[1]*v[1] + p.bx[1], p.ax[2]*v[2] + p.bx[2]}
}

func (p *Profile) blackPoint(in Intent) Vec3 {
	lp := p.a2bFor(in)
	if lp == nil {
		return Vec3{}
	}
	var probes [][4]float64
	switch p.Channels() {
	case 4:
		probes = [][4]float64{{0, 0, 0, 1}, {1, 1, 1, 1}, {0.8, 0.8, 0.8, 1}}
	default:
		probes = [][4]float64{{0, 0, 0}}
	}
	best := Vec3{}
	bestY := math.MaxFloat64
	for _, d := range probes {
		x := lp.toPCS(d)
		if x[1] < bestY {
			bestY, best = x[1], x
		}
	}
	for i := range best {
		best[i] = math.Min(math.Max(best[i], 0), d50[i]*0.5)
	}
	return best
}

func bands(w, h int, fn func(y0, y1 int)) {
	if w <= 0 || h <= 0 {
		return
	}
	workers := par.Count()
	if int64(w)*int64(h) < 1<<16 || workers < 2 || h < 8 {
		fn(0, h)
		return
	}
	rows := max(4, h/(workers*4))
	tasks := (h + rows - 1) / rows
	par.Parallel(tasks, workers, func(i int) {
		fn(i*rows, min((i+1)*rows, h))
	})
}

func (t *Transform) Apply(img *image.NRGBA) {
	if t == nil || img == nil || t.mode == modeIdentity {
		return
	}
	b := img.Rect
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	bands(w, h, func(y0, y1 int) {
		row := make([]byte, 0)
		for y := y0; y < y1; y++ {
			o := img.PixOffset(b.Min.X, b.Min.Y+y)
			px := img.Pix[o : o+4*w : o+4*w]
			switch t.mode {
			case modeMatrix:
				t.mp.row8(px)
			case modeLUT3:
				t.lp.row8(px)
			case modeHDR:
				row = t.hp.row8(px, row)
			}
		}
	})
}

func (t *Transform) Convert(dst *image.NRGBA, src *image.NRGBA64) error {
	if t == nil || dst == nil || src == nil {
		return ErrProfile
	}
	if t.mode == modeCMYK {
		return ErrChannels
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	if dst.Rect.Dx() != w || dst.Rect.Dy() != h {
		return fmt.Errorf("colorm: size mismatch %dx%d vs %dx%d", dst.Rect.Dx(), dst.Rect.Dy(), w, h)
	}
	if w <= 0 || h <= 0 {
		return nil
	}
	dither := !t.opt.NoDither
	bands(w, h, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			so := src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+y)
			do := dst.PixOffset(dst.Rect.Min.X, dst.Rect.Min.Y+y)
			s := src.Pix[so : so+8*w : so+8*w]
			d := dst.Pix[do : do+4*w : do+4*w]
			switch t.mode {
			case modeIdentity:
				identityRow16(d, s, y, dither)
			case modeMatrix:
				t.mp.row16(d, s, y, dither)
			case modeLUT3:
				t.lp.row16(d, s, y, dither)
			case modeHDR:
				t.hp.row16(d, s, y, dither)
			}
		}
	})
	return nil
}

func (t *Transform) ConvertCMYK(dst *image.NRGBA, src *image.CMYK) error {
	if t == nil || t.mode != modeCMYK {
		return ErrChannels
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	if dst.Rect.Dx() != w || dst.Rect.Dy() != h {
		return fmt.Errorf("colorm: size mismatch")
	}
	bands(w, h, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			so := src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+y)
			do := dst.PixOffset(dst.Rect.Min.X, dst.Rect.Min.Y+y)
			t.lp.rowCMYK(dst.Pix[do:do+4*w:do+4*w], src.Pix[so:so+4*w:so+4*w])
		}
	})
	return nil
}

func (t *Transform) Pixel(r, g, b float64) (float64, float64, float64) {
	if t.mode == modeIdentity {
		return r, g, b
	}
	return t.pixelFn(r, g, b)
}

var bayer4 = [16]uint32{0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5}

func ditherAt(x, y int, on bool) uint32 {
	if !on {
		return 128
	}
	return bayer4[(y&3)*4+(x&3)]*16 + 8
}

func identityRow16(d, s []byte, y int, dither bool) {
	n := len(d) / 4
	for x := 0; x < n; x++ {
		th := uint32(32767)
		if dither {
			th = bayer4[(y&3)*4+(x&3)]*4096 + 2048
		}
		so, do := x*8, x*4
		for c := 0; c < 3; c++ {
			v := uint32(s[so+2*c])<<8 | uint32(s[so+2*c+1])
			d[do+c] = uint8((v*255 + th) / 65535)
		}
		a := uint32(s[so+6])<<8 | uint32(s[so+7])
		d[do+3] = uint8((a*255 + 32767) / 65535)
	}
}
