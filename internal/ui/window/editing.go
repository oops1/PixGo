package window

import "github.com/oops1/headless-gui/v3/widget"

type editHost interface{ Editing() bool }

func (r *Root) editing() bool {
	h, ok := r.host.(editHost)
	return ok && h.Editing()
}

func (r *Root) handleEditKey(e widget.KeyEvent) bool {
	h, ok := r.host.Content().(KeyHandler)
	return ok && h.HandleKey(e)
}

type modelHost interface {
	ShowsModel() bool
	Viewing3D() bool
}

func (r *Root) model() (shows, active bool) {
	h, ok := r.host.(modelHost)
	if !ok {
		return false, false
	}
	return h.ShowsModel(), h.Viewing3D()
}

func (r *Root) viewing3D() bool {
	_, active := r.model()
	return active
}
