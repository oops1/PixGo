package jpegxform

const (
	symbolSentinel = 256
	maxCodeLength  = 16
	deepestCode    = symbolSentinel
)

type tableSpec struct {
	counts  [16]uint8
	symbols []byte
}

type encoderTable struct {
	code [256]uint16
	size [256]uint8
}

func optimalSpec(freq *[symbolSentinel + 1]int) tableSpec {
	var f [symbolSentinel + 1]int
	copy(f[:], freq[:])
	f[symbolSentinel] = 1
	used := false
	for i := 0; i < symbolSentinel; i++ {
		if f[i] > 0 {
			used = true
		}
	}
	if !used {
		f[0] = 1
	}
	var codeSize [symbolSentinel + 1]int
	var others [symbolSentinel + 1]int
	for i := range others {
		others[i] = -1
	}
	for {
		c1, v := -1, int(^uint(0)>>1)
		for i := 0; i <= symbolSentinel; i++ {
			if f[i] > 0 && f[i] <= v {
				v, c1 = f[i], i
			}
		}
		c2, v := -1, int(^uint(0)>>1)
		for i := 0; i <= symbolSentinel; i++ {
			if f[i] > 0 && f[i] <= v && i != c1 {
				v, c2 = f[i], i
			}
		}
		if c2 < 0 {
			break
		}
		f[c1] += f[c2]
		f[c2] = 0
		codeSize[c1]++
		for others[c1] >= 0 {
			c1 = others[c1]
			codeSize[c1]++
		}
		others[c1] = c2
		codeSize[c2]++
		for others[c2] >= 0 {
			c2 = others[c2]
			codeSize[c2]++
		}
	}
	var bits [deepestCode + 1]int
	for i := 0; i <= symbolSentinel; i++ {
		if codeSize[i] > 0 {
			bits[codeSize[i]]++
		}
	}
	for i := deepestCode; i > maxCodeLength; i-- {
		for bits[i] > 0 {
			j := i - 2
			for bits[j] == 0 {
				j--
			}
			bits[i] -= 2
			bits[i-1]++
			bits[j+1] += 2
			bits[j]--
		}
	}
	i := maxCodeLength
	for bits[i] == 0 {
		i--
	}
	bits[i]--
	var spec tableSpec
	for l := 1; l <= maxCodeLength; l++ {
		spec.counts[l-1] = uint8(bits[l])
	}
	for l := 1; l <= deepestCode; l++ {
		for s := 0; s < symbolSentinel; s++ {
			if codeSize[s] == l {
				spec.symbols = append(spec.symbols, byte(s))
			}
		}
	}
	return spec
}

func (s tableSpec) encoder() *encoderTable {
	t := &encoderTable{}
	code := uint32(0)
	k := 0
	for l := 1; l <= maxCodeLength; l++ {
		for i := 0; i < int(s.counts[l-1]); i++ {
			sym := s.symbols[k]
			t.code[sym] = uint16(code)
			t.size[sym] = uint8(l)
			code++
			k++
		}
		code <<= 1
	}
	return t
}
