package jpegxform

import (
	"bytes"
	"math/rand"
	"testing"
)

func specLengths(s tableSpec) map[int]int {
	lengths := map[int]int{}
	k := 0
	for l := 1; l <= 16; l++ {
		for i := 0; i < int(s.counts[l-1]); i++ {
			lengths[int(s.symbols[k])] = l
			k++
		}
	}
	return lengths
}

func kraft(s tableSpec) float64 {
	sum := 0.0
	for l := 1; l <= 16; l++ {
		sum += float64(s.counts[l-1]) / float64(uint(1)<<uint(l))
	}
	return sum
}

func checkSpec(t *testing.T, freq *[symbolSentinel + 1]int, s tableSpec) {
	t.Helper()
	used := 0
	for i := 0; i < symbolSentinel; i++ {
		if freq[i] > 0 {
			used++
		}
	}
	lengths := specLengths(s)
	if used > 0 && len(s.symbols) != used {
		t.Fatalf("%d symbols for %d used", len(s.symbols), used)
	}
	for sym := 0; sym < symbolSentinel; sym++ {
		if freq[sym] > 0 {
			if _, ok := lengths[sym]; !ok {
				t.Fatalf("symbol %d has no code", sym)
			}
		}
	}
	if k := kraft(s); k >= 1 {
		t.Fatalf("the code space is full (%g): a code of all ones exists", k)
	}
	if _, err := newHuffDecoder(huffSpec{present: true, counts: s.counts, symbols: s.symbols}); err != nil {
		t.Fatal(err)
	}
}

func TestOptimalTablesAreValidForRandomFrequencies(t *testing.T) {
	rng := rand.New(rand.NewSource(20))
	for iter := 0; iter < 300; iter++ {
		var freq [symbolSentinel + 1]int
		n := 1 + rng.Intn(256)
		for i := 0; i < n; i++ {
			freq[rng.Intn(256)] = 1 + rng.Intn(1+rng.Intn(100000))
		}
		checkSpec(t, &freq, optimalSpec(&freq))
	}
}

func TestOptimalTableLimitsCodeLengthsToSixteenBits(t *testing.T) {
	var freq [symbolSentinel + 1]int
	a, b := 1, 1
	for i := 0; i < 40; i++ {
		freq[i] = a
		a, b = b, a+b
	}
	for i := 40; i < 120; i++ {
		freq[i] = 1
	}

	spec := optimalSpec(&freq)

	checkSpec(t, &freq, spec)
	longest := 0
	for _, l := range specLengths(spec) {
		longest = max(longest, l)
	}
	if longest > 16 {
		t.Fatalf("longest code has %d bits", longest)
	}
	var total int
	for _, c := range spec.counts {
		total += int(c)
	}
	if total != 120 {
		t.Fatalf("%d codes for 120 symbols", total)
	}
}

func TestOptimalTableForAnEmptyOrSingleSymbolHistogram(t *testing.T) {
	var none [symbolSentinel + 1]int
	spec := optimalSpec(&none)
	if len(spec.symbols) != 1 || spec.counts[0] != 1 {
		t.Fatalf("empty histogram gives %+v", spec)
	}
	var one [symbolSentinel + 1]int
	one[0x35] = 9
	spec = optimalSpec(&one)
	if len(spec.symbols) != 1 || spec.symbols[0] != 0x35 || spec.counts[0] != 1 {
		t.Fatalf("single symbol gives %+v", spec)
	}
	enc := spec.encoder()
	if enc.size[0x35] != 1 || enc.code[0x35] != 0 {
		t.Fatalf("code %d of %d bits", enc.code[0x35], enc.size[0x35])
	}
}

func TestOptimalTableForAllSymbolsEquallyLikely(t *testing.T) {
	var freq [symbolSentinel + 1]int
	for i := 0; i < 256; i++ {
		freq[i] = 100
	}

	spec := optimalSpec(&freq)

	checkSpec(t, &freq, spec)
	if len(spec.symbols) != 256 {
		t.Fatalf("%d symbols", len(spec.symbols))
	}
}

func TestOptimalCodesRoundTripThroughTheBitReader(t *testing.T) {
	rng := rand.New(rand.NewSource(21))
	var freq [symbolSentinel + 1]int
	symbols := make([]int, 5000)
	for i := range symbols {
		symbols[i] = int(rng.ExpFloat64()*20) % 256
		freq[symbols[i]]++
	}
	spec := optimalSpec(&freq)
	enc := spec.encoder()
	var sink bytes.Buffer
	w := newBitWriter(&sink)
	for _, s := range symbols {
		w.put(uint32(enc.code[s]), uint(enc.size[s]))
	}
	if err := w.finish(); err != nil {
		t.Fatal(err)
	}
	written := sink.Bytes()
	dec, err := newHuffDecoder(huffSpec{present: true, counts: spec.counts, symbols: spec.symbols})
	if err != nil {
		t.Fatal(err)
	}
	br := newBitReader(newByteSource(bytes.NewReader(written), int64(len(written)), 0))

	for i, want := range symbols {
		got, ok := br.decode(dec)
		if !ok || got != want {
			t.Fatalf("symbol %d: got %d (%v), want %d", i, got, ok, want)
		}
	}
}

func TestOversubscribedHuffmanTableIsRejected(t *testing.T) {
	var counts [16]uint8
	counts[0] = 3
	if _, err := newHuffDecoder(huffSpec{present: true, counts: counts, symbols: []byte{0, 1, 2}}); err == nil {
		t.Fatal("three one-bit codes are impossible")
	}
}

func TestTransposeQuantIsItsOwnInverse(t *testing.T) {
	var q [64]uint16
	for i := range q {
		q[i] = uint16(i*7 + 3)
	}
	if got := transposeQuant(transposeQuant(q)); got != q {
		t.Fatal("transposing twice must give the table back")
	}
	if transposeQuant(q) == q {
		t.Fatal("a non-symmetric table must change")
	}
	var sym [64]uint16
	for i := range sym {
		sym[i] = uint16(10 + zigzag[i]/8 + zigzag[i]%8)
	}
	if transposeQuant(sym) != sym {
		t.Fatal("a symmetric table must stay the same")
	}
}
