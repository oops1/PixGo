package tiff

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"image"
	"math/rand"
	"sort"
)

type rawTag struct {
	tag    uint16
	typ    uint16
	values []uint64
}

type fixture struct {
	bigTIFF      bool
	bigEndian    bool
	width        int
	height       int
	bits         int
	samples      int
	photometric  int
	extra        int
	compression  int
	predictor    int
	tileW, tileH int
	rowsPerStrip int
	palette      []uint16
	orientation  int
	values       []uint16
	alias        bool
	planar       int
	fillOrder    int
	sampleFormat int
	omit         []uint16
	custom       []rawTag
	noops        bool
	rawOverride  []byte
	claimW       int
	claimH       int
}

func (f fixture) declaredWidth() int {
	if f.claimW > 0 {
		return f.claimW
	}
	return f.width
}

func (f fixture) declaredHeight() int {
	if f.claimH > 0 {
		return f.claimH
	}
	return f.height
}

func (f fixture) byteOrder() order {
	if f.bigEndian {
		return binary.BigEndian
	}
	return binary.LittleEndian
}

func (f fixture) maxValue() int { return 1<<uint(f.bits) - 1 }

func (f *fixture) fill(seed int64) {
	rng := rand.New(rand.NewSource(seed))
	f.values = make([]uint16, f.width*f.height*f.samples)
	mask := f.maxValue()
	for y := 0; y < f.height; y++ {
		for x := 0; x < f.width; x++ {
			block := ((x/9)*7 + (y/5)*13) & 0xff
			for c := 0; c < f.samples; c++ {
				v := block*(c+1)*3 + x/2 + y
				if rng.Intn(9) == 0 {
					v = rng.Intn(1 << 16)
				}
				if f.bits == 16 {
					v = v*257 + rng.Intn(3)
				}
				f.values[(y*f.width+x)*f.samples+c] = uint16(v & mask)
			}
		}
	}
}

func makePalette(bits int, seed int64) []uint16 {
	rng := rand.New(rand.NewSource(seed))
	colors := 1 << uint(bits)
	words := make([]uint16, 3*colors)
	for i := range words {
		words[i] = uint16(rng.Intn(1 << 16))
	}
	return words
}

func (f fixture) geometry() (across, down, chunkW, chunkH int) {
	width, height := f.declaredWidth(), f.declaredHeight()
	if f.tileW > 0 {
		return (width + f.tileW - 1) / f.tileW, (height + f.tileH - 1) / f.tileH, f.tileW, f.tileH
	}
	rows := f.rowsPerStrip
	if rows <= 0 || rows > height {
		rows = height
	}
	return 1, (height + rows - 1) / rows, width, rows
}

func (f fixture) sampleAt(x, y, c int) uint16 {
	if x >= f.width || y >= f.height {
		return 0
	}
	return f.values[(y*f.width+x)*f.samples+c]
}

func (f fixture) rawChunk(cx, cy int) []byte {
	_, _, chunkW, chunkH := f.geometry()
	rows := chunkH
	if f.tileW == 0 {
		rows = min(chunkH, f.declaredHeight()-cy*chunkH)
	}
	var out []byte
	row := make([]uint16, chunkW*f.samples)
	for r := 0; r < rows; r++ {
		for x := 0; x < chunkW; x++ {
			for c := 0; c < f.samples; c++ {
				row[x*f.samples+c] = f.sampleAt(cx*chunkW+x, cy*chunkH+r, c)
			}
		}
		if f.predictor == 2 {
			for i := len(row) - 1; i >= f.samples; i-- {
				row[i] = (row[i] - row[i-f.samples]) & uint16(f.maxValue())
			}
		}
		out = append(out, f.packRow(row)...)
	}
	return out
}

func (f fixture) packRow(row []uint16) []byte {
	switch {
	case f.bits == 16:
		out := make([]byte, 0, len(row)*2)
		for _, v := range row {
			out = f.byteOrder().AppendUint16(out, v)
		}
		return out
	case f.bits == 8:
		out := make([]byte, len(row))
		for i, v := range row {
			out[i] = byte(v)
		}
		return out
	}
	out := make([]byte, (len(row)*f.bits+7)/8)
	for i, v := range row {
		position := i * f.bits
		out[position>>3] |= byte(v) << uint(8-f.bits-(position&7))
	}
	return out
}

func (f fixture) compress(raw []byte) []byte {
	switch f.compression {
	case 5:
		return lzwEncode(raw)
	case 8, 32946:
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		w.Write(raw)
		w.Close()
		return buf.Bytes()
	case 32773:
		return packBitsEncode(raw, f.noops)
	}
	return raw
}

type bitWriter struct {
	out   []byte
	acc   uint32
	count uint
}

func (w *bitWriter) write(code uint16, width uint) {
	w.acc = w.acc<<width | uint32(code)
	w.count += width
	for w.count >= 8 {
		w.out = append(w.out, byte(w.acc>>(w.count-8)))
		w.count -= 8
	}
	w.acc &= 1<<w.count - 1
}

