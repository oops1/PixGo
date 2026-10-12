package fonts

import (
	"fmt"
	"io/fs"
	"sync"

	enginefonts "github.com/oops1/headless-gui/v3/assets/fonts"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	UI         = "OpenSans"
	UILight    = "OpenSans-Light"
	UISemiBold = "OpenSans-SemiBold"
	UIBold     = "OpenSans-Bold"
)

var files = map[string]string{
	UI:         "OpenSans-Regular.ttf",
	UILight:    "OpenSans-Light.ttf",
	UISemiBold: "OpenSans-SemiBold.ttf",
	UIBold:     "OpenSans-Bold.ttf",
}

type Registrar interface {
	RegisterFontFS(fsys fs.FS, dir string) error
	SetDefaultFont(name string) bool
}

func Register(r Registrar) error {
	if err := enginefonts.Register(r); err != nil {
		return err
	}
	if !r.SetDefaultFont(UI) {
		return fmt.Errorf("fonts: default font %q not accepted", UI)
	}
	return nil
}

func Data(name string) ([]byte, error) {
	file, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("fonts: unknown face %q", name)
	}
	return fs.ReadFile(enginefonts.FS, file)
}

func License() string {
	b, _ := fs.ReadFile(enginefonts.FS, "OpenSans-OFL.txt")
	return string(b)
}

type Metrics struct {
	Ascent  int
	Descent int
	Advance float64
}

type metricKey struct {
	name string
	pt   float64
	dpi  float64
}

var (
	parsedMu sync.Mutex
	parsed   = map[string]*opentype.Font{}
	metrics  = map[metricKey]Metrics{}
)

func MetricsOf(name string, pt, dpi float64) Metrics {
	if dpi == 0 {
		dpi = 96
	}
	parsedMu.Lock()
	defer parsedMu.Unlock()
	key := metricKey{name, pt, dpi}
	if m, ok := metrics[key]; ok {
		return m
	}
	f := parsed[name]
	if f == nil {
		data, err := Data(name)
		if err != nil {
			return Metrics{}
		}
		if f, err = opentype.Parse(data); err != nil {
			return Metrics{}
		}
		parsed[name] = f
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: pt, DPI: dpi, Hinting: font.HintingFull})
	if err != nil {
		return Metrics{}
	}
	defer face.Close()
	met := face.Metrics()
	adv, _ := face.GlyphAdvance('M')
	m := Metrics{Ascent: met.Ascent.Round(), Descent: met.Descent.Round(), Advance: fixedFloat(adv)}
	metrics[key] = m
	return m
}

func fixedFloat(v fixed.Int26_6) float64 { return float64(v) / 64 }
