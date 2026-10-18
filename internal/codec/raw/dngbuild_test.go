package raw

import (
	"bytes"
	"compress/zlib"
	"math"

	"github.com/oops1/PixGo/internal/rawdev/rawdevtest"
)

const (
	typeRational  = 5
	typeSRational = 10
	typeSLong     = 9
	typeDouble    = 12
	typeFloat     = 11
)

type decoy struct {
	w, h    int
	subType int
}

type dngSpec struct {
	order        byteOrder
	w, h         int
	spp          int
	bps          int
	cfa          [4]uint8
	pix          []uint16
	compression  int
	predictor    int
	tileW        int
	tileH        int
	rowsPerStrip int
	ljComps      int
	ljPredictor  int
	ljLengths    [17]int
	blackDim     [2]int
	black        []float64
	white        []float64
	table        []uint16
	area         []int
	cropOrigin   []float64
	cropSize     []float64
	exposure     float64
	hasExposure  bool
	matrix1      *[9]float64
	matrix2      *[9]float64
	illum1       int
	illum2       int
	forward1     *[9]float64
	neutral      []float64
	whiteXY      []float64
	orientation  uint16
	preview      []byte
	rawInIFD0    bool
	extra        []entry
	skipCFA      bool
	deltaH       []float64
	deltaV       []float64
	photometric  int
	sampleFmt    int
	override     []entry
	decoys       []decoy
	drop         []uint16
}

func rationalData(order byteOrder, signed bool, values ...float64) []byte {
	var out []byte
	for _, v := range values {
		den := int64(10000)
		num := int64(math.Round(v * float64(den)))
		if signed {
			out = order.AppendUint32(out, uint32(int32(num)))
			out = order.AppendUint32(out, uint32(int32(den)))
			continue
		}
		out = order.AppendUint32(out, uint32(num))
		out = order.AppendUint32(out, uint32(den))
	}
	return out
}

func shortData(order byteOrder, values ...int) []byte {
	var out []byte
	for _, v := range values {
		out = order.AppendUint16(out, uint16(v))
	}
	return out
}

func longData(order byteOrder, values ...int) []byte {
	var out []byte
	for _, v := range values {
		out = order.AppendUint32(out, uint32(v))
	}
	return out
}

func (t *tiffFile) dataEntry(tag uint16, typ uint16, count int, data []byte) entry {
	if len(data) <= 4 {
		return entry{tag: tag, typ: typ, count: uint32(count), inline: data}
	}
	return entry{tag: tag, typ: typ, count: uint32(count), ref: t.later(data)}
}

func (t *tiffFile) shorts(tag uint16, values ...int) entry {
	return t.dataEntry(tag, typeShort, len(values), shortData(t.order, values...))
}

func (t *tiffFile) longs(tag uint16, values ...int) entry {
	return t.dataEntry(tag, typeLong, len(values), longData(t.order, values...))
}

func (t *tiffFile) rationals(tag uint16, values ...float64) entry {
	return t.dataEntry(tag, typeRational, len(values), rationalData(t.order, false, values...))
}

func (t *tiffFile) srationals(tag uint16, values ...float64) entry {
	return t.dataEntry(tag, typeSRational, len(values), rationalData(t.order, true, values...))
}

func (t *tiffFile) bytesEntry(tag uint16, values ...byte) entry {
	return t.dataEntry(tag, typeByte, len(values), values)
}

func (s *dngSpec) samplesAt(x, y, k int) uint16 {
	if x >= s.w || y >= s.h {
		return 0
	}
	return s.pix[(y*s.w+x)*s.spp+k]
}

func (s *dngSpec) chunkSamples(x0, y0, w, h int) []uint16 {
	out := make([]uint16, 0, w*h*s.spp)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for k := 0; k < s.spp; k++ {
				out = append(out, s.samplesAt(x0+x, y0+y, k))
			}
		}
	}
	return out
}

func (s *dngSpec) packBits(samples []uint16, w int) []byte {
	var out []byte
	perRow := w * s.spp
	for start := 0; start < len(samples); start += perRow {
		row := samples[start : start+perRow]
		switch s.bps {
		case 8:
			for _, v := range row {
				out = append(out, byte(v))
			}
		case 16:
			for _, v := range row {
				out = s.order.AppendUint16(out, v)
			}
		default:
			var acc uint32
			have := 0
			for _, v := range row {
				acc = acc<<uint(s.bps) | uint32(v)
				have += s.bps
				for have >= 8 {
					out = append(out, byte(acc>>uint(have-8)))
					have -= 8
				}
				acc &= 1<<uint(have) - 1
			}
			if have > 0 {
				out = append(out, byte(acc<<uint(8-have)))
			}
		}
	}
	return out
}

