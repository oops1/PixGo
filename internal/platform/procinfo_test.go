package platform

import (
	"testing"
	"time"
)

const sampleStat = "1234 (pix go) S 1 1234 1234 0 -1 4194560 100 0 0 0 5 3 0 0 20 0 9 0 424242 12345678 2000 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0"

func TestParseProcStatStartSurvivesParenthesesInName(t *testing.T) {
	ticks, err := parseProcStatStart(sampleStat)
	if err != nil || ticks != 424242 {
		t.Fatalf("ticks %d err %v", ticks, err)
	}
}

func TestParseProcStatStartRejectsShortLines(t *testing.T) {
	for _, s := range []string{"", "1 (a) S 1 2", "no parentheses at all"} {
		if _, err := parseProcStatStart(s); err == nil {
			t.Errorf("%q must fail", s)
		}
	}
	if _, err := parseProcStatStart("1 (a) S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 x"); err == nil {
		t.Error("a non numeric start time must fail")
	}
}

func TestParseBootTime(t *testing.T) {
	got, err := parseBootTime("cpu  1 2 3\nbtime 1700000000\nprocesses 9\n")
	if err != nil || got != 1700000000 {
		t.Fatalf("boot %d err %v", got, err)
	}
	if _, err := parseBootTime("cpu 1 2 3\n"); err == nil {
		t.Fatal("missing btime must fail")
	}
	if _, err := parseBootTime("btime soon\n"); err == nil {
		t.Fatal("bad btime must fail")
	}
}

func TestLinuxStartTime(t *testing.T) {
	got := linuxStartTime(1700000000, 150)
	want := time.Unix(1700000001, 500_000_000)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestParseMemoryStatus(t *testing.T) {
	status := "Name:\tpixgo\nVmHWM:\t  204800 kB\nVmRSS:\t  102400 kB\nThreads:\t9\n"
	u, ok := parseMemoryStatus(status)
	if !ok || u.Peak != 204800<<10 || u.Current != 102400<<10 {
		t.Fatalf("usage %+v ok %v", u, ok)
	}
	if _, ok := parseMemoryStatus("Name:\tx\n"); ok {
		t.Fatal("no memory lines must report not ok")
	}
	if _, ok := parseStatusKB("VmRSS:\n", "VmRSS"); ok {
		t.Fatal("empty value must fail")
	}
	if _, ok := parseStatusKB("VmRSS: lots kB\n", "VmRSS"); ok {
		t.Fatal("non numeric value must fail")
	}
	u, ok = parseMemoryStatus("VmRSS: 10 kB\n")
	if !ok || u.Peak != u.Current {
		t.Fatalf("peak must not be below the current size: %+v", u)
	}
}

func TestProcessStartAndMemoryOfThisProcess(t *testing.T) {
	start, err := ProcessStart()
	if err == ErrProcInfoUnavailable {
		t.Skip("not supported on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	if age := time.Since(start); age < 0 || age > 24*time.Hour {
		t.Fatalf("process start %v is %v ago", start, age)
	}
	mem, err := ProcessMemory()
	if err != nil {
		t.Fatal(err)
	}
	if mem.Current < 1<<20 || mem.Peak < mem.Current {
		t.Fatalf("memory %+v", mem)
	}
}
