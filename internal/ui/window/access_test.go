package window

import (
	"image"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

var accessTitles = map[commands.ID]string{
	"app.back": "cmd.back", "view.info": "cmd.info", "file.favorite": "cmd.favorite.add", "app.more": "cmd.more",
	"nav.prev": "cmd.prev", "nav.next": "cmd.next", "view.gallery": "cmd.gallery", "file.delete": "cmd.delete",
	"view.rotateLeft": "cmd.rotateLeft", "view.zoomOut": "cmd.zoomOut", "view.zoomToggle": "zoom.label",
	"view.zoomIn": "cmd.zoomIn", "view.rotateRight": "cmd.rotateRight", "view.fullscreen": "cmd.fullscreen",
	"slideshow.start": "cmd.slideshow", "slideshow.toggle": "cmd.slideshowPause",
}

var accessKeys = map[commands.ID]commands.Chord{
	"view.gallery": commands.C(commands.Ctrl, 'G'),
	"view.info":    commands.C(commands.Ctrl, 'I'),
}

func accessRegistry(h *fakeHost) *commands.Registry {
	reg := commands.NewRegistry()
	for id, title := range accessTitles {
		id := id
		c := commands.Command{ID: id, Title: title, Run: func() { h.ran = append(h.ran, id) }}
		if k, ok := accessKeys[id]; ok {
			c.Keys = []commands.Chord{k}
		}
		reg.Add(c)
	}
	return reg
}

func newAccessRig(t *testing.T, lang string, mutate func(*fakeHost)) *rig {
	t.Helper()
	r := newRig(t, func(h *fakeHost) {
		h.reg = accessRegistry(h)
		if mutate != nil {
			mutate(h)
		}
	})
	i18n.SetLang(lang)
	r.root.OnMore = func(image.Rectangle) {}
	r.root.Refresh()
	return r
}

func accessTree(r *rig) *widget.AccessNode { return widget.BuildAccessTree(r.root, nil) }

func TestViewerAccessTreeHasLocalisedNames(t *testing.T) {
	for _, lang := range []string{"ru", "en"} {
		r := newAccessRig(t, lang, nil)
		tree := accessTree(r)
		testutil.RequireNamed(t, tree)
		for _, key := range []string{"cmd.back", "cmd.info", "access.favorite", "cmd.more", "cmd.gallery", "cmd.delete", "cmd.rotateLeft", "cmd.rotateRight", "cmd.zoomOut", "cmd.zoomIn", "zoom.label", "cmd.fullscreen", "cmd.prev", "cmd.next", "window.minimize", "window.maximize", "window.close"} {
			want := i18n.T(key)
			if want == key {
				t.Fatalf("%s: key %q has no text", lang, key)
			}
			if testutil.FindAccess(tree, widget.RoleButton, want) == nil && testutil.FindAccess(tree, widget.RoleCheckBox, want) == nil {
				t.Errorf("%s: no control named %q for %s\n%s", lang, want, key, testutil.DescribeAccess(tree))
			}
		}
	}
}

func TestViewerAccessNamesDifferByLanguage(t *testing.T) {
	ru := strings.Join(testutil.AccessNames(accessTree(newAccessRig(t, "ru", nil))), "|")
	en := strings.Join(testutil.AccessNames(accessTree(newAccessRig(t, "en", nil))), "|")
	if ru == en || !strings.Contains(ru, "Назад") || !strings.Contains(en, "Back") {
		t.Errorf("names do not follow the language\nru: %s\nen: %s", ru, en)
	}
}

func TestViewerAccessRolesAndStates(t *testing.T) {
	r := newAccessRig(t, "en", func(h *fakeHost) { h.info = true; h.zoom = "98%" })
	tree := accessTree(r)
	info := testutil.FindAccess(tree, widget.RoleCheckBox, "Info")
	if info == nil || !testutil.HasState(info, widget.StateChecked) {
		t.Fatalf("info toggle not checked: %+v", info)
	}
	if info.Description != "Ctrl+I" {
		t.Errorf("info shortcut %q", info.Description)
	}
	fav := testutil.FindAccess(tree, widget.RoleCheckBox, "Favorite")
	if fav == nil || testutil.HasState(fav, widget.StateChecked) {
		t.Errorf("favorite toggle: %+v", fav)
	}
	zoom := testutil.FindAccess(tree, widget.RoleButton, "Zoom")
	if zoom == nil || zoom.Value != "98%" {
		t.Errorf("zoom button: %+v", zoom)
	}
	label := testutil.FindAccess(tree, widget.RoleLabel, "mountain.jpg — 6000 × 4000")
	if label == nil {
		t.Errorf("title label missing\n%s", testutil.DescribeAccess(tree))
	}
	if root := tree; root.Role != widget.RoleWindow || root.Name != "PixGo" {
		t.Errorf("root node %s %q", root.Role, root.Name)
	}
}

func TestDisabledCommandsAreMarkedDisabled(t *testing.T) {
	r := newAccessRig(t, "en", func(h *fakeHost) { h.canPrev = false; h.canNext = false })
	r.root.Actions[ZonePrev] = "app.disabled"
	r.host.reg.Add(commands.Command{ID: "app.disabled", Title: "cmd.prev", Run: func() {}, Enabled: func() bool { return false }})
	tree := accessTree(r)
	prev := testutil.FindAccess(tree, widget.RoleButton, "Previous image")
	if prev == nil || !testutil.HasState(prev, widget.StateDisabled) {
		t.Errorf("previous arrow %+v", prev)
	}
}

func TestMoreButtonsFollowTheMenuCallback(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	for _, n := range testutil.FlatAccess(accessTree(r)) {
		if n.Name == "More" && testutil.HasState(n, widget.StateDisabled) {
			t.Errorf("more button disabled although the menu is wired: %+v", n)
		}
	}
	r.root.OnMore = nil
	for _, n := range testutil.FlatAccess(accessTree(r)) {
		if n.Name == "More" && !testutil.HasState(n, widget.StateDisabled) {
			t.Errorf("more button enabled without a menu: %+v", n)
		}
	}
}

func TestAccessBoundsMatchTheHitZones(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	tree := accessTree(r)
	l := r.root.fullOverlayLayout()
	byBounds := map[image.Rectangle]*widget.AccessNode{}
	for _, n := range testutil.FlatAccess(tree) {
		if testutil.InteractiveRole(n.Role) {
			byBounds[n.Bounds] = n
		}
	}
	for _, k := range append(append(append([]ZoneKind{}, titleZones...), toolZones...), arrowZones...) {
		z := l.Rect(k)
		n := byBounds[z]
		if n == nil {
			t.Errorf("zone %d at %v has no node", k, z)
			continue
		}
		want := r.root.zoneLabel(k)
		if k == ZoneFavorite {
			want = i18n.T("access.favorite")
		}
		if n.Name != want {
			t.Errorf("zone %d: node %q, want %q", k, n.Name, want)
		}
	}
}

func TestEmptyAndFailedStatesAreAnnounced(t *testing.T) {
	r := newAccessRig(t, "en", func(h *fakeHost) { h.state = StateEmpty })
	tree := accessTree(r)
	n := testutil.FindAccess(tree, widget.RoleLabel, i18n.T("empty.title"))
	if n == nil || n.Description != i18n.T("empty.hint") {
		t.Errorf("empty state: %+v", n)
	}
	r = newAccessRig(t, "ru", func(h *fakeHost) { h.state = StateFailed; h.failure = "Файл повреждён" })
	n = testutil.FindAccess(accessTree(r), widget.RoleLabel, i18n.T("error.open"))
	if n == nil || n.Description != "Файл повреждён" {
		t.Errorf("failed state: %+v", n)
	}
	r = newAccessRig(t, "en", func(h *fakeHost) { h.state = StateLoading; h.content.has = false })
	if testutil.FindAccess(accessTree(r), widget.RoleLabel, i18n.T("viewer.loading")) == nil {
		t.Error("loading state missing")
	}
}

func TestToastAndPreviewAreLabels(t *testing.T) {
	r := newAccessRig(t, "en", func(h *fakeHost) { h.preview = true })
	r.root.Toast("File saved")
	tree := accessTree(r)
	if testutil.FindAccess(tree, widget.RoleLabel, "File saved") == nil {
		t.Error("toast missing")
	}
	if testutil.FindAccess(tree, widget.RoleLabel, i18n.T("viewer.preview")) == nil {
		t.Error("preview badge missing")
	}
}

func TestFullscreenSlideshowControlsAreNamed(t *testing.T) {
	for _, lang := range []string{"ru", "en"} {
		r := newAccessRig(t, lang, func(h *fakeHost) {
			h.fullscreen = true
			h.slide = SlideshowView{Active: true, Playing: true, Index: 3, Total: 24, Progress: 0.4}
		})
		tree := accessTree(r)
		testutil.RequireNamed(t, tree)
		for _, key := range []string{"cmd.slideshowPause", "slideshow.settings", "cmd.fullscreenExit"} {
			if testutil.FindAccess(tree, widget.RoleButton, i18n.T(key)) == nil {
				t.Errorf("%s: no button %q\n%s", lang, i18n.T(key), testutil.DescribeAccess(tree))
			}
		}
		if testutil.FindAccess(tree, widget.RoleLabel, i18n.T("slideshow.counter", 3, 24)) == nil {
			t.Errorf("%s: slideshow counter missing", lang)
		}
		bar := testutil.FindAccess(tree, widget.RoleProgressBar, i18n.T("access.slideshow"))
		if bar == nil || bar.Value != "0.40" {
			t.Errorf("%s: progress bar %+v", lang, bar)
		}
	}
}

func TestPausedSlideshowOffersResume(t *testing.T) {
	r := newAccessRig(t, "en", func(h *fakeHost) {
		h.fullscreen = true
		h.slide = SlideshowView{Active: true, Playing: false, Index: 1, Total: 5}
	})
	if testutil.FindAccess(accessTree(r), widget.RoleButton, i18n.T("cmd.slideshowResume")) == nil {
		t.Error("resume button missing")
	}
}

func TestGalleryAndSettingsPagesOnlyExposeCaptionButtons(t *testing.T) {
	for _, p := range []Page{PageGallery, PageSettings} {
		r := newAccessRig(t, "en", func(h *fakeHost) { h.page = p })
		tree := accessTree(r)
		testutil.RequireNamed(t, tree)
		var buttons []string
		for _, c := range tree.Children {
			if c.Role == widget.RoleButton {
				buttons = append(buttons, c.Name)
			}
		}
		want := []string{"Minimize", "Maximize", "Close"}
		if strings.Join(buttons, ",") != strings.Join(want, ",") {
			t.Errorf("page %d buttons %v, want %v", p, buttons, want)
		}
	}
}

func TestMaximizedWindowOffersRestore(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	r.root.SetOps(WindowOps{Maximized: func() bool { return true }, Close: func() {}})
	if testutil.FindAccess(accessTree(r), widget.RoleButton, "Restore") == nil {
		t.Error("restore button missing")
	}
}
