package edit

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/imaging"
)

func inside(c *Crop) bool {
	return c.Rect().In(image.Rectangle{Max: c.Size()}) && !c.Rect().Empty()
}

func TestNewCropIsWhole(t *testing.T) {
	c := NewCrop(640, 480)
	if !c.Whole() || c.Rect() != image.Rect(0, 0, 640, 480) || c.Aspect() != AspectFree {
		t.Fatalf("got %v whole=%v aspect=%v", c.Rect(), c.Whole(), c.Aspect())
	}
}

func TestFreeDragEveryHandle(t *testing.T) {
	start := image.Rect(100, 80, 300, 240)
	cases := []struct {
		h      Handle
		dx, dy float64
		want   image.Rectangle
	}{
		{HandleNW, 10, 20, image.Rect(110, 100, 300, 240)},
		{HandleN, 99, -30, image.Rect(100, 50, 300, 240)},
		{HandleNE, -20, 10, image.Rect(100, 90, 280, 240)},
		{HandleE, 15, 99, image.Rect(100, 80, 315, 240)},
		{HandleSE, 5, 7, image.Rect(100, 80, 305, 247)},
		{HandleS, 0, -10, image.Rect(100, 80, 300, 230)},
		{HandleSW, -40, 8, image.Rect(60, 80, 300, 248)},
		{HandleW, 25, 0, image.Rect(125, 80, 300, 240)},
		{HandleMove, 12, -9, image.Rect(112, 71, 312, 231)},
	}
	for _, tc := range cases {
		c := NewCrop(640, 480)
		c.Drag(tc.h, start, tc.dx, tc.dy)
		if c.Rect() != tc.want {
			t.Errorf("handle %d: got %v want %v", tc.h, c.Rect(), tc.want)
		}
	}
}

func TestFreeDragClampsToThePicture(t *testing.T) {
	start := image.Rect(100, 80, 300, 240)
	c := NewCrop(640, 480)
	c.Drag(HandleNW, start, -1e6, -1e6)
	if c.Rect() != image.Rect(0, 0, 300, 240) {
		t.Fatalf("NW: %v", c.Rect())
	}
	c.Drag(HandleSE, start, 1e6, 1e6)
	if c.Rect() != image.Rect(100, 80, 640, 480) {
		t.Fatalf("SE: %v", c.Rect())
	}
	c.Drag(HandleMove, start, 1e6, 1e6)
	if c.Rect() != image.Rect(440, 320, 640, 480) {
		t.Fatalf("move: %v", c.Rect())
	}
	c.Drag(HandleMove, start, -1e6, -1e6)
	if c.Rect() != image.Rect(0, 0, 200, 160) {
		t.Fatalf("move back: %v", c.Rect())
	}
}

func TestMinimumSize(t *testing.T) {
	start := image.Rect(100, 80, 300, 240)
	for _, h := range handleList {
		c := NewCrop(640, 480)
		c.Drag(h, start, 1e6, 1e6)
		c.Drag(h, start, -1e6, -1e6)
		r := c.Rect()
		if r.Dx() < MinSide || r.Dy() < MinSide {
			t.Errorf("handle %d: %v is below the minimum", h, r)
		}
	}
	c := NewCrop(640, 480)
	c.Drag(HandleSE, start, -1e6, -1e6)
	if c.Rect() != image.Rect(100, 80, 100+MinSide, 80+MinSide) {
		t.Fatalf("got %v", c.Rect())
	}
	c.Drag(HandleNW, start, 1e6, 1e6)
	if c.Rect() != image.Rect(300-MinSide, 240-MinSide, 300, 240) {
		t.Fatalf("got %v", c.Rect())
	}
}

func TestSmallPictureMinimumShrinks(t *testing.T) {
	c := NewCrop(10, 40)
	c.Drag(HandleSE, c.Rect(), -1e6, -1e6)
	if c.Rect().Dx() < 10 || c.Rect().Dy() < 10 {
		t.Fatalf("got %v", c.Rect())
	}
}

var presets = []struct {
	a      Aspect
	ratio  float64
	pw, ph int
}{
	{AspectSquare, 1, 1, 1},
	{Aspect43, 4.0 / 3, 4, 3},
	{Aspect32, 3.0 / 2, 3, 2},
	{Aspect169, 16.0 / 9, 16, 9},
}

