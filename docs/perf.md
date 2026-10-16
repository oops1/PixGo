# Performance and memory

0.6 is the "big files" release. This page lists what was measured, how to repeat it, the numbers, and the decisions behind them. Everything here was measured with the tools in this repository; nothing is quoted from another program.

## Targets of 0.6

| Target | Result | Status |
|---|---|---|
| A 200 Mpx JPEG opens and pans with the process under 600 MB (peak working set), while open and while panning | 131 MB when open, 237 MB after panning, 307 MB after zooming through every step (window 1100x700). 301 and 380 MB in a 1920x1080 window, 523 and 576 MB in a 3840x2160 window | **PASS** |
| A 24 Mpx JPEG, warm start: process creation to the first picture under 200 ms | 125 ms in 1.0.0 (median of 9 runs, min 121, max 137), full resolution 186 ms; 120 ms in 0.6.0; 0.5.0 measured the same way: 237 ms | **PASS** (offscreen, see the limits below) |

The tiled-picture numbers are the same with and without neighbouring files in the folder (0.6 does not prefetch next to a tiled picture, see "Decisions").

## What is measured and how

`pixgo -perf FILE` runs the real application on the offscreen engine (no window, no real mouse or keyboard) and prints a table. The same flags work on Windows and Linux.

```
pixgo -perf FILE [-width W -height H] [-scale K] [-perf-scenarios timeline,pan,zoom,damage]
      [-perf-json out.json] [-perf-runs N] [-perf-cpuprofile cpu.prof] [-perf-memprofile mem.prof]
      [-perf-frames 120] [-perf-pan-step 32] [-perf-unpaced] [-perf-damage-seconds 2.5]
pixgo -perf-gallery DIR [-perf-gallery-n 24] [-perf-json out.json]
```

The window is 1100x700 unless `-width` and `-height` say otherwise (the default window of the program). The harness does not touch the real settings when `PIXGO_CONFIG_DIR`, `PIXGO_STATE_DIR` and `PIXGO_CACHE_DIR` point to empty folders; the thumbnail scenario always uses a temporary cache.

- **Timeline.** Every time is counted from the creation of the process, taken from the operating system (`GetProcessTimes` on Windows, `/proc/self/stat` on Linux), so the start of the runtime and the initialisation of the packages are included. Events: packages initialised, settings loaded, engine ready, app ready, open requested, folder scanned, loading frame rendered, probe done, first picture published (the preview or the full picture is on the canvas), first picture rendered (a frame that contains it has been drawn by the engine), full resolution ready, first full frame rendered, background loads done (neighbours prefetched). The loading frame is drawn right after the open request, as the real window does, so the cost of the first frame with the toolbar is not charged to the picture.
- **Memory.** After each event: the Go heap in use and allocated, the runtime total, the process working set and its peak (`PeakWorkingSetSize` on Windows, `VmHWM` on Linux), bytes allocated so far, garbage collections and their pauses.
- **Pan.** At 100 % zoom, 120 frames. The picture is dragged through the canvas (the same mouse path as a user drag, delivered inside the process), 32 pixels per frame by default, paced to 60 frames per second; `-perf-unpaced` removes the pacing and `-perf-pan-step 128` makes it a fast fling. A frame is `RenderFrameNow`, the whole draw path including the engine. For tiled pictures the report adds the window requests, the frames drawn with a placeholder (a requested tile that was not decoded yet), the decoded tiles, and the time until the picture is sharp again.
- **Zoom.** From fit in steps up to 800 %, and back; the time of the first frame after every step and the time until every tile has arrived.
- **Damage.** The engine runs its real loop; the report counts the pixels of the tiles the engine marks as changed in every frame, as pixels and as a percentage of the window: a still picture, an animation (when the file is animated), and the progress bar of the slideshow (when the folder holds another picture).
- **Gallery.** `-perf-gallery DIR` asks the thumbnail service for the first N pictures of a folder (192 px, as the gallery does) and waits for all of them, once with an empty disk cache (cold) and once with the cache just written (warm).
- **Repeats.** `-perf-runs N` starts N fresh processes and prints the median, minimum and maximum of every event.
- **Profiles.** `-perf-cpuprofile` samples at 1000 Hz from the first line of `main`; `-perf-memprofile` records allocations. Open them with `go tool pprof pixgo.exe cpu.prof`.

