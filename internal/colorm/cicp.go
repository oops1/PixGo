package colorm

import (
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/oops1/PixGo/internal/codec"
)

type hdrKind uint8

const (
	hdrNone hdrKind = iota
	hdrPQ
	hdrHLG
)

func hdrKindOf(transfer uint8) hdrKind {
	switch transfer {
	case 16:
		return hdrPQ
	case 18:
		return hdrHLG
	}
	return hdrNone
}

type primariesSet struct {
	name           string
	rx, ry, gx, gy float64
	bx, by, wx, wy float64
}

var (
	whiteD65 = [2]float64{0.3127, 0.3290}
	whiteC   = [2]float64{0.310, 0.316}
	whiteDCI = [2]float64{0.314, 0.351}
)

var cicpPrimaries = map[uint8]primariesSet{
	1:  {"BT.709", 0.640, 0.330, 0.300, 0.600, 0.150, 0.060, whiteD65[0], whiteD65[1]},
	4:  {"BT.470 System M", 0.670, 0.330, 0.210, 0.710, 0.140, 0.080, whiteC[0], whiteC[1]},
	5:  {"BT.601 625", 0.640, 0.330, 0.290, 0.600, 0.150, 0.060, whiteD65[0], whiteD65[1]},
	6:  {"BT.601 525", 0.630, 0.340, 0.310, 0.595, 0.155, 0.070, whiteD65[0], whiteD65[1]},
	7:  {"SMPTE 240M", 0.630, 0.340, 0.310, 0.595, 0.155, 0.070, whiteD65[0], whiteD65[1]},
	8:  {"Generic film", 0.681, 0.319, 0.243, 0.692, 0.145, 0.049, whiteC[0], whiteC[1]},
	9:  {"BT.2020", 0.708, 0.292, 0.170, 0.797, 0.131, 0.046, whiteD65[0], whiteD65[1]},
	11: {"DCI P3", 0.680, 0.320, 0.265, 0.690, 0.150, 0.060, whiteDCI[0], whiteDCI[1]},
	12: {"Display P3", 0.680, 0.320, 0.265, 0.690, 0.150, 0.060, whiteD65[0], whiteD65[1]},
	22: {"EBU 3213", 0.630, 0.340, 0.295, 0.605, 0.155, 0.077, whiteD65[0], whiteD65[1]},
}

type transferDef struct {
	name string
	make func() *Curve
}

var cicpTransfers = map[uint8]transferDef{
	1:  {"BT.709", func() *Curve { return funcCurve(bt709Decode, bt709Encode) }},
	4:  {"Gamma 2.2", func() *Curve { return gammaCurve(2.2) }},
	5:  {"Gamma 2.8", func() *Curve { return gammaCurve(2.8) }},
	6:  {"BT.601", func() *Curve { return funcCurve(bt709Decode, bt709Encode) }},
	7:  {"SMPTE 240M", func() *Curve { return funcCurve(bt709Decode, bt709Encode) }},
	8:  {"Linear", identityCurve},
	13: {"sRGB", srgbCurve},
	14: {"BT.709", func() *Curve { return funcCurve(bt709Decode, bt709Encode) }},
	15: {"BT.709", func() *Curve { return funcCurve(bt709Decode, bt709Encode) }},
	16: {"PQ", func() *Curve { return funcCurve(pqEOTF, pqInverse) }},
	17: {"Gamma 2.6", func() *Curve { return gammaCurve(2.6) }},
	18: {"HLG", func() *Curve { return funcCurve(hlgOETFInverse, hlgOETF) }},
}

func FromCICP(c codec.CICP) (*Profile, error) {
	ps, ok := cicpPrimaries[c.Primaries]
	if !ok {
		if c.Primaries != 0 && c.Primaries != 2 {
			return nil, ErrUnsupported
		}
		ps = cicpPrimaries[1]
	}
	tf, ok := cicpTransfers[c.Transfer]
	if !ok {
		if c.Transfer != 0 && c.Transfer != 2 {
			return nil, ErrUnsupported
		}
		tf = cicpTransfers[13]
	}
	m, ok := PrimariesToD50(ps.rx, ps.ry, ps.gx, ps.gy, ps.bx, ps.by, ps.wx, ps.wy)
	if !ok {
		return nil, ErrProfile
	}
	cv := c
	name := ps.name
	if c.Primaries == 1 && c.Transfer == 13 {
		name = "sRGB"
	} else {
		name = ps.name + " / " + tf.name
	}
	curve := tf.make()
	p := &Profile{
		Description: name,
		Major:       4,
		Class:       "mntr",
		Space:       SpaceRGB,
		kind:        kindMatrix,
		colorants:   m,
		cicp:        &cv,
		hdr:         hdrKindOf(c.Transfer),
		trc:         [3]*Curve{curve, curve, curve},
		builtin:     true,
	}
	adapt := Adaptation(xyFromXYZ(ps.wx, ps.wy), d50)
	p.chad = &adapt
	w := Vec3{ps.wx / ps.wy, 1, (1 - ps.wx - ps.wy) / ps.wy}
	p.wtpt = &w
	p.Hash = sha256.Sum256([]byte(fmt.Sprintf("cicp:%d/%d", c.Primaries, c.Transfer)))
	return p, nil
}

var (
	srgbOnce sync.Once
	srgbProf *Profile
)

func SRGB() *Profile {
	srgbOnce.Do(func() {
		p, err := FromCICP(codec.CICP{Primaries: 1, Transfer: 13, Matrix: 0, FullRange: true})
		if err != nil {
			panic(err)
		}
		p.Description = "sRGB"
		srgbProf = p
	})
	return srgbProf
}

func (p *Profile) HDR() string {
	switch p.hdr {
	case hdrPQ:
		return "PQ"
	case hdrHLG:
		return "HLG"
	}
	return ""
}

func (p *Profile) IsHDR() bool { return p.hdr != hdrNone }
