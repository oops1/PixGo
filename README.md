# PixGo

A fast, small image viewer for Windows and Linux with a Windows 11 look, written in Go on top of [headless-gui](https://github.com/oops1/headless-gui). No CGO, no WebView, no runtime dependencies.

PixGo follows the simplicity of the classic Windows Photo Viewer: the image fills the window, the folder is navigable at once, and the controls stay out of the way.

## Features

- Opens a file and lets you page through the rest of its folder with preloading of the neighbours.
- Zoom around the cursor, fit to window or width, 100 %, pan by dragging, double click toggles fit and 100 %.
- Rotation and flips, EXIF orientation applied automatically.
- JPEG, PNG, GIF (animated), BMP, TIFF, WebP, SVG, ICO and CUR; AVIF, JPEG XL, HEIC and RAW previews are planned.
- Formats are detected by signature, not by extension.
- Dark and light themes, English and Russian interface.
- Decoding never blocks the interface; a corrupt file never crashes the program.

## Build

Go 1.27 or newer.

```bash
CGO_ENABLED=0 go build -o pixgo ./cmd/pixgo
```

On Windows add `-ldflags="-H windowsgui"` to build without a console window.

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
| `internal/app` | controller and command table |
| `internal/ui` | window, view widget, panels |
| `internal/platform` | trash, clipboard, themes, single instance, per-OS code |

## License

Apache License 2.0, see `LICENSE` and `NOTICE`. Third-party licenses are in `third_party/`.