Test pictures are made with `go run ./tools/genbig -w 16384 -h 12288 -q 85 -o big200.jpg` (200 Mpx, 47 MB) and `-w 6000 -h 4000` (24 Mpx, 6.2 MB). They are synthetic: rings, a grid and noise, encoded as baseline 4:2:0 JPEG with the standard library. The noise makes the bitrate close to a photograph (about 2 bits per pixel), but the content is easier than most photographs for the entropy decoder.

## Machine

Intel Core Ultra 7 265K (20 logical cores), 63.5 GiB RAM, Windows 11 (10.0.26200), Go 1.27.1, all files in the file cache (warm). The numbers scale with the cores and the clock; the relations between the rows are what to compare.

## Results

### 24 Mpx JPEG, start (1.0.0, window 1100x700, median of 9 fresh processes)

| Event | ms from process creation |
|---|---|
| packages initialised | 34.2 |
| settings loaded | 34.6 |
| engine ready | 56.8 |
| app ready | 61.0 |
| open returned (since 1.0 the folder is listed in the background, after the picture is requested) | 61.4 |
| loading frame rendered | 87.9 |
| probe done | 87.9 |
| first picture published (preview at 1/4, 1500x1000) | 119.8 |
| **first picture rendered** | **125.0** |
| full resolution ready | 186.3 |
| first full frame rendered | 191.3 |

Process peak: 64 MB when the app is ready, 79 MB with the preview, 199 MB with the full picture.

### 0.5.0 and 0.6.0

The 0.5.0 numbers come from a timeline tool built from the v0.5.0 tree with the same method (process creation, engine, app, open, wait until ready, render) and from the old test of the thumbnail service.

| | 0.5.0 | 0.6.0 |
|---|---|---|
| 24 Mpx JPEG, first picture rendered | 237 ms (full decode only) | 120 ms |
| 24 Mpx JPEG, full resolution ready | 233 ms | 183 ms |
| 24 Mpx JPEG, process peak when shown | 217 MB | 199 MB |
| `pixgo -screenshot`, 24 Mpx, wall time and peak | 373 ms, 220 MB | 311 ms, 195 MB |
| 200 Mpx JPEG | refused after 80 ms | first picture 486 ms, 131 MB when open |
| `pixgo -screenshot`, 200 Mpx, wall time and peak | 140 ms (refusal), 80 MB | 606 ms, 130 MB |
| 24 thumbnails of 24 Mpx JPEGs, empty cache | 1070 ms (first after 211 ms) | 287 ms (first after 55 ms) |
| the same, cache filled | 8 to 14 ms | 7 to 8 ms |
| 6 thumbnails of 200 Mpx JPEGs, empty cache | all failed | 797 ms |

### 200 Mpx JPEG (median of 5 fresh processes)

| Window | first picture rendered | process peak when open | after the pan | after the zoom steps | pan frame p50 / p95 / max | zoom first frame p95 | frames with a placeholder |
|---|---|---|---|---|---|---|---|
| 1100x700 | 486 ms | 131 MB | 237 MB | 307 MB | 4.0 / 4.5 / 5.0 ms | 5.5 ms | 0 of 120 |
| 1920x1080 | 493 ms | 147 MB | 301 MB | 380 MB | 8.5 / 9.5 / 10.0 ms | 15.5 ms | 0 of 120 |
| 1920x1080, unpaced, 128 px per frame | 495 ms | 145 MB | 346 MB | 387 MB | 9.0 / 10.0 / 11.5 ms | 14.0 ms | 0 of 120 |
| 3840x2160 | 525 ms | 258 MB | 523 MB | 576 MB | 33.2 / 38.1 / 46.0 ms | 48.0 ms | 0 of 120 |

