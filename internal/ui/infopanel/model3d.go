package infopanel

import (
	"math"

	"github.com/oops1/PixGo/internal/i18n"
)

type ModelFacts struct {
	Size      [3]float64
	Triangles int
	Parts     int
	Volume    float64
}

func modelLength(v float64) string {
	prec := 1
	if v < 10 {
		prec = 2
	}
	return i18n.FormatFloat(v, prec)
}

func modelCard(f *ModelFacts) Card {
	c := Card{Kind: CardModel, Title: i18n.T("info.mesh")}
	if f == nil {
		return c
	}
	size := i18n.T("info.mesh.sizeValue", modelLength(f.Size[0]), modelLength(f.Size[1]), modelLength(f.Size[2]))
	c.Rows = append(c.Rows, Row{Label: i18n.T("info.mesh.size"), Value: size})
	if f.Volume > 0 && !math.IsInf(f.Volume, 0) {
		c.Rows = append(c.Rows, Row{Label: i18n.T("info.mesh.volume"), Value: i18n.T("info.mesh.volumeValue", i18n.FormatFloat(f.Volume/1000, 2))})
	}
	c.Rows = append(c.Rows, Row{Label: i18n.T("info.mesh.triangles"), Value: i18n.FormatInt(int64(f.Triangles))})
	if f.Parts > 1 {
		c.Rows = append(c.Rows, Row{Label: i18n.T("info.mesh.parts"), Value: i18n.FormatInt(int64(f.Parts))})
	}
	return c
}
