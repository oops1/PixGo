package window

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/icons"
)

var iconNudge = map[string]image.Point{
	"back":             {1, 1},
	"info":             {1, 1},
	"heart":            {1, 1},
	"heart-filled":     {1, 1},
	"more-title":       {1, 1},
	"caption-min":      {1, 1},
	"caption-max":      {0, 1},
	"caption-restore":  {0, 1},
	"caption-close":    {1, 1},
	"gallery":          {1, 0},
	"zoom-out":         {1, 0},
	"more":             {3, 0},
	"chevron-left":     {-1, 1},
	"chevron-right":    {3, 1},
	"chevron-left-sm":  {-2, 1},
	"chevron-right-sm": {2, 1},
}

func drawIcon(c widget.DrawContext, name string, cx, cy int, col color.RGBA) {
	n := iconNudge[name]
	icons.DrawCentered(c, name, cx+n.X, cy+n.Y, col)
}
