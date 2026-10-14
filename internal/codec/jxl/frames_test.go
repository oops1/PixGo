package jxl

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func nakedStream(data []byte) *codestream {
	size := int64(len(data))
	return &codestream{source: bytes.NewReader(data), size: size, level: supportedLevel, segments: []segment{{file: 0, length: size}}}
}

func streamOf(spec headerSpec, frames ...[]byte) []byte {
	header, end := spec.write()
	w := &bitWriter{data: append([]byte(nil), header...), position: end}
	w.align()
	out := w.data
	for _, frame := range frames {
		out = append(out, frame...)
	}
	return out
}

func scanStream(t *testing.T, data []byte, maxVisible int) (frameCount, error) {
	t.Helper()
	header, err := parseImageHeader(data)
	if err != nil {
		t.Fatal(err)
	}
	return nakedStream(data).countFrames(context.Background(), header, maxVisible)
}

func animatedSpec(width, height uint32) headerSpec {
	spec := baseHeader(width, height)
	spec.animated = true
	return spec
}

func TestFrameScanCountsOnlyTheFramesTheDecoderShows(t *testing.T) {
	spec := animatedSpec(100, 80)
	hidden := frameSpec{modular: true, groupSizeShift: 1, savedReference: 1}
	shown := func(duration uint32) frameSpec {
		return frameSpec{modular: true, groupSizeShift: 1, duration: duration}
	}
	last := frameSpec{modular: true, groupSizeShift: 1, last: true}
	tests := []struct {
		name    string
		frames  []frameSpec
		visible int
		total   int
	}{
		{"single default frame", []frameSpec{{allDefault: true}}, 1, 1},
		{"single last frame", []frameSpec{last}, 1, 1},
		{"three frames with durations", []frameSpec{shown(3), shown(1), last}, 3, 3},
		{"hidden layers are not frames", []frameSpec{hidden, shown(2), hidden, hidden, last}, 2, 5},
		{"a last frame without duration still shows", []frameSpec{hidden, last}, 1, 2},
		{"reference-only and low frequency frames", []frameSpec{{kind: frameReferenceOnly, modular: true, groupSizeShift: 1}, {kind: frameLowFrequency, lowFreqLevel: 1}, shown(4), last}, 2, 4},
		{"skip-progressive frame", []frameSpec{{kind: frameSkipProgressive, modular: true, groupSizeShift: 1, duration: 5}, last}, 2, 2},
		{"cropped frame", []frameSpec{{modular: true, groupSizeShift: 1, cropped: true, originX: -10, originY: 7, frameWidth: 300, frameHeight: 30, duration: 2}, last}, 2, 2},
		{"cropped frame that covers the image", []frameSpec{{modular: true, groupSizeShift: 1, cropped: true, originX: -10, originY: -7, frameWidth: 300, frameHeight: 300, duration: 2}, last}, 2, 2},
		{"several passes", []frameSpec{{modular: true, groupSizeShift: 1, passes: 3, duration: 1}, last}, 2, 2},
		{"smaller groups", []frameSpec{{modular: true, groupSizeShift: 0, duration: 1}, {modular: true, groupSizeShift: 3, duration: 1}, last}, 3, 3},
		{"upsampled frames", []frameSpec{{modular: true, groupSizeShift: 1, upsampling: 4, duration: 1}, last}, 2, 2},
		{"blend modes and names", []frameSpec{{modular: true, groupSizeShift: 1, blendMode: 2, name: "a layer with a name", duration: 1}, {modular: true, groupSizeShift: 1, blendMode: 4, duration: 1}, last}, 3, 3},
		{"loop filter overrides", []frameSpec{{loopFilter: true, duration: 1}, {loopFilter: true, modular: true, groupSizeShift: 1, duration: 1}, last}, 3, 3},
		{"vardct frames", []frameSpec{{duration: 1}, {kind: frameRegular, duration: 1, last: false}, {last: true}}, 3, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var frames [][]byte
			for _, f := range tt.frames {
				frames = append(frames, f.write(spec, 100, 80))
			}
			counted, err := scanStream(t, streamOf(spec, frames...), 100)
			if err != nil || counted.visible != tt.visible || counted.total != tt.total {
				t.Fatalf("%+v %v, want %d visible of %d", counted, err, tt.visible, tt.total)
			}
		})
	}
}

