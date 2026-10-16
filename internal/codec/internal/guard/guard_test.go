package guard

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func TestLimitsFillsZeroFieldsOnly(t *testing.T) {
	defaults := codec.DefaultLimits()
	if got := Limits(codec.Limits{}); got != defaults {
		t.Fatalf("got %+v", got)
	}
	custom := codec.Limits{MaxFileBytes: 5, MaxPixels: 6, MaxFrames: 7, MaxFramePixels: 8, MaxTiledPixels: 9}
	if got := Limits(custom); got != custom {
		t.Fatalf("got %+v", got)
	}
	partial := Limits(codec.Limits{MaxPixels: 99})
	if partial.MaxPixels != 99 || partial.MaxFrames != defaults.MaxFrames {
		t.Fatalf("got %+v", partial)
	}
}

func TestCheckSize(t *testing.T) {
	lim := codec.Limits{MaxFileBytes: 100}
	if err := CheckSize(100, lim); err != nil {
		t.Fatal(err)
	}
	if err := CheckSize(101, lim); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
	if err := CheckSize(-1, lim); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
}

func TestReadAtChecksEveryBound(t *testing.T) {
	data := []byte("0123456789")
	r := bytes.NewReader(data)
	got, err := ReadAt(r, 2, 3, int64(len(data)))
	if err != nil || string(got) != "234" {
		t.Fatalf("%q %v", got, err)
	}
	if got, err := ReadAt(r, 10, 0, 10); err != nil || len(got) != 0 {
		t.Fatalf("empty read at the end: %v", err)
	}
	for _, c := range [][3]int64{{-1, 1, 10}, {0, -1, 10}, {11, 0, 10}, {5, 6, 10}, {math.MaxInt64, 1, 10}, {1, math.MaxInt64, 10}} {
		if _, err := ReadAt(r, c[0], c[1], c[2]); !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("%v: %v", c, err)
		}
	}
	if _, err := ReadAt(r, 0, math.MaxInt32+1, math.MaxInt64); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("huge read: %v", err)
	}
	if _, err := ReadAt(r, 0, 20, 20); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("size larger than the reader: %v", err)
	}
}

func TestReadAtIntoChecksBounds(t *testing.T) {
	r := bytes.NewReader([]byte("abcdef"))
	buf := make([]byte, 3)
	if err := ReadAtInto(r, buf, 3, 6); err != nil || string(buf) != "def" {
		t.Fatalf("%q %v", buf, err)
	}
	if err := ReadAtInto(r, buf, 4, 6); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
	if err := ReadAtInto(r, buf, -1, 6); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
	if err := ReadAtInto(r, nil, 6, 6); err != nil {
		t.Fatal(err)
	}
}

func TestClassify(t *testing.T) {
	ctx := context.Background()
	if Classify(ctx, nil, nil) != nil {
		t.Fatal("nil must stay nil")
	}
	plain := errors.New("boom")
	if err := Classify(ctx, plain, nil); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
	if err := Classify(ctx, plain, func(error) bool { return true }); !errors.Is(err, codec.ErrUnsupported) {
		t.Fatalf("got %v", err)
	}
	if err := Classify(ctx, TooLarge("x"), func(error) bool { return true }); !errors.Is(err, codec.ErrTooLarge) || errors.Is(err, codec.ErrUnsupported) {
		t.Fatalf("codec errors must pass through: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := Classify(cancelled, plain, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if err := Classify(ctx, context.DeadlineExceeded, nil); !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
}

func TestReaderStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := NewReader(ctx, bytes.NewReader([]byte("hello world")), 11)
	buf := make([]byte, 5)
	if n, err := r.Read(buf); err != nil || n != 5 {
		t.Fatalf("%d %v", n, err)
	}
	if n, err := r.ReadAt(buf, 6); err != nil || string(buf[:n]) != "world" {
		t.Fatalf("%q %v", buf[:n], err)
	}
	cancel()
	if _, err := r.Read(buf); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if _, err := r.ReadAt(buf, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if r.Size() != 11 {
		t.Fatalf("size %d", r.Size())
	}
	stream := Stream(context.Background(), bytes.NewReader([]byte("abc")), 3)
	all, err := io.ReadAll(stream)
	if err != nil || string(all) != "abc" {
		t.Fatalf("%q %v", all, err)
	}
	if NewReader(context.Background(), bytes.NewReader(nil), -5).Size() != 0 {
		t.Fatal("negative size must become zero")
	}
}

func TestFrameBudget(t *testing.T) {
	lim := codec.Limits{MaxPixels: 100, MaxFrames: 3, MaxFramePixels: 250}
	b := NewFrameBudget(lim)
	if err := b.Add(10, 10); err != nil {
		t.Fatal(err)
	}
	if err := b.Add(10, 10); err != nil {
		t.Fatal(err)
	}
	if err := b.Add(10, 6); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("total pixels: %v", err)
	}
	if err := NewFrameBudget(lim).Add(11, 10); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("single frame: %v", err)
	}
	if err := NewFrameBudget(lim).Add(0, 5); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("empty frame: %v", err)
	}
	small := NewFrameBudget(codec.Limits{MaxPixels: 100, MaxFrames: 2, MaxFramePixels: 1000})
	small.Add(1, 1)
	small.Add(1, 1)
	if err := small.Add(1, 1); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("frame count: %v", err)
	}
	if small.Frames() != 3 {
		t.Fatalf("frames %d", small.Frames())
	}
}

func TestCheckFrames(t *testing.T) {
	lim := codec.Limits{MaxPixels: 100, MaxFrames: 10, MaxFramePixels: 500}
	if err := CheckFrames(lim, 5, 10, 10); err != nil {
		t.Fatal(err)
	}
	if err := CheckFrames(lim, 6, 10, 10); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("pixel budget: %v", err)
	}
	if err := CheckFrames(lim, 11, 1, 1); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("frame budget: %v", err)
	}
	if err := CheckFrames(lim, 1, 101, 1); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("frame size: %v", err)
	}
	if err := CheckFrames(lim, 1, 0, 1); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("empty: %v", err)
	}
}