func (s *dngSpec) compress(samples []uint16, w, h int) []byte {
	switch s.compression {
	case compressionJPEG:
		comps := s.ljComps
		if comps == 0 {
			comps = s.spp
		}
		lw := w * s.spp / comps
		pred := s.ljPredictor
		if pred == 0 {
			pred = 1
		}
		return rawdevtest.EncodeLJ92(lw, h, comps, s.bps, samples, rawdevtest.LJOptions{Predictor: pred, Lengths: s.ljLengths})
	case compressionDeflate, compressionAdobe:
		work := append([]uint16(nil), samples...)
		step := 0
		switch s.predictor {
		case predictorHorizontal:
			step = s.spp
		case predictorHorizontal2:
			step = 2 * s.spp
		case predictorHorizontal4:
			step = 4 * s.spp
		}
		if step > 0 {
			per := w * s.spp
			for y := 0; y < h; y++ {
				row := work[y*per : (y+1)*per]
				for i := per - 1; i >= step; i-- {
					row[i] = (row[i] - row[i-step]) & uint16(1<<uint(s.bps)-1)
				}
			}
		}
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		zw.Write(s.packBits(work, w))
		zw.Close()
		return buf.Bytes()
	}
	return s.packBits(samples, w)
}

func (s *dngSpec) defaults() {
	if s.order == nil {
		s.order = little
	}
	if s.spp == 0 {
		s.spp = 1
	}
	if s.bps == 0 {
		s.bps = 14
	}
	if s.compression == 0 {
		s.compression = compressionNone
	}
	if s.photometric == 0 {
		s.photometric = photometricCFA
		if s.spp == 3 {
			s.photometric = photometricLinearRaw
		}
	}
}

func (s *dngSpec) encodeChunks() ([][]byte, []int) {
	var data [][]byte
	var counts []int
	add := func(b []byte) {
		data = append(data, b)
		counts = append(counts, len(b))
	}
	if s.tileW > 0 {
		across, down := (s.w+s.tileW-1)/s.tileW, (s.h+s.tileH-1)/s.tileH
		for ty := 0; ty < down; ty++ {
			for tx := 0; tx < across; tx++ {
				add(s.compress(s.chunkSamples(tx*s.tileW, ty*s.tileH, s.tileW, s.tileH), s.tileW, s.tileH))
			}
		}
		return data, counts
	}
	rps := s.rowsPerStrip
	if rps == 0 {
		rps = s.h
	}
	for y := 0; y < s.h; y += rps {
		rows := min(rps, s.h-y)
		add(s.compress(s.chunkSamples(0, y, s.w, rows), s.w, rows))
	}
	return data, counts
}

func (s *dngSpec) build() []byte {
	s.defaults()
	data, counts := s.encodeChunks()
	offsets := make([]int, len(data))
	_, refs := s.assemble(data, counts, offsets)
	for i, r := range refs {
		offsets[i] = int(r.offset)
	}
	out, _ := s.assemble(data, counts, offsets)
	return out
}