func ratioOK(r image.Rectangle, want float64) bool {
	w, h := float64(r.Dx()), float64(r.Dy())
	return math.Abs(w-h*want) <= 0.5*math.Max(1, want)+1e-9 || math.Abs(h-w/want) <= 0.5+1e-9
}

func TestAspectLockEveryPresetEveryHandle(t *testing.T) {
	deltas := [][2]float64{{0, 0}, {37, 11}, {-52, 90}, {-130, -40}, {200, 300}, {1e6, 1e6}, {-1e6, -1e6}, {1e6, -1e6}, {-1e6, 1e6}}
	for _, p := range presets {
		for _, h := range handleList {
			for _, d := range deltas {
				c := NewCrop(1200, 800)
				c.SetAspect(p.a)
				start := c.Rect()
				if !ratioOK(start, c.Ratio()) {
					t.Fatalf("%d initial %v is not %.3f", p.a, start, c.Ratio())
				}
				c.SetRect(image.Rect(300, 200, 300+p.pw*60, 200+p.ph*60))
				start = c.Rect()
				c.Drag(h, start, d[0], d[1])
				r := c.Rect()
				if !inside(c) {
					t.Fatalf("aspect %d handle %d delta %v: %v leaves the picture", p.a, h, d, r)
				}
				if r.Dx() < MinSide || r.Dy() < MinSide {
					t.Fatalf("aspect %d handle %d delta %v: %v too small", p.a, h, d, r)
				}
				if !ratioOK(r, c.Ratio()) {
					t.Fatalf("aspect %d handle %d delta %v: %v is not %.3f", p.a, h, d, r, c.Ratio())
				}
			}
		}
	}
}

func TestSquareIsExact(t *testing.T) {
	c := NewCrop(900, 700)
	c.SetAspect(AspectSquare)
	for _, h := range handleList {
		c.Drag(h, image.Rect(100, 100, 300, 300), 37.3, -21.9)
		if c.Rect().Dx() != c.Rect().Dy() {
			t.Fatalf("handle %d: %v is not square", h, c.Rect())
		}
	}
}

func TestAspectLockCornerKeepsTheAnchor(t *testing.T) {
	c := NewCrop(1600, 900)
	c.SetAspect(Aspect169)
	start := image.Rect(400, 200, 400+320, 200+180)
	c.SetRect(start)
	c.Drag(HandleSE, start, 160, 5)
	r := c.Rect()
	if r.Min != start.Min || r.Dx() != 480 {
		t.Fatalf("got %v", r)
	}
	c.Drag(HandleNW, start, -80, -10)
	r = c.Rect()
	if r.Max != start.Max || r.Dx() != 400 {
		t.Fatalf("got %v", r)
	}
}

func TestAspectLockEdgeGrowsAroundTheCentre(t *testing.T) {
	c := NewCrop(1600, 900)
	c.SetAspect(Aspect43)
	start := image.Rect(400, 300, 400+400, 300+300)
	c.SetRect(start)
	c.Drag(HandleE, start, 80, 0)
	r := c.Rect()
	if r.Min.X != 400 || r.Dx() != 480 || r.Dy() != 360 {
		t.Fatalf("E: %v", r)
	}
	if got := (r.Min.Y + r.Max.Y) / 2; got != 450 {
		t.Fatalf("not centred: %v", r)
	}
	c.Drag(HandleN, start, 0, -60)
	r = c.Rect()
	if r.Max.Y != start.Max.Y || r.Dy() != 360 {
		t.Fatalf("N: %v", r)
	}
}

func TestSetAspectFitsInsideTheCurrentRect(t *testing.T) {
	c := NewCrop(1200, 800)
	c.SetRect(image.Rect(100, 100, 700, 500))
	c.SetAspect(Aspect169)
	r := c.Rect()
	if !r.In(image.Rect(100, 100, 700, 500)) || !ratioOK(r, 16.0/9) {
		t.Fatalf("got %v", r)
	}
	if r.Dx() != 600 {
		t.Fatalf("should use the full width, got %v", r)
	}
	c.SetAspect(AspectFree)
	if c.Rect() != r {
		t.Fatal("free must not change the rect")
	}
}