func TestFrameScanFollowsTheTableOfContentsOfLargeFrames(t *testing.T) {
	for _, size := range [][2]uint32{{3000, 2000}, {257, 257}, {256, 256}, {4096, 17}, {1, 1}} {
		spec := animatedSpec(size[0], size[1])
		first := frameSpec{modular: true, groupSizeShift: 1, duration: 1}.write(spec, size[0], size[1])
		second := frameSpec{modular: true, groupSizeShift: 1, last: true}.write(spec, size[0], size[1])
		counted, err := scanStream(t, streamOf(spec, first, second), 10)
		if err != nil || counted.visible != 2 {
			t.Fatalf("%v: %+v %v", size, counted, err)
		}
	}
}

func independentEntryCount(width, height uint32, shift uint32, passes int, ycbcr [3]uint32) int {
	group := uint32(128) << shift
	groupsX, groupsY := (width+group-1)/group, (height+group-1)/group
	groups := int(groupsX * groupsY)
	if groups == 1 && passes == 1 {
		return 1
	}
	var maxH, maxV uint32
	horizontal := map[uint32]uint32{0: 0, 1: 1, 2: 1, 3: 0}
	vertical := map[uint32]uint32{0: 0, 1: 1, 2: 0, 3: 1}
	for _, c := range ycbcr {
		maxH, maxV = max(maxH, horizontal[c]), max(maxV, vertical[c])
	}
	blocksX := ((width + (8 << maxH) - 1) / (8 << maxH)) << maxH
	blocksY := ((height + (8 << maxV) - 1) / (8 << maxV)) << maxV
	lowFrequency := int(((blocksX + group - 1) / group) * ((blocksY + group - 1) / group))
	return 2 + lowFrequency + groups*passes
}

func TestTableOfContentsSizeMatchesAnIndependentCount(t *testing.T) {
	for _, size := range [][2]uint32{{1, 1}, {255, 256}, {256, 257}, {2049, 1025}, {5000, 3000}, {40000, 90}} {
		for shift := uint32(0); shift < 4; shift++ {
			for _, passes := range []uint32{1, 2, 5} {
				for _, ycbcr := range [][3]uint32{{0, 0, 0}, {1, 1, 1}, {2, 3, 0}} {
					f := frameHeader{upsampling: 1, groupSizeShift: shift, passes: passes, jpegUpsampling: ycbcr}
					want := independentEntryCount(size[0], size[1], shift, int(passes), ycbcr)
					if got := f.tableEntries(size[0], size[1]); got != uint64(want) {
						t.Fatalf("%v shift %d passes %d ycbcr %v: %d entries, want %d", size, shift, passes, ycbcr, got, want)
					}
				}
			}
		}
	}
	f := frameHeader{upsampling: 2, groupSizeShift: 1, passes: 1, frameWidth: 1000, frameHeight: 300}
	if got := f.tableEntries(5, 5); got != uint64(independentEntryCount(500, 150, 1, 1, [3]uint32{})) {
		t.Fatalf("upsampled cropped frame: %d", got)
	}
	f = frameHeader{upsampling: 1, groupSizeShift: 1, passes: 1, kind: frameLowFrequency, lowFreqLevel: 2}
	if got := f.tableEntries(2048, 2048); got != uint64(independentEntryCount(32, 32, 1, 1, [3]uint32{})) {
		t.Fatalf("low frequency frame: %d", got)
	}
}

func TestFrameScanSkipsThePreviewFrame(t *testing.T) {
	spec := animatedSpec(100, 80)
	spec.preview, spec.previewSize = true, 16
	preview := frameSpec{modular: true, groupSizeShift: 1, last: true, entries: []uint32{40}}.write(spec, 16, 16)
	frame := frameSpec{modular: true, groupSizeShift: 1, last: true}.write(spec, 100, 80)
	counted, err := scanStream(t, streamOf(spec, preview, frame), 10)
	if err != nil || counted.visible != 1 {
		t.Fatalf("%+v %v", counted, err)
	}
	if _, err := scanStream(t, streamOf(spec, frame), 10); err == nil {
		t.Fatal("a stream without its preview frame was accepted")
	}
}

func TestFrameScanRefusesWhatItCannotMeasure(t *testing.T) {
	spec := animatedSpec(100, 80)
	frame := frameSpec{modular: true, groupSizeShift: 1, last: true}
	permuted := frame
	permuted.permuted = true
	header, err := parseImageHeader(streamOf(spec))
	if err != nil {
		t.Fatal(err)
	}
	_, err = nakedStream(streamOf(spec, permuted.write(spec, 100, 80))).countFrames(context.Background(), header, 10)
	if !errors.Is(err, errPermutedTable) {
		t.Fatalf("permuted table: %v", err)
	}
	huge := frameSpec{modular: true, groupSizeShift: 0, entries: []uint32{1}, last: true}
	hugeHeader := animatedSpec(1<<30, 1<<30)
	if _, err := scanStream(t, streamOf(hugeHeader, huge.write(hugeHeader, 1<<30, 1<<30)), 10); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("absurd table of contents: %v", err)
	}
}

