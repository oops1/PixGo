package view

import (
	"sync"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

var descriptions sync.Map

func (c *Canvas) SetDescription(describe func() string) {
	if describe == nil {
		descriptions.Delete(c)
		return
	}
	descriptions.Store(c, describe)
}

func (c *Canvas) description() string {
	if f, ok := descriptions.Load(c); ok {
		return f.(func() string)()
	}
	return ""
}

func (c *Canvas) AccessInfo() widget.AccessInfo {
	info := widget.AccessInfo{Role: widget.RolePanel, Bounds: c.Bounds()}
	if !c.HasImage() {
		return info
	}
	info.Role = widget.RoleImage
	info.Name = c.description()
	if info.Name == "" {
		info.Name = i18n.T("format.dimensions", c.src.Width, c.src.Height)
	}
	info.Value = i18n.T("zoom.percent", c.ZoomPercent())
	if c.Preview() {
		info.Description = i18n.T("viewer.preview")
	}
	if c.src.Shift > 0 && c.src.Detail == nil {
		info.Description = joinSentences(info.Description, i18n.T("viewer.reduced"))
	}
	return info
}

func joinSentences(a, b string) string {
	if a == "" {
		return b
	}
	return a + ". " + b
}