With a 24 Mpx picture on each side in the folder the rows are the same (237 / 308 MB at 1100x700; 302 / 381 MB at 1920x1080). The picture appears after the entropy decoding of 47 MB of JPEG data (about 400 ms for the overview of 4096x3072); that part cannot use more than one core because the file has no restart markers.

A 24 Mpx picture in the same windows: pan 3.5 / 4.3 / 5.0 ms (1100x700) and 8.5 / 9.0 / 9.5 ms (1920x1080).

When a folder of 24 Mpx pictures is browsed, the neighbours are prefetched after the current picture is ready and the process peaks at 446 MB (current picture 128 MB plus two neighbours of 128 MB each, plus the base of 64 MB).

### Threads (setting "Decoding and rendering threads", 24 Mpx and 200 Mpx JPEG, window 1100x700)

| Threads | 24 Mpx first picture | 24 Mpx full resolution | 200 Mpx first picture | gallery, 24 thumbnails of 24 Mpx, empty cache (thumbnail workers) |
|---|---|---|---|---|
| 1 | 131 ms | 305 ms | 580 ms | 1417 ms (1) |
| 2 | 121 ms | 193 ms | 489 ms | 1353 ms (1) |
| 4 | 121 ms | 186 ms | 489 ms | 1372 ms (1) |
| 8 | 121 ms | 183 ms | 480 ms | 676 ms (2) |
| automatic (20) | 120 ms | 183 ms | 486 ms | 287 ms (5) |

The first picture hardly depends on the threads: it is limited by the entropy decoding of one file. The full resolution and the gallery use the cores.

### Damage per frame

Window 1100x700, 2.5 s of observation after the picture settled. The tiles are 64x64, so the numbers are slightly above the exact changed area.

| Scenario | frames per second | mean damage | max damage |
|---|---|---|---|
| still picture | 0 | 0 | 0 |
| slideshow progress bar (JPEG) | 3.6 | 7 936 px (1.03 %) | 33 024 px (4.29 %) |
| animated GIF 480x320, 12 frames, limited to 10 fps | 10 | 31 949 px (4.15 %) | 49 152 px (6.38 %) |

## Decisions

**One thread budget.** The setting "Decoding and rendering threads" (0 = automatic) now sets `par.Count()`, the number every part of the program asks: the scaling and rendering workers of `render` and `imaging`, the JPEG and TIFF decoders, the loader (`max(2, count/2)` files at a time), the tile workers (`count/2`, at most 3) and the thumbnail pool (`count/4`, at least 1). Automatic is `min(GOMAXPROCS, cores)`, never above 64. A change in Settings applies at once: the pools grow and shrink while the program runs. Before, the setting sized only the loader and needed a restart.

**Memory budget.** The cache setting is now one budget for the cached pictures (tiled pictures count their tile cache and window), the tile caches and the decodes in flight. Before a decode the loader reserves its estimated peak: width x height x 4 bytes for the picture, a third of that for the pyramid and a format-dependent scratch (JPEG 2 bytes per pixel, PNG and BMP 1, WebP, AVIF, HEIC, JPEG XL and RAW 12, TIFF 8; animations multiply the first term by the frames). The numbers come from allocation counts of the decoders at 6 Mpx; WebP allocates about 15 bytes per pixel in total, JPEG 5.8. The picture on screen is first: its job waits, ordered before every prefetch, until the reservation fits (least recently used entries that are not shown are dropped to make room), except that one job may always run, so a picture larger than the budget still opens. A prefetch that does not fit is skipped, and is queued again by the next navigation. Reservations are released when the result is in the cache or the job is cancelled.

**Prefetch policy.** The neighbours of the current picture start only after the current one is ready (they used to compete with it for the cores and the memory bandwidth), and are not prefetched at all while a tiled picture is shown: in the first measurement a 200 Mpx picture with two 24 Mpx neighbours needed 700 MB while panning (800 MB after zooming), against 315 and 357 MB without them.

