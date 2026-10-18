package rawdev

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/oops1/PixGo/internal/rawdev/rawdevtest"
)

func randomSamples(rng *rand.Rand, w, h, comps, precision int) []uint16 {
	out := make([]uint16, w*h*comps)
	mask := uint16(1<<uint(precision) - 1)
	base := rng.Intn(1 << uint(precision))
	for i := range out {
		if rng.Intn(16) == 0 {
			out[i] = uint16(rng.Intn(1<<uint(precision))) & mask
			continue
		}
		base += rng.Intn(9) - 4
		if base < 0 {
			base = 0
		}
		if base > int(mask) {
			base = int(mask)
		}
		out[i] = uint16(base)
	}
	return out
}

func decodeAll(t testing.TB, data []byte) ([]uint16, LJ92Info) {
	t.Helper()
	d, err := ParseLJ92(data)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]uint16, d.Info.Width*d.Info.Height*d.Info.Components)
	if err := d.Decode(context.Background(), out, d.Info.Width*d.Info.Components); err != nil {
		t.Fatal(err)
	}
	return out, d.Info
}

func TestLJ92RoundTripAcrossPredictorsPrecisionsAndComponents(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, precision := range []int{8, 10, 12, 14, 16} {
		for comps := 1; comps <= 4; comps++ {
			for pred := 1; pred <= 7; pred++ {
				w, h := 1+rng.Intn(40), 1+rng.Intn(20)
				src := randomSamples(rng, w, h, comps, precision)
				for _, lengths := range [][17]int{rawdevtest.LengthsShort, rawdevtest.LengthsLong} {
					data := rawdevtest.EncodeLJ92(w, h, comps, precision, src, rawdevtest.LJOptions{Predictor: pred, Lengths: lengths})
					got, info := decodeAll(t, data)
					if info.Width != w || info.Height != h || info.Components != comps || info.Precision != precision {
						t.Fatalf("header %+v", info)
					}
					for i := range src {
						if got[i] != src[i] {
							t.Fatalf("precision %d comps %d predictor %d: sample %d is %d, want %d", precision, comps, pred, i, got[i], src[i])
						}
					}
				}
			}
		}
	}
}

func TestLJ92ExtremeDifferences(t *testing.T) {
	src := []uint16{0, 65535, 0, 65535, 32768, 0, 65535, 1, 65534}
	for pred := 1; pred <= 7; pred++ {
		data := rawdevtest.EncodeLJ92(3, 3, 1, 16, src, rawdevtest.LJOptions{Predictor: pred, Lengths: rawdevtest.LengthsLong})
		got, _ := decodeAll(t, data)
		for i := range src {
			if got[i] != src[i] {
				t.Fatalf("predictor %d sample %d: %d want %d", pred, i, got[i], src[i])
			}
		}
	}
}

func TestLJ92PointTransform(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	w, h := 21, 9
	src := randomSamples(rng, w, h, 2, 14)
	for i := range src {
		src[i] &^= 3
	}
	data := rawdevtest.EncodeLJ92(w, h, 2, 14, src, rawdevtest.LJOptions{Predictor: 1, Pt: 2})
	got, _ := decodeAll(t, data)
	for i := range src {
		if got[i] != src[i] {
			t.Fatalf("sample %d: %d want %d", i, got[i], src[i])
		}
	}
}

func TestLJ92RestartIntervals(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for _, restart := range []int{1, 7, 16, 32} {
		w, h := 16, 12
		src := randomSamples(rng, w, h, 1, 12)
		data := rawdevtest.EncodeLJ92(w, h, 1, 12, src, rawdevtest.LJOptions{Predictor: 6, Restart: restart})
		got, _ := decodeAll(t, data)
		for i := range src {
			if got[i] != src[i] {
				t.Fatalf("restart %d sample %d: %d want %d", restart, i, got[i], src[i])
			}
		}
	}
}

