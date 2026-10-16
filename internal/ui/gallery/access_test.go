package gallery

import (
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

func accessRig(t *testing.T, lang string, mutate func(*fakeSource)) *rig {
	t.Helper()
	old := testLang
	testLang = lang
	defer func() { testLang = old }()
	src := mockSource()
	if mutate != nil {
		mutate(src)
	}
	return newRig(t, src, mockW, mockH)
}

func tree(r *rig) *widget.AccessNode { return widget.BuildAccessTree(r.page, nil) }

func TestGalleryAccessTreeIsNamedInBothLanguages(t *testing.T) {
	for _, lang := range []string{"ru", "en"} {
		r := accessRig(t, lang, nil)
		tr := tree(r)
		testutil.RequireNamed(t, tr)
		if tr.Role != widget.RoleGroup || tr.Name != i18n.T("gallery.title") {
			t.Errorf("%s: page node %s %q", lang, tr.Role, tr.Name)
		}
		for _, key := range []string{"cmd.historyBack", "cmd.historyUp", "gallery.filter", "access.refresh", "gallery.thumbSize", "cmd.open", "gallery.openMenu"} {
			if testutil.FindAccess(tr, widget.RoleButton, i18n.T(key)) == nil {
				t.Errorf("%s: no button %q\n%s", lang, i18n.T(key), testutil.DescribeAccess(tr))
			}
		}
		for _, key := range []string{"gallery.viewList", "gallery.viewGrid"} {
			if testutil.FindAccess(tr, widget.RoleRadioButton, i18n.T(key)) == nil {
				t.Errorf("%s: no radio button %q", lang, i18n.T(key))
			}
		}
		if testutil.FindAccess(tr, widget.RoleTextInput, i18n.T("gallery.address")) == nil {
			t.Errorf("%s: address field missing", lang)
		}
		if testutil.FindAccess(tr, widget.RoleList, i18n.T("access.places")) == nil || testutil.FindAccess(tr, widget.RoleList, i18n.T("access.items")) == nil {
			t.Errorf("%s: lists missing", lang)
		}
	}
}

func TestGalleryNamesFollowTheLanguage(t *testing.T) {
	ru := strings.Join(testutil.AccessNames(tree(accessRig(t, "ru", nil))), "|")
	en := strings.Join(testutil.AccessNames(tree(accessRig(t, "en", nil))), "|")
	if !strings.Contains(ru, "Назад в истории") || !strings.Contains(en, "Back in history") || ru == en {
		t.Errorf("names do not follow the language\nru: %s\nen: %s", ru, en)
	}
}

func TestGalleryHeaderStates(t *testing.T) {
	r := accessRig(t, "en", nil)
	tr := tree(r)
	if n := testutil.FindAccess(tr, widget.RoleButton, "Forward in history"); n == nil || !testutil.HasState(n, widget.StateDisabled) {
		t.Errorf("forward button %+v", n)
	}
	if n := testutil.FindAccess(tr, widget.RoleButton, "Back in history"); n == nil || testutil.HasState(n, widget.StateDisabled) {
		t.Errorf("back button %+v", n)
	}
	grid := testutil.FindAccess(tr, widget.RoleRadioButton, "Grid")
	list := testutil.FindAccess(tr, widget.RoleRadioButton, "List")
	if grid == nil || !testutil.HasState(grid, widget.StateSelected) || list == nil || testutil.HasState(list, widget.StateSelected) {
		t.Errorf("view toggle: grid %+v list %+v", grid, list)
	}
	if n := testutil.FindAccess(tr, widget.RoleButton, "Thumbnail size"); n == nil || n.Value != "96 px" {
		t.Errorf("thumbnail size %+v", n)
	}
	if n := testutil.FindAccess(tr, widget.RoleButton, "Filter by format"); n == nil || n.Value != "All formats" {
		t.Errorf("filter %+v", n)
	}
	r.src.format = "PNG"
	r.page.Refresh()
	if n := testutil.FindAccess(tree(r), widget.RoleButton, "Filter by format"); n == nil || n.Value != "PNG" {
		t.Errorf("filter after choosing PNG %+v", n)
	}
	r.src.mode = ViewList
	r.page.Refresh()
	if n := testutil.FindAccess(tree(r), widget.RoleRadioButton, "List"); n == nil || !testutil.HasState(n, widget.StateSelected) {
		t.Errorf("list radio %+v", n)
	}
}

func TestGalleryPlacesExposeSelectionHeadersAndDisabledRows(t *testing.T) {
	r := accessRig(t, "en", nil)
	tr := tree(r)
	cur := testutil.FindAccess(tr, widget.RoleRadioButton, "Data (D:)")
	if cur == nil || !testutil.HasState(cur, widget.StateSelected) {
		t.Errorf("current place %+v", cur)
	}
	if n := testutil.FindAccess(tr, widget.RoleRadioButton, "Network"); n == nil || !testutil.HasState(n, widget.StateDisabled) {
		t.Errorf("network place %+v", n)
	}
	if testutil.FindAccess(tr, widget.RoleLabel, "Folders") == nil {
		t.Error("section header missing")
	}
	if testutil.FindAccess(tr, widget.RoleRadioButton, "Folders") != nil {
		t.Error("section header exposed as a selectable place")
	}
}

func TestGalleryItemsExposeNamesSelectionAndKind(t *testing.T) {
	r := accessRig(t, "en", func(s *fakeSource) {
		s.items[1].Dir = true
		s.items[1].Name = "Trips"
	})
	tr := tree(r)
	first := testutil.FindAccess(tr, widget.RoleRadioButton, "mountain.jpg")
	if first == nil || !testutil.HasState(first, widget.StateSelected) || first.Value == "" {
		t.Fatalf("first item %+v", first)
	}
	if n := testutil.FindAccess(tr, widget.RoleRadioButton, "forest.jpg"); n == nil || testutil.HasState(n, widget.StateSelected) {
		t.Errorf("unselected item %+v", n)
	}
	if n := testutil.FindAccess(tr, widget.RoleRadioButton, "Trips"); n == nil || n.Value != "Folder" {
		t.Errorf("folder item %+v", n)
	}
	list := testutil.FindAccess(tr, widget.RoleList, "Images and folders")
	if list == nil || list.Value != "Items: 12" {
		t.Errorf("items list %+v", list)
	}
}

func TestGalleryOnlyVisibleItemsAreExposed(t *testing.T) {
	r := accessRig(t, "en", func(s *fakeSource) { s.items = manyItems(500) })
	n := 0
	for _, node := range testutil.FlatAccess(tree(r)) {
		if node.Role == widget.RoleRadioButton && strings.HasPrefix(node.Name, "img") {
			n++
		}
	}
	if n == 0 || n > 60 {
		t.Errorf("%d item nodes for 500 files", n)
	}
}

func TestGalleryEmptyLoadingAndStatusTexts(t *testing.T) {
	r := accessRig(t, "en", func(s *fakeSource) { s.items = nil; s.selected = -1 })
	tr := tree(r)
	if testutil.FindAccess(tr, widget.RoleLabel, i18n.T("gallery.empty")) == nil {
		t.Errorf("empty text missing\n%s", testutil.DescribeAccess(tr))
	}
	for _, name := range []string{"Open", "Actions for the selection"} {
		if n := testutil.FindAccess(tr, widget.RoleButton, name); n == nil || !testutil.HasState(n, widget.StateDisabled) {
			t.Errorf("%q button %+v", name, n)
		}
	}
	r = accessRig(t, "en", func(s *fakeSource) { s.items = nil; s.loading = true })
	if testutil.FindAccess(tree(r), widget.RoleLabel, i18n.T("gallery.loading")) == nil {
		t.Error("loading text missing")
	}
	r = accessRig(t, "en", func(s *fakeSource) { s.truncated = "Not all files are shown" })
	var status *widget.AccessNode
	for _, n := range testutil.FlatAccess(tree(r)) {
		if n.Role == widget.RoleLabel && strings.Contains(n.Name, "Not all files") {
			status = n
		}
	}
	if status == nil || !strings.Contains(status.Name, "6000") {
		t.Errorf("status label %+v", status)
	}
}

func TestEditingTheAddressSwapsInTheRealTextInput(t *testing.T) {
	r := accessRig(t, "en", nil)
	r.page.FocusAddress()
	tr := tree(r)
	var inputs []*widget.AccessNode
	for _, n := range testutil.FlatAccess(tr) {
		if n.Role == widget.RoleTextInput {
			inputs = append(inputs, n)
		}
	}
	if len(inputs) != 1 || inputs[0].Name != "Address" || inputs[0].Value != `D:\Photos\Nature` || testutil.HasState(inputs[0], widget.StateReadOnly) {
		t.Fatalf("address nodes %+v", inputs)
	}
	if testutil.FindAccess(tr, widget.RoleButton, "Go") == nil {
		t.Error("refresh button is not offered as Go while editing")
	}
}

func TestAddressErrorBubbleIsExposed(t *testing.T) {
	r := accessRig(t, "en", nil)
	r.page.showBubble("Cannot find X")
	if testutil.FindAccess(tree(r), widget.RoleLabel, "Cannot find X") == nil {
		t.Error("error bubble missing")
	}
}

func TestAccessBoundsStayInsideThePage(t *testing.T) {
	r := accessRig(t, "en", nil)
	page := r.page.Bounds()
	for _, n := range testutil.FlatAccess(tree(r)) {
		if !n.Bounds.Empty() && !n.Bounds.In(page.Inset(-3)) {
			t.Errorf("%s %q at %v leaves the page %v", n.Role, n.Name, n.Bounds, page)
		}
	}
}

func TestFocusedStopIsMarkedInTheTree(t *testing.T) {
	r := accessRig(t, "en", nil)
	r.key(widget.KeyTab, 0)
	r.key(widget.KeyTab, 0)
	r.key(widget.KeyTab, 0)
	var focused []string
	for _, n := range testutil.FlatAccess(tree(r)) {
		if testutil.HasState(n, widget.StateFocused) {
			focused = append(focused, n.Name)
		}
	}
	if len(focused) != 1 || focused[0] != "Back in history" {
		t.Errorf("focused nodes %v", focused)
	}
}
