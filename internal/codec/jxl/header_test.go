package jxl

import (
	"bytes"
	"errors"
	"testing"

	gjxl "github.com/gen2brain/jxl"

	"github.com/oops1/PixGo/internal/codec"
)

func libConfig(t *testing.T, data []byte) (int, int) {
	t.Helper()
	cfg, err := gjxl.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("library rejects the header: %v", err)
	}
	return cfg.Width, cfg.Height
}

func TestImageHeadersAreReadLikeTheLibraryReadsThem(t *testing.T) {
	alpha := []extraSpec{{}}
	tests := []struct {
		name   string
		spec   headerSpec
		check  func(imageHeader) bool
		width  int
		height int
	}{
		{"plain large size", baseHeader(1234, 567), func(h imageHeader) bool { return h.width == 1234 && h.height == 567 }, 1234, 567},
		{"small size", func() headerSpec { s := baseHeader(48, 32); s.smallSize = true; return s }(), func(h imageHeader) bool { return h.width == 48 && h.height == 32 }, 48, 32},
		{"small square", func() headerSpec { s := baseHeader(64, 64); s.smallSize = true; s.ratio = 1; return s }(), func(h imageHeader) bool { return h.width == 64 && h.height == 64 }, 64, 64},
		{"square ratio", func() headerSpec { s := baseHeader(300, 300); s.ratio = 1; return s }(), func(h imageHeader) bool { return h.width == 300 }, 300, 300},
		{"two to one ratio", func() headerSpec { s := baseHeader(600, 300); s.ratio = 7; return s }(), func(h imageHeader) bool { return h.width == 600 }, 600, 300},
		{"four to three ratio", func() headerSpec { s := baseHeader(400, 300); s.ratio = 3; return s }(), func(h imageHeader) bool { return h.width == 400 }, 400, 300},
		{"huge size", baseHeader(1<<28, 1<<20), func(h imageHeader) bool { return h.width == 1<<28 && h.height == 1<<20 }, 1 << 28, 1 << 20},
		{"every default", func() headerSpec { s := baseHeader(100, 80); s.allDefault = true; return s }(), func(h imageHeader) bool {
			return h.xybEncoded && h.bitsPerSample == 8 && len(h.extraChannels) == 0 && !h.hasAlpha()
		}, 100, 80},
		{"intrinsic size", func() headerSpec { s := baseHeader(100, 80); s.intrinsic = true; return s }(), func(h imageHeader) bool { return h.width == 100 }, 100, 80},
		{"tone mapping", func() headerSpec { s := baseHeader(100, 80); s.orientation = 3; s.toneMapping = true; return s }(), func(h imageHeader) bool { return h.orientation == 3 }, 100, 80},
		{"extensions", func() headerSpec { s := baseHeader(100, 80); s.extensions = true; return s }(), func(h imageHeader) bool { return h.width == 100 }, 100, 80},
		{"transform data without xyb", func() headerSpec { s := baseHeader(100, 80); s.transformData = true; return s }(), func(h imageHeader) bool { return !h.xybEncoded }, 100, 80},
		{"transform data with xyb", func() headerSpec { s := baseHeader(100, 80); s.xyb = true; s.transformData = true; return s }(), func(h imageHeader) bool { return h.xybEncoded }, 100, 80},
		{"alpha channel", func() headerSpec { s := baseHeader(100, 80); s.extra = alpha; return s }(), func(h imageHeader) bool { return h.hasAlpha() && h.planes() == 4 }, 100, 80},
		{"explicit alpha channel", func() headerSpec {
			s := baseHeader(100, 80)
			s.extra = []extraSpec{{kind: 0, explicit: true, name: "alpha"}}
			return s
		}(), func(h imageHeader) bool { return h.hasAlpha() && h.extraChannels[0].bitsPerSample == 8 }, 100, 80},
		{"depth map is not alpha", func() headerSpec {
			s := baseHeader(100, 80)
			s.extra = []extraSpec{{kind: 1, explicit: true, bits: 16}}
			return s
		}(), func(h imageHeader) bool { return !h.hasAlpha() && len(h.extraChannels) == 1 }, 100, 80},
		{"spot colour and cfa channels", func() headerSpec {
			s := baseHeader(100, 80)
			s.extra = []extraSpec{{kind: 2, explicit: true, name: "spot"}, {kind: 5, explicit: true, name: "a longer name than sixteen bytes needs more bits"}, {}}
			return s
		}(), func(h imageHeader) bool { return h.hasAlpha() && len(h.extraChannels) == 3 }, 100, 80},
		{"float samples", func() headerSpec { s := baseHeader(100, 80); s.floatSample = true; s.bitsPerSample = 32; return s }(), func(h imageHeader) bool { return h.floatSample && h.bitsPerSample == 32 }, 100, 80},
		{"half float samples", func() headerSpec { s := baseHeader(100, 80); s.floatSample = true; s.bitsPerSample = 16; return s }(), func(h imageHeader) bool { return h.floatSample && h.bitsPerSample == 16 }, 100, 80},
		{"ten bit samples", func() headerSpec { s := baseHeader(100, 80); s.bitsPerSample = 10; return s }(), func(h imageHeader) bool { return h.bitsPerSample == 10 }, 100, 80},
		{"sixteen bit samples", func() headerSpec { s := baseHeader(100, 80); s.bitsPerSample = 16; return s }(), func(h imageHeader) bool { return h.bitsPerSample == 16 }, 100, 80},
		{"grey", func() headerSpec {
			s := baseHeader(100, 80)
			s.colorAllDefault, s.colorSpace = false, colorSpaceGray
			return s
		}(), func(h imageHeader) bool { return h.colorChannels() == 1 && h.planes() == 1 }, 100, 80},
		{"grey with xyb", func() headerSpec {
			s := baseHeader(100, 80)
			s.xyb, s.colorAllDefault, s.colorSpace = true, false, colorSpaceGray
			return s
		}(), func(h imageHeader) bool { return h.colorChannels() == 1 && h.planes() == 3 }, 100, 80},
		{"custom white point and primaries", func() headerSpec {
			s := baseHeader(100, 80)
			s.colorAllDefault, s.whiteCustom, s.primariesCustom = false, true, true
			return s
		}(), func(h imageHeader) bool { return h.colorSpace == colorSpaceRGB && !h.embeddedProfile }, 100, 80},
		{"gamma transfer", func() headerSpec {
			s := baseHeader(100, 80)
			s.colorAllDefault, s.gamma = false, true
			return s
		}(), func(h imageHeader) bool { return h.colorSpace == colorSpaceRGB }, 100, 80},
		{"embedded profile", func() headerSpec {
			s := baseHeader(100, 80)
			s.colorAllDefault, s.embeddedProfile = false, true
			return s
		}(), func(h imageHeader) bool { return h.embeddedProfile }, 100, 80},
		{"timecodes", func() headerSpec {
			s := baseHeader(100, 80)
			s.animated, s.timecodes, s.loops = true, true, 7
			return s
		}(), func(h imageHeader) bool { return h.animated && h.timecodes && h.loops == 7 }, 100, 80},
		{"animation timing", func() headerSpec {
			s := baseHeader(100, 80)
			s.animated, s.ticksNumerator, s.ticksDenom, s.loops = true, 24000, 1001, 100000
			return s
		}(), func(h imageHeader) bool {
			return h.animated && h.ticksNumerator == 24000 && h.ticksDenom == 1001 && h.loops == 100000
		}, 100, 80},
		{"preview in pixels", func() headerSpec { s := baseHeader(100, 80); s.preview, s.previewSize = true, 20; return s }(), func(h imageHeader) bool {
			return h.hasPreview && h.previewWidth == 20 && h.previewHeight == 20
		}, 100, 80},
		{"large preview", func() headerSpec { s := baseHeader(1000, 800); s.preview, s.previewSize = true, 256; return s }(), func(h imageHeader) bool {
			return h.hasPreview && h.previewWidth == 256
		}, 1000, 800},
		{"preview", func() headerSpec { s := baseHeader(100, 80); s.preview, s.previewSize = true, 24; return s }(), func(h imageHeader) bool {
			return h.hasPreview && h.previewWidth == 24 && h.previewHeight == 24
		}, 100, 80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, end := tt.spec.write()
			header, err := parseImageHeader(data)
			if err != nil {
				t.Fatal(err)
			}
			if !tt.check(header) {
				t.Fatalf("%+v", header)
			}
			if header.endBit != end {
				t.Fatalf("header ends at bit %d, the writer wrote %d", header.endBit, end)
			}
			if w, h := header.displaySize(); w != tt.width || h != tt.height {
				t.Fatalf("display size %dx%d", w, h)
			}
			if tt.width < 1<<20 {
				if w, h := libConfig(t, data); w != tt.width || h != tt.height {
					t.Fatalf("library says %dx%d", w, h)
				}
			}
		})
	}
}

