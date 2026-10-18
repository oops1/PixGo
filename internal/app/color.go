package app

import (
	"github.com/oops1/PixGo/internal/colorm"
	"github.com/oops1/PixGo/internal/platform"
)

func displayProvider(p *platform.Platform) colorm.DisplayProvider {
	if p == nil || p.Display == nil {
		return nil
	}
	return p.Display.Profile
}
