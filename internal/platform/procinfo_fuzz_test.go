package platform

import "testing"

func FuzzProcInfoParsers(f *testing.F) {
	f.Add(sampleStat, "cpu 1 2\nbtime 1700000000\n", "VmHWM:\t204800 kB\nVmRSS:\t102400 kB\n")
	f.Add("", "", "")
	f.Add(")", "btime", "VmRSS:")
	f.Add("1 (a) S "+string(make([]byte, 64)), "btime -1\n", "VmRSS: 18446744073709551615 kB\n")
	f.Fuzz(func(t *testing.T, stat, system, status string) {
		parseProcStatStart(stat)
		if boot, err := parseBootTime(system); err == nil {
			linuxStartTime(boot, 12345)
		}
		if usage, ok := parseMemoryStatus(status); ok && usage.Peak < usage.Current {
			t.Fatalf("peak %d below current %d", usage.Peak, usage.Current)
		}
		parseStatusKB(status, "VmRSS")
	})
}
