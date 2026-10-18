package raw

import (
	"context"
	"encoding/binary"
	"image"
	"io"
	"math"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/meta/exif"
	"github.com/oops1/PixGo/internal/rawdev"
)

const (
	dngNewSubFileType   uint16 = 0x00fe
	dngImageWidth       uint16 = 0x0100
	dngImageLength      uint16 = 0x0101
	dngBitsPerSample    uint16 = 0x0102
	dngPhotometric      uint16 = 0x0106
	dngSamplesPerPixel  uint16 = 0x0115
	dngRowsPerStrip     uint16 = 0x0116
	dngPlanarConfig     uint16 = 0x011c
	dngPredictor        uint16 = 0x013d
	dngTileWidth        uint16 = 0x0142
	dngTileLength       uint16 = 0x0143
	dngTileOffsets      uint16 = 0x0144
	dngTileByteCounts   uint16 = 0x0145
	dngSampleFormat     uint16 = 0x0153
	dngCFARepeatDim     uint16 = 0x828d
	dngCFAPattern       uint16 = 0x828e
	dngCFAPlaneColor    uint16 = 0xc616
	dngCFALayout        uint16 = 0xc617
	dngLinearization    uint16 = 0xc618
	dngBlackRepeatDim   uint16 = 0xc619
	dngBlackLevel       uint16 = 0xc61a
	dngBlackDeltaH      uint16 = 0xc61b
	dngBlackDeltaV      uint16 = 0xc61c
	dngWhiteLevel       uint16 = 0xc61d
	dngCropOrigin       uint16 = 0xc61f
	dngCropSize         uint16 = 0xc620
	dngColorMatrix1     uint16 = 0xc621
	dngColorMatrix2     uint16 = 0xc622
	dngCalibration1     uint16 = 0xc623
	dngCalibration2     uint16 = 0xc624
	dngAnalogBalance    uint16 = 0xc627
	dngAsShotNeutral    uint16 = 0xc628
	dngAsShotWhiteXY    uint16 = 0xc629
	dngBaselineExposure uint16 = 0xc62a
	dngIlluminant1      uint16 = 0xc65a
	dngIlluminant2      uint16 = 0xc65b
	dngActiveArea       uint16 = 0xc68d
	dngForwardMatrix1   uint16 = 0xc714
	dngForwardMatrix2   uint16 = 0xc715

	photometricCFA       = 32803
	photometricLinearRaw = 34892

	compressionNone    = 1
	compressionDeflate = 8
	compressionAdobe   = 32946

	predictorNone        = 1
	predictorHorizontal  = 2
	predictorHorizontal2 = 34892
	predictorHorizontal4 = 34893

	maxChunks       = 1 << 18
	maxTableEntries = 1 << 16
	maxNumberBytes  = 4 << 20
	maxRawIFDs      = 40
)

type dngChunk struct {
	x, y, w, h     int
	codedW, codedH int
	offset         int64
	length         int64
}

type dngPlan struct {
	width, height int
	spp, bps      int
	photometric   int
	compression   int
	predictor     int
	order         binary.ByteOrder
	limits        codec.Limits
	chunks        []dngChunk
	area          image.Rectangle
	params        rawdev.Params
	orientation   codec.Orientation
	hasOrient     bool
}

var develop atomic.Bool

func init() { develop.Store(true) }

func (Decoder) SetDevelop(on bool) { develop.Store(on) }

func (Decoder) Develops() bool { return develop.Load() }

type tagSet []*exif.Directory

func (t tagSet) entry(tag uint16) (exif.Entry, bool) {
	for _, d := range t {
		if e, ok := d.Find(tag); ok {
			return e, true
		}
	}
	return exif.Entry{}, false
}

