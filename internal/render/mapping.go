package render

import (
	"image"
	"image/color"

	"github.com/oops1/PixGo/internal/codec"
)

type Filter uint8

const (
	FilterAuto Filter = iota
	FilterBilinear
	FilterNearest
)

type Mapping struct {
	Scale   float64
	OriginX float64
	OriginY float64
	Orient  codec.Orientation
	Filter  Filter
}

type BackgroundKind uint8

const (
	BgSolid BackgroundKind = iota
	BgChecker
)

type Background struct {
	Canvas  color.RGBA
	Kind    BackgroundKind
	Fill    color.RGBA
	Checker [2]color.RGBA
	Cell    int
}

type Source struct {
	Levels []*image.NRGBA
	Width  int
	Height int
}
