package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/PixGo/internal/buildinfo"
)

const (
	hangBeat    = time.Second
	hangLimit   = 8 * time.Second
	hangMaxKept = 5
)

type hangWatch struct {
	post  func(func())
	dir   string
	now   func() time.Time
	beat  time.Duration
	limit time.Duration

	last   atomic.Int64
	dumped atomic.Bool
	stop   chan struct{}
	once   sync.Once
	dumps  atomic.Int32
}

func newHangWatch(post func(func()), dir string) *hangWatch {
	h := &hangWatch{post: post, dir: dir, now: time.Now, beat: hangBeat, limit: hangLimit, stop: make(chan struct{})}
	h.last.Store(h.now().UnixNano())
	return h
}

func (h *hangWatch) start() {
	go h.loop()
}

func (h *hangWatch) close() { h.once.Do(func() { close(h.stop) }) }

func (h *hangWatch) loop() {
	tick := time.NewTicker(h.beat)
	defer tick.Stop()
	prev := h.now()
	for {
		select {
		case <-h.stop:
			return
		case <-tick.C:
		}
		now := h.now()
		if now.Sub(prev) > 3*h.beat {
			h.last.Store(now.UnixNano())
		}
		prev = now
		h.post(func() {
			h.last.Store(h.now().UnixNano())
			h.dumped.Store(false)
		})
		h.check(now)
	}
}

func (h *hangWatch) check(now time.Time) {
	stalled := now.Sub(time.Unix(0, h.last.Load()))
	if stalled < h.limit || h.dumped.Load() || h.dumps.Load() >= hangMaxKept {
		return
	}
	h.dumped.Store(true)
	h.dumps.Add(1)
	path, err := h.write(now, stalled)
	if err != nil {
		slog.Warn("interface stalled; writing the goroutine dump failed", "stalled", stalled, "err", err)
		return
	}
	slog.Warn("interface stalled; goroutine dump written", "stalled", stalled.Round(time.Second), "path", path)
}

func (h *hangWatch) write(now time.Time, stalled time.Duration) (string, error) {
	if err := os.MkdirAll(h.dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(h.dir, "hang-"+now.Format("20060102-150405")+".txt")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fmt.Fprintf(f, "PixGo %s: the interface thread did not answer for %s\n\n", buildinfo.String(), stalled.Round(time.Millisecond))
	if err := pprof.Lookup("goroutine").WriteTo(f, 2); err != nil {
		return "", err
	}
	return path, nil
}
