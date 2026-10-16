package main

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/oops1/PixGo/internal/app"
	"github.com/oops1/PixGo/internal/thumbs"
)

const (
	gallerySide    = int(thumbs.Size192)
	galleryTimeout = 180 * time.Second
	galleryPoll    = time.Millisecond
)

type galleryRun struct {
	Cache   string  `json:"cache"`
	FirstMs float64 `json:"firstMs"`
	AllMs   float64 `json:"allReadyMs"`
	Failed  int     `json:"failed"`
}

type galleryResult struct {
	Folder  string       `json:"folder"`
	Wanted  int          `json:"thumbnails"`
	Side    int          `json:"sidePixels"`
	Workers int          `json:"workers"`
	Runs    []galleryRun `json:"runs"`
}

func galleryFiles(a *app.App, dir string, limit int) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if full := filepath.Join(dir, e.Name()); a.Ctl.Supports(full) {
			files = append(files, full)
		}
	}
	sort.Strings(files)
	if len(files) > limit {
		files = files[:limit]
	}
	return files, nil
}

func measureGallery(a *app.App, opt perfOptions) *galleryResult {
	dir, err := filepath.Abs(opt.Gallery)
	res := &galleryResult{Folder: opt.Gallery, Side: gallerySide}
	if err != nil {
		return res
	}
	res.Folder = dir
	files, err := galleryFiles(a, dir, opt.GalleryN)
	if err != nil || len(files) == 0 {
		return res
	}
	res.Wanted = len(files)
	cacheDir, err := os.MkdirTemp("", "pixgo-perf-thumbs-")
	if err != nil {
		return res
	}
	defer os.RemoveAll(cacheDir)
	for _, cache := range []string{"cold", "warm"} {
		run, workers := runThumbs(a, files, cacheDir)
		run.Cache = cache
		res.Workers = workers
		res.Runs = append(res.Runs, run)
	}
	return res
}

func runThumbs(a *app.App, files []string, cacheDir string) (galleryRun, int) {
	opts := thumbs.DefaultOptions()
	opts.DiskDir = cacheDir
	opts.Limits = a.Ctl.Limits
	svc := thumbs.New(a.Ctl.Registry(), func(fn func()) { fn() }, opts)
	defer svc.Close()
	workers := svc.Workers()
	svc.SetVisible(files)
	start := time.Now()
	run := galleryRun{}
	ready := make(map[string]bool, len(files))
	deadline := start.Add(galleryTimeout)
	for len(ready) < len(files) && time.Now().Before(deadline) {
		for _, f := range files {
			if ready[f] {
				continue
			}
			t, ok := svc.Request(f, gallerySide, true)
			if !ok {
				continue
			}
			ready[f] = true
			if t.Err != nil {
				run.Failed++
			}
			if run.FirstMs == 0 {
				run.FirstMs = toMs(time.Since(start))
			}
		}
		time.Sleep(galleryPoll)
	}
	run.AllMs = toMs(time.Since(start))
	return run, workers
}
