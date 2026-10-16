package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type stuckLoop struct {
	mu      sync.Mutex
	blocked bool
	queue   []func()
}

func (s *stuckLoop) post(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked {
		s.queue = append(s.queue, f)
		return
	}
	f()
}

func (s *stuckLoop) release() {
	s.mu.Lock()
	q := s.queue
	s.queue, s.blocked = nil, false
	s.mu.Unlock()
	for _, f := range q {
		f()
	}
}

func dumpsIn(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "hang-*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestHangWatchWritesTheGoroutinesOnceWhenTheLoopStalls(t *testing.T) {
	dir := t.TempDir()
	loop := &stuckLoop{blocked: true}
	h := newHangWatch(loop.post, dir)
	h.beat, h.limit = 10*time.Millisecond, 60*time.Millisecond
	h.start()
	defer h.close()
	waitFor(t, func() bool { return len(dumpsIn(t, dir)) == 1 })
	time.Sleep(150 * time.Millisecond)
	files := dumpsIn(t, dir)
	if len(files) != 1 {
		t.Fatalf("one stall must give one dump, got %d", len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "did not answer") || !strings.Contains(string(data), "goroutine") {
		t.Fatalf("dump without stacks:\n%s", data)
	}
}

func TestHangWatchStaysQuietWhileTheLoopAnswers(t *testing.T) {
	dir := t.TempDir()
	loop := &stuckLoop{}
	h := newHangWatch(loop.post, dir)
	h.beat, h.limit = 10*time.Millisecond, 60*time.Millisecond
	h.start()
	time.Sleep(250 * time.Millisecond)
	h.close()
	if n := len(dumpsIn(t, dir)); n != 0 {
		t.Fatalf("%d dumps without a stall", n)
	}
}

func TestHangWatchDumpsAgainAfterTheLoopRecoversAndStallsAgain(t *testing.T) {
	dir := t.TempDir()
	loop := &stuckLoop{blocked: true}
	h := newHangWatch(loop.post, dir)
	h.beat, h.limit = 10*time.Millisecond, 60*time.Millisecond
	h.now = func() time.Time { return time.Now() }
	h.start()
	defer h.close()
	waitFor(t, func() bool { return len(dumpsIn(t, dir)) >= 1 })
	loop.release()
	time.Sleep(50 * time.Millisecond)
	if h.dumped.Load() {
		t.Fatal("the stall flag must clear when the loop answers")
	}
	loop.mu.Lock()
	loop.blocked = true
	loop.mu.Unlock()
	time.Sleep(1100 * time.Millisecond)
	waitFor(t, func() bool { return len(dumpsIn(t, dir)) >= 2 || h.dumps.Load() >= 2 })
}

func TestHangWatchReportsAnUnwritableFolderWithoutPanicking(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHangWatch(func(func()) {}, file)
	h.limit = 0
	h.check(time.Now().Add(time.Hour))
	if h.dumps.Load() != 1 {
		t.Fatal("the failed attempt must still count")
	}
}
