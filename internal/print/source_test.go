package print

import (
	"context"
	"errors"
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/imaging"
)

func pattern(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x*40 + 10), G: uint8(y*50 + 10), B: uint8((x+y)*20 + 5), A: 255})
		}
	}
	return img
}

func equalImages(a, b *image.NRGBA) bool {
	if a.Bounds().Size() != b.Bounds().Size() {
		return false
	}
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if a.NRGBAAt(a.Bounds().Min.X+x, a.Bounds().Min.Y+y) != b.NRGBAAt(b.Bounds().Min.X+x, b.Bounds().Min.Y+y) {
				return false
			}
		}
	}
	return true
}

func TestImageSourceMatchesTheOrientedPictureAtEveryOrientation(t *testing.T) {
	src := pattern(5, 3)
	for o := codec.OrientNormal; o <= codec.OrientRotate270; o++ {
		s := ImageSource{Img: src, Orient: o}
		want := imaging.Orient(src, o)
		if s.Size() != want.Bounds().Size() {
			t.Fatalf("orientation %d: size %v, want %v", o, s.Size(), want.Bounds().Size())
		}
		full := image.Rectangle{Max: s.Size()}
		got, err := s.Render(context.Background(), full, full.Dx(), full.Dy())
		if err != nil {
			t.Fatal(err)
		}
		if !equalImages(got, want) {
			t.Errorf("orientation %d: the 1:1 render differs from the oriented picture", o)
		}
		crop := image.Rect(1, 1, full.Dx(), full.Dy()-1)
		got, err = s.Render(context.Background(), crop, crop.Dx(), crop.Dy())
		if err != nil {
			t.Fatal(err)
		}
		if !equalImages(got, want.SubImage(crop).(*image.NRGBA)) {
			t.Errorf("orientation %d: the cropped render differs", o)
		}
	}
}

func TestImageSourceResamplesToTheRequestedSize(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 200, 100, 50, 255
	}
	for _, o := range []codec.Orientation{codec.OrientNormal, codec.OrientRotate90} {
		got, err := ImageSource{Img: src, Orient: o}.Render(context.Background(), image.Rectangle{Max: ImageSource{Img: src, Orient: o}.Size()}, 17, 9)
		if err != nil {
			t.Fatal(err)
		}
		if got.Bounds().Size() != image.Pt(17, 9) {
			t.Errorf("orientation %d: %v", o, got.Bounds().Size())
		}
		if c := got.NRGBAAt(8, 4); c.R < 195 || c.R > 205 || c.A != 255 {
			t.Errorf("a flat colour must stay flat: %v", c)
		}
	}
}

func TestImageSourceRejectsBadRequests(t *testing.T) {
	s := ImageSource{Img: pattern(4, 4)}
	ctx := context.Background()
	if _, err := s.Render(ctx, image.Rect(0, 0, 5, 5), 4, 4); !errors.Is(err, ErrNoPicture) {
		t.Errorf("crop outside: %v", err)
	}
	if _, err := s.Render(ctx, image.Rect(0, 0, 4, 4), 0, 4); !errors.Is(err, ErrTooLarge) {
		t.Errorf("zero width: %v", err)
	}
	if _, err := (ImageSource{}).Render(ctx, image.Rect(0, 0, 1, 1), 1, 1); !errors.Is(err, ErrNoPicture) {
		t.Errorf("no image: %v", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := (ImageSource{Img: pattern(40, 40)}).Render(cctx, image.Rect(0, 0, 40, 40), 10, 10); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
}

func TestRenderPlacesThePictureOnAWhiteSheet(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 30, 20))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 10, 20, 30, 255
	}
	o := DefaultOptions()
	o.Margin = MarginNormal
	pg, err := o.Layout(Picture{W: 30, H: 20}, 72)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := Render(context.Background(), ImageSource{Img: src}, pg)
	if err != nil {
		t.Fatal(err)
	}
	if sheet.Bounds().Size() != pg.Sheet {
		t.Fatalf("sheet %v, page %v", sheet.Bounds().Size(), pg.Sheet)
	}
	if c := sheet.RGBAAt(0, 0); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("corner is not white: %v", c)
	}
	mid := image.Pt((pg.Dest.Min.X+pg.Dest.Max.X)/2, (pg.Dest.Min.Y+pg.Dest.Max.Y)/2)
	if c := sheet.RGBAAt(mid.X, mid.Y); c != (color.RGBA{10, 20, 30, 255}) {
		t.Errorf("centre: %v", c)
	}
	if c := sheet.RGBAAt(pg.Dest.Min.X-1, mid.Y); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("just outside the picture: %v", c)
	}
}

