package saveopts

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/testutil"
)

func TestSnapshots(t *testing.T) {
	out := testutil.OutDir()
	if out == "" {
		t.Skip("PIXGO_SNAPSHOTS is not set")
	}
	eachCase(t, func(t *testing.T, kind Kind, k float64, lang string) {
		for _, dark := range []bool{true, false} {
			theme := "dark"
			if !dark {
				theme = "light"
			}
			r := newRigThemed(t, kind, Values{Quality: 72, Lossless: kind == WebP && lang == "en"}, 760, 420, dark)
			r.press('x')
			name := fmt.Sprintf("saveopts_%s_%s_k%02d_%s.png", kindName(kind), lang, int(k*10+.5), theme)
			testutil.SavePNG(filepath.Join(out, name), r.render())
		}
		small := newRigThemed(t, kind, Values{Quality: 72}, 420, 320, true)
		small.press(widget.KeyTab)
		testutil.SavePNG(filepath.Join(out, fmt.Sprintf("saveopts_%s_%s_k%02d_min.png", kindName(kind), lang, int(k*10+.5))), small.render())
	})
}
