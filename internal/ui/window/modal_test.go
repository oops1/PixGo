package window

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
)

type modalHost struct {
	*fakeHost
	open bool
}

func (m *modalHost) ModalOpen() bool { return m.open }

func TestShortcutsStayWithTheViewerUnlessAModalDialogIsOpen(t *testing.T) {
	tests := []struct {
		name     string
		open     bool
		wantKeep bool
	}{
		{"no dialog", false, true},
		{"dialog open", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, nil)
			var ran int
			r.host.reg.Add(commands.Command{ID: "test.right", Keys: []commands.Chord{commands.C(0, commands.KeyRight)}, Run: func() { ran++ }})
			r.root.host = &modalHost{fakeHost: r.host, open: tc.open}

			handled := r.root.HandleKey(widget.KeyEvent{Code: widget.KeyRight, Pressed: true})

			if handled != tc.wantKeep || (ran == 1) != tc.wantKeep {
				t.Fatalf("handled %v, command ran %d times", handled, ran)
			}
		})
	}
}

func TestEscapeAndTabAreNotTakenWhileAModalDialogIsOpen(t *testing.T) {
	r := newRig(t, nil)
	r.host.reg.Add(commands.Command{ID: "test.esc", Keys: []commands.Chord{commands.C(0, commands.KeyEscape)}, Run: func() {}})
	r.root.host = &modalHost{fakeHost: r.host, open: true}

	for _, code := range []widget.KeyCode{widget.KeyEscape, widget.KeyTab, widget.KeyEnter, widget.KeySpace, widget.KeyHome} {
		if r.root.HandleKey(widget.KeyEvent{Code: code, Pressed: true}) {
			t.Errorf("key %v was swallowed by the viewer", code)
		}
	}
}