func TestOriginalFollowsThePicture(t *testing.T) {
	c := NewCrop(1200, 800)
	c.SetAspect(AspectOriginal)
	if !c.Whole() {
		t.Fatalf("whole picture has the original ratio already, got %v", c.Rect())
	}
	c.SetRect(image.Rect(100, 100, 700, 600))
	c.SetAspect(AspectOriginal)
	if !ratioOK(c.Rect(), 1.5) {
		t.Fatalf("got %v", c.Rect())
	}
	p := NewCrop(800, 1200)
	p.SetAspect(AspectOriginal)
	if p.Ratio() >= 1 {
		t.Fatalf("a portrait picture has a ratio below 1, got %v", p.Ratio())
	}
}

func TestPortraitPictureStartsPortrait(t *testing.T) {
	c := NewCrop(800, 1200)
	c.SetAspect(Aspect169)
	if r := c.Rect(); r.Dy() <= r.Dx() {
		t.Fatalf("got %v", r)
	}
}

func TestSwapExchangesOrientation(t *testing.T) {
	c := NewCrop(1600, 1200)
	c.SetAspect(Aspect169)
	r := c.Rect()
	if r.Dx() <= r.Dy() {
		t.Fatalf("landscape expected, got %v", r)
	}
	c.Swap()
	s := c.Rect()
	if s.Dy() <= s.Dx() || !ratioOK(s, 9.0/16) || !c.Portrait() {
		t.Fatalf("got %v", s)
	}
	c.Swap()
	if got := c.Rect(); !ratioOK(got, 16.0/9) || got.Dx() <= got.Dy() {
		t.Fatalf("got %v", got)
	}
}

func TestSwapFreeTurnsTheRect(t *testing.T) {
	c := NewCrop(1000, 1000)
	c.SetRect(image.Rect(100, 100, 500, 300))
	c.Swap()
	if got := c.Rect(); got.Dx() != 200 || got.Dy() != 400 {
		t.Fatalf("got %v", got)
	}
	if got := c.Rect(); (got.Min.X+got.Max.X) != 600 || (got.Min.Y+got.Max.Y) != 400 {
		t.Fatalf("not around the centre: %v", got)
	}
}

func TestNudgeAndGrow(t *testing.T) {
	c := NewCrop(100, 100)
	c.SetRect(image.Rect(10, 10, 50, 50))
	c.Nudge(1, 0)
	c.Nudge(0, 10)
	if c.Rect() != image.Rect(11, 20, 51, 60) {
		t.Fatalf("got %v", c.Rect())
	}
	c.Nudge(-100, 500)
	if c.Rect() != image.Rect(0, 60, 40, 100) {
		t.Fatalf("got %v", c.Rect())
	}
	c.Grow(5, 0)
	if c.Rect() != image.Rect(0, 60, 45, 100) {
		t.Fatalf("got %v", c.Rect())
	}
	c.Grow(-1000, -1000)
	if c.Rect().Dx() != MinSide || c.Rect().Dy() != MinSide {
		t.Fatalf("got %v", c.Rect())
	}
}

func TestGrowKeepsTheRatio(t *testing.T) {
	c := NewCrop(1600, 900)
	c.SetAspect(Aspect169)
	c.SetRect(image.Rect(100, 100, 100+320, 100+180))
	c.Grow(16, 0)
	r := c.Rect()
	if !ratioOK(r, 16.0/9) || r.Dx() != 336 || r.Min != image.Pt(100, 100) {
		t.Fatalf("got %v", r)
	}
}

func marked(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x * 255 / max(w-1, 1)), uint8(y * 255 / max(h-1, 1)), uint8((x + y) % 251), 255})
		}
	}
	return img
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

func same(a, b *image.NRGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}

