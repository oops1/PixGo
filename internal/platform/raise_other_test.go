//go:build !windows

package platform

import (
	"errors"
	"testing"
)

type fakeForeground struct{ raised int }

func (f *fakeForeground) SetForeground() { f.raised++ }

func TestRaiseAsksTheWindowToComeForward(t *testing.T) {
	w := &fakeForeground{}

	if err := RaiseWindow(w); err != nil {
		t.Fatal(err)
	}

	if w.raised != 1 {
		t.Fatalf("raised %d times", w.raised)
	}
}

func TestRaiseReportsWindowsThatCannotComeForward(t *testing.T) {
	if err := RaiseWindow(struct{}{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if err := RaiseWindow(nil); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("nil window: err = %v", err)
	}
}
