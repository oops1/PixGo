package jpegdec

import (
	"errors"
	"image"
	"math/rand"
	"testing"
)

var progressiveFiles = []string{
	"video-001.progressive.jpeg",
	"video-001.q50.420.progressive.jpeg",
	"video-001.progressive.truncated.jpeg",
	"video-001.separate.dc.progression.progressive.jpeg",
}

func TestProgressiveFilesCannotRandomAccessAtFullResolutionBeforeDecoding(t *testing.T) {
	files := corpus(t)
	for _, name := range progressiveFiles {
		d := open(t, files[name], Options{})
		if d.CanRegion() {
			t.Fatalf("%s: CanRegion", name)
		}
		if d.MinShift() != MaxShift {
			t.Fatalf("%s: MinShift %d before any decode", name, d.MinShift())
		}
		if _, err := d.Region(t.Context(), 0, rect(0, 0, 10, 10)); err == nil {
			t.Fatalf("%s: region before the overview", name)
		}
		d.Close()
	}
}

func expectedStoreBytes(d *Decoder, shift int) int {
	g := d.f.geometry(shift, shift)
	mxx, myy := d.f.mcuCounts()
	total := 0
	for c := 0; c < d.f.nComp; c++ {
		blocks := mxx * myy * d.f.comp[c].h * d.f.comp[c].v
		total += blocks * g.kk[c] * 2
		if g.ks[c] < 8 {
			total += blocks * 8
		}
	}
	return total
}

func TestProgressiveCoefficientMemoryFollowsTheScale(t *testing.T) {
	files := corpus(t)
	for _, name := range progressiveFiles[:2] {
		var previous int
		for shift := MaxShift; shift >= 0; shift-- {
			d := open(t, files[name], Options{})
			if _, err := d.Decode(t.Context(), shift); err != nil {
				t.Fatal(err)
			}
			got := d.CoefficientBytes()
			if want := expectedStoreBytes(d, shift); got != want {
				t.Fatalf("%s shift %d: %d bytes, want %d", name, shift, got, want)
			}
			if previous != 0 && got <= previous {
				t.Fatalf("%s shift %d: %d bytes is not more than %d", name, shift, got, previous)
			}
			previous = got
			if shift == 0 {
				mxx, myy := d.f.mcuCounts()
				blocks := 0
				for c := 0; c < d.f.nComp; c++ {
					blocks += mxx * myy * d.f.comp[c].h * d.f.comp[c].v
				}
				if got != blocks*64*2 {
					t.Fatalf("%s: full set is %d bytes, want %d", name, got, blocks*128)
				}
			}
			d.Close()
		}
	}
}

func TestEightfoldReductionKeepsOnlyAFractionOfTheCoefficients(t *testing.T) {
	data := corpus(t)["video-001.progressive.jpeg"]
	d := open(t, data, Options{})
	defer d.Close()
	if _, err := d.Decode(t.Context(), 3); err != nil {
		t.Fatal(err)
	}
	mxx, myy := d.f.mcuCounts()
	coefficientBytes := 0
	for c := 0; c < d.f.nComp; c++ {
		coefficientBytes += len(d.store.coef[c]) * 2
	}
	pixels := d.Header().Width * d.Header().Height
	blocks := mxx * myy * 3
	if coefficientBytes != blocks*2 {
		t.Fatalf("%d coefficient bytes for %d blocks", coefficientBytes, blocks)
	}
	if coefficientBytes*30 > pixels*3 {
		t.Fatalf("%d coefficient bytes for %d pixels", coefficientBytes, pixels)
	}
}

func TestCoarserLevelsFromAFinerStoreEqualFreshDecodes(t *testing.T) {
	files := corpus(t)
	for _, name := range progressiveFiles {
		reused := open(t, files[name], Options{})
		if _, err := reused.Decode(t.Context(), 0); err != nil {
			t.Fatal(err)
		}
		for shift := 1; shift <= MaxShift; shift++ {
			got, err := reused.Decode(t.Context(), shift)
			if err != nil {
				t.Fatal(err)
			}
			want := decodeAt(t, files[name], shift, Options{})
			if !samePixels(got, want) {
				t.Fatalf("%s shift %d: %s", name, shift, firstDifference(got, want))
			}
		}
		reused.Close()
	}
}