func numbers(e exif.Entry, limit uint64) ([]float64, bool) {
	size := e.Type.Size()
	if size == 0 || e.Type == exif.TypeASCII {
		return nil, false
	}
	raw, ok := e.RawMax(limit)
	if !ok || len(raw)%size != 0 {
		return nil, false
	}
	order := e.ByteOrder()
	out := make([]float64, len(raw)/size)
	for i := range out {
		c := raw[i*size : (i+1)*size]
		var v float64
		switch e.Type {
		case exif.TypeByte, exif.TypeUndefined:
			v = float64(c[0])
		case exif.TypeSByte:
			v = float64(int8(c[0]))
		case exif.TypeShort:
			v = float64(order.Uint16(c))
		case exif.TypeSShort:
			v = float64(int16(order.Uint16(c)))
		case exif.TypeLong:
			v = float64(order.Uint32(c))
		case exif.TypeSLong:
			v = float64(int32(order.Uint32(c)))
		case exif.TypeRational:
			den := float64(order.Uint32(c[4:]))
			if den == 0 {
				return nil, false
			}
			v = float64(order.Uint32(c)) / den
		case exif.TypeSRational:
			den := float64(int32(order.Uint32(c[4:])))
			if den == 0 {
				return nil, false
			}
			v = float64(int32(order.Uint32(c))) / den
		case exif.TypeFloat:
			v = float64(math.Float32frombits(order.Uint32(c)))
		case exif.TypeDouble:
			v = math.Float64frombits(order.Uint64(c))
		default:
			return nil, false
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

func (t tagSet) nums(tag uint16, limit uint64) ([]float64, bool) {
	e, ok := t.entry(tag)
	if !ok {
		return nil, false
	}
	return numbers(e, limit)
}

func (t tagSet) integer(tag uint16, def int) int {
	v, ok := t.nums(tag, 64)
	if !ok || len(v) == 0 || v[0] < 0 || v[0] > math.MaxInt32 {
		return def
	}
	return int(v[0])
}

func (t tagSet) matrix(tag uint16) (rawdev.Matrix, bool, bool) {
	e, present := t.entry(tag)
	if !present {
		return rawdev.Matrix{}, false, true
	}
	v, ok := numbers(e, 1024)
	if !ok || len(v) != 9 {
		return rawdev.Matrix{}, false, false
	}
	var m rawdev.Matrix
	copy(m[:], v)
	return m, true, true
}

func rawCandidates(rd *exif.Reader, ifd0 *exif.Directory) []*exif.Directory {
	out := []*exif.Directory{ifd0}
	seen := map[uint32]bool{ifd0.Offset: true}
	add := func(offset uint32) {
		if offset == 0 || seen[offset] || len(out) >= maxRawIFDs {
			return
		}
		seen[offset] = true
		if dir, err := rd.Directory(offset); err == nil {
			out = append(out, dir)
		}
	}
	if e, ok := ifd0.Find(tagSubIFDs); ok {
		for i := 0; i < int(e.Count) && i < maxSubIFDs; i++ {
			if offset, ok := e.Uint(i); ok {
				add(offset)
			}
		}
	}
	add(ifd0.Next)
	return out
}

func isRawIFD(d *exif.Directory) bool {
	e, ok := d.Find(dngPhotometric)
	if !ok {
		return false
	}
	v, ok := e.Uint(0)
	if !ok || (v != photometricCFA && v != photometricLinearRaw) {
		return false
	}
	if sub, ok := d.Find(dngNewSubFileType); ok {
		if s, ok := sub.Uint(0); ok && s&1 != 0 {
			return false
		}
	}
	return true
}

func pixelsOf(d *exif.Directory) int64 {
	w, ok1 := dirUint(d, dngImageWidth)
	h, ok2 := dirUint(d, dngImageLength)
	if !ok1 || !ok2 {
		return 0
	}
	return int64(w) * int64(h)
}

func dirUint(d *exif.Directory, tag uint16) (uint32, bool) {
	e, ok := d.Find(tag)
	if !ok {
		return 0, false
	}
	return e.Uint(0)
}

func planDNG(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (*dngPlan, bool) {
	lim = guard.Limits(lim)
	if size < tiffHeaderSize || size > lim.MaxFileBytes || ctx.Err() != nil {
		return nil, false
	}
	rd, err := exif.NewReaderAt(r, 0, size)
	if err != nil {
		return nil, false
	}
	ifd0, err := rd.Directory(rd.FirstOffset())
	if err != nil {
		return nil, false
	}
	if _, ok := ifd0.Find(tagDNGVersion); !ok {
		return nil, false
	}
	var best *exif.Directory
	for _, d := range rawCandidates(rd, ifd0) {
		if isRawIFD(d) && (best == nil || pixelsOf(d) > pixelsOf(best)) {
			best = d
		}
	}
	if best == nil || ctx.Err() != nil {
		return nil, false
	}
	plan := &dngPlan{order: rd.ByteOrder(), limits: lim}
	tags := tagSet{best, ifd0}
	if !plan.geometry(tags, lim) || !plan.layout(tags) || !plan.levels(tags) || !plan.colour(tags) {
		return nil, false
	}
	plan.orientation, plan.hasOrient = rd.Orientation()
	if !plan.hasOrient {
		plan.orientation = codec.OrientNormal
	}
	return plan, true
}

func (p *dngPlan) geometry(t tagSet, lim codec.Limits) bool {
	w, h := t.integer(dngImageWidth, 0), t.integer(dngImageLength, 0)
	if w <= 0 || h <= 0 || lim.Check(w, h) != nil {
		return false
	}
	p.width, p.height = w, h
	p.photometric = t.integer(dngPhotometric, 0)
	p.spp = t.integer(dngSamplesPerPixel, 1)
	switch {
	case p.photometric == photometricCFA && p.spp == 1:
	case p.photometric == photometricLinearRaw && p.spp == 3:
	default:
		return false
	}
	if t.integer(dngPlanarConfig, 1) != 1 {
		return false
	}
	if f, ok := t.nums(dngSampleFormat, 16); ok {
		for _, v := range f {
			if v != 1 {
				return false
			}
		}
	}
	bits, ok := t.nums(dngBitsPerSample, 16)
	if !ok || len(bits) != p.spp {
		return false
	}
	for _, b := range bits {
		if b != bits[0] || b < 8 || b > 16 || b != math.Trunc(b) {
			return false
		}
	}
	p.bps = int(bits[0])
	p.compression = t.integer(tagCompression, compressionNone)
	if p.compression == compressionAdobe {
		p.compression = compressionDeflate
	}
	p.predictor = t.integer(dngPredictor, predictorNone)
	switch p.compression {
	case compressionNone, compressionJPEG:
	case compressionDeflate:
		switch p.predictor {
		case predictorNone, predictorHorizontal, predictorHorizontal2, predictorHorizontal4:
		default:
			return false
		}
	default:
		return false
	}
	p.area = image.Rect(0, 0, w, h)
	if v, ok := t.nums(dngActiveArea, 64); ok && len(v) == 4 {
		a := image.Rect(int(v[1]), int(v[0]), int(v[3]), int(v[2]))
		if v[0] >= 0 && v[1] >= 0 && v[2] <= float64(h) && v[3] <= float64(w) && !a.Empty() {
			p.area = a
		}
	}
	return true
}

func (p *dngPlan) layout(t tagSet) bool {
	tw, th := t.integer(dngTileWidth, 0), t.integer(dngTileLength, 0)
	if tw > 0 || th > 0 {
		return p.tileChunks(t, tw, th)
	}
	return p.stripChunks(t)
}

func chunkTable(t tagSet, offTag, lenTag uint16, n int) ([]float64, []float64, bool) {
	oe, ok := t.entry(offTag)
	if !ok || int64(oe.Count) < int64(n) {
		return nil, nil, false
	}
	ce, ok := t.entry(lenTag)
	if !ok || int64(ce.Count) < int64(n) {
		return nil, nil, false
	}
	offsets, ok := numbers(oe, maxNumberBytes)
	if !ok {
		return nil, nil, false
	}
	counts, ok := numbers(ce, maxNumberBytes)
	if !ok {
		return nil, nil, false
	}
	return offsets, counts, true
}

func (p *dngPlan) stripChunks(t tagSet) bool {
	rps := t.integer(dngRowsPerStrip, p.height)
	if rps <= 0 || rps > p.height {
		rps = p.height
	}
	n := (p.height + rps - 1) / rps
	if n > maxChunks {
		return false
	}
	offsets, counts, ok := chunkTable(t, tagStripOffsets, tagStripCounts, n)
	if !ok {
		return false
	}
	for i := 0; i < n; i++ {
		y := i * rps
		rows := min(rps, p.height-y)
		p.chunks = append(p.chunks, dngChunk{x: 0, y: y, w: p.width, h: rows, codedW: p.width, codedH: rows, offset: int64(offsets[i]), length: int64(counts[i])})
	}
	return true
}

func (p *dngPlan) tileChunks(t tagSet, tw, th int) bool {
	if tw <= 0 || th <= 0 || tw > 1<<16 || th > 1<<16 || int64(tw)*int64(th) > p.limits.MaxPixels {
		return false
	}
	across, down := (p.width+tw-1)/tw, (p.height+th-1)/th
	if int64(across)*int64(down) > maxChunks {
		return false
	}
	n := across * down
	offsets, counts, ok := chunkTable(t, dngTileOffsets, dngTileByteCounts, n)
	if !ok {
		return false
	}
	for i := 0; i < n; i++ {
		x, y := (i%across)*tw, (i/across)*th
		p.chunks = append(p.chunks, dngChunk{x: x, y: y, w: min(tw, p.width-x), h: min(th, p.height-y), codedW: tw, codedH: th, offset: int64(offsets[i]), length: int64(counts[i])})
	}
	return true
}

func (p *dngPlan) levels(t tagSet) bool {
	for i := range p.params.Black {
		p.params.Black[i] = 0
		p.params.White[i] = float64(int(1)<<uint(p.bps) - 1)
	}
	if e, ok := t.entry(dngLinearization); ok {
		table, ok := numbers(e, 2*maxTableEntries)
		if !ok || len(table) == 0 || len(table) > maxTableEntries {
			return false
		}
		p.params.Table = make([]uint16, len(table))
		for i, v := range table {
			if v < 0 || v > 65535 {
				return false
			}
			p.params.Table[i] = uint16(v)
		}
		last := min(int(1)<<uint(p.bps)-1, len(table)-1)
		for i := range p.params.White {
			p.params.White[i] = table[last]
		}
	}
	cells, rows, cols := 1, 1, 1
	if v, ok := t.nums(dngBlackRepeatDim, 16); ok && len(v) == 2 && v[0] >= 1 && v[1] >= 1 && v[0] <= 64 && v[1] <= 64 {
		rows, cols = int(v[0]), int(v[1])
		cells = rows * cols
	}
	if v, ok := t.nums(dngBlackLevel, 1<<16); ok && len(v) >= cells*p.spp {
		p.assignBlack(v, rows, cols)
	}
	if v, ok := t.nums(dngWhiteLevel, 64); ok && len(v) >= 1 {
		for i := range p.params.White {
			w := v[0]
			if p.spp == 3 && i < len(v) {
				w = v[i]
			}
			p.params.White[i] = w
		}
	}
	for i := range p.params.White {
		if !(p.params.White[i] > p.params.Black[i]) {
			return false
		}
	}
	if p.spp == 1 {
		aw, ah := p.area.Dx(), p.area.Dy()
		if v, ok := t.nums(dngBlackDeltaH, 8*maxNumberBytes/8); ok && len(v) == aw {
			p.params.DeltaH = v
		}
		if v, ok := t.nums(dngBlackDeltaV, 8*maxNumberBytes/8); ok && len(v) == ah {
			p.params.DeltaV = v
		}
	}
	p.params.Crop = p.cropRect(t)
	return true
}

func (p *dngPlan) assignBlack(v []float64, rows, cols int) {
	if p.spp == 3 {
		for s := 0; s < 3; s++ {
			sum, n := 0.0, 0
			for c := 0; c < rows*cols; c++ {
				sum += v[c*3+s]
				n++
			}
			p.params.Black[s] = sum / float64(n)
		}
		return
	}
	if rows <= 2 && cols <= 2 {
		for pos := 0; pos < 4; pos++ {
			p.params.Black[pos] = v[(pos>>1%rows)*cols+(pos&1)%cols]
		}
		return
	}
	sum := 0.0
	for c := 0; c < rows*cols; c++ {
		sum += v[c]
	}
	for pos := range p.params.Black {
		p.params.Black[pos] = sum / float64(rows*cols)
	}
}

func (p *dngPlan) cropRect(t tagSet) image.Rectangle {
	full := image.Rect(0, 0, p.area.Dx(), p.area.Dy())
	origin, size := [2]float64{0, 0}, [2]float64{float64(full.Dx()), float64(full.Dy())}
	if v, ok := t.nums(dngCropOrigin, 64); ok && len(v) == 2 {
		origin = [2]float64{v[0], v[1]}
	}
	if v, ok := t.nums(dngCropSize, 64); ok && len(v) == 2 {
		size = [2]float64{v[0], v[1]}
	}
	if origin[0] < 0 || origin[1] < 0 || size[0] < 1 || size[1] < 1 || origin[0]+size[0] > float64(full.Dx())+0.5 || origin[1]+size[1] > float64(full.Dy())+0.5 {
		return full
	}
	crop := image.Rect(int(math.Round(origin[0])), int(math.Round(origin[1])), int(math.Round(origin[0]+size[0])), int(math.Round(origin[1]+size[1])))
	crop = crop.Intersect(full)
	if crop.Empty() {
		return full
	}
	return crop
}

func (p *dngPlan) colour(t tagSet) bool {
	if p.spp == 1 {
		if v, ok := t.nums(dngCFARepeatDim, 16); !ok || len(v) != 2 || v[0] != 2 || v[1] != 2 {
			return false
		}
		pattern, ok := t.nums(dngCFAPattern, 16)
		if !ok || len(pattern) != 4 {
			return false
		}
		for i, v := range pattern {
			if v < 0 || v > 2 || v != math.Trunc(v) {
				return false
			}
			p.params.CFA[i] = uint8(v)
		}
		if !rawdev.ValidateCFA(p.params.CFA) {
			return false
		}
		if v, ok := t.nums(dngCFALayout, 16); ok && (len(v) != 1 || v[0] != 1) {
			return false
		}
	}
	if v, ok := t.nums(dngCFAPlaneColor, 16); ok && (len(v) != 3 || v[0] != 0 || v[1] != 1 || v[2] != 2) {
		return false
	}
	var c rawdev.Color
	c.Analog = [3]float64{1, 1, 1}
	for i, tag := range [2]uint16{dngColorMatrix1, dngColorMatrix2} {
		m, present, ok := t.matrix(tag)
		if !ok {
			return false
		}
		c.ColorMatrix[i], c.HasCM[i] = m, present
	}
	for i, tag := range [2]uint16{dngForwardMatrix1, dngForwardMatrix2} {
		m, present, ok := t.matrix(tag)
		if !ok {
			return false
		}
		c.Forward[i], c.HasFM[i] = m, present
	}
	for i, tag := range [2]uint16{dngCalibration1, dngCalibration2} {
		m, present, ok := t.matrix(tag)
		if !ok {
			return false
		}
		c.Calibration[i], c.HasCC[i] = m, present
	}
	c.Illuminant = [2]uint16{uint16(t.integer(dngIlluminant1, 0)), uint16(t.integer(dngIlluminant2, 0))}
	if v, ok := t.nums(dngAnalogBalance, 64); ok {
		if len(v) != 3 {
			return false
		}
		copy(c.Analog[:], v)
	}
	if v, ok := t.nums(dngAsShotNeutral, 64); ok {
		if len(v) != 3 {
			return false
		}
		n := [3]float64{v[0], v[1], v[2]}
		c.Neutral = &n
	} else if v, ok := t.nums(dngAsShotWhiteXY, 64); ok {
		if len(v) != 2 {
			return false
		}
		xy := [2]float64{v[0], v[1]}
		c.WhiteXY = &xy
	}
	render, err := c.Resolve()
	if err != nil {
		return false
	}
	p.params.Render = render
	if v, ok := t.nums(dngBaselineExposure, 64); ok && len(v) == 1 {
		p.params.Exposure = math.Min(math.Max(v[0], -4), 4)
	}
	return true
}

func (p *dngPlan) info() codec.Info {
	crop := p.params.Crop
	return codec.Info{
		Format:      codec.RAW,
		FormatName:  dngName,
		Width:       crop.Dx(),
		Height:      crop.Dy(),
		Frames:      1,
		Orientation: p.orientation,
		BitDepth:    3 * p.bps,
	}
}

const dngName = "DNG"
