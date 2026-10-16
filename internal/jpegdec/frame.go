package jpegdec

const (
	maxComponents = 4
	maxTc         = 1
	maxTh         = 3
	maxTq         = 3
)

const (
	sof0Marker  = 0xc0
	sof1Marker  = 0xc1
	sof2Marker  = 0xc2
	dhtMarker   = 0xc4
	rst0Marker  = 0xd0
	rst7Marker  = 0xd7
	soiMarker   = 0xd8
	eoiMarker   = 0xd9
	sosMarker   = 0xda
	dqtMarker   = 0xdb
	driMarker   = 0xdd
	comMarker   = 0xfe
	app0Marker  = 0xe0
	app14Marker = 0xee
	app15Marker = 0xef
)

const (
	adobeTransformUnknown = 0
)

var unzig = [blockSize]int{
	0, 1, 8, 16, 9, 2, 3, 10,
	17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34,
	27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36,
	29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46,
	53, 60, 61, 54, 47, 55, 62, 63,
}

type component struct {
	h, v           int
	id             uint8
	tq             uint8
	shiftH, shiftV uint8
}

type frame struct {
	width, height  int
	nComp          int
	comp           [maxComponents]component
	maxH, maxV     int
	baseline       bool
	progressive    bool
	ri             int
	jfif           bool
	adobeValid     bool
	adobeTransform uint8
	quant          [maxTq + 1][blockSize]int32
	huff           [maxTc + 1][maxTh + 1]huffTable
}

func (f *frame) mcuCounts() (int, int) {
	return (f.width + 8*f.maxH - 1) / (8 * f.maxH), (f.height + 8*f.maxV - 1) / (8 * f.maxV)
}

func (f *frame) isRGB() bool {
	if f.jfif {
		return false
	}
	if f.adobeValid && f.adobeTransform == adobeTransformUnknown {
		return true
	}
	return f.comp[0].id == 'R' && f.comp[1].id == 'G' && f.comp[2].id == 'B'
}

func (f *frame) colorMode() (colorMode, error) {
	switch f.nComp {
	case 1:
		return colorGray, nil
	case 3:
		if f.isRGB() {
			return colorRGB, nil
		}
		return colorYCbCr, nil
	case 4:
		if !f.adobeValid {
			return 0, UnsupportedError("unknown color model: 4-component JPEG doesn't have Adobe APP14 metadata")
		}
		if f.adobeTransform != adobeTransformUnknown {
			return colorYCCK, nil
		}
		return colorCMYK, nil
	}
	return 0, FormatError("missing SOF marker")
}

type parser struct {
	s          *stream
	f          frame
	configOnly bool
	tmp        [2 * blockSize]byte
}

func (p *parser) readSOI() error {
	if err := p.s.readFull(p.tmp[:2]); err != nil {
		return err
	}
	if p.tmp[0] != 0xff || p.tmp[1] != soiMarker {
		return FormatError("missing SOI marker")
	}
	return nil
}

func (p *parser) next() (marker byte, n int, err error) {
	s := p.s
	for {
		if err = s.readFull(p.tmp[:2]); err != nil {
			return 0, 0, err
		}
		for p.tmp[0] != 0xff {
			p.tmp[0] = p.tmp[1]
			p.tmp[1], err = s.readByte()
			if err != nil {
				return 0, 0, err
			}
		}
		marker = p.tmp[1]
		if marker == 0 {
			continue
		}
		for marker == 0xff {
			marker, err = s.readByte()
			if err != nil {
				return 0, 0, err
			}
		}
		if marker == eoiMarker {
			return marker, 0, nil
		}
		if rst0Marker <= marker && marker <= rst7Marker {
			continue
		}
		if err = s.readFull(p.tmp[:2]); err != nil {
			return 0, 0, err
		}
		n = int(p.tmp[0])<<8 + int(p.tmp[1]) - 2
		if n < 0 {
			return 0, 0, FormatError("short segment length")
		}
		return marker, n, nil
	}
}

func (p *parser) segment(marker byte, n int) error {
	switch marker {
	case sof0Marker, sof1Marker, sof2Marker:
		p.f.baseline = marker == sof0Marker
		p.f.progressive = marker == sof2Marker
		return p.processSOF(n)
	case dhtMarker:
		if p.configOnly {
			return p.s.skip(n)
		}
		return p.processDHT(n)
	case dqtMarker:
		if p.configOnly {
			return p.s.skip(n)
		}
		return p.processDQT(n)
	case driMarker:
		if p.configOnly {
			return p.s.skip(n)
		}
		return p.processDRI(n)
	case app0Marker:
		return p.processApp0(n)
	case app14Marker:
		return p.processApp14(n)
	}
	if app0Marker <= marker && marker <= app15Marker || marker == comMarker {
		return p.s.skip(n)
	}
	if marker < 0xc0 {
		return FormatError("unknown marker")
	}
	return UnsupportedError("unknown marker")
}