func (w *bitWriter) flush() []byte {
	if w.count > 0 {
		w.out = append(w.out, byte(w.acc<<(8-w.count)))
		w.count = 0
	}
	return w.out
}

func lzwEncode(data []byte) []byte {
	const (
		clearCode = 256
		endCode   = 257
		tableMax  = 4094
	)
	var w bitWriter
	dict := make(map[uint32]uint16)
	next := 258
	width := uint(9)
	emit := func(code uint16) {
		if next >= 1<<width && width < 12 {
			width++
		}
		w.write(code, width)
	}
	emit(clearCode)
	if len(data) > 0 {
		current := uint16(data[0])
		for _, b := range data[1:] {
			key := uint32(current)<<8 | uint32(b)
			if code, ok := dict[key]; ok {
				current = code
				continue
			}
			emit(current)
			dict[key] = uint16(next)
			next++
			if next >= tableMax {
				emit(clearCode)
				dict = make(map[uint32]uint16)
				next = 258
				width = 9
			}
			current = uint16(b)
		}
		emit(current)
		next++
	}
	emit(endCode)
	return w.flush()
}

func packBitsEncode(data []byte, noops bool) []byte {
	var out []byte
	runAt := func(i int) int {
		run := 1
		for i+run < len(data) && data[i+run] == data[i] && run < 128 {
			run++
		}
		return run
	}
	for i := 0; i < len(data); {
		if run := runAt(i); run >= 3 {
			out = append(out, byte(257-run), data[i])
			i += run
		} else {
			start := i
			for i < len(data) && i-start < 128 && (i == start || runAt(i) < 3) {
				i++
			}
			out = append(out, byte(i-start-1))
			out = append(out, data[start:i]...)
		}
		if noops && i%7 == 0 {
			out = append(out, 0x80)
		}
	}
	return out
}

type encodedTag struct {
	tag   uint16
	typ   uint16
	count uint64
	data  []byte
}

func (f fixture) encodeValues(typ uint16, values []uint64) []byte {
	o := f.byteOrder()
	var out []byte
	for _, v := range values {
		switch typ {
		case 1:
			out = append(out, byte(v))
		case 3:
			out = o.AppendUint16(out, uint16(v))
		case 4:
			out = o.AppendUint32(out, uint32(v))
		case 16:
			out = o.AppendUint64(out, v)
		}
	}
	return out
}

func (f fixture) tags(offsets, counts []uint64) []encodedTag {
	var tags []encodedTag
	add := func(tag, typ uint16, values ...uint64) {
		for _, omitted := range f.omit {
			if omitted == tag {
				return
			}
		}
		tags = append(tags, encodedTag{tag: tag, typ: typ, count: uint64(len(values)), data: f.encodeValues(typ, values)})
	}
	bits := make([]uint64, f.samples)
	for i := range bits {
		bits[i] = uint64(f.bits)
	}
	offsetType := uint16(4)
	if f.bigTIFF {
		offsetType = 16
	}
	add(0x100, 4, uint64(f.declaredWidth()))
	add(0x101, 4, uint64(f.declaredHeight()))
	add(0x102, 3, bits...)
	add(0x103, 3, uint64(f.compression))
	add(0x106, 3, uint64(f.photometric))
	if f.fillOrder != 0 {
		add(0x10a, 3, uint64(f.fillOrder))
	}
	if f.orientation != 0 {
		add(0x112, 3, uint64(f.orientation))
	}
	add(0x115, 3, uint64(f.samples))
	if f.tileW > 0 {
		add(0x142, 4, uint64(f.tileW))
		add(0x143, 4, uint64(f.tileH))
		add(0x144, offsetType, offsets...)
		add(0x145, offsetType, counts...)
	} else {
		_, _, _, rows := f.geometry()
		add(0x111, offsetType, offsets...)
		add(0x116, 4, uint64(rows))
		add(0x117, offsetType, counts...)
	}
	if f.planar != 0 {
		add(0x11c, 3, uint64(f.planar))
	}
	if f.predictor != 0 {
		add(0x13d, 3, uint64(f.predictor))
	}
	if len(f.palette) > 0 {
		words := make([]uint64, len(f.palette))
		for i, w := range f.palette {
			words[i] = uint64(w)
		}
		add(0x140, 3, words...)
	}
	if f.extra != 0 {
		add(0x152, 3, uint64(f.extra))
	}
	if f.sampleFormat != 0 {
		add(0x153, 3, uint64(f.sampleFormat))
	}
	for _, c := range f.custom {
		replaced := false
		for i := range tags {
			if tags[i].tag == c.tag {
				tags[i] = encodedTag{tag: c.tag, typ: c.typ, count: uint64(len(c.values)), data: f.encodeValues(c.typ, c.values)}
				replaced = true
			}
		}
		if !replaced {
			tags = append(tags, encodedTag{tag: c.tag, typ: c.typ, count: uint64(len(c.values)), data: f.encodeValues(c.typ, c.values)})
		}
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].tag < tags[j].tag })
	return tags
}