func TestRenderBlendsTransparencyWithPaperWhite(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			a := uint8(0)
			if x >= 2 {
				a = 128
			}
			src.SetNRGBA(x, y, color.NRGBA{0, 0, 0, a})
		}
	}
	pg := Page{Sheet: image.Pt(8, 8), Dest: image.Rect(2, 2, 6, 6), Crop: image.Rect(0, 0, 4, 4), DPI: 72}
	sheet, err := Render(context.Background(), ImageSource{Img: src}, pg)
	if err != nil {
		t.Fatal(err)
	}
	if c := sheet.RGBAAt(2, 3); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("transparent pixel: %v", c)
	}
	if c := sheet.RGBAAt(5, 3); c.R < 125 || c.R > 130 || c.A != 255 {
		t.Errorf("half transparent black over white must be mid grey: %v", c)
	}
}

type fakeTiles struct {
	mu      sync.Mutex
	w, h    int
	min     int
	regions []tileCall
	closed  int
}

type tileCall struct {
	shift int
	rect  image.Rectangle
}

func (f *fakeTiles) Info() codec.Info { return codec.Info{Width: f.w, Height: f.h} }
func (f *fakeTiles) MinShift() int    { return f.min }
func (f *fakeTiles) Overview(ctx context.Context, shift int) (*image.NRGBA, error) {
	return nil, errors.New("not used")
}
func (f *fakeTiles) Region(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA, error) {
	f.mu.Lock()
	f.regions = append(f.regions, tileCall{shift, r})
	f.mu.Unlock()
	img := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = uint8(shift*20), 90, 160, 255
	}
	return img, nil
}
func (f *fakeTiles) Close() error {
	f.mu.Lock()
	f.closed++
	f.mu.Unlock()
	return nil
}

func TestTiledSourceReadsAReducedLevelNotTheFullPicture(t *testing.T) {
	ft := &fakeTiles{w: 16000, h: 12000}
	s := TiledSource{Open: func(context.Context) (codec.TileReader, error) { return ft, nil }, Dim: image.Pt(16000, 12000)}
	crop := image.Rect(0, 0, 16000, 12000)
	img, err := s.Render(context.Background(), crop, 1000, 750)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Size() != image.Pt(1000, 750) {
		t.Fatalf("size %v", img.Bounds().Size())
	}
	if len(ft.regions) != 1 {
		t.Fatalf("%d region reads", len(ft.regions))
	}
	got := ft.regions[0]
	if got.shift < 3 {
		t.Errorf("shift %d: 16000x12000 for a 1000x750 page needs at least level 3", got.shift)
	}
	lr := got.rect
	if lr.Dx() < 1000 || lr.Dy() < 750 {
		t.Errorf("the level %v is smaller than the page area", lr)
	}
	if lr.Dx()*lr.Dy() > 4*1000*750 {
		t.Errorf("the level %v is more than 4x the page pixels", lr)
	}
	if ft.closed != 1 {
		t.Errorf("reader closed %d times", ft.closed)
	}
}

func TestTiledSourceNeverReadsPastThePixelLimit(t *testing.T) {
	ft := &fakeTiles{w: 40000, h: 30000}
	s := TiledSource{Open: func(context.Context) (codec.TileReader, error) { return ft, nil }, Dim: image.Pt(40000, 30000), MaxPixels: 2_000_000}
	if _, err := s.Render(context.Background(), image.Rect(0, 0, 40000, 30000), 3000, 2000); err != nil {
		t.Fatal(err)
	}
	r := ft.regions[0].rect
	if int64(r.Dx())*int64(r.Dy()) > 2_000_000 {
		t.Errorf("read %v", r)
	}
}

