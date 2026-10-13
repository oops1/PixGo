package app

import (
	"runtime"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/settings"
)

func TestSlideshowOfTwentyFourFilesRunsForAnHourWithoutLeaks(t *testing.T) {
	const (
		files   = 24
		hour    = 3600
		perStep = 4
	)
	for _, shuffle := range []bool{false, true} {
		name := "folder order"
		if shuffle {
			name = "shuffled"
		}
		t.Run(name, func(t *testing.T) {
			runtime.GC()
			goroutinesBefore := runtime.NumGoroutine()

			r := newSlideRig(t, files, func(s *settings.Settings) {
				s.Slideshow.Interval = 1
				s.Slideshow.Loop = true
				s.Slideshow.Shuffle = shuffle
			})
			r.visible = true
			if !r.c.Slides().Start() {
				t.Fatal("slideshow did not start")
			}

			visits := make([]int, files)
			visits[r.index()]++
			var (
				usedAfterFirstCycle int64
				goroutinesAtTen     int
				heapAfterFirstCycle uint64
				maxTimers           int
				switches            int
			)
			previous := r.index()
			for sec := 1; sec <= hour; sec++ {
				for q := 0; q < perStep; q++ {
					r.advance(time.Second / perStep)
					r.settle()
					if n := r.pendingTimers(); n > maxTimers {
						maxTimers = n
					}
				}
				if got := r.index(); got != previous {
					switches++
					visits[got]++
					previous = got
				} else {
					t.Fatalf("second %d: the slideshow stayed on image %d", sec, got)
				}
				if r.c.cache.Len() > files {
					t.Fatalf("second %d: the cache holds %d entries for %d files", sec, r.c.cache.Len(), files)
				}
				switch sec {
				case files:
					usedAfterFirstCycle = r.c.cache.Used()
					var m runtime.MemStats
					runtime.GC()
					runtime.ReadMemStats(&m)
					heapAfterFirstCycle = m.HeapAlloc
				case 600:
					goroutinesAtTen = runtime.NumGoroutine()
				}
			}

			if switches != hour {
				t.Errorf("%d switches in an hour of 1 s slides, want %d", switches, hour)
			}
			if maxTimers > 1 {
				t.Errorf("up to %d timers were pending at once, want at most one", maxTimers)
			}
			extra := 0
			for i, v := range visits {
				switch v {
				case hour / files:
				case hour/files + 1:
					extra++
				default:
					t.Errorf("file %d was shown %d times, want %d", i, v, hour/files)
				}
			}
			if extra != 1 {
				t.Errorf("%d files were shown once more than the others, want exactly the one that starts the 151st cycle", extra)
			}
			if !shuffle && visits[0] != hour/files+1 {
				t.Errorf("in folder order the first file starts the extra cycle, shown %d times", visits[0])
			}
			if used := r.c.cache.Used(); used > usedAfterFirstCycle {
				t.Errorf("cache grew from %d to %d bytes after the first cycle", usedAfterFirstCycle, used)
			}
			if got := runtime.NumGoroutine(); got > goroutinesAtTen {
				t.Errorf("goroutines grew from %d at ten minutes to %d at one hour", goroutinesAtTen, got)
			}
			var m runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m)
			if m.HeapAlloc > heapAfterFirstCycle+8<<20 {
				t.Errorf("heap grew from %d to %d bytes over the hour", heapAfterFirstCycle, m.HeapAlloc)
			}

			r.c.Close()
			if n := r.pendingTimers(); n != 0 {
				t.Errorf("%d timers left after Close", n)
			}
			time.Sleep(20 * time.Millisecond)
			runtime.GC()
			if after := runtime.NumGoroutine(); after > goroutinesBefore {
				t.Errorf("goroutines: %d before, %d after Close", goroutinesBefore, after)
			}
		})
	}
}