func TestFrameScanReportsTruncatedStreams(t *testing.T) {
	spec := animatedSpec(100, 80)
	a := frameSpec{modular: true, groupSizeShift: 1, duration: 1}.write(spec, 100, 80)
	b := frameSpec{modular: true, groupSizeShift: 1, last: true}.write(spec, 100, 80)
	whole := streamOf(spec, a, b)
	headerBytes, _ := spec.write()
	for cut := len(headerBytes); cut < len(whole); cut++ {
		header, err := parseImageHeader(whole[:cut])
		if err != nil {
			continue
		}
		_, err = nakedStream(whole[:cut]).countFrames(context.Background(), header, 10)
		if !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("cut at %d of %d: %v", cut, len(whole), err)
		}
	}
	noFrames := streamOf(spec)
	header, _ := parseImageHeader(noFrames)
	if _, err := nakedStream(noFrames).countFrames(context.Background(), header, 10); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("no frames: %v", err)
	}
}

func TestFrameScanGrowsItsWindowForLongHeaders(t *testing.T) {
	spec := animatedSpec(100, 80)
	long := frameSpec{modular: true, groupSizeShift: 1, duration: 1, name: string(make([]byte, 1000))}
	last := frameSpec{modular: true, groupSizeShift: 1, last: true}
	counted, err := scanStream(t, streamOf(spec, long.write(spec, 100, 80), last.write(spec, 100, 80)), 10)
	if err != nil || counted.visible != 2 {
		t.Fatalf("%+v %v", counted, err)
	}
	wide := animatedSpec(5000, 3000)
	many := frameSpec{modular: true, groupSizeShift: 0, duration: 1}.write(wide, 5000, 3000)
	final := frameSpec{modular: true, groupSizeShift: 0, last: true}.write(wide, 5000, 3000)
	if len(many) < frameWindow*2 {
		t.Fatalf("test frame of %d bytes does not outgrow the window", len(many))
	}
	counted, err = scanStream(t, streamOf(wide, many, final), 10)
	if err != nil || counted.visible != 2 {
		t.Fatalf("%+v %v", counted, err)
	}
}

func TestFrameScanStopsAtTheFrameBudget(t *testing.T) {
	spec := animatedSpec(100, 80)
	frame := frameSpec{modular: true, groupSizeShift: 1, duration: 1}.write(spec, 100, 80)
	last := frameSpec{modular: true, groupSizeShift: 1, last: true}.write(spec, 100, 80)
	data := streamOf(spec, bytes.Repeat(frame, 50), last)
	if _, err := scanStream(t, data, 49); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("%v", err)
	}
	if counted, err := scanStream(t, data, 51); err != nil || counted.visible != 51 {
		t.Fatalf("%+v %v", counted, err)
	}
}

func TestFrameScanBoundsTheNumberOfHiddenFrames(t *testing.T) {
	spec := animatedSpec(8, 8)
	hidden := frameSpec{modular: true, groupSizeShift: 1, savedReference: 1}.write(spec, 8, 8)
	data := streamOf(spec, bytes.Repeat(hidden, maxScannedFrames+1))
	if _, err := scanStream(t, data, 10); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("%v", err)
	}
}

func TestFrameScanStopsOnCanceledContext(t *testing.T) {
	spec := animatedSpec(100, 80)
	data := streamOf(spec, frameSpec{modular: true, groupSizeShift: 1, last: true}.write(spec, 100, 80))
	header, _ := parseImageHeader(data)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := nakedStream(data).countFrames(ctx, header, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}

func TestFrameScanReportsDamagedHeaders(t *testing.T) {
	spec := animatedSpec(100, 80)
	spec.extra = []extraSpec{{}}
	frame := frameSpec{modular: true, groupSizeShift: 1, last: true, flags: 1 << 40}.write(spec, 100, 80)
	if _, err := scanStream(t, streamOf(spec, frame), 10); err != nil {
		t.Fatalf("large flag values are legal: %v", err)
	}
	dirty := frameSpec{modular: true, groupSizeShift: 1, last: true}.write(spec, 100, 80)
	for i := range dirty {
		dirty[i] = 0xff
	}
	if _, err := scanStream(t, streamOf(spec, dirty), 10); !errors.Is(err, codec.ErrCorrupt) && !errors.Is(err, errPermutedTable) {
		t.Fatalf("garbage frame: %v", err)
	}
}