func (p *parser) processSOF(n int) error {
	f := &p.f
	if f.nComp != 0 {
		return FormatError("multiple SOF markers")
	}
	switch n {
	case 6 + 3*1:
		f.nComp = 1
	case 6 + 3*3:
		f.nComp = 3
	case 6 + 3*4:
		f.nComp = 4
	default:
		return UnsupportedError("number of components")
	}
	if err := p.s.readFull(p.tmp[:n]); err != nil {
		return err
	}
	if p.tmp[0] != 8 {
		return UnsupportedError("precision")
	}
	f.height = int(p.tmp[1])<<8 + int(p.tmp[2])
	f.width = int(p.tmp[3])<<8 + int(p.tmp[4])
	if int(p.tmp[5]) != f.nComp {
		return FormatError("SOF has wrong length")
	}

	for i := 0; i < f.nComp; i++ {
		f.comp[i].id = p.tmp[6+3*i]
		for j := 0; j < i; j++ {
			if f.comp[i].id == f.comp[j].id {
				return FormatError("repeated component identifier")
			}
		}

		f.comp[i].tq = p.tmp[8+3*i]
		if f.comp[i].tq > maxTq {
			return FormatError("bad Tq value")
		}

		hv := p.tmp[7+3*i]
		h, v := int(hv>>4), int(hv&0x0f)
		if h < 1 || 4 < h || v < 1 || 4 < v {
			return FormatError("luma/chroma subsampling ratio")
		}
		if h == 3 || v == 3 {
			return errUnsupportedRatio
		}
		switch f.nComp {
		case 1:
			h, v = 1, 1
		case 4:
			switch i {
			case 0:
				if hv != 0x11 && hv != 0x22 {
					return errUnsupportedRatio
				}
			case 1, 2:
				if hv != 0x11 {
					return errUnsupportedRatio
				}
			case 3:
				if f.comp[0].h != h || f.comp[0].v != v {
					return errUnsupportedRatio
				}
			}
		}

		f.maxH, f.maxV = max(f.maxH, h), max(f.maxV, v)
		f.comp[i].h = h
		f.comp[i].v = v
	}

	if f.nComp == 3 {
		for i := 0; i < 3; i++ {
			if f.maxH%f.comp[i].h != 0 || f.maxV%f.comp[i].v != 0 {
				return errUnsupportedRatio
			}
		}
	}

	for i := 0; i < f.nComp; i++ {
		f.comp[i].shiftH = log2Small(f.maxH / f.comp[i].h)
		f.comp[i].shiftV = log2Small(f.maxV / f.comp[i].v)
	}
	return nil
}

func log2Small(v int) uint8 {
	switch v {
	case 2:
		return 1
	case 4:
		return 2
	}
	return 0
}

func (p *parser) processDQT(n int) error {
	f := &p.f
loop:
	for n > 0 {
		n--
		x, err := p.s.readByte()
		if err != nil {
			return err
		}
		tq := x & 0x0f
		if tq > maxTq {
			return FormatError("bad Tq value")
		}
		switch x >> 4 {
		default:
			return FormatError("bad Pq value")
		case 0:
			if n < blockSize {
				break loop
			}
			n -= blockSize
			if err := p.s.readFull(p.tmp[:blockSize]); err != nil {
				return err
			}
			for i := range f.quant[tq] {
				f.quant[tq][i] = int32(p.tmp[i])
			}
		case 1:
			if n < 2*blockSize {
				break loop
			}
			n -= 2 * blockSize
			if err := p.s.readFull(p.tmp[:2*blockSize]); err != nil {
				return err
			}
			for i := range f.quant[tq] {
				f.quant[tq][i] = int32(p.tmp[2*i])<<8 | int32(p.tmp[2*i+1])
			}
		}
	}
	if n != 0 {
		return FormatError("DQT has wrong length")
	}
	return nil
}

func (p *parser) processDRI(n int) error {
	if n != 2 {
		return FormatError("DRI has wrong length")
	}
	if err := p.s.readFull(p.tmp[:2]); err != nil {
		return err
	}
	p.f.ri = int(p.tmp[0])<<8 + int(p.tmp[1])
	return nil
}

func (p *parser) processApp0(n int) error {
	if n < 5 {
		return p.s.skip(n)
	}
	if err := p.s.readFull(p.tmp[:5]); err != nil {
		return err
	}
	n -= 5
	p.f.jfif = p.tmp[0] == 'J' && p.tmp[1] == 'F' && p.tmp[2] == 'I' && p.tmp[3] == 'F' && p.tmp[4] == '\x00'
	if n > 0 {
		return p.s.skip(n)
	}
	return nil
}

func (p *parser) processApp14(n int) error {
	if n < 12 {
		return p.s.skip(n)
	}
	if err := p.s.readFull(p.tmp[:12]); err != nil {
		return err
	}
	n -= 12
	if p.tmp[0] == 'A' && p.tmp[1] == 'd' && p.tmp[2] == 'o' && p.tmp[3] == 'b' && p.tmp[4] == 'e' {
		p.f.adobeValid = true
		p.f.adobeTransform = p.tmp[11]
	}
	if n > 0 {
		return p.s.skip(n)
	}
	return nil
}

