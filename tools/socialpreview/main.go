package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"

	_ "github.com/gen2brain/vpx/webp"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	xdraw "golang.org/x/image/draw"

	"github.com/oops1/PixGo/internal/fonts"
)

//go:embed background.jpg
var backgroundJPEG []byte

const (
	width   = 1280
	height  = 640
	left    = 64
	iconPx  = 80
	shotW   = 640
	shotH   = 400
	shotX   = width - 56 - shotW
	shotY   = (height - shotH) / 2
	corner  = 8
	chipMax = shotX - 40
)

var (
	white  = color.RGBA{0xf4, 0xf5, 0xf8, 0xff}
	muted  = color.RGBA{0xaa, 0xb0, 0xbe, 0xff}
	amber  = color.RGBA{0xff, 0xb0, 0x5c, 0xff}
	teal   = color.RGBA{0x5c, 0xd6, 0xd0, 0xff}
	blue   = color.RGBA{0x7c, 0xb4, 0xff, 0xff}
	violet = color.RGBA{0xc3, 0x9b, 0xff, 0xff}
	green  = color.RGBA{0x7e, 0xe0, 0xa8, 0xff}
)

type chip struct {
	text string
	col  color.RGBA
}

var chips = []chip{
	{"Up to 1000 Mpx", amber},
	{"AVIF · HEIC · JPEG XL", teal},
	{"RAW preview", blue},
	{"EXIF · GPS", violet},
	{"Gallery", green},
	{"Slideshow", amber},
	{"Crop · Resize", teal},
}

type scene struct {
	widget.Base
	bg   *image.RGBA
	shot *image.RGBA
	icon *image.RGBA
}

func (s *scene) Draw(ctx widget.DrawContext) {
	ctx.DrawImage(s.bg, 0, 0)
	shot := image.Rect(shotX, shotY, shotX+shotW, shotY+shotH)
	if sd, ok := ctx.(widget.ShadowDrawer); ok {
		sd.DrawSoftShadow(shot, corner, 28, color.RGBA{0, 0, 0, 0xc0})
	}
	prev := ctx.Clip()
	if rc, ok := ctx.(widget.RoundClipper); ok {
		rc.SetRoundClip(shot, corner)
	}
	ctx.DrawImage(s.shot, shotX, shotY)
	if rc, ok := ctx.(widget.RoundClipper); ok {
		rc.ClearRoundClip()
	}
	ctx.SetClip(prev)
	ctx.DrawRoundBorder(shotX, shotY, shotW, shotH, corner, color.RGBA{0x50, 0x56, 0x66, 0xa0})

	y := 136
	ctx.DrawImage(s.icon, left, y)
	ctx.DrawTextFont("PixGo", left+iconPx+20, y+2, 50, fonts.UIBold, white)
	y += iconPx + 30
	for i := 0; i < 140; i++ {
		ctx.FillRect(left+i, y, 1, 4, lerp(amber, teal, float64(i)/139))
	}
	y += 26
	ctx.DrawTextFont("Fast image viewer for Windows and Linux", left, y, 18, fonts.UISemiBold, white)
	y += 38
	ctx.DrawTextSize("Pure Go, no CGO, no runtime dependencies", left, y, 13, muted)
	y += 52

	const chipH, chipPad, gap, chipPt = 32, 15, 10, 11.5
	x := left
	for _, c := range chips {
		w := ctx.MeasureText(c.text, chipPt) + 2*chipPad
		if x+w > chipMax {
			x = left
			y += chipH + gap
		}
		fill := c.col
		fill.A = 0x22
		ctx.FillRoundRect(x, y, w, chipH, chipH/2, premul(fill))
		border := c.col
		border.A = 0x78
		ctx.DrawRoundBorder(x, y, w, chipH, chipH/2, premul(border))
		ctx.DrawTextSize(c.text, x+chipPad, y+6, chipPt, c.col)
		x += w + gap
	}

	ctx.DrawTextSize("github.com/oops1/PixGo · Apache 2.0", left, height-76, 12, muted)
}

func main() {
	out := flag.String("out", "docs/social-preview.png", "output picture")
	shotPath := flag.String("shot", "docs/screenshots/viewer.webp", "screenshot of the main window")
	iconPath := flag.String("icon", "packaging/icons/hicolor/256x256/apps/pixgo.png", "application icon")
	flag.Parse()
	if err := run(*out, *shotPath, *iconPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(out, shotPath, iconPath string) error {
	bg, err := jpeg.Decode(bytes.NewReader(backgroundJPEG))
	if err != nil {
		return err
	}
	shot, err := load(shotPath)
	if err != nil {
		return err
	}
	icon, err := load(iconPath)
	if err != nil {
		return err
	}
	eng := engine.New(width, height, 30)
	if err := fonts.Register(eng); err != nil {
		return err
	}
	eng.SetTheme(widget.ThemeByName("Win11 Dark"))
	s := &scene{
		bg:   resize(bg, width, height),
		shot: resize(shot, shotW, shotH),
		icon: resize(icon, iconPx, iconPx),
	}
	s.SetBounds(image.Rect(0, 0, width, height))
	eng.SetRoot(s)
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := png.Encode(f, eng.RenderOnce()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func resize(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if src.Bounds().Dx() == w && src.Bounds().Dy() == h {
		draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
		return dst
	}
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	return dst
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return color.RGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 0xff}
}

func premul(c color.RGBA) color.RGBA {
	p := func(v uint8) uint8 { return uint8(uint16(v) * uint16(c.A) / 255) }
	return color.RGBA{p(c.R), p(c.G), p(c.B), c.A}
}
