package viewer

type Navigator struct {
	folder *Folder
	index  int
	loop   bool
}

func NewNavigator(f *Folder, index int) *Navigator {
	n := &Navigator{folder: f}
	n.index = n.clamp(index)
	return n
}

func (n *Navigator) Folder() *Folder { return n.folder }

func (n *Navigator) Count() int {
	if n.folder == nil {
		return 0
	}
	return n.folder.Len()
}

func (n *Navigator) Index() int { return n.index }

func (n *Navigator) Loop() bool { return n.loop }

func (n *Navigator) SetLoop(loop bool) { n.loop = loop }

func (n *Navigator) Current() (Entry, bool) {
	if n.index < 0 || n.index >= n.Count() {
		return Entry{}, false
	}
	return n.folder.At(n.index), true
}

func (n *Navigator) Next() bool {
	count := n.Count()
	if count == 0 {
		return false
	}
	i := n.clamp(n.index) + 1
	if i >= count {
		if !n.loop {
			return false
		}
		i = 0
	}
	return n.move(i)
}

func (n *Navigator) Prev() bool {
	count := n.Count()
	if count == 0 {
		return false
	}
	i := n.clamp(n.index) - 1
	if i < 0 {
		if !n.loop {
			return false
		}
		i = count - 1
	}
	return n.move(i)
}

func (n *Navigator) First() bool {
	if n.Count() == 0 {
		return false
	}
	return n.move(0)
}

func (n *Navigator) Last() bool {
	count := n.Count()
	if count == 0 {
		return false
	}
	return n.move(count - 1)
}

func (n *Navigator) Go(i int) bool {
	if i < 0 || i >= n.Count() {
		return false
	}
	return n.move(i)
}

func (n *Navigator) Neighbors(k int) []int {
	count := n.Count()
	if count < 2 || k <= 0 {
		return nil
	}
	cur := n.clamp(n.index)
	out := make([]int, 0, min(2*k, count-1))
	seen := map[int]struct{}{cur: {}}
	for d := 1; d <= k && len(out) < count-1; d++ {
		if !n.loop && cur+d >= count && cur-d < 0 {
			break
		}
		for _, j := range [2]int{cur + d, cur - d} {
			if n.loop {
				j = ((j % count) + count) % count
			} else if j < 0 || j >= count {
				continue
			}
			if _, dup := seen[j]; dup {
				continue
			}
			seen[j] = struct{}{}
			out = append(out, j)
		}
	}
	return out
}

func (n *Navigator) Follow(path string) bool {
	if n.folder != nil {
		if i := n.folder.IndexOf(path); i >= 0 {
			n.index = i
			return true
		}
	}
	n.index = n.clamp(n.index)
	return false
}

func (n *Navigator) move(i int) bool {
	if i == n.index {
		return false
	}
	n.index = i
	return true
}

func (n *Navigator) clamp(i int) int {
	count := n.Count()
	if i >= count {
		i = count - 1
	}
	if i < 0 {
		i = 0
	}
	return i
}
