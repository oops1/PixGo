package testutil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

func catalogueKeys() map[string]bool {
	keys := map[string]bool{}
	for _, lang := range i18n.Supported() {
		for _, k := range i18n.Keys(lang) {
			keys[k] = true
		}
	}
	return keys
}

func WalkAccess(n *widget.AccessNode, fn func(*widget.AccessNode)) {
	if n == nil {
		return
	}
	fn(n)
	for _, c := range n.Children {
		WalkAccess(c, fn)
	}
}

func FlatAccess(root *widget.AccessNode) []*widget.AccessNode {
	var out []*widget.AccessNode
	WalkAccess(root, func(n *widget.AccessNode) { out = append(out, n) })
	return out
}

func InteractiveRole(r widget.AccessRole) bool {
	switch r {
	case widget.RoleButton, widget.RoleCheckBox, widget.RoleRadioButton, widget.RoleSwitch,
		widget.RoleSlider, widget.RoleTextInput, widget.RoleComboBox, widget.RoleSpinner:
		return true
	}
	return false
}

func HasState(n *widget.AccessNode, state string) bool {
	for _, s := range n.States {
		if s == state {
			return true
		}
	}
	return false
}

func FindAccess(root *widget.AccessNode, role widget.AccessRole, name string) *widget.AccessNode {
	var found *widget.AccessNode
	WalkAccess(root, func(n *widget.AccessNode) {
		if found == nil && n.Role == role && n.Name == name {
			found = n
		}
	})
	return found
}

func AccessNames(root *widget.AccessNode) []string {
	var out []string
	WalkAccess(root, func(n *widget.AccessNode) {
		if n.Name != "" {
			out = append(out, n.Name)
		}
	})
	return out
}

func DescribeAccess(root *widget.AccessNode) string {
	var b strings.Builder
	var walk func(n *widget.AccessNode, depth int)
	walk = func(n *widget.AccessNode, depth int) {
		fmt.Fprintf(&b, "%s%s %q", strings.Repeat("  ", depth), n.Role, n.Name)
		if n.Value != "" {
			fmt.Fprintf(&b, " value=%q", n.Value)
		}
		if len(n.States) > 0 {
			fmt.Fprintf(&b, " %v", n.States)
		}
		fmt.Fprintf(&b, " %v\n", n.Bounds)
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	walk(root, 0)
	return b.String()
}

func RequireNamed(t testing.TB, root *widget.AccessNode) {
	t.Helper()
	if root == nil {
		t.Fatal("empty access tree")
	}
	keys := catalogueKeys()
	WalkAccess(root, func(n *widget.AccessNode) {
		if InteractiveRole(n.Role) && n.Name == "" {
			t.Errorf("%s node without a name at %v", n.Role, n.Bounds)
		}
		if keys[n.Name] || keys[n.Value] || keys[n.Description] {
			t.Errorf("%s node shows a catalogue key: name %q value %q", n.Role, n.Name, n.Value)
		}
		if n.Bounds.Empty() && InteractiveRole(n.Role) {
			t.Errorf("%s %q has empty bounds", n.Role, n.Name)
		}
	})
}
