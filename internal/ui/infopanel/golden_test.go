package infopanel

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/render"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/theme"
)

type frameHost struct {
	widget.Base
	child widget.Widget
}

func (f *frameHost) Children() []widget.Widget { return []widget.Widget{f.child} }

func (f *frameHost) Draw(c widget.DrawContext) {
	c.FillRect(0, 0, f.Bounds().Dx(), f.Bounds().Dy(), color.RGBA{16, 20, 24, 255})
	f.child.Draw(c)
}

func mockupModel() Model {
	path := "D:" + string(rune(92)) + "Photos" + string(rune(92)) + "Nature" + string(rune(92)) + "mountain.jpg"
	return Model{
		Path: path, Name: "mountain.jpg", Format: "JPEG (JFIF)", FileSize: "8,73 МБ",
		Cards: []Card{
			{Kind: CardSizes, Title: "Размеры", Rows: []Row{{Label: "Размеры", Value: "6000 × 4000"}, {Label: "Соотношение", Value: "3:2"}}},
			{Kind: CardFile, Title: "Файл", Rows: []Row{
				{Label: "Расположение", Value: path, Link: true, Copy: path},
				{Label: "Дата создания", Value: "14.09.2024 18:27"},
				{Label: "Дата изменения", Value: "14.09.2024 18:27"},
			}},
			{Kind: CardCamera, Title: "Камера (EXIF)", Rows: []Row{
				{Label: "Производитель", Value: "Canon"}, {Label: "Модель", Value: "EOS R5"},
				{Label: "Объектив", Value: "RF 24-70mm F2.8 L"}, {Label: "Выдержка", Value: "1/250 с"},
				{Label: "Диафрагма", Value: "f/4.0"}, {Label: "ISO", Value: "100"}, {Label: "Фокусное расстояние", Value: "24 мм"},
			}},
		},
	}
}

func photo(w, h int) render.Source {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(60 + x*100/w), uint8(110 + y*80/h), uint8(170 - x*60/w), 255})
		}
	}
	return render.Source{Levels: []*image.NRGBA{img}, Width: w, Height: h}
}

func renderPanel(t *testing.T, m Model) (*image.RGBA, *Panel) {
	t.Helper()
	i18n.SetLang("ru")
	eng := testutil.NewEngine(716, 506)
	t.Cleanup(eng.Stop)
	p := New(eng, theme.Dark())
	host := &frameHost{child: p}
	eng.SetRootFullCanvas(host)
	p.SetBounds(image.Rect(389, 49, 715, 503))
	p.SetModel(m)
	p.SetThumbnail(photo(600, 400), codec.OrientNormal)
	eng.Flush()
	return eng.RenderOnce(), p
}

func inkBox(img *image.RGBA, r image.Rectangle, threshold int) image.Rectangle {
	out := image.Rectangle{Min: image.Pt(1<<30, 1<<30), Max: image.Pt(-1, -1)}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if (int(c.R)*299+int(c.G)*587+int(c.B)*114)/1000 > threshold {
				out.Min.X, out.Min.Y = min(out.Min.X, x), min(out.Min.Y, y)
				out.Max.X, out.Max.Y = max(out.Max.X, x+1), max(out.Max.Y, y+1)
			}
		}
	}
	return out
}

func TestPanelMatchesMockup(t *testing.T) {
	ref, err := testutil.LoadRef("02_exif_panel.png")
	if err != nil {
		t.Skip(err)
	}
	got, _ := renderPanel(t, mockupModel())
	region := image.Rect(389, 49, 715, 503)
	e := testutil.MeanError(got, ref, region)
	if out := testutil.OutDir(); out != "" {
		testutil.SavePNG(filepath.Join(out, "panel.png"), got)
		testutil.SaveComparison(filepath.Join(out, "panel_compare.png"), got.SubImage(region).(*image.RGBA), ref.SubImage(region).(*image.RGBA))
	}
	t.Logf("panel mean error vs mockup: %.2f", e)
	if e > 12 {
		t.Errorf("panel mean error %.2f exceeds 12", e)
	}

	probes := []struct {
		name string
		r    image.Rectangle
	}{
		{"header", image.Rect(395, 55, 520, 90)},
		{"close", image.Rect(670, 55, 706, 90)},
		{"name", image.Rect(495, 90, 650, 115)},
		{"format", image.Rect(495, 115, 650, 135)},
		{"size", image.Rect(495, 135, 650, 155)},
		{"dims-title", image.Rect(405, 168, 520, 190)},
		{"file-title", image.Rect(405, 245, 480, 265)},
		{"location", image.Rect(405, 266, 500, 286)},
		{"path", image.Rect(500, 266, 670, 286)},
		{"copy", image.Rect(672, 262, 700, 290)},
		{"created", image.Rect(500, 288, 650, 308)},
		{"camera-title", image.Rect(405, 340, 520, 365)},
		{"make-label", image.Rect(405, 364, 500, 384)},
		{"make-value", image.Rect(520, 364, 600, 384)},
		{"lens-value", image.Rect(520, 402, 700, 422)},
	}
	for _, pr := range probes {
		want, have := inkBox(ref, pr.r, 150), inkBox(got, pr.r, 150)
		d := [4]int{have.Min.X - want.Min.X, have.Min.Y - want.Min.Y, have.Max.X - want.Max.X, have.Max.Y - want.Max.Y}
		t.Logf("%-13s mockup %v  ours %v  d=%v", pr.name, want, have, d)
		for _, v := range d {
			if v < -2 || v > 2 {
				t.Errorf("%s: ink box %v is off the mockup %v by more than 2 px", pr.name, have, want)
				break
			}
		}
	}
}