func TestLJ92DecodesIntoAStridedDestination(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	w, h, comps := 10, 6, 2
	src := randomSamples(rng, w, h, comps, 12)
	d, err := ParseLJ92(rawdevtest.EncodeLJ92(w, h, comps, 12, src, rawdevtest.LJOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	stride := w*comps + 5
	dst := make([]uint16, stride*h)
	for i := range dst {
		dst[i] = 0xdead
	}
	if err := d.Decode(context.Background(), dst, stride); err != nil {
		t.Fatal(err)
	}
	for y := 0; y < h; y++ {
		for i := 0; i < w*comps; i++ {
			if dst[y*stride+i] != src[y*w*comps+i] {
				t.Fatalf("row %d sample %d", y, i)
			}
		}
		for i := w * comps; i < stride && y < h-1; i++ {
			if dst[y*stride+i] != 0xdead {
				t.Fatalf("row %d padding touched", y)
			}
		}
	}
	if err := d.Decode(context.Background(), dst[:stride*h-stride], stride); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("short destination: %v", err)
	}
	if err := d.Decode(context.Background(), dst, w*comps-1); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("short stride: %v", err)
	}
}

func TestLJ92TruncationIsAnError(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	w, h := 64, 32
	src := randomSamples(rng, w, h, 1, 14)
	data := rawdevtest.EncodeLJ92(w, h, 1, 14, src, rawdevtest.LJOptions{})
	for cut := 0; cut < len(data)-2; cut += 7 {
		d, err := ParseLJ92(data[:cut])
		if err != nil {
			if !errors.Is(err, ErrCorrupt) && !errors.Is(err, ErrUnsupported) {
				t.Fatalf("cut %d: %v", cut, err)
			}
			continue
		}
		err = d.Decode(context.Background(), make([]uint16, w*h), w)
		if err == nil {
			t.Fatalf("cut %d decoded", cut)
		}
		if !errors.Is(err, ErrCorrupt) {
			t.Fatalf("cut %d: %v", cut, err)
		}
	}
}

func TestLJ92HeaderErrors(t *testing.T) {
	good := rawdevtest.EncodeLJ92(4, 4, 1, 12, make([]uint16, 16), rawdevtest.LJOptions{})
	cases := map[string]func([]byte) []byte{
		"no SOI": func(b []byte) []byte { b[0] = 0; return b },
		"empty":  func(b []byte) []byte { return nil },
		"baseline": func(b []byte) []byte {
			for i := range b {
				if b[i] == 0xff && b[i+1] == 0xc3 {
					b[i+1] = 0xc0
				}
			}
			return b
		},
	}
	for name, mutate := range cases {
		if _, err := ParseLJ92(mutate(append([]byte(nil), good...))); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for i := 2; i < len(good)-2; i++ {
		for _, v := range []byte{0, 0xff, 0x80, 1} {
			b := append([]byte(nil), good...)
			b[i] = v
			d, err := ParseLJ92(b)
			if err != nil {
				continue
			}
			if d.Info.Width*d.Info.Height*d.Info.Components > 1<<22 {
				continue
			}
			d.Decode(context.Background(), make([]uint16, d.Info.Width*d.Info.Height*d.Info.Components), d.Info.Width*d.Info.Components)
		}
	}
}

func TestLJ92Cancellation(t *testing.T) {
	w, h := 32, 64
	data := rawdevtest.EncodeLJ92(w, h, 1, 12, make([]uint16, w*h), rawdevtest.LJOptions{})
	d, err := ParseLJ92(data)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.Decode(ctx, make([]uint16, w*h), w); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func FuzzLJ92(f *testing.F) {
	rng := rand.New(rand.NewSource(6))
	for _, o := range []rawdevtest.LJOptions{{}, {Predictor: 4, Lengths: rawdevtest.LengthsLong}, {Predictor: 7, Restart: 5}} {
		src := randomSamples(rng, 9, 7, 2, 12)
		f.Add(rawdevtest.EncodeLJ92(9, 7, 2, 12, src, o))
	}
	f.Add([]byte{0xff, 0xd8, 0xff, 0xc3, 0, 8, 12, 0, 1, 0, 1, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := ParseLJ92(data)
		if err != nil {
			return
		}
		n := int64(d.Info.Width) * int64(d.Info.Height) * int64(d.Info.Components)
		if n > 1<<20 {
			return
		}
		d.Decode(context.Background(), make([]uint16, n), d.Info.Width*d.Info.Components)
	})
}
