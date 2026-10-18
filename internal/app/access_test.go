package app

import (
	"image"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/saveopts"
	uiwindow "github.com/oops1/PixGo/internal/ui/window"
	"github.com/oops1/PixGo/internal/viewer"
)

func viewerWithImage(t *testing.T) *App {
	t.Helper()
	a, _ := goldenApp(t, 770, 504, nil)
	a.Root.ForceEnabled = false
	path := filepath.Join(t.TempDir(), "mountain.jpg")
	writeBigJPEG(t, path)
	if err := a.Ctl.Open(path); err != nil {
		t.Fatal(err)
	}
	waitReady(t, a)
	a.Root.Refresh()
	a.Eng.Flush()
	a.Eng.RenderOnce()
	return a
}

func appTree(a *App, lang string) *widget.AccessNode {
	i18n.SetLang(lang)
	a.Eng.Flush()
	return a.Eng.AccessibilityTree()
}

func TestViewerTreeIsNamedInBothLanguages(t *testing.T) {
	a := viewerWithImage(t)
	for _, lang := range []string{"ru", "en"} {
		tr := appTree(a, lang)
		testutil.RequireNamed(t, tr)
		img := testutil.FindAccess(tr, widget.RoleImage, a.Ctl.Title())
		if img == nil || !strings.HasSuffix(img.Value, "%") {
			t.Fatalf("%s: image node %+v\n%s", lang, img, testutil.DescribeAccess(tr))
		}
		for _, key := range []string{"cmd.gallery", "cmd.delete", "cmd.rotateLeft", "cmd.rotateRight", "cmd.zoomIn", "cmd.zoomOut", "cmd.fullscreen"} {
			if n := testutil.FindAccess(tr, widget.RoleButton, i18n.T(key)); n == nil || testutil.HasState(n, widget.StateDisabled) {
				t.Errorf("%s: toolbar button %q is %+v", lang, i18n.T(key), n)
			}
		}
		if tr.Name != "PixGo" || tr.Role != widget.RoleWindow {
			t.Errorf("%s: root %s %q", lang, tr.Role, tr.Name)
		}
	}
}

func TestMoreButtonsLookEnabledAndExposeTheMenu(t *testing.T) {
	a := viewerWithImage(t)
	tr := appTree(a, "en")
	var more int
	for _, n := range testutil.FlatAccess(tr) {
		if n.Role == widget.RoleButton && n.Name == "More" {
			more++
			if testutil.HasState(n, widget.StateDisabled) {
				t.Errorf("more button disabled: %+v", n)
			}
		}
	}
	if more != 2 {
		t.Errorf("%d more buttons, want the title bar one and the toolbar one", more)
	}
}

func TestEmptyViewerAnnouncesItsHint(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	for _, lang := range []string{"ru", "en"} {
		tr := appTree(a, lang)
		testutil.RequireNamed(t, tr)
		n := testutil.FindAccess(tr, widget.RoleLabel, i18n.T("empty.title"))
		if n == nil || n.Description != i18n.T("empty.hint") {
			t.Errorf("%s: empty-state node %+v", lang, n)
		}
	}
}

func TestInfoPanelIsPartOfTheTree(t *testing.T) {
	a := viewerWithImage(t)
	a.toggleInfo()
	a.Eng.Flush()
	for _, lang := range []string{"ru", "en"} {
		tr := appTree(a, lang)
		testutil.RequireNamed(t, tr)
		group := testutil.FindAccess(tr, widget.RoleGroup, i18n.T("info.title"))
		if group == nil || group.Value != "mountain.jpg" {
			t.Fatalf("%s: info group %+v\n%s", lang, group, testutil.DescribeAccess(tr))
		}
		if testutil.FindAccess(tr, widget.RoleButton, i18n.T("info.close")) == nil {
			t.Errorf("%s: close button of the panel missing", lang)
		}
		toggle := testutil.FindAccess(tr, widget.RoleCheckBox, i18n.T("cmd.info"))
		if toggle == nil || !testutil.HasState(toggle, widget.StateChecked) {
			t.Errorf("%s: info toggle %+v", lang, toggle)
		}
	}
}

