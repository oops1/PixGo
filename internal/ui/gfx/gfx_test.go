package gfx

import (
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/fonts"
)

func TestFontSizeFollowsTheGlobalTextScale(t *testing.T) {
	old := fonts.TextScale()
	t.Cleanup(func() { fonts.SetTextScale(old) })

	fonts.SetTextScale(1)
	plain := UI(8).Size()
	fonts.SetTextScale(1.5)
	scaled := UI(8).Size()
	semi := UISemi(10).Size()

	if plain != 8 || math.Abs(scaled-12) > 1e-9 || math.Abs(semi-15) > 1e-9 {
		t.Fatalf("sizes: plain %v, scaled %v, semibold %v", plain, scaled, semi)
	}
	if UI(8).Ascent() <= 0 {
		t.Fatal("ascent must follow the scaled size")
	}
}
