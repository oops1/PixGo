# PixGo

A fast, small image viewer for Windows and Linux with a Windows 11 look, written in Go on top of [headless-gui](https://github.com/oops1/headless-gui). No CGO, no WebView, no runtime dependencies.

PixGo follows the simplicity of the classic Windows Photo Viewer: the image fills the window, the folder is navigable at once, and the controls stay out of the way.

## Features

- Opens a file and lets you page through the rest of its folder with preloading of the neighbours.
- The mouse wheel pages through the folder; Ctrl + wheel zooms around the cursor. Fit to window or width, 100 %, pan by dragging, double click toggles fit and 100 %.
- Opening an image from the file manager brings the running window to the front, even when it is minimized (Windows and X11).
- Gallery (Ctrl+O): folders, favorites, recent files and drives on the side, a grid or list of thumbnails with a disk cache, address bar with history, filter by format.
- Full screen with a compact bar and a slideshow (F5): interval, looping, shuffling.
- Settings page (Ctrl+,) with live changes: theme, language, wheel, order of files, caches, default viewer.
- Rotation and flips, EXIF orientation applied automatically.
- JPEG, PNG, GIF (animated), BMP, TIFF, WebP, SVG, ICO and CUR; AVIF, JPEG XL, HEIC and RAW previews are planned.
- Formats are detected by signature, not by extension.
- Information panel with EXIF, color profile and GPS.
- Dark and light themes, English and Russian interface.
- Decoding never blocks the interface; a corrupt file never crashes the program.
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

## Updates

Once every three days, a moment after start, PixGo reads the public release feed of this repository (`github.com/oops1/PixGo/releases.atom`) and asks whether to open the download page when a newer version exists. Nothing is downloaded or installed by the program itself, and nothing but the request for the feed is sent. **Check for updates** in the context menu runs the check at once.

To turn the automatic check off, set it in `settings.json` (`%APPDATA%\PixGo` on Windows, `~/.config/pixgo` on Linux):

```json
{ "updates": { "check": false } }
```

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
| `internal/imaging` | decode queue with priorities, cache, resolution pyramid |
| `internal/render` | viewport renderer |
| `internal/viewer` | folder, navigation, zoom and pan model, slideshow |
| `internal/gallery` | gallery model: places, history, listing, thumbnail grid math |
| `internal/thumbs` | thumbnail queue by visibility, memory and disk cache |
| `internal/app` | controller and command table |
| `internal/ui` | window, view widget, panels |
| `internal/platform` | trash, themes, file associations, single instance, per-OS code |
| `packaging` | Windows installer, Linux desktop file, icons |

## License

Apache License 2.0, see `LICENSE` and `NOTICE`. Third-party licenses are in `third_party/`.
