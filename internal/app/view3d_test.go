package app

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/render"
	"github.com/oops1/PixGo/internal/ui/infopanel"
)

func writeCubeSTL(t *testing.T, path string, side float32) {
	t.Helper()
	c := [8][3]float32{{0, 0, 0}, {side, 0, 0}, {side, side, 0}, {0, side, 0}, {0, 0, side}, {side, 0, side}, {side, side, side}, {0, side, side}}
	faces := [12][3]int{{0, 2, 1}, {0, 3, 2}, {4, 5, 6}, {4, 6, 7}, {0, 1, 5}, {0, 5, 4}, {1, 2, 6}, {1, 6, 5}, {2, 3, 7}, {2, 7, 6}, {3, 0, 4}, {3, 4, 7}}
	b := make([]byte, 84, 84+50*len(faces))
	binary.LittleEndian.PutUint32(b[80:], uint32(len(faces)))
	for _, f := range faces {
		rec := make([]byte, 50)
		for k, i := range f {
			for a := 0; a < 3; a++ {
				binary.LittleEndian.PutUint32(rec[12+12*k+4*a:], math.Float32bits(c[i][a]))
			}
		}
		b = append(b, rec...)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func newModelRig(t *testing.T) *editRig {
	return newEditRig(t, "a.stl", func(t *testing.T, p string) { writeCubeSTL(t, p, 20) }, nil)
}

func (r *editRig) enter3D() {
	r.t.Helper()
	r.app.Commands.Run(CmdView3D)
	eventually(r.t, r.app, "the 3D view gets its model", func() bool { return r.app.View3D.HasScene() })
	r.settle()
}

func TestModelOpensAsAStillPictureWithThe3DButton(t *testing.T) {
	r := newModelRig(t)
	a := r.app
	info := a.Ctl.Info()
	if info.Format != codec.STL || !info.Model || !info.Vector {
		t.Fatalf("info %+v", info)
	}
	if !a.ShowsModel() || a.Viewing3D() {
		t.Fatal("a model is shown as a picture first")
	}
	if got := a.Ctl.Title(); got != "a.stl" {
		t.Fatalf("title %q has no pixel size for a model", got)
	}
	if bg := a.background(a.Root.Theme); bg.Kind != render.BgSolid || bg.Fill != a.Root.Theme.Canvas {
		t.Fatalf("a model is drawn on the plain canvas, got %+v", bg)
	}
	if cmd := a.Commands.Get(CmdView3D); cmd == nil || !cmd.IsEnabled() {
		t.Fatal("the 3D command is available for a model")
	}
}

func TestThe3DViewTurnsZoomsAndLeaves(t *testing.T) {
	r := newModelRig(t)
	a := r.app
	r.enter3D()
	if !a.Viewing3D() || a.Content() != a.View3D {
		t.Fatal("the 3D view is the content")
	}
	if a.ZoomText() != "100%" && a.ZoomText() != "100 %" {
		t.Fatalf("zoom text %q", a.ZoomText())
	}
	yaw := a.View3D.Camera().Yaw
	a.Commands.Run(CmdRotateRight)
	if a.View3D.Camera().Yaw == yaw {
		t.Fatal("rotate turns the model")
	}
	zoom := a.View3D.Camera().Zoom
	a.Commands.Run(CmdZoomIn)
	if a.View3D.Camera().Zoom <= zoom {
		t.Fatal("zoom in brings the model closer")
	}
	path := a.Ctl.Path()
	yaw = a.View3D.Camera().Yaw
	r.key(widget.KeyLeft, 0)
	if a.Ctl.Path() != path {
		t.Fatal("the arrow keys turn the model instead of paging")
	}
	if a.View3D.Camera().Yaw == yaw {
		t.Fatal("the left arrow turns the model")
	}
	for _, id := range []commands.ID{CmdFlipH, CmdPrint, CmdSaveAs, CmdEditStart} {
		if cmd := a.Commands.Get(id); cmd != nil && cmd.IsEnabled() {
			t.Errorf("%s is off in the 3D view", id)
		}
	}
	r.key(widget.KeyEscape, 0)
	if a.Viewing3D() || a.Content() != a.Canvas {
		t.Fatal("Esc leaves the 3D view")
	}
}

func TestThe3DViewFollowsModelsAndLeavesForPictures(t *testing.T) {
	r := newModelRig(t)
	a := r.app
	writeCubeSTL(t, filepath.Join(r.dir, "b.stl"), 40)
	writePNGFile(t, filepath.Join(r.dir, "c.png"), 32, 24)
	a.Ctl.RescanAsync()
	eventually(t, a, "the folder lists three files", func() bool { return !a.Ctl.Scanning() && a.Ctl.Folder().Len() == 3 })
	r.enter3D()
	a.Commands.Run(CmdNext)
	waitName(t, a, "b.stl")
	eventually(t, a, "the next model is loaded into the 3D view", func() bool {
		return a.Viewing3D() && a.View3D.HasScene() && a.m3d.path == filepath.Join(r.dir, "b.stl")
	})
	a.Commands.Run(CmdNext)
	waitName(t, a, "c.png")
	eventually(t, a, "a picture ends the 3D view", func() bool { return !a.Viewing3D() })
	if a.Commands.Get(CmdView3D).IsEnabled() {
		t.Fatal("no 3D view for a picture")
	}
}

func TestEditingAModelIsRefused(t *testing.T) {
	r := newModelRig(t)
	if got := r.app.editRefusal(); got != "edit.refused.model" {
		t.Fatalf("refusal %q", got)
	}
	r.app.Commands.Run(CmdEditStart)
	if r.app.Editing() {
		t.Fatal("a model cannot be edited")
	}
}

func TestInfoPanelShowsTheModelSize(t *testing.T) {
	r := newModelRig(t)
	a := r.app
	a.toggleInfo()
	eventually(t, a, "the model card has its size", func() bool {
		for _, c := range a.info.Model().Cards {
			if c.Kind == infopanel.CardModel && len(c.Rows) >= 2 {
				return true
			}
		}
		return false
	})
	for _, c := range a.info.Model().Cards {
		if c.Kind == infopanel.CardSizes {
			t.Fatal("no pixel size card for a model")
		}
	}
}
