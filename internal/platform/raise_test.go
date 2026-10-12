package platform

import "testing"

func TestMainWindowIsTheLargestVisibleUnownedOne(t *testing.T) {
	candidates := []windowCandidate{
		{handle: 1, visible: true, owned: true, area: 900_000},
		{handle: 2, visible: true, area: 770_000},
		{handle: 3, visible: false, area: 2_000_000},
		{handle: 4, visible: true, area: 120_000},
		{handle: 5, visible: true, area: 0},
	}

	got, ok := pickMainWindow(candidates)

	if !ok || got != 2 {
		t.Fatalf("picked %d, %v; want window 2", got, ok)
	}
}

func TestMainWindowIsMissingWhenNothingQualifies(t *testing.T) {
	cases := map[string][]windowCandidate{
		"no windows":        nil,
		"only hidden":       {{handle: 1, visible: false, area: 100}},
		"only owned":        {{handle: 2, visible: true, owned: true, area: 100}},
		"only empty":        {{handle: 3, visible: true, area: 0}},
		"only negative box": {{handle: 4, visible: true, area: -5}},
	}
	for name, candidates := range cases {
		if got, ok := pickMainWindow(candidates); ok || got != 0 {
			t.Errorf("%s: picked %d, %v", name, got, ok)
		}
	}
}

func TestMainWindowKeepsTheFirstOfEqualAreas(t *testing.T) {
	got, ok := pickMainWindow([]windowCandidate{
		{handle: 7, visible: true, area: 500},
		{handle: 8, visible: true, area: 500},
	})

	if !ok || got != 7 {
		t.Fatalf("picked %d, %v", got, ok)
	}
}
