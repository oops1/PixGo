package gallery

import (
	"image"
	"math"

	model "github.com/oops1/PixGo/internal/gallery"
)

var folderIcons = map[string]bool{
	"desktop": true, "pictures": true, "video": true, "documents": true, "download": true,
}

var navIconNudge = map[string]image.Point{
	"images":       {0, -8},
	"recent":       {0, -8},
	"heart-small":  {0, -9},
	"desktop":      {0, -7},
	"pictures":     {0, -7},
	"video":        {0, -8},
	"documents":    {0, -8},
	"download":     {0, -8},
	"drive-system": {0, -9},
	"drive":        {0, -9},
	"network":      {-1, -9},
}

var defaultNavNudge = image.Point{0, -8}

type navRow struct {
	entry   model.Entry
	index   int
	center  float64
	section bool
}

func navGap(prev, cur model.Entry) float64 {
	switch {
	case cur.Header:
		if folderIcons[prev.Icon] {
			return 31
		}
		return 30
	case prev.Header:
		if folderIcons[cur.Icon] {
			return 24
		}
		return 25
	case folderIcons[prev.Icon] && folderIcons[cur.Icon]:
		return 21.75
	case cur.Disabled:
		return 26
	}
	return 23
}

func buildNavRows(entries []model.Entry) []navRow {
	if len(entries) == 0 {
		return nil
	}
	rows := make([]navRow, len(entries))
	center := navFirstCenter
	section := false
	for i, e := range entries {
		if i > 0 {
			center += navGap(entries[i-1], e)
		}
		if e.Header {
			section = true
		}
		rows[i] = navRow{entry: e, index: i, center: center, section: section}
	}
	return rows
}

func (r navRow) baseline(top int) int { return top + int(math.Floor(r.center+5)) }

func (r navRow) iconPos(top int) image.Point {
	n, ok := navIconNudge[r.entry.Icon]
	if !ok {
		n = defaultNavNudge
	}
	x := navIconTop
	if r.section {
		x = navIconSection
	}
	return image.Pt(x+n.X, top+int(math.Floor(r.center+.5))+n.Y)
}

func (r navRow) textX() int {
	switch {
	case r.entry.Header:
		return navHeaderX
	case r.section:
		return navTextSection
	}
	return navTextTop
}

func (r navRow) rect(top, right int) image.Rectangle {
	y := top + int(math.Floor(r.center-navPillHalf+.5))
	return image.Rect(navLeft, y, right-navRightInset, y+2*navPillHalf)
}

func navContentHeight(rows []navRow) int {
	if len(rows) == 0 {
		return 0
	}
	return int(math.Ceil(rows[len(rows)-1].center)) + 16
}
