# Changelog

## [0.3.0] - 2026-10-13

### Added
- Gallery. **Ctrl+O**, **Ctrl+G**, the gallery button of the toolbar and the back arrow of the viewer open a page that browses the folders for pictures; it replaces the Open dialog on every platform. PixGo with no file to show starts on it.
- Side panel with All images, Recent and Favorites, the known folders of the system (Desktop, Pictures, Videos, Documents, Downloads) and the drives.
- Grid of thumbnails with names. Subfolders come first and open with a double click. Thumbnails load in the background, the visible ones first, and are kept on disk, so a folder visited again fills at once.
- Address bar with history (back, forward, up): click it to type a path or paste the path of a file, which opens it.
- Filter by format, grid and list views, thumbnail sizes from 96 to 384 px; the choice is remembered.
- Keyboard: arrows, Home, End, Page Up and Down, Enter, Delete, Ctrl+D for favorites, Alt+arrows for history, F5, Ctrl+L, typing a name selects it.
- Context menu and the split **Open** button: open, open in a new window, show in folder, favorites, delete.
- Command line: `pixgo -gallery FOLDER` opens the gallery on a folder.

### Changed
- The side buttons of the mouse page through the history of the gallery while it is open.

## [0.2.2] - 2026-10-12

### Changed
- The mouse wheel now pages through the images of the folder. Ctrl + wheel zooms around the cursor. Settings saved by earlier versions move to the new behavior once; `"wheel": "zoom"` in `settings.json` brings the old behavior back.

### Fixed
- Opening an image from the file manager while PixGo is covered by other windows or minimized now brings the window to the front. On Wayland the compositor decides, so the window may only be highlighted there.

## [0.2.1] - 2026-10-12

### Added
- Update check. At most once every three days PixGo reads the release feed of the project on GitHub and, when a newer version exists, offers to open its download page. The context menu has **Check for updates** to run the check on demand.
- The automatic check can be turned off with `"updates": {"check": false}` in `settings.json`. Development builds never check by themselves.

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
