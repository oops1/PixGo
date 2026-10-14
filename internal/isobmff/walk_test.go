package isobmff_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/isobmff"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

type visited struct {
	kind    string
	payload int64
	length  int64
}

func walkAll(data []byte, start, end int64, limit int) ([]visited, error) {
	var out []visited
	err := isobmff.Walk(context.Background(), bytes.NewReader(data), int64(len(data)), start, end, limit, func(s isobmff.Span) error {
		out = append(out, visited{s.Type, s.Payload, s.Len()})
		return nil
	})
	return out, err
}

func TestWalkVisitsEveryBoxKind(t *testing.T) {
	data := append(isotest.Box("abcd", []byte("12")), isotest.LargeBox("larg", []byte("123"))...)
	data = append(data, isotest.OpenEndedBox("tail", []byte("12345"))...)
	got, err := walkAll(data, 0, int64(len(data)), 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []visited{{"abcd", 8, 2}, {"larg", 26, 3}, {"tail", 37, 5}}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("box %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestWalkDescendsIntoPayloadRange(t *testing.T) {
	inner := append(isotest.Box("aaaa", []byte("x")), isotest.Box("bbbb")...)
	data := isotest.Box("outr", inner)
	var children []visited
	err := isobmff.Walk(context.Background(), bytes.NewReader(data), int64(len(data)), 0, int64(len(data)), 10, func(s isobmff.Span) error {
		if s.Start != 0 || s.End != int64(len(data)) {
			t.Fatalf("outer span %+v", s)
		}
		var err error
		children, err = walkAll(data, s.Payload, s.End, 10)
		return err
	})
	if err != nil || len(children) != 2 || children[0].kind != "aaaa" || children[1].kind != "bbbb" || children[1].length != 0 {
		t.Fatalf("%+v %v", children, err)
	}
}

func TestWalkStopsOnVisitorError(t *testing.T) {
	data := append(isotest.Box("abcd"), isotest.Box("efgh")...)
	stop := errors.New("stop")
	count := 0
	err := isobmff.Walk(context.Background(), bytes.NewReader(data), int64(len(data)), 0, int64(len(data)), 10, func(isobmff.Span) error {
		count++
		return stop
	})
	if !errors.Is(err, stop) || count != 1 {
		t.Fatalf("count %d err %v", count, err)
	}
}

func TestWalkRejectsBrokenBoxes(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"box longer than file", append(append(isotest.U32(100), "free"...), 0, 0, 0, 0)},
		{"box shorter than its header", append(isotest.U32(4), "abcd"...)},
		{"large header cut off", append(append(isotest.U32(1), "larg"...), 0, 0, 0, 0)},
		{"large size beyond file", append(append(isotest.U32(1), "larg"...), isotest.U64(1<<40)...)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := walkAll(c.data, 0, int64(len(c.data)), 10); !errors.Is(err, codec.ErrCorrupt) {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestWalkKeepsBoxesSeenBeforeTheBreak(t *testing.T) {
	data := append(isotest.Box("good", []byte("12")), isotest.U32(1<<20)...)
	data = append(data, "bad!"...)
	got, err := walkAll(data, 0, int64(len(data)), 10)
	if len(got) != 1 || got[0].kind != "good" || !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestWalkRejectsBadRanges(t *testing.T) {
	data := isotest.Box("abcd")
	for _, r := range [][2]int64{{-1, 4}, {8, 4}, {0, 100}} {
		if _, err := walkAll(data, r[0], r[1], 10); !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("range %v: %v", r, err)
		}
	}
	if got, err := walkAll(data, 4, 8, 10); err != nil || len(got) != 0 {
		t.Fatalf("tail shorter than a header: %+v %v", got, err)
	}
}

func TestWalkLimitsTheNumberOfBoxes(t *testing.T) {
	var data []byte
	for i := 0; i < 50; i++ {
		data = append(data, isotest.Box("free")...)
	}
	if got, err := walkAll(data, 0, int64(len(data)), 50); err != nil || len(got) != 50 {
		t.Fatalf("%d boxes, %v", len(got), err)
	}
	if _, err := walkAll(data, 0, int64(len(data)), 49); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("%v", err)
	}
}

func TestWalkObeysCancelledContext(t *testing.T) {
	data := isotest.Box("abcd")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := isobmff.Walk(ctx, bytes.NewReader(data), int64(len(data)), 0, int64(len(data)), 10, func(isobmff.Span) error {
		t.Fatal("visited after cancel")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}

func TestWalkReportsReaderFailures(t *testing.T) {
	data := isotest.Box("abcd", []byte("12345678"))
	err := isobmff.Walk(context.Background(), bytes.NewReader(data[:10]), int64(len(data)), 0, int64(len(data)), 10, func(isobmff.Span) error { return nil })
	if !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("%v", err)
	}
}

func TestPropertiesOfReturnsEveryPropertyOfAKind(t *testing.T) {
	profile := bytes.Repeat([]byte{7}, 20)
	f := av1Still(4, 4, isotest.Colr(), isotest.ColrProfile("prof", profile), isotest.ColrProfile("rICC", profile[:5]))
	file := open(t, f)
	item := file.PrimaryItem()
	colrs := file.PropertiesOf(item, "colr")
	if len(colrs) != 3 {
		t.Fatalf("%d colr properties", len(colrs))
	}
	if string(colrs[0].Payload[:4]) != "nclx" || string(colrs[1].Payload[:4]) != "prof" || string(colrs[2].Payload[:4]) != "rICC" {
		t.Fatalf("%q %q %q", colrs[0].Payload[:4], colrs[1].Payload[:4], colrs[2].Payload[:4])
	}
	if got := file.PropertiesOf(item, "clap"); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	if file.PropertiesOf(nil, "colr") != nil {
		t.Fatal("properties of a nil item")
	}
}
