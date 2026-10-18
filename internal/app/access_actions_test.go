package app

import (
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
	uiwindow "github.com/oops1/PixGo/internal/ui/window"
)

func actionNode(a *App, role widget.AccessRole, name string) *widget.AccessNode {
	i18n.SetLang("en")
	a.Eng.Flush()
	return testutil.FindAccess(a.Eng.AccessibilityTree(), role, name)
}

func actionNodeByID(a *App, id string) *widget.AccessNode {
	a.Eng.Flush()
	var found *widget.AccessNode
	testutil.WalkAccess(a.Eng.AccessibilityTree(), func(n *widget.AccessNode) {
		if found == nil && n.ActionID == id && n.Widget != nil {
			found = n
		}
	})
	return found
}

func TestScreenReaderPressesTheViewerToolbarThroughTheRealEngine(t *testing.T) {
	a := viewerWithImage(t)
	i18n.SetLang("en")
	if a.InfoOpen() {
		t.Fatal("the panel is open at the start")
	}
	info := actionNode(a, widget.RoleCheckBox, "Info")
	if info == nil || info.ActionID == "" {
		t.Fatalf("info toggle %+v", info)
	}
	if !testutil.AccessDo(a.Eng, info, widget.AccessToggle) || !a.InfoOpen() {
		t.Fatal("toggle on the info check box did not open the panel")
	}
	if n := actionNode(a, widget.RoleCheckBox, "Info"); n == nil || !testutil.HasState(n, widget.StateChecked) {
		t.Errorf("info check box after the toggle: %+v", n)
	}
	if !testutil.AccessDo(a.Eng, actionNodeByID(a, "tool.fullscreen"), widget.AccessInvoke) || !a.Fullscreen() {
		t.Fatal("invoke on the full screen button did not switch to full screen")
	}
	if !testutil.AccessDo(a.Eng, actionNodeByID(a, "exit"), widget.AccessInvoke) || a.Fullscreen() {
		t.Fatal("invoke on the exit button did not leave full screen")
	}
}

func TestScreenReaderFocusOnTheInfoPanelMovesTheWindowFocusIntoIt(t *testing.T) {
	a := viewerWithImage(t)
	i18n.SetLang("en")
	a.toggleInfo()
	a.Eng.Flush()
	copyBtn := actionNode(a, widget.RoleButton, i18n.T("info.copyPath"))
	if copyBtn == nil || copyBtn.ActionID == "" {
		t.Fatalf("copy button %+v", copyBtn)
	}
	if !testutil.AccessDo(a.Eng, copyBtn, widget.AccessFocus) {
		t.Fatal("focus refused on the copy button")
	}
	if k, kbd := a.Root.KeyboardFocus(); k != uiwindow.ZonePanel || !kbd || !a.info.FocusActive() {
		t.Errorf("window focus %d kbd %v, panel active %v", k, kbd, a.info.FocusActive())
	}
	if n := actionNode(a, widget.RoleButton, i18n.T("info.copyPath")); n == nil || !testutil.HasState(n, widget.StateFocused) {
		t.Errorf("copy button after the focus: %+v", n)
	}
	if !testutil.AccessDo(a.Eng, actionNode(a, widget.RoleButton, i18n.T("info.close")), widget.AccessInvoke) || a.InfoOpen() {
		t.Error("invoke on the close button did not close the panel")
	}
}

