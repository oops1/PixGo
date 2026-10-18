package print

import (
	"math"
	"strings"
)

type Orientation uint8

const (
	OrientAuto Orientation = iota
	OrientPortrait
	OrientLandscape
)

var Orientations = []Orientation{OrientAuto, OrientPortrait, OrientLandscape}

func (o Orientation) ID() string {
	switch o {
	case OrientPortrait:
		return "portrait"
	case OrientLandscape:
		return "landscape"
	}
	return "auto"
}

func ParseOrientation(id string) Orientation {
	for _, o := range Orientations {
		if o.ID() == strings.ToLower(strings.TrimSpace(id)) {
			return o
		}
	}
	return OrientAuto
}

type Placement uint8

const (
	PlaceFit Placement = iota
	PlaceFill
	PlaceActual
	PlaceCustom
)

var Placements = []Placement{PlaceFit, PlaceFill, PlaceActual, PlaceCustom}

func (p Placement) ID() string {
	switch p {
	case PlaceFill:
		return "fill"
	case PlaceActual:
		return "actual"
	case PlaceCustom:
		return "custom"
	}
	return "fit"
}

func ParsePlacement(id string) Placement {
	for _, p := range Placements {
		if p.ID() == strings.ToLower(strings.TrimSpace(id)) {
			return p
		}
	}
	return PlaceFit
}

type Margin uint8

const (
	MarginNone Margin = iota
	MarginNarrow
	MarginNormal
)

var Margins = []Margin{MarginNone, MarginNarrow, MarginNormal}

func (m Margin) ID() string {
	switch m {
	case MarginNarrow:
		return "narrow"
	case MarginNormal:
		return "normal"
	}
	return "none"
}

func (m Margin) MM() float64 {
	switch m {
	case MarginNarrow:
		return 6
	case MarginNormal:
		return 12
	}
	return 0
}

func ParseMargin(id string) Margin {
	for _, m := range Margins {
		if m.ID() == strings.ToLower(strings.TrimSpace(id)) {
			return m
		}
	}
	return MarginNone
}

type Unit uint8

const (
	UnitCM Unit = iota
	UnitInch
)

func (u Unit) ID() string {
	if u == UnitInch {
		return "in"
	}
	return "cm"
}

func (u Unit) MM() float64 {
	if u == UnitInch {
		return mmPerInch
	}
	return 10
}

func ParseUnit(id string) Unit {
	if strings.EqualFold(strings.TrimSpace(id), "in") {
		return UnitInch
	}
	return UnitCM
}

const mmPerInch = 25.4

type Paper struct {
	ID      string
	Name    string
	Photo   bool
	ShortMM float64
	LongMM  float64
}

var (
	A4     = Paper{ID: "a4", Name: "A4", ShortMM: 210, LongMM: 297}
	A5     = Paper{ID: "a5", Name: "A5", ShortMM: 148, LongMM: 210}
	A3     = Paper{ID: "a3", Name: "A3", ShortMM: 297, LongMM: 420}
	Letter = Paper{ID: "letter", Name: "Letter", ShortMM: 215.9, LongMM: 279.4}
	Legal  = Paper{ID: "legal", Name: "Legal", ShortMM: 215.9, LongMM: 355.6}
	Photo1 = Paper{ID: "10x15", Name: "10 × 15", Photo: true, ShortMM: 100, LongMM: 150}
	Photo2 = Paper{ID: "13x18", Name: "13 × 18", Photo: true, ShortMM: 130, LongMM: 180}
)

var Papers = []Paper{A4, A5, A3, Letter, Legal, Photo1, Photo2}

func PaperByID(id string) (Paper, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, p := range Papers {
		if p.ID == id {
			return p, true
		}
	}
	return A4, false
}

func (p Paper) Size(landscape bool) (w, h float64) {
	if landscape {
		return p.LongMM, p.ShortMM
	}
	return p.ShortMM, p.LongMM
}

const (
	DefaultDPI = 300
	MinDPI     = 72
	MaxDPI     = 1200
	UnknownDPI = 300
	VectorDPI  = 96

	MaxSheetPixels = 100 << 20
	MinCustomMM    = 1.0
	MaxCustomMM    = 2000.0
)

type Options struct {
	Paper       string
	Orientation Orientation
	Placement   Placement
	Margin      Margin
	Unit        Unit
	CustomW     float64
	CustomH     float64
	Keep        bool
}

func DefaultOptions() Options {
	return Options{Paper: A4.ID, Orientation: OrientAuto, Placement: PlaceFit, Margin: MarginNarrow, Unit: UnitCM, Keep: true}
}

func (o Options) Normalize() Options {
	p, _ := PaperByID(o.Paper)
	o.Paper = p.ID
	if o.Orientation > OrientLandscape {
		o.Orientation = OrientAuto
	}
	if o.Placement > PlaceCustom {
		o.Placement = PlaceFit
	}
	if o.Margin > MarginNormal {
		o.Margin = MarginNone
	}
	if o.Unit > UnitInch {
		o.Unit = UnitCM
	}
	o.CustomW = cleanMM(o.CustomW)
	o.CustomH = cleanMM(o.CustomH)
	return o
}

func cleanMM(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return math.Min(v, MaxCustomMM)
}

func (o Options) PaperValue() Paper {
	p, _ := PaperByID(o.Paper)
	return p
}

func (o Options) CustomValid() bool {
	return o.CustomW >= MinCustomMM && o.CustomW <= MaxCustomMM && o.CustomH >= MinCustomMM && o.CustomH <= MaxCustomMM
}
