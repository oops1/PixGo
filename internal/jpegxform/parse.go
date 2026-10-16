package jpegxform

import "bytes"

const (
	markerTEM  = 0x01
	markerSOF0 = 0xC0
	markerSOF1 = 0xC1
	markerDHT  = 0xC4
	markerJPG  = 0xC8
	markerDAC  = 0xCC
	markerRST0 = 0xD0
	markerRST7 = 0xD7
	markerSOI  = 0xD8
	markerEOI  = 0xD9
	markerSOS  = 0xDA
	markerDQT  = 0xDB
	markerDNL  = 0xDC
	markerDRI  = 0xDD
	markerAPP0 = 0xE0
	markerAPPF = 0xEF
	markerCOM  = 0xFE

	maxMCUBlocks      = 10
	maxKeptSegmentSum = 32 << 20
)

type component struct {
	id         byte
	h, v       int
	quant      int
	dcTable    int
	acTable    int
	blocksWide int
	blocksHigh int
}

type quantTable struct {
	present bool
	wide    bool
	values  [64]uint16
}

type huffSpec struct {
	present bool
	counts  [16]uint8
	symbols []byte
}

type segment struct {
	marker  byte
	payload []byte
}

type frame struct {
	marker        byte
	width, height int
	comps         []component
	hmax, vmax    int
	mcuW, mcuH    int
	mcusX, mcusY  int
}

type header struct {
	segments  []segment
	keptBytes int
	quant     [4]quantTable
	dc, ac    [4]huffSpec
	restart   int
	fr        frame
	hasFrame  bool
	scan      []int
}

func isMetadata(m byte) bool { return (m >= markerAPP0 && m <= markerAPPF) || m == markerCOM }

func readHeader(src *byteSource, keepMetadata bool) (*header, error) {
	var soi [2]byte
	if !src.readFull(soi[:]) {
		return nil, src.failure()
	}
	if soi[0] != 0xFF || soi[1] != markerSOI {
		return nil, corrupt("no SOI marker")
	}
	h := &header{}
	for {
		m, ok := src.marker()
		if !ok {
			return nil, src.failure()
		}
		switch {
		case m == markerSOI:
			return nil, corrupt("second SOI marker")
		case m == markerEOI:
			return nil, corrupt("no scan before EOI")
		case m == markerTEM || (m >= markerRST0 && m <= markerRST7):
			continue
		}
		length, ok := src.u16()
		if !ok {
			return nil, src.failure()
		}
		if length < 2 {
			return nil, corrupt("segment length %d", length)
		}
		n := length - 2
		if !keepMetadata && isMetadata(m) {
			if !src.skip(n) {
				return nil, src.failure()
			}
			continue
		}
		payload := make([]byte, n)
		if !src.readFull(payload) {
			return nil, src.failure()
		}
		done, err := h.apply(m, payload)
		if err != nil {
			return nil, err
		}
		if done {
			return h, nil
		}
	}
}

