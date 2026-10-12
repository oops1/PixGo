package gallery

const HistoryLimit = 100

type History struct {
	items []Location
	pos   int
}

func NewHistory(start Location) *History {
	return &History{items: []Location{start}}
}

func (h *History) Current() Location {
	if len(h.items) == 0 {
		return Location{}
	}
	return h.items[h.pos]
}

func (h *History) Push(loc Location) bool {
	if len(h.items) == 0 {
		h.items = []Location{loc}
		h.pos = 0
		return true
	}
	if h.items[h.pos].Equal(loc) {
		return false
	}
	h.items = append(h.items[:h.pos+1], loc)
	if extra := len(h.items) - HistoryLimit; extra > 0 {
		copy(h.items, h.items[extra:])
		h.items = h.items[:HistoryLimit]
	}
	h.pos = len(h.items) - 1
	return true
}

func (h *History) CanBack() bool { return h.pos > 0 }

func (h *History) CanForward() bool { return h.pos+1 < len(h.items) }

func (h *History) Back() (Location, bool) {
	if !h.CanBack() {
		return Location{}, false
	}
	h.pos--
	return h.items[h.pos], true
}

func (h *History) Forward() (Location, bool) {
	if !h.CanForward() {
		return Location{}, false
	}
	h.pos++
	return h.items[h.pos], true
}

func (h *History) CanUp() bool {
	_, ok := h.Current().Parent()
	return ok
}

func (h *History) Up() (Location, bool) {
	parent, ok := h.Current().Parent()
	if !ok {
		return Location{}, false
	}
	h.Push(parent)
	return parent, true
}