func TestScreenReaderRunsAndHighlightsMenuItems(t *testing.T) {
	a := viewerWithImage(t)
	i18n.SetLang("en")
	a.showMenuFromKeyboard()
	if !a.menu.IsOpen() {
		t.Fatal("menu did not open")
	}
	a.Eng.Flush()
	tr := a.Eng.AccessibilityTree()
	infoItem := testutil.FindAccess(tr, widget.RoleMenuItem, i18n.T("cmd.info"))
	other := testutil.FindAccess(tr, widget.RoleMenuItem, i18n.T("cmd.fullscreen"))
	if infoItem == nil || other == nil || infoItem.ActionID == "" || infoItem.ActionID == other.ActionID {
		t.Fatalf("menu items %+v %+v\n%s", infoItem, other, testutil.DescribeAccess(tr))
	}
	if !testutil.AccessDo(a.Eng, other, widget.AccessFocus) || !a.menu.IsOpen() {
		t.Fatal("focus on a menu item failed or closed the menu")
	}
	if n := testutil.FindAccess(a.Eng.AccessibilityTree(), widget.RoleMenuItem, i18n.T("cmd.fullscreen")); n == nil || !testutil.HasState(n, widget.StateFocused) {
		t.Errorf("highlighted item: %+v", n)
	}
	if !testutil.AccessDo(a.Eng, infoItem, widget.AccessInvoke) {
		t.Fatal("invoke on a menu item refused")
	}
	if a.menu.IsOpen() || !a.InfoOpen() {
		t.Errorf("menu open %v, panel open %v", a.menu.IsOpen(), a.InfoOpen())
	}
}

func TestScreenReaderSelectsAndOpensGalleryItems(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	i18n.SetLang("en")
	names := r.names()
	b := -1
	for i, n := range names {
		if n == "b.jpg" {
			b = i
		}
	}
	if b < 0 {
		t.Fatalf("items %v", names)
	}
	id := "item." + string(rune('0'+b))
	node := actionNodeByID(r.app, id)
	if node == nil || node.Name != "b.jpg" {
		t.Fatalf("node %q: %+v", id, node)
	}
	if !testutil.AccessDo(r.app.Eng, node, widget.AccessSelect) || r.app.gal.Selected() != b {
		t.Fatalf("select did not choose the item: %d", r.app.gal.Selected())
	}
	if !r.app.GalleryOpen() {
		t.Fatal("select left the gallery")
	}
	if !testutil.AccessDo(r.app.Eng, actionNodeByID(r.app, id), widget.AccessInvoke) {
		t.Fatal("invoke refused")
	}
	if r.app.GalleryOpen() {
		t.Fatal("invoke did not open the picture")
	}
	waitName(t, r.app, "b.jpg")
}

func TestScreenReaderNavigatesTheGalleryFolders(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	up := actionNodeByID(r.app, "up")
	if up == nil || !testutil.AccessDo(r.app.Eng, up, widget.AccessInvoke) {
		t.Fatalf("up button %+v", up)
	}
	r.settle(t)
	if got := r.app.gal.Location().Display(); strings.EqualFold(got, r.root) {
		t.Errorf("still in %s after Up", got)
	}
	if !r.app.GalleryOpen() {
		t.Error("the gallery closed")
	}
}

func TestScreenReaderTogglesSettingsAndGoesBack(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	a.OpenSettings()
	a.Eng.Flush()
	i18n.SetLang("en")
	a.Eng.Flush()
	var check *widget.AccessNode
	for _, n := range testutil.FlatAccess(a.Eng.AccessibilityTree()) {
		if n.Role == widget.RoleCheckBox && strings.HasPrefix(n.ActionID, "item.") && !testutil.HasState(n, widget.StateDisabled) {
			check = n
			break
		}
	}
	if check == nil {
		t.Fatalf("no check box in the settings\n%s", testutil.DescribeAccess(a.Eng.AccessibilityTree()))
	}
	was := testutil.HasState(check, widget.StateChecked)
	if !testutil.AccessDo(a.Eng, check, widget.AccessToggle) {
		t.Fatalf("toggle refused on %q", check.Name)
	}
	a.Eng.Flush()
	now := actionNodeByID(a, check.ActionID)
	if now == nil || testutil.HasState(now, widget.StateChecked) == was {
		t.Errorf("check box %q did not change: was %v, now %+v", check.Name, was, now)
	}
	if !testutil.AccessDo(a.Eng, actionNodeByID(a, "back"), widget.AccessInvoke) || a.Page() == uiwindow.PageSettings {
		t.Error("back did not leave the settings")
	}
}
