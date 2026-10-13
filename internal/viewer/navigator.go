package viewer

import (
	"math/rand/v2"
	"time"
)

type Navigator struct {
	folder *Folder
	index  int
	loop   bool
	order  []int
	rank   []int
	rnd    *rand.Rand
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

func (n *Navigator) Shuffled() bool { return n.order != nil }

func (n *Navigator) Shuffle(rnd *rand.Rand) {
	if rnd == nil {
		rnd = newRand()
	}
	n.rnd = rnd
	n.rebuild()
}

func (n *Navigator) Unshuffle() {
	n.order, n.rank = nil, nil
}

func (n *Navigator) Order() []int {
	n.ensureOrder()
	if n.order == nil {
		return nil
	}
	return append([]int(nil), n.order...)
}

func (n *Navigator) Position() int {
	n.ensureOrder()
	return n.position()
}

func (n *Navigator) Next() bool { return n.stepBy(1, n.loop) }

func (n *Navigator) Prev() bool { return n.stepBy(-1, n.loop) }

func (n *Navigator) Forward(wrap bool) bool { return n.stepBy(1, wrap) }

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

func (n *Navigator) AtEnd() bool {
	count := n.Count()
	if count == 0 {
		return true
	}
	n.ensureOrder()
	return n.position() == count-1
}

func (n *Navigator) Neighbors(k int) []int {
	count := n.Count()
	if count < 2 || k <= 0 {
		return nil
	}
	n.ensureOrder()
	cur := n.position()
	out := make([]int, 0, min(2*k, count-1))
	seen := map[int]struct{}{n.clamp(n.index): {}}
	for d := 1; d <= k && len(out) < count-1; d++ {
		if !n.loop && cur+d >= count && cur-d < 0 {
			break
		}
		for _, p := range [2]int{cur + d, cur - d} {
			if n.loop {
				p = ((p % count) + count) % count
			} else if p < 0 || p >= count {
				continue
			}
			j := n.indexAt(p)
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

func (n *Navigator) stepBy(dir int, wrap bool) bool {
	count := n.Count()
	if count == 0 {
		return false
	}
	n.ensureOrder()
	pos := n.position() + dir
	if pos >= count || pos < 0 {
		if !wrap {
			return false
		}
		if n.order != nil && dir > 0 {
			n.restartCycle()
			pos = 0
		} else {
			pos = (pos + count) % count
		}
	}
	return n.move(n.indexAt(pos))
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

func (n *Navigator) position() int {
	cur := n.clamp(n.index)
	if cur >= len(n.rank) {
		return cur
	}
	return n.rank[cur]
}

func (n *Navigator) indexAt(pos int) int {
	if n.order == nil {
		return pos
	}
	return n.order[pos]
}

func (n *Navigator) ensureOrder() {
	if n.order != nil && len(n.order) != n.Count() {
		n.rebuild()
	}
}

func (n *Navigator) rebuild() {
	count := n.Count()
	if count == 0 {
		n.order, n.rank = []int{}, []int{}
		return
	}
	n.setOrder(ShuffleOrder(count, n.clamp(n.index), n.rnd))
}

func (n *Navigator) restartCycle() {
	count := n.Count()
	order := n.rnd.Perm(count)
	cur := n.clamp(n.index)
	if count > 1 && order[0] == cur {
		j := 1 + n.rnd.IntN(count-1)
		order[0], order[j] = order[j], order[0]
	}
	n.setOrder(order)
}

func (n *Navigator) setOrder(order []int) {
	rank := make([]int, len(order))
	for pos, idx := range order {
		rank[idx] = pos
	}
	n.order, n.rank = order, rank
}

func ShuffleOrder(count, first int, rnd *rand.Rand) []int {
	if count <= 0 {
		return []int{}
	}
	if rnd == nil {
		rnd = newRand()
	}
	order := rnd.Perm(count)
	if first >= 0 && first < count {
		for pos, idx := range order {
			if idx == first {
				order[0], order[pos] = order[pos], order[0]
				break
			}
		}
	}
	return order
}

func newRand() *rand.Rand {
	seed := uint64(time.Now().UnixNano())
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
}
