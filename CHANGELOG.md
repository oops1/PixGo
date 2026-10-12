# Changelog

## [0.2.0] - 2026-10-12

First public release.

### Viewing
- Opens a file and pages through the rest of its folder; the neighbours are preloaded.
- Zoom around the cursor, fit to window or width, 100 %, panning by dragging, double click toggles fit and 100 %.
- Rotation, flips and automatic EXIF orientation.
- Touch gestures: pinch, pan, swipe, double tap. Mouse back and forward buttons page through the folder.
- Fullscreen mode (F11, Alt+Enter) with the controls and the cursor hiding after a pause.

### Formats
- JPEG, PNG, GIF (animated), BMP, TIFF, WebP, SVG, ICO and CUR, detected by their signature.
- Corrupt or oversized files are reported instead of crashing the viewer.

### Details and actions
- Information panel with the file, size, camera (EXIF), color profile (ICC) and GPS location.
- Copy the file, the image or the path, show the file in its folder, save as JPEG, PNG or BMP.
- Delete to the recycle bin with confirmation; favorites and recent files are remembered.
- Context menu, tooltips and notifications.

### Platforms
- Windows 10 and 11 and Linux (static binary, no dependencies), light and dark themes, English and Russian.
- Windows installer with an optional step that associates PixGo with image file types.
- Linux packages: `.deb`, `.rpm` and a tarball with an install script; `pixgo -set-default` makes PixGo the default viewer.