func (s *dngSpec) assemble(chunkData [][]byte, counts, offsets []int) ([]byte, []*blobRef) {
	order := s.order
	t := newTIFF(order, [4]byte{'I', 'I', 42, 0})
	if order == big {
		t = newTIFF(order, [4]byte{'M', 'M', 0, 42})
	}
	var refs []*blobRef
	for _, d := range chunkData {
		refs = append(refs, t.later(d))
	}
	var layout []entry
	if s.tileW > 0 {
		layout = []entry{t.longs(0x0142, s.tileW), t.longs(0x0143, s.tileH), t.longs(0x0144, offsets...), t.longs(0x0145, counts...)}
	} else {
		rps := s.rowsPerStrip
		if rps == 0 {
			rps = s.h
		}
		layout = []entry{t.longs(0x0116, rps), t.longs(tagStripOffsets, offsets...), t.longs(tagStripCounts, counts...)}
	}
	bits := make([]int, s.spp)
	for i := range bits {
		bits[i] = s.bps
	}
	rawEntries := join(layout, []entry{
		longEntry(0x0100, uint32(s.w)), longEntry(0x0101, uint32(s.h)),
		t.shorts(0x0102, bits...),
		shortEntry(tagCompression, uint16(s.compression)),
		shortEntry(0x0106, uint16(s.photometric)),
		shortEntry(0x0115, uint16(s.spp)),
		shortEntry(0x011c, 1),
		longEntry(0x00fe, 0),
	})
	if s.predictor != 0 {
		rawEntries = append(rawEntries, shortEntry(0x013d, uint16(s.predictor)))
	}
	if s.sampleFmt != 0 {
		rawEntries = append(rawEntries, shortEntry(0x0153, uint16(s.sampleFmt)))
	}
	if s.spp == 1 && !s.skipCFA {
		rawEntries = append(rawEntries, t.shorts(0x828d, 2, 2), t.bytesEntry(0x828e, s.cfa[0], s.cfa[1], s.cfa[2], s.cfa[3]))
	}
	if len(s.black) > 0 {
		dim := s.blackDim
		if dim == [2]int{} {
			dim = [2]int{1, 1}
		}
		rawEntries = append(rawEntries, t.shorts(0xc619, dim[0], dim[1]), t.rationals(0xc61a, s.black...))
	}
	white := s.white
	if len(white) == 0 {
		white = []float64{float64(int(1)<<uint(s.bps) - 1)}
	}
	rawEntries = append(rawEntries, t.longs(0xc61d, floatsToInts(white)...))
	if len(s.table) > 0 {
		vals := make([]int, len(s.table))
		for i, v := range s.table {
			vals[i] = int(v)
		}
		rawEntries = append(rawEntries, t.shorts(0xc618, vals...))
	}
	if s.area != nil {
		rawEntries = append(rawEntries, t.longs(0xc68d, s.area...))
	}
	if s.cropOrigin != nil {
		rawEntries = append(rawEntries, t.rationals(0xc61f, s.cropOrigin...))
	}
	if s.cropSize != nil {
		rawEntries = append(rawEntries, t.rationals(0xc620, s.cropSize...))
	}
	if s.deltaH != nil {
		rawEntries = append(rawEntries, t.srationals(0xc61b, s.deltaH...))
	}
	if s.deltaV != nil {
		rawEntries = append(rawEntries, t.srationals(0xc61c, s.deltaV...))
	}
	rawEntries = append(rawEntries, s.extra...)

	colour := []entry{dngVersion(), shortEntry(tagOrient, uint16(max(s.orientation, 1)))}
	if s.matrix1 != nil {
		colour = append(colour, t.srationals(0xc621, s.matrix1[:]...), t.shorts(0xc65a, s.illum1))
	}
	if s.matrix2 != nil {
		colour = append(colour, t.srationals(0xc622, s.matrix2[:]...), t.shorts(0xc65b, s.illum2))
	}
	if s.forward1 != nil {
		colour = append(colour, t.srationals(0xc714, s.forward1[:]...))
	}
	if s.neutral != nil {
		colour = append(colour, t.rationals(0xc628, s.neutral...))
	}
	if s.whiteXY != nil {
		colour = append(colour, t.rationals(0xc629, s.whiteXY...))
	}
	if s.hasExposure {
		colour = append(colour, t.srationals(0xc62a, s.exposure))
	}
	colour = append(colour, t.asciiEntry(tagMake, "PixGo test"))

	rawEntries = s.patch(rawEntries)
	colour = s.patch(colour)
	if s.rawInIFD0 {
		ifd0, _ := t.ifd(join(rawEntries, colour)...)
		t.setFirst(ifd0)
	} else {
		var subs []int
		rawSub, _ := t.ifd(rawEntries...)
		if s.preview != nil {
			large := t.later(s.preview)
			previewSub, _ := t.ifd(join(stripTags(large, compressionJPEG), []entry{longEntry(0x00fe, 1)})...)
			subs = append(subs, int(previewSub))
		}
		subs = append(subs, int(rawSub))
		for _, d := range s.decoys {
			blob := t.later(make([]byte, 2*d.w*d.h))
			decoySub, _ := t.ifd(
				longEntry(0x0100, uint32(d.w)), longEntry(0x0101, uint32(d.h)), shortEntry(0x0102, 16),
				shortEntry(tagCompression, 1), shortEntry(0x0106, photometricCFA), shortEntry(0x0115, 1), shortEntry(0x011c, 1),
				longEntry(0x00fe, uint32(d.subType)), refEntry(tagStripOffsets, blob), longEntry(tagStripCounts, uint32(2*d.w*d.h)),
				t.shorts(0x828d, 2, 2), t.bytesEntry(0x828e, 0, 1, 1, 2),
			)
			subs = append(subs, int(decoySub))
		}
		ifd0, _ := t.ifd(join(colour, []entry{t.longs(tagSubIFDs, subs...)})...)
		t.setFirst(ifd0)
	}
	return t.data(), refs
}

func (s *dngSpec) patch(list []entry) []entry {
	var out []entry
	for _, e := range list {
		dropped := false
		for _, tag := range s.drop {
			if tag == e.tag {
				dropped = true
			}
		}
		for _, o := range s.override {
			if o.tag == e.tag {
				dropped = true
			}
		}
		if !dropped {
			out = append(out, e)
		}
	}
	for _, o := range s.override {
		for _, e := range list {
			if e.tag == o.tag {
				out = append(out, o)
				break
			}
		}
	}
	return out
}

func floatsToInts(v []float64) []int {
	out := make([]int, len(v))
	for i, x := range v {
		out[i] = int(math.Round(x))
	}
	return out
}
