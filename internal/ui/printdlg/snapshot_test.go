package printdlg

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/print"
	"github.com/oops1/PixGo/internal/testutil"
)

func TestSnapshots(t *testing.T) {
	out := testutil.OutDir()
	if out == "" {
		t.Skip("PIXGO_SNAPSHOTS is not set")
	}
	eachCase(t, func(t *testing.T, k float64, lang string) {
		for _, dark := range []bool{true, false} {
			theme := "dark"
			if !dark {
				theme = "light"
			}
			v := testValues()
			v.Options.Placement = print.PlaceCustom
			r := newRigThemed(t, testEnv(), v, 1000, 640, dark)
			r.press(widget.KeyTab)
			name := fmt.Sprintf("print_%s_k%02d_%s.png", lang, int(k*10+.5), theme)
			testutil.SavePNG(filepath.Join(out, name), r.render())
		}
		open := newRigThemed(t, testEnv(), testValues(), 1000, 640, true)
		open.focus(PartPaper)
		open.press(widget.KeySpace)
		testutil.SavePNG(filepath.Join(out, fmt.Sprintf("print_%s_k%02d_list.png", lang, int(k*10+.5))), open.render())
		small := newRigThemed(t, testEnv(), testValues(), 420, 320, true)
		testutil.SavePNG(filepath.Join(out, fmt.Sprintf("print_%s_k%02d_min.png", lang, int(k*10+.5))), small.render())
		export := testEnv()
		export.Export = true
		ex := newRigThemed(t, export, testValues(), 1000, 640, false)
		testutil.SavePNG(filepath.Join(out, fmt.Sprintf("print_%s_k%02d_export.png", lang, int(k*10+.5))), ex.render())
	})
}