func TestRotateAndFlipKeepTheSameContent(t *testing.T) {
	ops := []struct {
		name   string
		orient codec.Orientation
		apply  func(*Crop)
	}{
		{"cw", codec.OrientNormal.RotateCW(), func(c *Crop) { c.Rotate(true) }},
		{"ccw", codec.OrientNormal.RotateCCW(), func(c *Crop) { c.Rotate(false) }},
		{"flipH", codec.OrientNormal.FlipH(), func(c *Crop) { c.FlipH() }},
		{"flipV", codec.OrientNormal.FlipV(), func(c *Crop) { c.FlipV() }},
	}
	img := marked(97, 61)
	for _, op := range ops {
		for _, r := range []image.Rectangle{image.Rect(0, 0, 97, 61), image.Rect(5, 7, 60, 40), image.Rect(30, 2, 97, 33), image.Rect(0, 20, 20, 61)} {
			c := NewCrop(97, 61)
			c.SetRect(r)
			before := cropOf(img, c.Rect())
			op.apply(c)
			turned := imaging.Orient(img, op.orient)
			if c.Size() != turned.Bounds().Size() {
				t.Fatalf("%s: size %v want %v", op.name, c.Size(), turned.Bounds().Size())
			}
			after := cropOf(turned, c.Rect())
			want := imaging.Orient(before, op.orient)
			if !same(after, want) {
				t.Fatalf("%s %v: the content under the rect changed (%v)", op.name, r, c.Rect())
			}
		}
	}
}

func TestFourRotationsAreIdentity(t *testing.T) {
	c := NewCrop(300, 200)
	c.SetRect(image.Rect(10, 20, 150, 90))
	want := c.Rect()
	for i := 0; i < 4; i++ {
		c.Rotate(true)
	}
	if c.Rect() != want || c.Size() != image.Pt(300, 200) {
		t.Fatalf("got %v %v", c.Rect(), c.Size())
	}
	c.Rotate(true)
	c.Rotate(false)
	if c.Rect() != want {
		t.Fatalf("got %v", c.Rect())
	}
}

func TestRotateSwapsThePortraitFlag(t *testing.T) {
	c := NewCrop(1600, 900)
	c.SetAspect(Aspect43)
	c.SetRect(image.Rect(0, 0, 400, 300))
	c.Rotate(true)
	if !c.Portrait() || !ratioOK(c.Rect(), 3.0/4) {
		t.Fatalf("got %v portrait=%v", c.Rect(), c.Portrait())
	}
	c.FlipH()
	if !c.Portrait() {
		t.Fatal("a flip must not change the orientation of the ratio")
	}
}

func TestResetRestoresEverything(t *testing.T) {
	c := NewCrop(500, 300)
	c.SetAspect(Aspect32)
	c.SetRect(image.Rect(10, 10, 200, 130))
	c.Rotate(true)
	c.Reset()
	if !c.Whole() || c.Aspect() != AspectFree || c.Portrait() {
		t.Fatalf("got %v %v", c.Rect(), c.Aspect())
	}
}

func TestSetRectRejectsTooSmall(t *testing.T) {
	c := NewCrop(100, 100)
	c.SetRect(image.Rect(10, 10, 20, 20))
	if !c.Whole() {
		t.Fatal("a rect below the minimum must be ignored")
	}
	c.SetRect(image.Rect(-20, -20, 500, 60))
	if c.Rect() != image.Rect(0, 0, 100, 60) {
		t.Fatalf("got %v", c.Rect())
	}
}

func FuzzCropStaysValid(f *testing.F) {
	f.Add(uint8(1), uint8(2), 10.0, 20.0, uint8(3))
	f.Fuzz(func(t *testing.T, handle, aspect uint8, dx, dy float64, ops uint8) {
		if math.IsNaN(dx) || math.IsNaN(dy) || math.IsInf(dx, 0) || math.IsInf(dy, 0) {
			return
		}
		c := NewCrop(500, 333)
		c.SetAspect(Aspects[int(aspect)%len(Aspects)])
		for i := uint8(0); i < ops%5; i++ {
			c.Rotate(i%2 == 0)
		}
		h := Handle(handle%9 + 1)
		c.Drag(h, c.Rect(), dx, dy)
		if !inside(c) {
			t.Fatalf("handle %d aspect %d delta %v,%v: %v in %v", h, aspect, dx, dy, c.Rect(), c.Size())
		}
		if c.Rect().Dx() < min(MinSide, c.Size().X) || c.Rect().Dy() < min(MinSide, c.Size().Y) {
			t.Fatalf("too small: %v", c.Rect())
		}
	})
}