func TestOrientationsTransposeTheDisplayedSize(t *testing.T) {
	for orientation := uint32(1); orientation <= 8; orientation++ {
		spec := baseHeader(120, 50)
		spec.orientation = orientation
		data, _ := spec.write()
		header, err := parseImageHeader(data)
		if err != nil {
			t.Fatal(err)
		}
		width, height := header.displaySize()
		transposed := orientation >= 5
		if header.orientation != orientation || header.transposes() != transposed || (transposed && (width != 50 || height != 120)) || (!transposed && (width != 120 || height != 50)) {
			t.Fatalf("orientation %d: %dx%d", orientation, width, height)
		}
		if w, h := libConfig(t, data); w != width || h != height {
			t.Fatalf("orientation %d: library says %dx%d", orientation, w, h)
		}
	}
}

func TestEveryPrefixOfAHeaderAsksForMoreOrFails(t *testing.T) {
	spec := baseHeader(100, 80)
	spec.orientation = 6
	spec.animated, spec.timecodes, spec.loops = true, true, 4
	spec.preview, spec.previewSize = true, 16
	spec.extra = []extraSpec{{}, {kind: 2, explicit: true, name: "spot"}}
	spec.colorAllDefault, spec.whiteCustom, spec.primariesCustom, spec.gamma = false, true, true, true
	spec.toneMapping, spec.extensions, spec.transformData, spec.xyb = true, true, true, true
	data, end := spec.write()
	if _, err := parseImageHeader(data); err != nil {
		t.Fatal(err)
	}
	for cut := 0; cut < (end+7)/8; cut++ {
		if _, err := parseImageHeader(data[:cut]); err == nil {
			t.Fatalf("a header cut to %d bytes parsed", cut)
		} else if !errors.Is(err, errNeedMoreData) && !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("cut %d: %v", cut, err)
		}
	}
}

