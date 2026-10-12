package gallery

import (
	"slices"
	"testing"

	"github.com/oops1/PixGo/internal/thumbs"
	"github.com/oops1/PixGo/internal/viewer"
)

func TestNormalizeThumbSizeSnapsToThumbnailSizes(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero means default", 0, DefaultThumbSize},
		{"negative means default", -10, DefaultThumbSize},
		{"smallest", 96, 96},
		{"below the smallest", 40, 96},
		{"closer to smaller", 100, 96},
		{"tie goes to the larger", 112, 128},
		{"closer to larger", 120, 128},
		{"exact middle value", 160, 160},
		{"between 160 and 192 closer to 192", 180, 192},
		{"tie between 192 and 256 goes to 256", 224, 256},
		{"closer to 256", 250, 256},
		{"closer to 384", 330, 384},
		{"largest", 384, 384},
		{"above the largest", 5000, 384},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeThumbSize(tc.in); got != tc.want {
				t.Fatalf("NormalizeThumbSize(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeThumbSizeOnlyReturnsKnownSizes(t *testing.T) {
	known := map[int]bool{}
	for _, s := range thumbs.Sizes() {
		known[s.Px()] = true
	}
	if !known[DefaultThumbSize] {
		t.Fatalf("default %d is not a thumbnail size", DefaultThumbSize)
	}
	for px := -5; px < 500; px++ {
		if got := NormalizeThumbSize(px); !known[got] {
			t.Fatalf("NormalizeThumbSize(%d) = %d is not a thumbnail size", px, got)
		}
	}
}

func TestStepThumbSizeMovesAlongTheSizes(t *testing.T) {
	cases := []struct {
		name  string
		px    int
		steps int
		want  int
	}{
		{"one up", 160, 1, 192},
		{"one down", 160, -1, 128},
		{"no step normalizes", 100, 0, 96},
		{"off-grid value is normalized first", 100, 1, 128},
		{"several up", 96, 3, 192},
		{"clamped at the top", 256, 5, 384},
		{"clamped at the bottom", 128, -9, 96},
		{"top stays", 384, 1, 384},
		{"bottom stays", 96, -1, 96},
		{"unset size starts from the default", 0, 1, 192},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StepThumbSize(tc.px, tc.steps); got != tc.want {
				t.Fatalf("StepThumbSize(%d, %d) = %d, want %d", tc.px, tc.steps, got, tc.want)
			}
		})
	}
}

func TestViewStateNormalized(t *testing.T) {
	cases := []struct {
		name string
		in   ViewState
		want ViewState
	}{
		{"zero value gets defaults", ViewState{}, ViewState{ThumbSize: DefaultThumbSize}},
		{"valid state is unchanged", ViewState{Order: Order{Mode: viewer.SortDate, Desc: true}, Format: "PNG", ThumbSize: 256}, ViewState{Order: Order{Mode: viewer.SortDate, Desc: true}, Format: "PNG", ThumbSize: 256}},
		{"unknown sort mode falls back to name", ViewState{Order: Order{Mode: viewer.SortMode(9), Desc: true}, ThumbSize: 96}, ViewState{Order: Order{Mode: viewer.SortName, Desc: true}, ThumbSize: 96}},
		{"format synonym is canonical", ViewState{Format: "jpg", ThumbSize: 96}, ViewState{Format: "JPEG", ThumbSize: 96}},
		{"format spelling is cleaned", ViewState{Format: " .tif ", ThumbSize: 96}, ViewState{Format: "TIFF", ThumbSize: 96}},
		{"thumbnail size is snapped", ViewState{ThumbSize: 200}, ViewState{ThumbSize: 192}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Normalized()
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			if again := got.Normalized(); again != got {
				t.Fatalf("Normalized is not idempotent: %+v -> %+v", got, again)
			}
		})
	}
}

func TestThumbnailSizesStayInSyncWithTheThumbsPackage(t *testing.T) {
	var got []int
	for _, s := range thumbs.Sizes() {
		got = append(got, NormalizeThumbSize(s.Px()))
	}
	if want := []int{96, 128, 160, 192, 256, 384}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
