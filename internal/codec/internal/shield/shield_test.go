package shield

import (
	"errors"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func TestRunPassesResultsThrough(t *testing.T) {
	if err := Run("test", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("plain failure")
	if err := Run("test", func() error { return sentinel }); err != sentinel {
		t.Fatalf("%v", err)
	}
}

func TestRunTurnsPanicsIntoCorruptFileErrors(t *testing.T) {
	var nilMap map[string]int
	values := []func() error{
		func() error { panic("a string") },
		func() error { panic(errors.New("an error")) },
		func() error { panic(42) },
		func() error { var s []int; _ = s[3]; return nil },
		func() error { nilMap["x"] = 1; return nil },
		func() error { var p *int; return errors.New(string(rune(*p))) },
	}
	for i, call := range values {
		err := Run("demo", call)
		if !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}