func TestTiledSourceHonoursOrientationAndCrop(t *testing.T) {
	ft := &fakeTiles{w: 4000, h: 2000}
	s := TiledSource{Open: func(context.Context) (codec.TileReader, error) { return ft, nil }, Dim: image.Pt(4000, 2000), Orient: codec.OrientRotate90}
	if s.Size() != image.Pt(2000, 4000) {
		t.Fatalf("size %v", s.Size())
	}
	img, err := s.Render(context.Background(), image.Rect(0, 1000, 2000, 3000), 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Size() != image.Pt(100, 100) {
		t.Fatalf("size %v", img.Bounds().Size())
	}
	r := ft.regions[0]
	scale := 1 << uint(r.shift)
	if got := image.Rect(r.rect.Min.X*scale, r.rect.Min.Y*scale, r.rect.Max.X*scale, r.rect.Max.Y*scale); got.Dx() < 1900 || got.Dx() > 2100 || got.Dy() < 1900 || got.Dy() > 2100 {
		t.Errorf("the stored area read is %v, expected about 2000x2000", got)
	}
}

func TestTiledSourcePropagatesOpenAndReadErrors(t *testing.T) {
	boom := errors.New("boom")
	s := TiledSource{Open: func(context.Context) (codec.TileReader, error) { return nil, boom }, Dim: image.Pt(10, 10)}
	if _, err := s.Render(context.Background(), image.Rect(0, 0, 10, 10), 5, 5); !errors.Is(err, boom) {
		t.Errorf("open error: %v", err)
	}
}

func TestVectorSourceDrawsAtThePrintResolution(t *testing.T) {
	var asked []image.Point
	decode := func(ctx context.Context, w, h int) (*image.NRGBA, error) {
		asked = append(asked, image.Pt(w, h))
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 1, 2, 3, 255
		}
		return img, nil
	}
	s := VectorSource{Dim: image.Pt(100, 50), Decode: decode}
	img, err := s.Render(context.Background(), image.Rect(0, 0, 100, 50), 2000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != image.Pt(2000, 1000) {
		t.Errorf("rasterised at %v", asked)
	}
	if img.Bounds().Size() != image.Pt(2000, 1000) {
		t.Errorf("size %v", img.Bounds().Size())
	}
	asked = nil
	if _, err := s.Render(context.Background(), image.Rect(25, 0, 75, 50), 1000, 1000); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != image.Pt(2000, 1000) {
		t.Errorf("a half-width crop drawn into 1000 px needs the whole drawing at 2000x1000, got %v", asked)
	}
	big := VectorSource{Dim: image.Pt(100, 100), Decode: decode}
	if _, err := big.Render(context.Background(), image.Rect(0, 0, 1, 1), 20000, 20000); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a 400 MP raster must be refused: %v", err)
	}
}

func TestRenderRejectsAnOversizedSheet(t *testing.T) {
	pg := Page{Sheet: image.Pt(20000, 20000), Dest: image.Rect(0, 0, 1, 1), Crop: image.Rect(0, 0, 1, 1)}
	if _, err := Render(context.Background(), ImageSource{Img: pattern(2, 2)}, pg); !errors.Is(err, ErrTooLarge) {
		t.Errorf("%v", err)
	}
	if _, err := Render(context.Background(), nil, Page{}); !errors.Is(err, ErrNoPicture) {
		t.Errorf("%v", err)
	}
}

func TestProxyMapsTheCropToTheSmallerSource(t *testing.T) {
	small := pattern(5, 4)
	p := Proxy{Source: ImageSource{Img: small}, Full: image.Pt(50, 40)}
	if p.Size() != image.Pt(50, 40) {
		t.Fatalf("size %v", p.Size())
	}
	full, err := p.Render(context.Background(), image.Rect(0, 0, 50, 40), 5, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !equalImages(full, small) {
		t.Error("the whole picture must come back unchanged")
	}
	half, err := p.Render(context.Background(), image.Rect(0, 0, 25, 40), 10, 8)
	if err != nil {
		t.Fatal(err)
	}
	if half.Bounds().Size() != image.Pt(10, 8) {
		t.Fatalf("size %v", half.Bounds().Size())
	}
	c, want := half.NRGBAAt(0, 0), small.NRGBAAt(0, 0)
	if abs(int(c.R)-int(want.R)) > 40 || abs(int(c.G)-int(want.G)) > 40 {
		t.Errorf("the corner is %v, want about %v", c, want)
	}
	last := half.NRGBAAt(9, 0)
	if last.R <= c.R {
		t.Errorf("the crop must cover about 2.5 source columns, the red channel must rise: %v then %v", c, last)
	}
	if _, err := p.Render(context.Background(), image.Rect(0, 0, 51, 40), 5, 4); !errors.Is(err, ErrNoPicture) {
		t.Errorf("crop outside: %v", err)
	}
	if _, err := (Proxy{}).Render(context.Background(), image.Rect(0, 0, 1, 1), 1, 1); !errors.Is(err, ErrNoPicture) {
		t.Errorf("empty proxy: %v", err)
	}
}