func (h *header) apply(m byte, payload []byte) (bool, error) {
	switch {
	case isMetadata(m):
		h.keptBytes += len(payload)
		if h.keptBytes > maxKeptSegmentSum {
			return false, unsupported("metadata segments are too large")
		}
		h.segments = append(h.segments, segment{marker: m, payload: payload})
	case m == markerDQT:
		return false, h.parseDQT(payload)
	case m == markerDHT:
		return false, h.parseDHT(payload)
	case m == markerDRI:
		if len(payload) != 2 {
			return false, corrupt("DRI length %d", len(payload))
		}
		h.restart = int(payload[0])<<8 | int(payload[1])
	case m == markerSOF0 || m == markerSOF1:
		if h.hasFrame {
			return false, corrupt("second frame header")
		}
		if err := h.parseSOF(m, payload); err != nil {
			return false, err
		}
		return false, nil
	case m >= markerSOF0 && m <= 0xCF && m != markerDHT && m != markerJPG && m != markerDAC:
		return false, unsupported("coding process %#x", m)
	case m == markerDNL:
		return false, unsupported("DNL marker")
	case m == markerSOS:
		if err := h.parseSOS(payload); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (h *header) parseDQT(p []byte) error {
	for len(p) > 0 {
		pq, tq := int(p[0]>>4), int(p[0]&15)
		if pq > 1 || tq > 3 {
			return corrupt("DQT precision %d table %d", pq, tq)
		}
		size := 1 + 64*(pq+1)
		if len(p) < size {
			return corrupt("short DQT")
		}
		t := quantTable{present: true, wide: pq == 1}
		for i := 0; i < 64; i++ {
			if pq == 0 {
				t.values[i] = uint16(p[1+i])
			} else {
				t.values[i] = uint16(p[1+2*i])<<8 | uint16(p[2+2*i])
			}
		}
		h.quant[tq] = t
		p = p[size:]
	}
	return nil
}

func (h *header) parseDHT(p []byte) error {
	for len(p) > 0 {
		if len(p) < 17 {
			return corrupt("short DHT")
		}
		tc, th := int(p[0]>>4), int(p[0]&15)
		if tc > 1 || th > 3 {
			return corrupt("DHT class %d table %d", tc, th)
		}
		s := huffSpec{present: true}
		total := 0
		for i := 0; i < 16; i++ {
			s.counts[i] = p[1+i]
			total += int(p[1+i])
		}
		if total > 256 || len(p) < 17+total {
			return corrupt("DHT with %d symbols", total)
		}
		s.symbols = append([]byte(nil), p[17:17+total]...)
		if tc == 0 {
			h.dc[th] = s
		} else {
			h.ac[th] = s
		}
		p = p[17+total:]
	}
	return nil
}

func (h *header) parseSOF(m byte, p []byte) error {
	if len(p) < 6 {
		return corrupt("short SOF")
	}
	if p[0] != 8 {
		return unsupported("%d-bit samples", p[0])
	}
	height := int(p[1])<<8 | int(p[2])
	width := int(p[3])<<8 | int(p[4])
	nc := int(p[5])
	if width == 0 {
		return corrupt("zero width")
	}
	if height == 0 {
		return unsupported("height defined by DNL")
	}
	if nc != 1 && nc != 3 && nc != 4 {
		return unsupported("%d components", nc)
	}
	if len(p) != 6+3*nc {
		return corrupt("SOF length %d for %d components", len(p), nc)
	}
	fr := frame{marker: m, width: width, height: height}
	blocks := 0
	for i := 0; i < nc; i++ {
		c := component{id: p[6+3*i], h: int(p[7+3*i] >> 4), v: int(p[7+3*i] & 15), quant: int(p[8+3*i])}
		if c.h < 1 || c.h > 4 || c.v < 1 || c.v > 4 || c.quant > 3 {
			return corrupt("component %d: sampling %dx%d table %d", i, c.h, c.v, c.quant)
		}
		for _, o := range fr.comps {
			if o.id == c.id {
				return corrupt("repeated component id %d", c.id)
			}
		}
		if nc == 1 {
			c.h, c.v = 1, 1
		}
		blocks += c.h * c.v
		fr.hmax, fr.vmax = max(fr.hmax, c.h), max(fr.vmax, c.v)
		fr.comps = append(fr.comps, c)
	}
	if blocks > maxMCUBlocks {
		return corrupt("%d blocks in an MCU", blocks)
	}
	fr.mcuW, fr.mcuH = 8*fr.hmax, 8*fr.vmax
	fr.mcusX = (width + fr.mcuW - 1) / fr.mcuW
	fr.mcusY = (height + fr.mcuH - 1) / fr.mcuH
	for i := range fr.comps {
		c := &fr.comps[i]
		c.blocksWide, c.blocksHigh = fr.mcusX*c.h, fr.mcusY*c.v
	}
	h.fr, h.hasFrame = fr, true
	return nil
}

func (h *header) parseSOS(p []byte) error {
	if !h.hasFrame {
		return corrupt("scan before the frame header")
	}
	if len(p) < 1 {
		return corrupt("short SOS")
	}
	ns := int(p[0])
	if len(p) != 4+2*ns {
		return corrupt("SOS length %d for %d components", len(p), ns)
	}
	nc := len(h.fr.comps)
	if ns != nc {
		return unsupported("scan of %d components in a frame of %d", ns, nc)
	}
	ss, se, a := p[1+2*ns], p[2+2*ns], p[3+2*ns]
	if ss != 0 || se != 63 || a != 0 {
		return unsupported("spectral selection %d-%d, approximation %#x", ss, se, a)
	}
	seen := make([]bool, nc)
	h.scan = h.scan[:0]
	for i := 0; i < ns; i++ {
		id, tables := p[1+2*i], p[2+2*i]
		at := -1
		for j := range h.fr.comps {
			if h.fr.comps[j].id == id {
				at = j
			}
		}
		if at < 0 || seen[at] {
			return corrupt("scan component %d", id)
		}
		seen[at] = true
		td, ta := int(tables>>4), int(tables&15)
		if td > 3 || ta > 3 {
			return corrupt("scan tables %d/%d", td, ta)
		}
		h.fr.comps[at].dcTable, h.fr.comps[at].acTable = td, ta
		h.scan = append(h.scan, at)
	}
	for _, i := range h.scan {
		c := h.fr.comps[i]
		if !h.dc[c.dcTable].present || !h.ac[c.acTable].present {
			return corrupt("component %d uses a missing Huffman table", c.id)
		}
		if !h.quant[c.quant].present {
			return corrupt("component %d uses a missing quantization table", c.id)
		}
	}
	return nil
}

func isExif(payload []byte) bool {
	return len(payload) >= exifPrefix && bytes.HasPrefix(payload, []byte{'E', 'x', 'i', 'f', 0}) && (payload[5] == 0 || payload[5] == 0xFF)
}

func isXMP(payload []byte) bool {
	return bytes.HasPrefix(payload, []byte(xmpPrefix))
}

func isMPF(payload []byte) bool {
	return bytes.HasPrefix(payload, []byte{'M', 'P', 'F', 0})
}
