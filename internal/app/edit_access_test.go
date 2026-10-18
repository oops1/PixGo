package app

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

func TestEditTreeIsNamedInBothLanguagesAndHidesTheViewerToolbar(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(10, 20, 410, 320))
	for _, lang := range []string{"ru", "en"} {
		tr := appTree(r.app, lang)
		testutil.RequireNamed(t, tr)
		for _, key := range []string{"edit.aspect", "edit.swap", "cmd.rotateLeft", "cmd.rotateRight", "cmd.flipH", "cmd.flipV", "edit.resize", "edit.reset", "edit.saveCopy", "edit.saveMenu", "edit.cancel"} {
			if testutil.FindAccess(tr, widget.RoleButton, i18n.T(key)) == nil && testutil.FindAccess(tr, widget.RoleComboBox, i18n.T(key)) == nil {
				t.Errorf("%s: no control named %q\n%s", lang, i18n.T(key), testutil.DescribeAccess(tr))
			}
		}
		crop := testutil.FindAccess(tr, widget.RoleGroup, i18n.T("edit.cropArea"))
		if crop == nil || crop.Value != "10, 20, 400 × 300" {
			t.Fatalf("%s: crop node %+v", lang, crop)
		}
		for _, key := range []string{"cmd.gallery", "cmd.delete", "cmd.zoomIn", "cmd.fullscreen", "cmd.edit"} {
			if n := testutil.FindAccess(tr, widget.RoleButton, i18n.T(key)); n != nil {
				t.Errorf("%s: viewer control %q is still in the tree", lang, i18n.T(key))
			}
		}
	}
}

func TestEditNodesActLikeClicks(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(10, 20, 410, 320))
	i18n.SetLang("en")
	tr := appTree(r.app, "en")
	rot := testutil.FindAccess(tr, widget.RoleButton, i18n.T("cmd.rotateRight"))
	if rot == nil || !testutil.AccessDo(r.app.Eng, rot, widget.AccessInvoke) {
		t.Fatal("invoke rotate")
	}
	if r.app.Editor.Crop().Size() != image.Pt(480, 640) {
		t.Fatalf("size %v", r.app.Editor.Crop().Size())
	}
	tr = appTree(r.app, "en")
	combo := testutil.FindAccess(tr, widget.RoleComboBox, i18n.T("edit.aspect"))
	if combo == nil || !testutil.AccessDo(r.app.Eng, combo, widget.AccessExpand) {
		t.Fatal("expand aspect")
	}
	r.settle()
	if !r.app.menu.IsOpen() {
		t.Fatal("the aspect menu did not open")
	}
	r.app.menu.Close()
	tr = appTree(r.app, "en")
	save := testutil.FindAccess(tr, widget.RoleButton, i18n.T("edit.saveCopy"))
	if save == nil || !testutil.AccessDo(r.app.Eng, save, widget.AccessInvoke) {
		t.Fatal("invoke save")
	}
	r.waitFile("photo (2).png")
}

func TestResizeDialogNodesActLikeClicks(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.app.Commands.Run(CmdResize)
	r.settle()
	i18n.SetLang("en")
	tr := appTree(r.app, "en")
	testutil.RequireNamed(t, tr)
	pct := testutil.FindAccess(tr, widget.RoleRadioButton, i18n.T("resize.unit.pct"))
	if pct == nil || !testutil.AccessDo(r.app.Eng, pct, widget.AccessSelect) {
		t.Fatal("select percent")
	}
	if got := r.app.resizeDlg.Model().WidthText(); got != "100" {
		t.Fatalf("width %q", got)
	}
	tr = appTree(r.app, "en")
	apply := testutil.FindAccess(tr, widget.RoleButton, i18n.T("resize.apply"))
	if apply == nil || !testutil.AccessDo(r.app.Eng, apply, widget.AccessInvoke) {
		t.Fatal("invoke apply")
	}
	r.settle()
	if r.app.resizeDlg != nil {
		t.Fatal("dialog stays")
	}
}

func TestOverwriteConfirmationIsNamed(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(0, 0, 100, 100))
	i18n.SetLang("en")
	r.app.saveEditOverwrite()
	r.settle()
	tr := appTree(r.app, "en")
	testutil.RequireNamed(t, tr)
	if testutil.FindAccess(tr, widget.RoleCheckBox, i18n.T("dialog.dontAskAgain")) == nil {
		t.Fatalf("no check box\n%s", testutil.DescribeAccess(tr))
	}
	if testutil.FindAccess(tr, widget.RoleButton, i18n.T("dialog.overwrite.yes")) == nil {
		t.Fatal("no confirm button")
	}
}