**Garbage collector.** After the start-up the program sets the collector to 30 % (`GOGC=30`, unless the user set `GOGC`), instead of 100 %. The heap of a viewer is mostly large pictures without pointers, so a collection is cheap (the pauses in the runs above add up to 2 to 25 ms). The 3840x2160 pan allocates 33 MB per frame in the engine alone, and with 100 % the collector let the heap grow to twice the live data first. Measured on the 200 Mpx picture in a 1920x1080 window with fast panning, before the window and tile changes below: the peak was 570 MB at 100 %, 480 MB at 50 %, 425 MB at 30 % and the frame times did not change (pan p95 10.0 ms in every case); the first picture of the 24 Mpx file stayed at 120 ms. `debug.SetMemoryLimit` was evaluated and **not adopted**. A limit at twice the cache setting (2 GB with the default 1 GB) never triggers and changes nothing; limits that do cut the peak (480 MB gave 489 MB, 400 MB gave 410 MB in the same run) are below twice the budget, where a user who raised the cache setting would see the collector running all the time.

**Tiles and windows.** The window the renderer asks the tile manager for is a second copy of the tiles, so it is capped at 16 Mpx (it was 36 Mpx, 144 MB), and when the finest level does not fit the renderer tries the next coarser levels before it falls back to the held overview. The tile cache budget is 96 MB (it was 128 MB). Together with the collector this brought the 3840x2160 zoom peak from 904 MB to 576 MB, and the longest wait for sharp tiles from 490 to 206 ms.

**Preview hint.** The first preview is chosen for the window size from the first frame instead of the 1920x1080 default, so a 24 Mpx picture in a 1100x700 window is previewed at 1/4 (1500x1000) instead of 1/2. It saved 17 ms.

## Where the time goes, and what is outside the program

At 24 Mpx the first 34 ms are the operating system and the Go runtime starting the 29 MB program and the initialisation of the packages. 23 ms of that is the initialisation of the JPEG XL library (`github.com/gen2brain/jxl`, tables built at start; found with `GODEBUG=inittrace=1`); it cannot be deferred without a patched copy of the library. The next 21 ms are the engine: `engine.New` reads the system fallback fonts of Windows (about 22 MB for Segoe UI Symbol, Nirmala, Segoe UI Emoji and others) with a growing buffer, which allocates 53 MB of which 22 MB stay in memory. The preview decode starts at the open request and is finished 57 ms later; the loading frame (27 ms with the toolbar and the first glyphs) is drawn on the main thread at the same time, and drawing the picture takes 5 ms.

Pan at 3840x2160 takes 33 ms per frame, 30 frames per second. Of that, about 22 ms is the engine copying the picture buffer into its canvas with `image/draw` in `Over` mode on one core (the buffer is opaque, a plain copy would be several times faster), and about 20 ms of CPU, spread over the cores, is the engine's comparison of the old and new frame and the copy of the changed tiles (33 MB allocated for every frame). These are engine costs; PixGo's own rendering of the 4K frame takes about 13 ms of CPU, spread over the cores. At 1920x1080 the frame takes 8.5 ms.

## Limits of these numbers

- The window is not created: native window creation, the first presentation and the compositor are not in the timeline, so a real start is later by that amount. The 120 ms is "process creation to a frame with the picture is rendered by the engine".
- Warm start only: the program and the picture are in the file cache. A cold start after a reboot is not measured.
- The pictures are synthetic and baseline. A progressive JPEG keeps only the coefficients it needs at the chosen level (scale 8 needs about 1/64 of the memory), but no 200 Mpx progressive file was available, so its memory is not measured. A progressive JPEG larger than the pixel limit opens only at 1/8 detail.
- The pan is delivered inside the process through the canvas, not through the window system.
- One machine, 20 cores. With fewer cores the full resolution and the gallery are slower (see the thread table); the first picture is not.
- 0.5.0 numbers come from the v0.5.0 sources built with the same method, not from the released installer.
