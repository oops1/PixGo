package view

import (
	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/viewer"
)

func (c *Canvas) ImageToScreen() (ox, oy, k float64, ok bool) {
	if !c.HasImage() || c.areaW <= 0 || c.areaH <= 0 || c.scale <= 0 {
		return 0, 0, 0, false
	}
	m := c.view.Mapping(c.Frame())
	b := c.Bounds()
	k = m.Scale / c.scale
	ox = float64(b.Min.X) - m.OriginX*k
	oy = float64(b.Min.Y) - m.OriginY*k
	return ox, oy, k, true
}

func (c *Canvas) Snapshot() viewer.View { return c.view }

func (c *Canvas) Restore(v viewer.View) {
	c.cancelSmooth()
	c.view = v
	c.invalidate()
	c.changed()
}

func (c *Canvas) ForceFit() {
	c.cancelSmooth()
	c.view.Mode = viewer.ZoomFit
	c.view.CenterU, c.view.CenterV = 0, 0
	c.invalidate()
	c.changed()
}

func (c *Canvas) SetUserOrientation(o codec.Orientation) {
	c.cancelSmooth()
	if !o.Valid() {
		o = codec.OrientNormal
	}
	c.view.User = o
	c.view.CenterU, c.view.CenterV = 0, 0
	c.invalidate()
	c.changed()
}

func (c *Canvas) EnsureArea() {
	b := c.Bounds()
	if c.areaW > 0 && c.areaH > 0 || b.Empty() {
		return
	}
	k := c.scale
	if k <= 0 {
		k = 1
	}
	c.areaW, c.areaH = int(float64(b.Dx())*k+.5), int(float64(b.Dy())*k+.5)
}
