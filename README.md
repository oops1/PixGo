<p align="center"><img src="packaging/icons/hicolor/128x128/apps/pixgo.png" alt="PixGo" width="128" height="128"></p>

# PixGo

A fast, small image viewer for Windows and Linux with a Windows 11 look, written in Go on top of [headless-gui](https://github.com/oops1/headless-gui). No CGO, no WebView, no runtime dependencies.

PixGo follows the simplicity of the classic Windows Photo Viewer: the image fills the window, the folder is navigable at once, and the controls stay out of the way.

## Features

- Opens a file and lets you page through the rest of its folder with preloading of the neighbours. Very large JPEG and TIFF pictures (up to 1000 Mpx) open in parts and stay sharp when you zoom; a 200 Mpx JPEG needs about 130 MB when open.
- The mouse wheel pages through the folder; Ctrl + wheel zooms around the cursor. Fit to window or width, 100 %, pan by dragging, double click toggles fit and 100 %.
- Opening an image from the file manager brings the running window to the front, even when it is minimized (Windows and X11).
- Gallery (Ctrl+O): folders, favorites, recent files and drives on the side, a grid or list of thumbnails with a disk cache, address bar with history, filter by format.
- Full screen with a compact bar and a slideshow (F5): interval, looping, shuffling.
- Settings page (Ctrl+,) with live changes: theme, language, wheel, order of files, caches, default viewer.
- Rotation and flips, EXIF orientation applied automatically; Save as JPEG, PNG, BMP or WebP with a choice of quality, and lossless rotation of JPEG files.
- JPEG, PNG, APNG, GIF, BMP, TIFF, WebP (also animated), SVG, ICO and CUR, AVIF, JPEG XL, HEIC and the embedded preview of RAW files; animated files play. Details and limits: [docs/formats.md](docs/formats.md).
- Formats are detected by signature, not by extension.
- Information panel with EXIF, color profile and GPS.
- Dark and light themes, English and Russian interface; the text follows the size of the system font (`-font-size` overrides it); sharp on displays up to 200 % scale.
- Usable from the keyboard and with screen readers, see [docs/accessibility.md](docs/accessibility.md).
- Decoding never blocks the interface; a corrupt file never crashes the program. Measured speed and memory: [docs/perf.md](docs/perf.md).
- Tells you when a newer release is available, see [Updates](#updates).

## Install

Download the files from the [releases page](https://github.com/oops1/PixGo/releases).

### Windows

Run `pixgo-<version>-windows-x64-setup.exe`. The installer works without administrator rights and has an optional step, **Associate PixGo with image files**. Windows keeps the choice of the default program for the user, so the last page offers to open *Default apps* where PixGo is preselected. A portable `.zip` is available too.

### Linux

| Package | Install |
|---|---|
| Debian, Ubuntu | `sudo dpkg -i pixgo_<version>_amd64.deb` |
| Fedora, openSUSE | `sudo rpm -i pixgo-<version>-1.x86_64.rpm` |
| Any distribution | unpack `pixgo-<version>-linux-<arch>.tar.gz` and run `./install.sh` (to `~/.local`, or `--prefix DIR`) |

The packages register PixGo for image types, so it appears in *Open with*. To make it the default viewer run:

```bash
pixgo -set-default
```

It writes `~/.config/mimeapps.list`; no extra tools are needed. `install.sh --set-default` does the same during installation.

## Using PixGo

| You want to | Do this |
|---|---|
| Open a picture | Double-click it in the file manager, run `pixgo photo.jpg`, drop it onto the window, or press **Ctrl+O** to browse folders in the gallery |
| Go to the next or previous picture | The arrow keys, **Page Down** and **Page Up**, the mouse wheel, the round arrows at the sides of the window, a swipe on a touch screen, or the side buttons of the mouse |
| Zoom in or out | **+** and **-**, **Ctrl** + wheel, or the buttons of the bar at the bottom; **1** fits the window, **2** fits the width, **0** shows 100 %, a double click switches between fit and 100 % |
| Move a zoomed picture | Drag it with the mouse, or use **Ctrl** + arrow keys |
| Rotate or flip | **R** turns right, **Shift+R** left; **H** and **V** flip; Rotate 180° is in the menu |
| Crop, resize, or bake a rotation into the file | **Ctrl+E** (the pencil in the bar, or **Crop** and **Resize** in the menu). Drag the corners and edges of the frame, choose an aspect ratio (free, original, square, 4:3, 3:2, 16:9), rotate or flip, then **Save a copy** (writes `name (2).jpg` next to the original), or use the arrow for **Save** (replaces the original after a confirmation) and **Save as**. **Esc** cancels. Animations and SVG cannot be edited |
| Resize | In the edit mode press **Resize**: width and height in pixels or percent, proportions kept |
| Delete | **Delete** moves the file to the recycle bin after a confirmation |
| See the details | **Ctrl+I** or the information button in the title bar: file, camera, color profile, location |
| Go full screen or start a slideshow | **F11** (**Alt+Enter**) for full screen, **F5** for the slideshow, **Esc** to leave |
| Save, copy, show in the folder, add to favorites | The **⋯** button, a right click, or **Ctrl+Shift+S**, **Ctrl+C**, **Ctrl+Shift+R**, **Ctrl+D** |
| Change the settings | **Ctrl+,**; changes apply at once |

The whole list of keys, including the gallery and the settings, is in [docs/accessibility.md](docs/accessibility.md). What PixGo does not do is in [docs/limitations.md](docs/limitations.md).

## Command line

```
pixgo [options] [file...]
```

| Option | Meaning |
|---|---|
| `file...` | Open the first file and page through its folder; a running PixGo takes the file over instead of starting another window |
| `-new-window` | Do not hand the files to a running window |
| `-gallery DIR` | Open the gallery on a folder |
| `-settings` | Open the settings page |
| `-theme system\|light\|dark`, `-lang ru\|en` | Override the theme and the language for this run |
| `-font-size PT`, `-scale K` | Override the text size in points and the display scale (1, 1.25, 1.5, 1.75, 2) |
| `-set-default` | Make PixGo the default image viewer and exit (Linux; Windows opens *Default apps*) |
| `-version` | Print the version and exit |
| `-screenshot FILE [-width W -height H]` | Draw one picture of the window without showing it, then exit |
| `-perf FILE`, `-perf-gallery DIR` | Measure speed and memory, see [docs/perf.md](docs/perf.md) |

## Environment

| Variable | Meaning |
|---|---|
| `PIXGO_CONFIG_DIR`, `PIXGO_STATE_DIR`, `PIXGO_CACHE_DIR` | Folders of the settings, of the favorites, recent files and window position, and of the thumbnail cache; the defaults are listed under [Files](#files) |
| `PIXGO_NO_UPDATE_CHECK=1` | Never look for a newer release, for every user of the machine |
| `PIXGO_THREADS=N` | Number of threads for decoding, scaling and rendering when Settings says Automatic (1 to 64); useful where many programs share one machine |
| `PIXGO_FONT_PT=10.5` | Text size in points instead of the system font |
| `PIXGO_DESKTOP_ID=name.desktop` | The desktop file that `-set-default` registers, when the program is installed under another name |
| `PIXGO_OWN_DIALOGS=1` | Use the dialogs of PixGo instead of the native ones on Windows |
| `GOGC` | The garbage collector runs at 30 % after the start; set `GOGC` to choose another value |

## Files

| What | Windows | Linux |
|---|---|---|
| `settings.json` | `%APPDATA%\PixGo` | `~/.config/pixgo` |
| `favorites.json`, `recent.json`, `window.json`; `hang-<date>.txt` when the interface stopped answering | `%LOCALAPPDATA%\PixGo` | `~/.local/state/pixgo` |
| Thumbnails | `%LOCALAPPDATA%\PixGo\cache` | `~/.cache/pixgo` |

`settings.json` is plain JSON and can be edited by hand while PixGo is closed. Versions 1.x keep its keys and the options above working.

## Updates

Once every three days, a moment after start, PixGo reads the public release feed of this repository (`github.com/oops1/PixGo/releases.atom`) and asks whether to open the download page when a newer version exists. Nothing is downloaded or installed by the program itself, and nothing but the request for the feed is sent. **Check for updates** in the context menu runs the check at once.

To turn the automatic check off, set it in `settings.json`:

```json
{ "updates": { "check": false } }
```

or set the environment variable `PIXGO_NO_UPDATE_CHECK=1` to turn it off for every user of the machine; the menu entry then only says that checks are switched off.

## Build

Go 1.27 or newer.

```bash
CGO_ENABLED=0 go build -o pixgo ./cmd/pixgo
```

On Windows add `-ldflags="-H windowsgui"` to build without a console window. Packages are built by `scripts/package-linux.sh` and `packaging/windows/pixgo.iss` (Inno Setup).

## Test

```bash
go test ./...
```

## Layout

| Path | Purpose |
|---|---|
| `internal/codec` | decoder and encoder contracts, registry, one package per format |
| `internal/imaging` | decode queue with priorities and a memory budget, cache, resolution pyramid, tiles of big pictures |
| `internal/render` | viewport renderer |
| `internal/jpegdec`, `internal/jpegxform` | JPEG decoder with scaling and regions, lossless JPEG transform |
| `internal/par` | the shared thread budget |
| `internal/meta`, `internal/isobmff` | EXIF, GPS, color profile and XMP readers and writers (metadata carried into saved files); the ISO box reader for AVIF and HEIC |
| `internal/edit` | editing plan: orientation, crop and resize with a Lanczos-3 resampler |
| `internal/viewer` | folder, navigation, zoom and pan model, slideshow |
| `internal/gallery` | gallery model: places, history, listing, thumbnail grid math |
| `internal/thumbs` | thumbnail queue by visibility, memory and disk cache |
| `internal/app` | controller and command table |
| `internal/ui` | window, view widget, panels |
| `internal/platform` | trash, themes, file associations, single instance, per-OS code |
| `internal/settings`, `internal/i18n`, `internal/updates` | settings file, Russian and English texts, the release check |
| `packaging` | Windows installer, Linux desktop file, icons |

## License

Apache License 2.0, see `LICENSE` and `NOTICE`. Third-party licenses are in `third_party/`.