func TestInvalidImageHeadersAreCorrupt(t *testing.T) {
	bad := func(change func(*headerSpec)) []byte {
		spec := baseHeader(100, 80)
		change(&spec)
		data, _ := spec.write()
		return data
	}
	tests := []struct {
		name string
		data []byte
	}{
		{"wrong signature", []byte{0xff, 0xd8, 0x00, 0x00, 0x00, 0x00}},
		{"colour space out of range", bad(func(s *headerSpec) { s.colorAllDefault, s.colorSpace = false, 9 })},
		{"too many extra channels", bad(func(s *headerSpec) { s.extra = make([]extraSpec, 300) })},
		{"sample size out of range", bad(func(s *headerSpec) { s.bitsPerSample = 40 })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseImageHeader(tt.data); !errors.Is(err, codec.ErrCorrupt) {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestBitReaderReadsLeastSignificantBitsFirst(t *testing.T) {
	r := newBitReader([]byte{0b10110100, 0xff, 0x01})
	if r.bits(3) != 0b100 || r.bits(5) != 0b10110 || r.left() != 16 {
		t.Fatal("bits")
	}
	if r.bits(0) != 0 || r.bits(9) != 0x1ff || r.left() != 7 {
		t.Fatal("wide read")
	}
	r.skip(7)
	if r.exhausted || r.left() != 0 {
		t.Fatal("exact skip")
	}
	r.bits(1)
	if !r.exhausted {
		t.Fatal("reading past the end must be flagged")
	}
	r = newBitReader([]byte{1})
	r.skip(9)
	if !r.exhausted {
		t.Fatal("skip past the end must be flagged")
	}
	r = newBitReader([]byte{0xff, 0x00})
	r.bits(3)
	r.alignToByte()
	if !r.invalid {
		t.Fatal("non-zero padding must be flagged")
	}
}

func TestVariableLengthIntegersRoundTrip(t *testing.T) {
	for _, v := range []uint64{0, 1, 16, 17, 272, 273, 4095, 4096, 1 << 20, 1 << 33, 1 << 59, 1<<64 - 1} {
		w := &bitWriter{}
		w.u64(v)
		if got := newBitReader(w.data).u64(); got != v {
			t.Fatalf("%d came back as %d", v, got)
		}
	}
	for _, v := range []uint32{0, 1, 7, 8, 100, 1023, 1024, 17408, 1 << 30} {
		w := &bitWriter{}
		w.u32(tableChoice, v)
		if got := newBitReader(w.data).u32(tableChoice); got != v {
			t.Fatalf("%d came back as %d", v, got)
		}
	}
	for _, v := range []int32{0, 1, -1, 100, -100, 1 << 20, -(1 << 20)} {
		if got := unpackSigned(packSigned(v)); got != v {
			t.Fatalf("%d came back as %d", v, got)
		}
	}
}
