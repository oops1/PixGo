//go:build nojxl

package all

import (
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func TestJXLIsAbsentWhenTheBuildTagRemovesIt(t *testing.T) {
	r := newRegistry()
	for _, d := range r.Decoders() {
		if d.ID() == codec.JXL {
			t.Fatal("JXL decoder is registered")
		}
	}
	if r.Supports("photo.jxl") {
		t.Fatal(".jxl is supported")
	}
}
