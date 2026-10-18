package jxl

import (
	"context"
	"image"
	"io"
	"math"
	"sync"

	gjxl "github.com/gen2brain/jxl"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/assemble"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/codec/internal/shield"
)

const (
	referenceWhiteNits = 203.0
	sdrIntensityTarget = 255.0
	pqPeakNits         = 10000.0

	extraChannelSpot = 2

	transferBT709Code = 1
	transferLinear    = 8
	transferGamma22   = 4
	transferGamma28   = 5

	pqM1 = 2610.0 / 16384
	pqM2 = 2523.0 / 4096 * 128
	pqC1 = 3424.0 / 4096
	pqC2 = 2413.0 / 4096 * 32
	pqC3 = 2392.0 / 4096 * 32

	pqTableMinExp   = -40
	pqTableOctaves  = 41
	pqTableSteps    = 1024
	pqTableMantBits = 13
)

type hdrPlan struct {
	transfer uint8
	xyb      bool
	scale    float64
	color    int
	alpha    int
}

func (h imageHeader) hdrPlan() (hdrPlan, bool) {
	if h.embeddedProfile || h.animated {
		return hdrPlan{}, false
	}
	cicp := h.cicp()
	if cicp == nil {
		return hdrPlan{}, false
	}
	for _, channel := range h.extraChannels {
		if channel.kind == extraChannelSpot {
			return hdrPlan{}, false
		}
	}
	switch {
	case h.xybEncoded:
		switch cicp.Transfer {
		case transferLinear, codec.TransferPQ, codec.TransferHLG:
		default:
			return hdrPlan{}, false
		}
	case h.floatSample:
		switch cicp.Transfer {
		case transferLinear, transferSRGB, transferBT709Code, transferGamma22, transferGamma28:
		default:
			return hdrPlan{}, false
		}
	default:
		return hdrPlan{}, false
	}
	plan := hdrPlan{transfer: cicp.Transfer, xyb: h.xybEncoded, color: h.colorChannels(), alpha: -1}
	for i, channel := range h.extraChannels {
		if channel.kind == extraChannelAlpha {
			plan.alpha = plan.color + i
			break
		}
	}
	plan.scale = h.intensityTarget
	if cicp.Transfer != codec.TransferPQ && cicp.Transfer != codec.TransferHLG && plan.scale <= sdrIntensityTarget {
		plan.scale *= referenceWhiteNits / sdrIntensityTarget
	}
	return plan, true
}

func (p hdrPlan) toLinear(v float32) float64 {
	x := float64(v)
	if !(x > 0) {
		return 0
	}
	if p.xyb {
		return x
	}
	switch p.transfer {
	case transferSRGB:
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	case transferBT709Code:
		if x < 0.081 {
			return x / 4.5
		}
		return math.Pow((x+0.099)/1.099, 1/0.45)
	case transferGamma22:
		return math.Pow(x, 2.2)
	case transferGamma28:
		return math.Pow(x, 2.8)
	}
	return x
}

func pqEncode(x float64) float64 {
	t := math.Pow(x, pqM1)
	return math.Pow((pqC1+pqC2*t)/(1+pqC3*t), pqM2)
}

var (
	pqTableOnce sync.Once
	pqTable     []float32
)

func pqLookup() []float32 {
	pqTableOnce.Do(func() {
		table := make([]float32, pqTableOctaves*pqTableSteps+1)
		for octave := 0; octave < pqTableOctaves; octave++ {
			base := math.Ldexp(1, pqTableMinExp+octave)
			for step := 0; step < pqTableSteps; step++ {
				table[octave*pqTableSteps+step] = float32(pqEncode(base * (1 + float64(step)/pqTableSteps)))
			}
		}
		table[len(table)-1] = 1
		pqTable = table
	})
	return pqTable
}

func pqCode(table []float32, nits float64) uint16 {
	x := nits / pqPeakNits
	if !(x > 0) {
		return 0
	}
	if x >= 1 {
		return 0xffff
	}
	bits := math.Float32bits(float32(x))
	exponent := int(bits>>23) - 127
	if exponent < pqTableMinExp {
		return 0
	}
	fraction := bits & 0x7fffff
	index := (exponent-pqTableMinExp)*pqTableSteps + int(fraction>>pqTableMantBits)
	t := float32(fraction&(1<<pqTableMantBits-1)) * (1.0 / (1 << pqTableMantBits))
	y := table[index] + (table[index+1]-table[index])*t
	return uint16(y*65535 + 0.5)
}

func unitCode(v float32) uint16 {
	if !(v > 0) {
		return 0
	}
	if v >= 1 {
		return 0xffff
	}
	return uint16(float64(v)*65535 + 0.5)
}

func (p hdrPlan) frame(f *gjxl.Float, samples []float32) *image.NRGBA64 {
	out := image.NewNRGBA64(image.Rect(0, 0, f.Width, f.Height))
	table := pqLookup()
	n := f.Channels
	for y := 0; y < f.Height; y++ {
		row := samples[y*f.Width*n : (y+1)*f.Width*n]
		dst := out.Pix[y*out.Stride : y*out.Stride+f.Width*8]
		for x := 0; x < f.Width; x++ {
			px := row[x*n : (x+1)*n]
			q := dst[x*8 : x*8+8]
			for c := 0; c < 3; c++ {
				src := min(c, p.color-1)
				code := pqCode(table, p.toLinear(px[src])*p.scale)
				q[c*2], q[c*2+1] = uint8(code>>8), uint8(code)
			}
			a := uint16(0xffff)
			if p.alpha >= 0 {
				a = unitCode(px[p.alpha])
			}
			q[6], q[7] = uint8(a>>8), uint8(a)
		}
	}
	return out
}

func (d Decoder) decodeHDR(ctx context.Context, source io.Reader, found inspection, plan hdrPlan, lim codec.Limits) (*codec.Image, error) {
	var result *gjxl.Float
	err := shield.Run(formatTag, func() error {
		var err error
		result, err = gjxl.DecodeFloat(source, gjxl.Options{Threads: d.threads(), FrameSizeLimit: frameSizeLimit(lim)})
		return err
	})
	if err != nil {
		return nil, classify(ctx, err)
	}
	if result.Width != found.width || result.Height != found.height || len(result.Frames) == 0 || result.Channels <= max(plan.color-1, plan.alpha) {
		return nil, guard.Corrupt("jxl: the decoded picture does not match its header")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var deep *image.NRGBA64
	err = shield.Run(formatTag, func() error {
		deep = plan.frame(result, result.Frames[0])
		return nil
	})
	if err != nil {
		return nil, classify(ctx, err)
	}
	cicp := found.header.cicp()
	color := assemble.Color(nil, uint16(cicp.Primaries), codec.TransferPQ, 0, true)
	return &codec.Image{Info: found.info, Color: color, Frames: []codec.Frame{{Deep: deep}}, Scale: 1}, nil
}
