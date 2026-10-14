//go:build noavif

package all

import (
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func TestAVIFIsAbsentWhenTheBuildTagRemovesIt(t *testing.T) {
	r := newRegistry()
	for _, d := range r.Decoders() {
		if d.ID() == codec.AVIF {
			t.Fatal("AVIF decoder is registered")
		}
	}
	if r.Supports("photo.avif") {
		t.Fatal(".avif is supported")
	}
}