func (p *parser) processDHT(n int) error {
	f := &p.f
	for n > 0 {
		if n < 17 {
			return FormatError("DHT has wrong length")
		}
		if err := p.s.readFull(p.tmp[:17]); err != nil {
			return err
		}
		tc := p.tmp[0] >> 4
		if tc > maxTc {
			return FormatError("bad Tc value")
		}
		th := p.tmp[0] & 0x0f
		if th > maxTh || (f.baseline && th > 1) {
			return FormatError("bad Th value")
		}
		h := &f.huff[tc][th]

		var counts [16]int32
		total := int32(0)
		for i := range counts {
			counts[i] = int32(p.tmp[i+1])
			total += counts[i]
		}
		if total == 0 {
			return FormatError("Huffman table has zero length")
		}
		if total > maxNCodes {
			return FormatError("Huffman table has excessive length")
		}
		n -= int(total) + 17
		if n < 0 {
			return FormatError("DHT has wrong length")
		}
		var vals [maxNCodes]uint8
		if err := p.s.readFull(vals[:total]); err != nil {
			return err
		}
		if err := h.build(&counts, &vals, total); err != nil {
			return err
		}
	}
	return nil
}

type scanComponent struct {
	index uint8
	td    uint8
	ta    uint8
}

type scanHeader struct {
	n      int
	comp   [maxComponents]scanComponent
	ss, se int32
	ah, al uint32
}

func (p *parser) processSOS(n int) (scanHeader, error) {
	f := &p.f
	var sh scanHeader
	if f.nComp == 0 {
		return sh, FormatError("missing SOF marker")
	}
	if n < 6 || 4+2*f.nComp < n || n%2 != 0 {
		return sh, FormatError("SOS has wrong length")
	}
	if err := p.s.readFull(p.tmp[:n]); err != nil {
		return sh, err
	}
	nComp := int(p.tmp[0])
	if n != 4+2*nComp {
		return sh, FormatError("SOS length inconsistent with number of components")
	}
	sh.n = nComp
	totalHV := 0
	for i := 0; i < nComp; i++ {
		cs := p.tmp[1+2*i]
		compIndex := -1
		for j, comp := range f.comp[:f.nComp] {
			if cs == comp.id {
				compIndex = j
			}
		}
		if compIndex < 0 {
			return sh, FormatError("unknown component selector")
		}
		sh.comp[i].index = uint8(compIndex)
		for j := 0; j < i; j++ {
			if sh.comp[i].index == sh.comp[j].index {
				return sh, FormatError("repeated component selector")
			}
		}
		totalHV += f.comp[compIndex].h * f.comp[compIndex].v

		sh.comp[i].td = p.tmp[2+2*i] >> 4
		if t := sh.comp[i].td; t > maxTh || (f.baseline && t > 1) {
			return sh, FormatError("bad Td value")
		}
		sh.comp[i].ta = p.tmp[2+2*i] & 0x0f
		if t := sh.comp[i].ta; t > maxTh || (f.baseline && t > 1) {
			return sh, FormatError("bad Ta value")
		}
	}
	if f.nComp > 1 && totalHV > 10 {
		return sh, FormatError("total sampling factors too large")
	}

	sh.ss, sh.se, sh.ah, sh.al = 0, blockSize-1, 0, 0
	if f.progressive {
		sh.ss = int32(p.tmp[1+2*nComp])
		sh.se = int32(p.tmp[2+2*nComp])
		sh.ah = uint32(p.tmp[3+2*nComp] >> 4)
		sh.al = uint32(p.tmp[3+2*nComp] & 0x0f)
		if (sh.ss == 0 && sh.se != 0) || sh.ss > sh.se || blockSize <= sh.se {
			return sh, FormatError("bad spectral selection bounds")
		}
		if sh.ss != 0 && nComp != 1 {
			return sh, FormatError("progressive AC coefficients for more than one component")
		}
		if sh.ah != 0 && sh.ah != sh.al+1 {
			return sh, FormatError("bad successive approximation values")
		}
	}
	return sh, nil
}

func newParser(s *stream, configOnly bool) *parser {
	return &parser{s: s, configOnly: configOnly}
}

func readConfig(s *stream) (*frame, error) {
	p := newParser(s, true)
	if err := p.readSOI(); err != nil {
		return nil, err
	}
	for {
		marker, n, err := p.next()
		if err != nil {
			return nil, err
		}
		switch marker {
		case eoiMarker:
			return nil, FormatError("missing SOF marker")
		case sosMarker:
			if p.f.nComp == 0 {
				return nil, FormatError("missing SOF marker")
			}
			return &p.f, nil
		}
		err = p.segment(marker, n)
		if marker >= sof0Marker && marker <= sof2Marker && p.f.jfif {
			if err != nil {
				return nil, err
			}
			return &p.f, nil
		}
		if err != nil {
			return nil, err
		}
	}
}
