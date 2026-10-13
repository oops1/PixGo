package viewer

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
)

func seeded(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed+1)) }

func isPermutation(order []int, count int) bool {
	if len(order) != count {
		return false
	}
	seen := make([]bool, count)
	for _, v := range order {
		if v < 0 || v >= count || seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}

func TestShuffleOrderIsAPermutationStartingAtTheChosenFile(t *testing.T) {
	cases := []struct {
		count, first int
	}{{1, 0}, {2, 0}, {2, 1}, {5, 3}, {24, 0}, {24, 23}, {24, 11}}
	for _, c := range cases {
		for seed := uint64(1); seed <= 20; seed++ {
			order := ShuffleOrder(c.count, c.first, seeded(seed))
			if !isPermutation(order, c.count) || order[0] != c.first {
				t.Fatalf("count %d first %d seed %d: order %v", c.count, c.first, seed, order)
			}
		}
	}
}

func TestShuffleOrderEdgeCases(t *testing.T) {
	if got := ShuffleOrder(0, 0, seeded(1)); len(got) != 0 {
		t.Errorf("empty folder: %v", got)
	}
	if got := ShuffleOrder(-3, 0, nil); len(got) != 0 {
		t.Errorf("negative count: %v", got)
	}
	order := ShuffleOrder(6, 99, seeded(2))
	if !isPermutation(order, 6) {
		t.Errorf("first out of range must still give a permutation: %v", order)
	}
	if !isPermutation(ShuffleOrder(6, -1, nil), 6) {
		t.Error("nil source must still give a permutation")
	}
}

func TestShuffledNavigationVisitsEveryFileOnceWithoutWrap(t *testing.T) {
	for _, count := range []int{2, 3, 7, 24} {
		for _, start := range []int{0, count / 2, count - 1} {
			t.Run(fmt.Sprintf("count %d start %d", count, start), func(t *testing.T) {
				n := NewNavigator(folderOf(count), start)
				n.Shuffle(seeded(uint64(count*100 + start)))
				if !n.Shuffled() || n.Order()[0] != start || n.Index() != start {
					t.Fatalf("shuffle must keep the current file first: order %v index %d", n.Order(), n.Index())
				}
				seen := []int{n.Index()}
				for n.Forward(false) {
					seen = append(seen, n.Index())
				}
				if !isPermutation(seen, count) {
					t.Fatalf("a cycle must show every file once: %v", seen)
				}
				if !n.AtEnd() || n.Forward(false) {
					t.Error("the cycle must end after the last file when wrapping is off")
				}
			})
		}
	}
}

func TestShuffledWrapStartsANewCycleWithoutRepeatingTheLastFile(t *testing.T) {
	for _, count := range []int{2, 3, 5, 24} {
		for seed := uint64(1); seed <= 40; seed++ {
			n := NewNavigator(folderOf(count), 0)
			n.Shuffle(seeded(seed))
			var played []int
			played = append(played, n.Index())
			for step := 0; step < 3*count-1; step++ {
				if !n.Forward(true) {
					t.Fatalf("count %d seed %d step %d: wrap must always move", count, seed, step)
				}
				played = append(played, n.Index())
			}
			for cycle := 0; cycle < 3; cycle++ {
				if chunk := played[cycle*count : (cycle+1)*count]; !isPermutation(chunk, count) {
					t.Fatalf("count %d seed %d cycle %d is not a permutation: %v", count, seed, cycle, chunk)
				}
			}
			for i := 1; i < len(played); i++ {
				if played[i] == played[i-1] {
					t.Fatalf("count %d seed %d: file %d shown twice in a row: %v", count, seed, played[i], played)
				}
			}
		}
	}
}

func TestShuffledPrevRetracesNext(t *testing.T) {
	n := NewNavigator(folderOf(9), 4)
	n.Shuffle(seeded(7))
	forward := []int{n.Index()}
	for i := 0; i < 5; i++ {
		n.Next()
		forward = append(forward, n.Index())
	}
	var back []int
	for i := 0; i < 5; i++ {
		n.Prev()
		back = append(back, n.Index())
	}
	want := slices.Clone(forward[:5])
	slices.Reverse(want)
	if !slices.Equal(back, want) {
		t.Errorf("prev walked %v, want %v (forward was %v)", back, want, forward)
	}
	if n.Prev() {
		t.Error("prev before the first position must stop without wrapping")
	}
	n.SetLoop(true)
	if !n.Prev() || n.Position() != 8 {
		t.Errorf("prev with wrapping must land on the last position, got %d", n.Position())
	}
}

func TestShuffledGoContinuesFromTheChosenFile(t *testing.T) {
	n := NewNavigator(folderOf(10), 0)
	n.Shuffle(seeded(11))
	order := n.Order()
	target := order[6]
	if !n.Go(target) || n.Index() != target || n.Position() != 6 {
		t.Fatalf("go: index %d position %d", n.Index(), n.Position())
	}
	n.Next()
	if n.Index() != order[7] {
		t.Errorf("next after go = %d, want %d", n.Index(), order[7])
	}
}

func TestShuffledOrderFollowsFolderChanges(t *testing.T) {
	f := folderOf(8)
	n := NewNavigator(f, 3)
	n.Shuffle(seeded(5))
	f.Remove(f.At(0).Path)
	f.Remove(f.At(0).Path)
	if got := n.Order(); !isPermutation(got, 6) {
		t.Fatalf("order after removing files: %v", got)
	}
	n.Go(n.Order()[0])
	seen := []int{n.Index()}
	for n.Forward(false) {
		seen = append(seen, n.Index())
	}
	if !isPermutation(seen, 6) {
		t.Errorf("after the folder shrank a cycle must still show every file once: %v", seen)
	}
}

func TestUnshuffleRestoresFolderOrder(t *testing.T) {
	n := NewNavigator(folderOf(5), 2)
	n.Shuffle(seeded(3))
	n.Unshuffle()
	if n.Shuffled() || n.Order() != nil {
		t.Fatal("unshuffle must drop the order")
	}
	n.Next()
	if n.Index() != 3 {
		t.Errorf("next after unshuffle = %d, want 3", n.Index())
	}
}

func TestShuffledNeighborsFollowThePlayOrder(t *testing.T) {
	n := NewNavigator(folderOf(10), 0)
	n.Shuffle(seeded(21))
	order := n.Order()
	got := n.Neighbors(2)
	if len(got) != 2 || got[0] != order[1] || got[1] != order[2] {
		t.Errorf("neighbors at the start of the cycle = %v, order %v", got, order)
	}
	n.SetLoop(true)
	got = n.Neighbors(1)
	if len(got) != 2 || got[0] != order[1] || got[1] != order[9] {
		t.Errorf("looping neighbors = %v, order %v", got, order)
	}
}

func TestForwardWithoutShuffleHonoursTheWrapArgument(t *testing.T) {
	n := NewNavigator(folderOf(3), 2)
	if n.Forward(false) || n.Index() != 2 || !n.AtEnd() {
		t.Fatal("forward without wrapping must stop at the last file")
	}
	if !n.Forward(true) || n.Index() != 0 || n.AtEnd() {
		t.Fatalf("forward with wrapping must go to the first file, index %d", n.Index())
	}
	empty := NewNavigator(folderOf(0), 0)
	if empty.Forward(true) || !empty.AtEnd() || empty.Position() != 0 {
		t.Error("an empty navigator has nowhere to go")
	}
}