func (f fixture) bytes() []byte {
	o := f.byteOrder()
	out := []byte{'I', 'I'}
	if f.bigEndian {
		out = []byte{'M', 'M'}
	}
	if f.bigTIFF {
		out = o.AppendUint16(out, 43)
		out = o.AppendUint16(out, 8)
		out = o.AppendUint16(out, 0)
		out = o.AppendUint64(out, 0)
	} else {
		out = o.AppendUint16(out, 42)
		out = o.AppendUint32(out, 0)
	}
	across, down, _, _ := f.geometry()
	var offsets, counts []uint64
	for cy := 0; cy < down; cy++ {
		for cx := 0; cx < across; cx++ {
			if f.alias && len(offsets) > 0 {
				offsets = append(offsets, offsets[0])
				counts = append(counts, counts[0])
				continue
			}
			raw := f.rawOverride
			if raw == nil {
				raw = f.rawChunk(cx, cy)
			}
			data := f.compress(raw)
			offsets = append(offsets, uint64(len(out)))
			counts = append(counts, uint64(len(data)))
			out = append(out, data...)
			if len(out)%2 == 1 {
				out = append(out, 0)
			}
		}
	}
	tags := f.tags(offsets, counts)
	inline := 4
	if f.bigTIFF {
		inline = 8
	}
	extraStart := len(out)
	var extra []byte
	extraOffsets := make([]int, len(tags))
	for i, t := range tags {
		if len(t.data) > inline {
			extraOffsets[i] = extraStart + len(extra)
			extra = append(extra, t.data...)
			if len(extra)%2 == 1 {
				extra = append(extra, 0)
			}
		}
	}
	out = append(out, extra...)
	ifdOffset := len(out)
	if f.bigTIFF {
		out = o.AppendUint64(out, uint64(len(tags)))
	} else {
		out = o.AppendUint16(out, uint16(len(tags)))
	}
	for i, t := range tags {
		out = o.AppendUint16(out, t.tag)
		out = o.AppendUint16(out, t.typ)
		field := make([]byte, inline)
		if len(t.data) > inline {
			if f.bigTIFF {
				o.PutUint64(field, uint64(extraOffsets[i]))
			} else {
				o.PutUint32(field, uint32(extraOffsets[i]))
			}
		} else {
			copy(field, t.data)
		}
		if f.bigTIFF {
			out = o.AppendUint64(out, t.count)
		} else {
			out = o.AppendUint32(out, uint32(t.count))
		}
		out = append(out, field...)
	}
	if f.bigTIFF {
		out = o.AppendUint64(out, 0)
		o.PutUint64(out[8:], uint64(ifdOffset))
	} else {
		out = o.AppendUint32(out, 0)
		o.PutUint32(out[4:], uint32(ifdOffset))
	}
	return out
}

func (f fixture) expected() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, f.width, f.height))
	for y := 0; y < f.height; y++ {
		for x := 0; x < f.width; x++ {
			pixel := f.expectedPixel(x, y)
			copy(img.Pix[y*img.Stride+x*4:], pixel[:])
		}
	}
	return img
}

func (f fixture) expectedPixel(x, y int) [4]byte {
	s := func(c int) int { return int(f.sampleAt(x, y, c)) }
	switch f.photometric {
	case 0, 1:
		var g int
		if f.bits == 16 {
			g = s(0) >> 8
		} else {
			g = s(0) * 255 / f.maxValue()
		}
		if f.photometric == 0 {
			g = 255 - g
		}
		return [4]byte{byte(g), byte(g), byte(g), 255}
	case 3:
		i := s(0)
		colors := len(f.palette) / 3
		return [4]byte{byte(f.palette[i] >> 8), byte(f.palette[colors+i] >> 8), byte(f.palette[2*colors+i] >> 8), 255}
	}
	shift := uint(f.bits - 8)
	if f.samples == 3 {
		return [4]byte{byte(s(0) >> shift), byte(s(1) >> shift), byte(s(2) >> shift), 255}
	}
	if f.extra == 2 {
		return [4]byte{byte(s(0) >> shift), byte(s(1) >> shift), byte(s(2) >> shift), byte(s(3) >> shift)}
	}
	a := s(3)
	full := f.maxValue()
	switch a {
	case full:
		return [4]byte{byte(s(0) >> shift), byte(s(1) >> shift), byte(s(2) >> shift), 255}
	case 0:
		return [4]byte{0, 0, 0, 0}
	}
	un := func(v int) byte { return byte((uint32(v) * 0xffff / uint32(a)) >> 8) }
	if f.bits == 16 {
		return [4]byte{un(s(0)), un(s(1)), un(s(2)), byte(a >> 8)}
	}
	return [4]byte{un(s(0)), un(s(1)), un(s(2)), byte(a)}
}
