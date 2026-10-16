package jpegdec

const (
	lutBits       = 9
	lutMask       = 1<<lutBits - 1
	maxNCodes     = 256
	maxCodeLength = 16
)

type huffTable struct {
	nCodes    int32
	lut       [1 << lutBits]uint16
	vals      [maxNCodes]uint8
	maxCode   [maxCodeLength + 1]int32
	valOffset [maxCodeLength + 1]int32
}

func (t *huffTable) build(counts *[maxCodeLength]int32, vals *[maxNCodes]uint8, total int32) error {
	var next huffTable
	next.nCodes = total
	next.vals = *vals
	code := int32(0)
	index := int32(0)
	for length := 1; length <= maxCodeLength; length++ {
		count := counts[length-1]
		if count == 0 {
			next.maxCode[length] = -1
			code <<= 1
			continue
		}
		if code+count > 1<<uint(length) {
			return FormatError("bad Huffman table")
		}
		next.valOffset[length] = index - code
		for i := int32(0); i < count; i++ {
			if length <= lutBits {
				base := int(code) << uint(lutBits-length)
				entry := uint16(vals[index])<<8 | uint16(length)
				for j := 0; j < 1<<uint(lutBits-length); j++ {
					next.lut[base|j] = entry
				}
			}
			code++
			index++
		}
		next.maxCode[length] = code - 1
		code <<= 1
	}
	*t = next
	return nil
}

func (t *huffTable) slow(acc uint64, n int) (uint8, int, bool) {
	for length := lutBits + 1; length <= maxCodeLength; length++ {
		code := int32(acc>>uint(n-length)) & (1<<uint(length) - 1)
		if code <= t.maxCode[length] {
			return t.vals[uint8(t.valOffset[length]+code)], length, true
		}
	}
	return 0, 0, false
}
