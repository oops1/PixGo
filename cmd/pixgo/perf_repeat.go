package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
)

var repeatedValueFlags = map[string]bool{"perf-runs": true, "perf-json": true}

func childArguments(args []string, jsonPath string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		name := strings.TrimLeft(args[i], "-")
		key, _, hasValue := strings.Cut(name, "=")
		if strings.HasPrefix(args[i], "-") && repeatedValueFlags[key] {
			if !hasValue {
				i++
			}
			continue
		}
		out = append(out, args[i])
	}
	return append(out, "-perf-runs=1", "-perf-json="+jsonPath)
}

func runRepeated(exe string, runs int, args []string, jsonPath string, out io.Writer) int {
	dir, err := os.MkdirTemp("", "pixgo-perf-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "pixgo: perf:", err)
		return 1
	}
	defer os.RemoveAll(dir)
	var reports []*perfReport
	for i := 0; i < runs; i++ {
		path := filepath.Join(dir, fmt.Sprintf("run%d.json", i))
		cmd := exec.Command(exe, childArguments(args, path)...)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "pixgo: perf: run", i+1, "failed:", err)
			return 1
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pixgo: perf:", err)
			return 1
		}
		var rep perfReport
		if err := json.Unmarshal(data, &rep); err != nil {
			fmt.Fprintln(os.Stderr, "pixgo: perf:", err)
			return 1
		}
		reports = append(reports, &rep)
	}
	printAggregate(out, reports)
	if jsonPath != "" {
		data, err := json.MarshalIndent(reports, "", "  ")
		if err == nil {
			err = os.WriteFile(jsonPath, append(data, '\n'), 0o644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "pixgo: perf:", err)
			return 1
		}
	}
	return 0
}

func spread(values []float64) (median, lo, hi float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	n := len(sorted)
	median = sorted[n/2]
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return median, sorted[0], sorted[n-1]
}

func printAggregate(w io.Writer, reports []*perfReport) {
	if len(reports) == 0 {
		return
	}
	first := reports[0]
	var b strings.Builder
	first.printHeader(&b)
	fmt.Fprintf(&b, "Median, minimum and maximum of %d runs, each in a fresh process\n", len(reports))
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "event\tmedian ms\tmin\tmax")
	for i, m := range first.Marks {
		var values []float64
		for _, r := range reports {
			if i < len(r.Marks) && r.Marks[i].Name == m.Name {
				values = append(values, r.Marks[i].Ms)
			}
		}
		med, lo, hi := spread(values)
		fmt.Fprintf(tw, "%s\t%.1f\t%.1f\t%.1f\n", m.Name, med, lo, hi)
	}
	tw.Flush()
	b.WriteString("\n")
	aggregatePeaks(&b, reports)
	aggregateFrames(&b, reports)
	io.WriteString(w, b.String())
}

func aggregatePeaks(b *strings.Builder, reports []*perfReport) {
	tw := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "process peak MiB after\tmedian\tmin\tmax")
	for i, m := range reports[0].Memory {
		var values []float64
		for _, r := range reports {
			if i < len(r.Memory) && r.Memory[i].Phase == m.Phase {
				values = append(values, mb(r.Memory[i].ProcessPeak))
			}
		}
		med, lo, hi := spread(values)
		fmt.Fprintf(tw, "%s\t%.0f\t%.0f\t%.0f\n", m.Phase, med, lo, hi)
	}
	tw.Flush()
	b.WriteString("\n")
}

func aggregateFrames(b *strings.Builder, reports []*perfReport) {
	var p50, p95, mx, placeholders []float64
	for _, r := range reports {
		if r.Pan != nil {
			p50 = append(p50, r.Pan.P50Ms)
			p95 = append(p95, r.Pan.P95Ms)
			mx = append(mx, r.Pan.MaxMs)
			placeholders = append(placeholders, float64(r.Pan.Placeholders))
		}
	}
	if len(p50) == 0 {
		return
	}
	a, _, _ := spread(p50)
	c, _, _ := spread(p95)
	d, _, hi := spread(mx)
	e, _, _ := spread(placeholders)
	fmt.Fprintf(b, "Pan frame ms, medians over runs: p50 %.2f, p95 %.2f, max %.2f (worst %.2f), placeholder frames %.0f\n\n", a, c, d, hi, e)
}

func runRepeatedSelf(runs int, args []string, jsonPath string) int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pixgo: perf:", err)
		return 1
	}
	return runRepeated(exe, runs, args, jsonPath, os.Stdout)
}
