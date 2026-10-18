package testutil

import (
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

func RequireActionIDs(t testing.TB, infos []widget.AccessInfo) {
	t.Helper()
	seen := map[string]string{}
	for _, in := range infos {
		if in.ID == "" {
			if InteractiveRole(in.Role) {
				t.Errorf("%s %q has no action id", in.Role, in.Name)
			}
			continue
		}
		if prev, dup := seen[in.ID]; dup {
			t.Errorf("action id %q is used by %q and %q", in.ID, prev, in.Name)
		}
		seen[in.ID] = in.Name
	}
}

func AccessDo(eng *engine.Engine, n *widget.AccessNode, kind widget.AccessActionKind) bool {
	if n == nil || n.Widget == nil {
		return false
	}
	return eng.AccessAction(n.Widget, n.ActionID, kind)
}