func TestProgressiveRegionsFollowTheDecodedLevel(t *testing.T) {
	files := corpus(t)
	rng := rand.New(rand.NewSource(8))
	for _, name := range progressiveFiles {
		d := open(t, files[name], Options{})
		overview, err := d.Decode(t.Context(), 2)
		if err != nil {
			t.Fatal(err)
		}
		if d.MinShift() != 2 {
			t.Fatalf("%s: MinShift %d", name, d.MinShift())
		}
		for _, shift := range []int{0, 1} {
			if _, err := d.Region(t.Context(), shift, rect(0, 0, 8, 8)); err == nil {
				t.Fatalf("%s: region at shift %d served from a level-2 store", name, shift)
			}
		}
		for i := 0; i < 20; i++ {
			r := randomRect(rng, overview.Bounds().Dx(), overview.Bounds().Dy())
			got, err := d.Region(t.Context(), 2, r)
			if err != nil || !samePixels(got, crop(overview, r)) {
				t.Fatalf("%s: region %v: %v", name, r, err)
			}
		}
		coarse, err := d.Decode(t.Context(), 3)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 20; i++ {
			r := randomRect(rng, coarse.Bounds().Dx(), coarse.Bounds().Dy())
			got, err := d.Region(t.Context(), 3, r)
			if err != nil || !samePixels(got, crop(coarse, r)) {
				t.Fatalf("%s: level 3 region %v: %v", name, r, err)
			}
		}
		before := d.CoefficientBytes()
		if _, err := d.Decode(t.Context(), 1); err != nil {
			t.Fatal(err)
		}
		if d.MinShift() != 1 || d.CoefficientBytes() <= before {
			t.Fatalf("%s: finer decode did not replace the store (min shift %d)", name, d.MinShift())
		}
		d.Close()
		if d.CoefficientBytes() != 0 {
			t.Fatalf("%s: store survives Close", name)
		}
	}
}

func TestMultiScanSequentialFilesUseTheCoefficientPath(t *testing.T) {
	files := syntheticFiles()
	for _, name := range []string{"separate-444", "separate-420", "separate-444-restart"} {
		d := open(t, files[name], Options{})
		if d.CanRegion() {
			t.Fatalf("%s: CanRegion", name)
		}
		if _, err := d.Decode(t.Context(), 0); err != nil {
			t.Fatal(err)
		}
		if d.MinShift() != 0 {
			t.Fatalf("%s: MinShift %d after a full decode", name, d.MinShift())
		}
		full, _ := d.Decode(t.Context(), 0)
		got, err := d.Region(t.Context(), 0, image.Rect(3, 4, 30, 25))
		if err != nil || !samePixels(got, crop(full, image.Rect(3, 4, 30, 25))) {
			t.Fatalf("%s: %v", name, err)
		}
		d.Close()
	}
}

func TestSecondScanAfterACompleteScanIsReported(t *testing.T) {
	data := syntheticFiles()["444-150x103"]
	sos := lastIndex(data, []byte{0xff, 0xda})
	if sos < 0 {
		t.Fatal("no SOS")
	}
	end := len(data) - 2
	doubled := append(append([]byte(nil), data[:end]...), data[sos:end]...)
	doubled = append(doubled, 0xff, 0xd9)
	d := open(t, doubled, Options{})
	defer d.Close()
	_, err := d.Decode(t.Context(), 0)
	var unsupported UnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatalf("got %v", err)
	}
}

func lastIndex(data, pattern []byte) int {
	for i := len(data) - len(pattern); i >= 0; i-- {
		if string(data[i:i+len(pattern)]) == string(pattern) {
			return i
		}
	}
	return -1
}
