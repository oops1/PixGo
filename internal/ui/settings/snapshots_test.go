package settings

import (
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/testutil"
)

var sectionNames = [sectionCount]string{"general", "view", "formats", "slideshow", "performance", "language", "about"}

func TestSnapshots(t *testing.T) {
	out := testutil.OutDir()
	if out == "" {
		t.Skip("PIXGO_SNAPSHOTS is not set")
	}
	save := func(name string, r *rig) {
		testutil.SavePNG(filepath.Join(out, "settings_"+name+".png"), r.render())
	}
	for s := Section(0); s < sectionCount; s++ {
		r := newRig(t, mockW, mockH)
		r.page.SetSection(s)
		save(sectionNames[s], r)
		r.page.setScroll(r.page.maxScroll())
		save(sectionNames[s]+"_end", r)
	}
	for name, mk := range layoutThemes {
		r := newRig(t, mockW, mockH)
		r.th = mk()
		save("theme_"+name, r)
		r.page.SetSection(SectionView)
		save("theme_"+name+"_view", r)
	}
	save("narrow", newRig(t, 420, 320))
	save("wide", newRig(t, 900, 600))
	r := newRig(t, mockW, 600)
	r.tab(3)
	x, y := r.center("rememberWindow")
	r.page.OnMouseMove(x, y)
	save("states_general", r)
	r = newRig(t, mockW, 600)
	r.page.SetSection(SectionView)
	r.focusOn("wheel:navigate")
	x, y = r.center("preload:2")
	r.page.OnMouseMove(x, y)
	save("states_view", r)
	r = newRig(t, mockW, 600)
	r.page.SetSection(SectionPerformance)
	r.focusOn("cacheMB")
	x, y = r.center("animationFPS")
	r.page.OnMouseMove(x, y)
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseWheelDown, Pressed: true})
	save("states_performance", r)
}