func TestFullscreenControlsAreNamed(t *testing.T) {
	a := viewerWithImage(t)
	a.SetFullscreen(true)
	a.Root.Refresh()
	a.Eng.Flush()
	for _, lang := range []string{"ru", "en"} {
		tr := appTree(a, lang)
		testutil.RequireNamed(t, tr)
		if testutil.FindAccess(tr, widget.RoleButton, i18n.T("cmd.fullscreenExit")) == nil {
			t.Errorf("%s: no exit button\n%s", lang, testutil.DescribeAccess(tr))
		}
		if testutil.FindAccess(tr, widget.RoleButton, i18n.T("slideshow.settings")) == nil {
			t.Errorf("%s: no slideshow settings button", lang)
		}
	}
}

func TestGalleryTreeListsTheFolder(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	for _, lang := range []string{"ru", "en"} {
		tr := appTree(r.app, lang)
		testutil.RequireNamed(t, tr)
		for _, name := range []string{"a.jpg", "b.jpg", "sub"} {
			if testutil.FindAccess(tr, widget.RoleRadioButton, name) == nil {
				t.Errorf("%s: item %q missing\n%s", lang, name, testutil.DescribeAccess(tr))
			}
		}
		if testutil.FindAccess(tr, widget.RoleGroup, i18n.T("gallery.title")) == nil {
			t.Errorf("%s: gallery group missing", lang)
		}
		if testutil.FindAccess(tr, widget.RoleButton, i18n.T("window.close")) == nil {
			t.Errorf("%s: caption buttons missing on the gallery page", lang)
		}
	}
}

func TestSettingsTreeIsNamed(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	a.OpenSettings()
	a.Eng.Flush()
	for _, lang := range []string{"ru", "en"} {
		tr := appTree(a, lang)
		testutil.RequireNamed(t, tr)
		if testutil.FindAccess(tr, widget.RoleGroup, i18n.T("settings.title")) == nil {
			t.Errorf("%s: settings group missing", lang)
		}
	}
}

func inside(tr *widget.AccessNode, role widget.AccessRole, name string, area image.Rectangle) *widget.AccessNode {
	for _, n := range testutil.FlatAccess(tr) {
		if n.Role == role && n.Name == name && n.Bounds.Min.X >= area.Min.X && n.Bounds.Max.X <= area.Max.X {
			return n
		}
	}
	return nil
}

func TestOpenMenuExposesItsItems(t *testing.T) {
	a := viewerWithImage(t)
	for _, lang := range []string{"ru", "en"} {
		i18n.SetLang(lang)
		a.menu.Close()
		a.showMenuFromKeyboard()
		if !a.menu.IsOpen() {
			t.Fatal("menu did not open")
		}
		tr := appTree(a, lang)
		testutil.RequireNamed(t, tr)
		menu := testutil.FindAccess(tr, widget.RoleMenu, i18n.T("access.menu"))
		if menu == nil || !testutil.HasState(menu, widget.StatePopup) || testutil.HasState(menu, widget.StateOffscreen) {
			t.Fatalf("%s: open menu node %+v: %s", lang, menu, testutil.DescribeAccess(tr))
		}
		for _, it := range a.menuItems() {
			if it.Separator {
				continue
			}
			n := inside(tr, widget.RoleMenuItem, it.Text, menu.Bounds)
			if n == nil {
				t.Errorf("%s: menu item %q missing in %s", lang, it.Text, testutil.DescribeAccess(tr))
				continue
			}
			if n.Description != it.Shortcut || testutil.HasState(n, widget.StateDisabled) != it.Disabled {
				t.Errorf("%s: menu item %q: %+v", lang, it.Text, n)
			}
		}
	}
	a.menu.Close()
	closed := testutil.FindAccess(appTree(a, "en"), widget.RoleMenu, "Menu")
	if closed == nil || !testutil.HasState(closed, widget.StateOffscreen) || len(closed.Children) != 0 {
		t.Errorf("closed menu node %+v", closed)
	}
}

