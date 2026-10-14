//go:build noheic

package all

import (
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func TestHEICIsAbsentWhenTheBuildTagRemovesIt(t *testing.T) {
	r := newRegistry()
	for _, d := range r.Decoders() {
		if d.ID() == codec.HEIC {
			t.Fatal("HEIC decoder is registered")
		}
	}
	if r.Supports("photo.heic") {
		t.Fatal(".heic is supported")
	}
}
