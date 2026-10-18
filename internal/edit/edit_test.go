package edit

import (
	"context"
	"errors"
	"image"
	"image/color"
	"runtime"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/imaging"
)

var allOrientations = []codec.Orientation{
	codec.OrientNormal, codec.OrientFlipH, codec.OrientRotate180, codec.OrientFlipV,
	codec.OrientTranspose, codec.OrientRotate90, codec.OrientTransverse, codec.OrientRotate270,
}

func cropOf(img *image.NRGBA, r image.Rectangle) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			out.SetNRGBA(x, y, img.NRGBAAt(r.Min.X+x, r.Min.Y+y))
		}
	}
	return out
}

func TestPlanGeometry(t *testing.T) {
	p := Plan{Orientation: codec.OrientRotate90, Crop: image.Rect(10, 20, 110, 90)}
	if w, h := p.OrientedSize(300, 200); w != 200 || h != 300 {
		t.Fatalf("oriented %dx%d", w, h)
	}
	if got := p.CropRect(300, 200); got != image.Rect(10, 20, 110, 90) {
		t.Fatalf("crop %v", got)
	}
	if w, h := p.OutputSize(300, 200); w != 100 || h != 70 {
		t.Fatalf("output %dx%d", w, h)
	}
	if p.Resizes(300, 200) || !p.LosslessCapable(300, 200) || !p.Changes(300, 200) {
		t.Fatal("a crop alone is lossless-capable and a change")
	}
	p.Size = image.Pt(50, 35)
	if !p.Resizes(300, 200) || p.LosslessCapable(300, 200) {
		t.Fatal("a size makes the plan lossy")
	}
	if got := (Plan{Crop: image.Rect(-50, -50, 5000, 5000)}).CropRect(300, 200); got != image.Rect(0, 0, 300, 200) {
		t.Fatalf("crop is clamped, got %v", got)
	}
	if (Plan{}).Changes(300, 200) {
		t.Fatal("an empty plan changes nothing")
	}
	if (Plan{Orientation: 99}).Changes(10, 10) {
		t.Fatal("an invalid orientation is the identity")
	}
	if !(Plan{Size: image.Pt(10, 10)}).Changes(20, 20) {
		t.Fatal("a size is a change")
	}
}

func TestApplyWithoutResizeMatchesOrientThenCrop(t *testing.T) {
	src := noisy(37, 23, 2, true)
	for _, o := range allOrientations {
		ow, oh := o.Size(37, 23)
		for _, crop := range []image.Rectangle{{}, image.Rect(0, 0, ow, oh), image.Rect(3, 2, ow-4, oh-1), image.Rect(ow-5, oh-3, ow, oh), image.Rect(0, 0, 1, 1), image.Rect(ow/2, 0, ow, oh/2)} {
			p := Plan{Orientation: o, Crop: crop}
			got, err := Apply(context.Background(), src, p)
			if err != nil {
				t.Fatal(err)
			}
			full := imaging.Orient(src, o)
			want := cropOf(full, p.CropRect(37, 23))
			if got.Bounds() != want.Bounds() || maxDiff(got, want) != 0 {
				t.Fatalf("orientation %d crop %v: differs from orient then crop", o, crop)
			}
		}
	}
}

func TestApplyDoesNotAliasTheSource(t *testing.T) {
	src := noisy(20, 10, 4, false)
	out, err := Apply(context.Background(), src, Plan{})
	if err != nil {
		t.Fatal(err)
	}
	out.Pix[0] ^= 0xff
	if src.Pix[0] == out.Pix[0] {
		t.Fatal("the result shares memory with the source")
	}
}

func TestApplyResizeMatchesOrientCropResample(t *testing.T) {
	src := gradient(61, 43)
	for _, o := range allOrientations {
		ow, oh := o.Size(61, 43)
		for _, sz := range []image.Point{{X: 20, Y: 15}, {X: 130, Y: 97}, {X: 7, Y: 200}} {
			p := Plan{Orientation: o, Crop: image.Rect(2, 3, ow-5, oh-2), Size: sz}
			got, err := Apply(context.Background(), src, p)
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds() != image.Rect(0, 0, sz.X, sz.Y) {
				t.Fatalf("orientation %d size %v: bounds %v", o, sz, got.Bounds())
			}
			want := resample(t, cropOf(imaging.Orient(src, o), p.CropRect(61, 43)), sz.X, sz.Y)
			d := maxDiff(got, want)
			if o == codec.OrientNormal && d != 0 {
				t.Fatalf("no orientation: difference %d, want exact", d)
			}
			if d > 1 {
				t.Fatalf("orientation %d size %v: difference %d, want at most 1", o, sz, d)
			}
		}
	}
}

func TestApplyErrors(t *testing.T) {
	src := solid(10, 10, color.NRGBA{A: 255})
	if _, err := Apply(context.Background(), nil, Plan{}); !errors.Is(err, ErrEmptyPicture) {
		t.Fatalf("nil: %v", err)
	}
	if _, err := Apply(context.Background(), src, Plan{Crop: image.Rect(20, 20, 30, 30)}); !errors.Is(err, ErrEmptyCrop) {
		t.Fatalf("outside crop: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Apply(ctx, src, Plan{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	if _, err := Apply(context.Background(), src, Plan{Size: image.Pt(1<<20, 1<<20)}); !errors.Is(err, ErrSize) {
		t.Fatalf("huge size: %v", err)
	}
}

func TestApplyOnSubImageSource(t *testing.T) {
	big := noisy(80, 60, 3, false)
	sub := big.SubImage(image.Rect(10, 5, 50, 45)).(*image.NRGBA)
	flat := cropOf(big, image.Rect(10, 5, 50, 45))
	for _, o := range allOrientations {
		p := Plan{Orientation: o, Crop: image.Rect(1, 2, 30, 31), Size: image.Pt(12, 14)}
		a, err := Apply(context.Background(), sub, p)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Apply(context.Background(), flat, p)
		if err != nil {
			t.Fatal(err)
		}
		if maxDiff(a, b) != 0 {
			t.Fatalf("orientation %d: a sub-image source gives another result", o)
		}
	}
}

func TestApplyCropsBeforeOrientingToSaveMemory(t *testing.T) {
	src := noisy(2000, 1500, 1, false)
	p := Plan{Orientation: codec.OrientRotate90, Crop: image.Rect(0, 0, 40, 30)}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	res, err := Apply(context.Background(), src, p)
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if used := after.TotalAlloc - before.TotalAlloc; used > 1<<20 {
		t.Fatalf("a 40x30 crop of a 12 MB picture allocated %d bytes", used)
	}
	if cap(res.Pix) > 40*30*4*2 {
		t.Fatalf("the result holds %d bytes for a 40x30 picture", cap(res.Pix))
	}
}
