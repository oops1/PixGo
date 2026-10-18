package saveopts

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

func TestAccessChildrenHaveUniqueActionIDs(t *testing.T) {
	for _, kind := range kinds {
		r := newRig(t, kind, Values{Quality: 80}, 760, 420)
		testutil.RequireActionIDs(t, r.d.form.AccessChildren())
	}
}

func accessNode(r *rig, role widget.AccessRole, name string) *widget.AccessNode {
	return testutil.FindAccess(widget.BuildAccessTree(r.d.form, nil), role, name)
}

func TestAccessInvokeOnSaveAndCancelActsLikeAClick(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 70}, 760, 420)
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleButton, i18n.T("save.options.save")), widget.AccessInvoke) {
		t.Fatal("invoke on Save refused")
	}
	if got := r.last(); !got.ok || got.values.Quality != 70 {
		t.Errorf("result %+v", got)
	}
	r = newRig(t, JPEG, Values{Quality: 70}, 760, 420)
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleButton, i18n.T("dialog.cancel")), widget.AccessInvoke) {
		t.Fatal("invoke on Cancel refused")
	}
	if got := r.last(); got.ok {
		t.Errorf("result %+v", got)
	}
}

func TestAccessToggleAndInvokeFlipTheLosslessSwitch(t *testing.T) {
	r := newRig(t, JPEGLossless, Values{Quality: 70}, 760, 420)
	name := i18n.T("save.lossless")
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleCheckBox, name), widget.AccessToggle) || !r.d.Values().Lossless {
		t.Fatalf("toggle did not switch lossless on: %+v", r.d.Values())
	}
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleCheckBox, name), widget.AccessInvoke) || r.d.Values().Lossless {
		t.Fatalf("invoke did not switch lossless off: %+v", r.d.Values())
	}
	if n := accessNode(r, widget.RoleSlider, i18n.T("save.quality")); testutil.AccessDo(r.eng, n, widget.AccessToggle) {
		t.Error("toggle accepted on the slider")
	}
}

func TestAccessFocusMovesTheKeyboardStop(t *testing.T) {
	r := newRig(t, WebP, Values{Quality: 70}, 760, 420)
	for _, c := range []struct {
		role widget.AccessRole
		name string
		want Part
	}{
		{widget.RoleSlider, i18n.T("save.quality"), PartSlider},
		{widget.RoleButton, i18n.T("dialog.cancel"), PartCancel},
		{widget.RoleCheckBox, i18n.T("save.lossless"), PartCheck},
	} {
		if !testutil.AccessDo(r.eng, accessNode(r, c.role, c.name), widget.AccessFocus) {
			t.Fatalf("focus on %q refused", c.name)
		}
		if r.d.Focused() != c.want || !r.d.form.keyNav {
			t.Errorf("focus on %q: stop %v keyNav %v", c.name, r.d.Focused(), r.d.form.keyNav)
		}
	}
}

func TestAccessRefusesDisabledUnknownAndUnsupportedActions(t *testing.T) {
	r := newRig(t, WebP, Values{Quality: 70, Lossless: true}, 760, 420)
	slider := accessNode(r, widget.RoleSlider, i18n.T("save.quality"))
	if testutil.AccessDo(r.eng, slider, widget.AccessFocus) {
		t.Error("focus accepted on the disabled slider")
	}
	if r.eng.AccessAction(r.d.form, "nope", widget.AccessInvoke) || r.eng.AccessAction(r.d.form, "", widget.AccessFocus) {
		t.Error("unknown id accepted")
	}
	save := accessNode(r, widget.RoleButton, i18n.T("save.options.save"))
	for _, k := range []widget.AccessActionKind{widget.AccessToggle, widget.AccessSelect, widget.AccessExpand, widget.AccessCollapse} {
		if testutil.AccessDo(r.eng, save, k) {
			t.Errorf("%s accepted on a button", k)
		}
	}
	if len(r.results) != 0 {
		t.Errorf("refused actions produced results %+v", r.results)
	}
}
