package app

import (
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/widget"
)

type modalTracker struct {
	open atomic.Int32
}

func (m *modalTracker) ShowModal(widget.ModalWidget) bool {
	m.open.Add(1)
	return false
}

func (m *modalTracker) CloseModal(widget.ModalWidget) bool { return false }

func (m *modalTracker) closed(widget.ModalWidget) {
	if m.open.Add(-1) < 0 {
		m.open.Store(0)
	}
}

func (a *App) trackModals() {
	a.Eng.SetModalHost(&a.modals)
	a.Eng.AddOnModalClosed(a.modals.closed)
}

func (a *App) ModalsOpen() bool { return a.modals.open.Load() > 0 }