func TestConfirmationDialogButtonsAreNamed(t *testing.T) {
	a := viewerWithImage(t)
	a.confirm(i18n.T("dialog.delete.title"), i18n.T("dialog.delete.message", "x.jpg"), func() {})
	a.Eng.Flush()
	for _, lang := range []string{"ru", "en"} {
		tr := appTree(a, lang)
		testutil.RequireNamed(t, tr)
		var modal *widget.AccessNode
		for _, c := range tr.Children {
			if testutil.HasState(c, widget.StateModal) {
				modal = c
			}
		}
		if modal == nil {
			t.Fatalf("%s: dialog is not in the tree", lang)
		}
		var buttons int
		for _, n := range testutil.FlatAccess(modal) {
			if n.Role == widget.RoleButton {
				buttons++
			}
		}
		if buttons < 2 {
			t.Errorf("%s: %d dialog buttons\n%s", lang, buttons, testutil.DescribeAccess(modal))
		}
	}
}

func TestSaveOptionsDialogIsNamed(t *testing.T) {
	a := viewerWithImage(t)
	saveopts.Show(a.Eng, saveopts.JPEG, saveopts.Values{Quality: 90}, func(saveopts.Values, bool) {})
	a.Eng.Flush()
	for _, lang := range []string{"ru", "en"} {
		tr := appTree(a, lang)
		testutil.RequireNamed(t, tr)
		if testutil.FindAccess(tr, widget.RoleSlider, i18n.T("save.quality")) == nil {
			t.Errorf("%s: quality slider missing\n%s", lang, testutil.DescribeAccess(tr))
		}
	}
}

func TestToastIsExposedAsALabel(t *testing.T) {
	a := viewerWithImage(t)
	i18n.SetLang("en")
	a.Root.Toast(i18n.T("toast.saved"))
	tr := appTree(a, "en")
	if testutil.FindAccess(tr, widget.RoleLabel, "File saved") == nil {
		t.Errorf("toast missing\n%s", testutil.DescribeAccess(tr))
	}
}

func TestZoomedImageValueTracksTheZoom(t *testing.T) {
	a := viewerWithImage(t)
	a.Canvas.SetMode(viewer.ZoomActual)
	a.Eng.Flush()
	a.Eng.RenderOnce()
	img := testutil.FindAccess(appTree(a, "en"), widget.RoleImage, a.Ctl.Title())
	if img == nil || img.Value != "100%" {
		t.Errorf("image node %+v", img)
	}
}

func TestPreviewStageShowsAnAnnouncedLoadingIndicator(t *testing.T) {
	a := viewerWithImage(t)
	a.Ctl.status = StatusLoading
	a.Ctl.shown.Reduced = true
	a.Root.Refresh()
	a.Eng.Flush()
	if a.State() != uiwindow.StateLoading || !a.Reduced() {
		t.Fatalf("state %d reduced %v", a.State(), a.Reduced())
	}
	for _, lang := range []string{"ru", "en"} {
		tr := appTree(a, lang)
		testutil.RequireNamed(t, tr)
		if testutil.FindAccess(tr, widget.RoleProgressBar, i18n.T("access.loadingFull")) == nil {
			t.Errorf("%s: no loading indicator\n%s", lang, testutil.DescribeAccess(tr))
		}
		if testutil.FindAccess(tr, widget.RoleLabel, i18n.T("viewer.loading")) != nil {
			t.Errorf("%s: the full-screen loading text hides the preview", lang)
		}
	}
	a.Ctl.status = StatusReady
	a.Ctl.shown.Reduced = false
	a.Root.Refresh()
	if testutil.FindAccess(appTree(a, "en"), widget.RoleProgressBar, "Loading full resolution") != nil {
		t.Error("the indicator stays after the full picture arrived")
	}
}

func TestLoadingNextPictureIsNotCalledFullResolution(t *testing.T) {
	a := viewerWithImage(t)
	a.Ctl.status = StatusLoading
	a.Root.Refresh()
	if a.Reduced() {
		t.Fatal("a full picture is reported as reduced")
	}
	tr := appTree(a, "en")
	if testutil.FindAccess(tr, widget.RoleProgressBar, "Loading…") == nil {
		t.Errorf("no loading announcement\n%s", testutil.DescribeAccess(tr))
	}
}
