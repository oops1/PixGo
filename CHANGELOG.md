# Changelog

## [1.3.0] - 2026-10-18

### Added
- **Printing.** **Print…** (Ctrl+P) in the **⋯** menu and the right click menu opens one print dialog on Windows and Linux: the printer or **Save as PDF**, copies, paper (A4, A5, A3, Letter, Legal, 10 × 15 cm, 13 × 18 cm), orientation (automatic follows the picture), placement (fit to the page, fill the page, actual size from the resolution stored in the file, or a custom size in cm or inches), margins, and a live preview of the page with warnings for a low resolution or cut edges. On Windows **System dialog…** opens the printer driver's options. The page is built at 300 dpi from the full-resolution picture; a picture shown in parts is read at the level the page needs. An edited picture prints as edited, an animation as its current frame, and SVG is drawn again at the size of the page. The dialog remembers its last choices.
- **Export to PDF…** in the same menus saves the picture as a one-page PDF with the same layout.
- **CMYK JPEG files are color managed.** A CMYK or YCCK JPEG with an embedded CMYK profile is converted with that profile, also in the first reduced picture, the thumbnails and the parts of huge files.
- **JPEG XL highlights above white.** HDR JPEG XL files (PQ, HLG or linear, and 32-bit floating point) are read without clipping, so their highlights are tone mapped like those of other HDR pictures instead of ending at white.

### Fixed
- PQ and HLG JPEG XL files coded in XYB were taken as already PQ or HLG coded while the decoding library returns linear light, so they were shown too dark.

### Notes
- Printing was exercised with fake printers and PDF output; it has not been tried on a real printer. On Linux it needs CUPS; without it only **Save as PDF** works. The papers are a fixed list; the sizes a printer reports itself are not read.
- Animated HDR JPEG XL files and files with spot colors keep the clipped result of the decoding library.

## [1.2.0] - 2026-10-18

### Added
- **Color management.** Embedded ICC profiles (version 2 and 4, matrix and table based) and the color descriptions of AVIF, HEIC, JPEG XL and PNG (CICP) are applied: Display P3, Adobe RGB, Rec. 2020 and other wide-gamut pictures show their real colors instead of oversaturated or dull ones. On Windows the picture is converted to the profile of the monitor the window is on; on Linux to sRGB. Settings → View has **Color management** and **Display profile** (system or sRGB); turned off, PixGo behaves as before.
- **HDR pictures** (PQ and HLG in AVIF, HEIC, JPEG XL and PNG) are tone mapped to the screen with the BT.2390 curve, keeping the hue, and colors outside the screen's gamut are compressed softly instead of clipped.
- **High bit depth.** 16-bit PNG and TIFF and 10 to 16-bit AVIF, HEIC and JPEG XL keep their precision until the final conversion, with a light dither, so gradients stay smooth.
- **DNG development.** DNG files are developed from the sensor data: lossless JPEG, Deflate and uncompressed data, the four Bayer patterns and linear DNG, black and white levels, linearization, white balance, the color matrices of the camera and the exposure, then a soft highlight curve. A reduced picture appears first, the full development follows (about 50 ms for 24 Mpx on a fast machine). Settings → Formats can switch back to the embedded preview. Other RAW formats still show their embedded preview.
- **Twelve languages.** English, Russian, Ukrainian, German, French, Spanish, Italian, Portuguese (Brazil), Polish, Czech, Dutch and Turkish, with grammatical plurals and the date, number and file size formats of each language. The language of the system is used by default (on Linux also the language chosen for the desktop session, before the locale); Settings → Language lists every language under its own name.
- The information panel shows the color profile and where it came from, HDR, the bit depth and the display profile.
- `-ldflags "-X main.updateCheck=off"` builds a program without the update check, for distributions that update it themselves.

### Changed
- A saved copy of a converted picture carries the profile it was converted to (none for sRGB).
- `-theme` and `-lang` on the command line apply to that run only and are no longer written into `settings.json`.

### Notes
- CMYK JPEG files are still converted without their color profile, and JPEG XL HDR values above the reference white are clipped by the decoder library; both are planned for 1.3.
- The DNG development was tested with generated files; vendor-specific extras (opcode lists, camera profiles, floating-point data, X-Trans) are not applied, and such files show their preview.

## [1.1.0] - 2026-10-18

### Added
- **Editing.** **Edit** on the toolbar, Ctrl+E or the menu (**Crop…**, **Resize…**) opens the edit mode: a crop frame with eight handles and rule-of-thirds lines, the aspect ratios Free, Original, Square, 4:3, 3:2 and 16:9 (portrait or landscape), rotation and flips, **Reset**, and the size of the result. Arrows move the frame, Shift moves it faster, Ctrl+arrows change its size, Enter saves a copy, Esc cancels.
- **Resize** dialog: width and height in pixels or percent, with or without keeping the proportions. Pictures are scaled with a Lanczos filter in premultiplied alpha, so edges of transparent pictures stay clean and shrinking does not alias.
- **Save copy** writes the edited picture next to the original as `name (2).ext`; **Save as…** chooses another place or format; **Save** replaces the original after a confirmation (it can be turned off) and writes it safely through a temporary file.
- **Lossless crop of JPEG files.** A crop, a turn and a flip of a baseline JPEG are saved without recompression when **Lossless** is on; the frame then snaps to the 8 or 16 pixel blocks of the file, and the snapped frame is shown while the dialog is open.
- **Metadata is kept when saving.** Every saved JPEG, PNG and WebP keeps the camera data (EXIF), the color profile and the XMP description of the original; the orientation becomes normal, the stored sizes follow the new size and the small preview inside EXIF is removed. The save dialog has **Remove location data (GPS)**.
- Screen readers can press, toggle, select and focus every control of the viewer, the gallery, the information panel, the settings and the dialogs, not only read them.

### Changed
- Graphics engine updated to v3.38.0.
- The window opens where it was closed, maximized if it was, without a visible jump.
- Panning large pictures in big windows is several times faster: the picture buffer goes to the screen by the opaque fast path of the engine, and the engine starts faster because it no longer reads every fallback font at start.
- The context menu is announced to screen readers as a real menu with its items.

### Notes
- Animated pictures and SVG cannot be edited yet.
- A picture that is shown in parts can be cropped when the cropped area fits the pixel limit, and resized from a reduced level.

## [1.0.1] - 2026-10-16

### Fixed
- The program froze when the focus left the address bar of the gallery while it was being edited: for example when a picture was opened from the gallery after a click into the address, or when the window became active with the address selected. The window stayed on the screen but no longer redrew.
- The **Open** button of the gallery and its arrow stand inside the status bar, in its middle, instead of over its top line.
- The name of the single-instance socket follows the program name set when building (`-X main.appID=name`), like the window class and the desktop file.

### Added
- When the interface does not answer for 8 seconds, PixGo writes the stacks of all its threads to `hang-<date>.txt` in its state folder (`%LOCALAPPDATA%\PixGo` on Windows, `~/.local/state/pixgo` on Linux), at most five files per run. Attach the file when you report a freeze.

## [1.0.0] - 2026-10-16

First stable release. Within 1.x the command line, the environment variables and the keys of `settings.json` keep working.

### Added
- New application icon, on a transparent background, for the window, the taskbar, the Windows installer and the Linux menus.
- **Rotate 180°** in the menu, next to rotate left and right.
- **Fit to window**, **Fit to width** and **100 %** in the menu, so every zoom mode has a visible control.
- The settings open from the gallery too: Ctrl+, and a menu on the empty space of the grid and the side panel (right click or the Menu key), which also has **Refresh**.
- The `+` key works as it is typed on every layout (Shift and `=`, with or without Ctrl), and the digits 1 and 2 of the numeric keypad fit the picture to the window and to the width.
- The list of files of the folder is read again when the window becomes active, at most every two seconds, so files added or removed by other programs appear without reopening.
- `PIXGO_NO_UPDATE_CHECK=1` turns the update check off for every user of a machine; `PIXGO_THREADS=N` sets the number of threads when Settings says Automatic. The program name used for the window class, the single-instance socket and the desktop file can be set when building (`-ldflags "-X main.appID=name"`).
- README: how to use the program, the command line, the environment variables and the files it keeps. `docs/limitations.md` lists what PixGo does not do.

### Changed
- The first picture no longer waits for the folder: the file is requested at once and the folder is listed in the background. Until the list is read, the previous and next buttons are disabled.
- **Restore last image** works on its own; it used to be ignored when **Remember window size** was off.
- The window opens with its last size clamped to the monitor it appears on.
- The unused setting `slideshow.transition` was removed; files that still have it keep working.

### Fixed
- Opening a picture from the file manager while the settings were shown loaded it behind the settings page; the page now closes and the picture appears.
- The SVG and ICO lines of `docs/formats.md` described the program wrongly: SVG is drawn once, up to 1024 pixels on the long side, and ICO shows its largest picture.

### Notes
- The position of the window and the maximized state are saved but not restored yet: the window would visibly jump after it is shown.
- The fuzz tests now also cover the PNG and SVG decoders; all 32 targets ran for ten minutes without a failure.

## [0.6.0] - 2026-10-16

### Added
- **Very large pictures.** A JPEG or TIFF that is larger than the decoding limit opens in parts instead of being refused: the whole picture appears at once at reduced size, and the detail is decoded for the visible area as you zoom and pan, up to 1000 Mpx. A 200 Mpx JPEG opens in about half a second, uses under 400 MB while you pan it at 1080p, and stays sharp at 100 % and above. Rotation and flips work on such pictures too. Other formats above the limit are still refused with a message (see `docs/formats.md`).
- **Faster first picture.** A big JPEG shows a reduced copy as soon as it is read, with a small progress mark while the full resolution is finished: a 24 Mpx photo is on the screen 120 ms after the process starts (it was 237 ms), full resolution after 183 ms.
- **Lossless JPEG rotation.** Saving a JPEG as JPEG offers the switch **Lossless** in the Save options dialog. With it the picture is not decoded and encoded again: the blocks of the original file are moved and mirrored the way you turned the picture, the EXIF orientation becomes 1 and the rest of the metadata is kept. It also works for pictures that are shown in parts. When the size is not a multiple of the block, the partial edge is cut and a notice says so. Details and limits are in `docs/formats.md`.
- **Accessibility.** Every element of the viewer, the gallery, the information panel, the settings and the dialogs has a role, a name in the interface language and its states for screen readers (UI Automation on Windows, AT-SPI on Linux). Everything is reachable from the keyboard: Tab and Shift+Tab visit the controls, Enter and Space press them, arrows move inside the gallery, lists and settings, Esc steps back, Ctrl+arrows pan a zoomed picture, Shift+F10 or the Menu key opens the menu. A focus ring appears while the keyboard is used, with an inner line in the high-contrast theme. The full list of keys is in `docs/accessibility.md`.
- **High-resolution displays.** Icons, borders, thumbnails and the focus ring are drawn at the physical size of the screen at 125 %, 150 %, 175 % and 200 %, so they stay sharp and aligned. The Windows executable declares per-monitor DPI awareness. `pixgo -screenshot … -scale 1.5` renders a window at a given scale.
- **Measurements.** `pixgo -perf FILE` runs the program on the offscreen engine and reports the timeline from the creation of the process, the frame times of panning and zooming, the memory, the redrawn area per frame and the gallery speed. `docs/perf.md` has the method, the numbers of this release against 0.5.0 and the limits of the measurement.

### Changed
- **New JPEG decoder** written for PixGo (a fork of the standard library decoder): it decodes at 1/2, 1/4 and 1/8 size with the reduced inverse DCT, reads regions through a small index of the entropy stream, and uses several cores. A 24 Mpx file decodes in 67 ms instead of 175 ms, and thumbnails of big JPEGs in the gallery use the reduced decoding: 24 thumbnails of 24 Mpx pictures take 0.3 s instead of 1.1 s, and thumbnails of 200 Mpx pictures work.
- **New TIFF reader** for strips and tiles (classic and BigTIFF; none, LZW, Deflate and PackBits; 1 to 16 bits): 3 to 6 times faster than before, and a TIFF is read by region without loading it whole. Layouts it does not handle still go through the old reader.
- The cache size in Settings → Performance is now one memory budget for the cached pictures, the tiles of big pictures and the decodes in progress. A picture waits until its decode fits; neighbours are prefetched only after the current picture is ready, and not next to a picture shown in parts.
- **Decoding and rendering threads** (Settings → Performance) now applies at once and to everything that works in parallel: decoding, scaling, rendering, thumbnails. It used to size only the loader and needed a restart.
- **Pixels per picture** (Settings → Performance) is now the limit for decoding a picture whole; larger JPEG and TIFF pictures open in parts.
- Save as is not offered as a pixel copy for a picture that is shown in parts or at reduced size; only the lossless JPEG copy is possible then, and a message says why. Copy image says so too.
- After the start the garbage collector runs at 30 % (unless `GOGC` is set), which lowers the peak memory of big pictures without changing the speed.

### Fixed
- The **More** buttons were always drawn as disabled.
- Esc could not close a message box on Linux.
- The window grew by the scale factor at every start on displays above 100 %.
- A file that is open as a picture in parts no longer blocks deleting or renaming it on Windows.

### Notes
- Progressive JPEG files above the decoding limit open at 1/8 detail only. Region reading of a progressive file is not possible without keeping every coefficient in memory.
- Panning a 200 Mpx picture in a 4K window runs at about 30 frames per second; most of the frame time is in the engine (see `docs/perf.md`).
- Not verified on real hardware: the switch of a window between monitors with different scales, a screen reader reading the pages, real photographs of 100 Mpx and more (the tests use generated pictures), and the Linux run of the measurements.

## [0.5.0] - 2026-10-14

### Added
- New formats: **AVIF**, **JPEG XL**, **HEIC/HEIF** (photos of phones with tiled images and rotation), **APNG** (animated PNG) and the embedded preview of **RAW** files (CR2, CR3, NEF, ARW, DNG, ORF, RW2, PEF, RAF). The preview of a RAW file is marked with the badge "Preview". The formats are recognized by their signature, shown in the gallery with thumbnails, and registered in the "Open with" lists of Windows and Linux.
- WebP is now read with a new decoder: lossy, lossless, transparency and **animation** (it was a still picture only). **Save as** writes WebP, lossy with a quality of 1 to 100 or lossless.
- The dialog **Save options** appears before a JPEG or WebP is written: quality 1 to 100 and the switch "Lossless"; the last choice is remembered.
- Settings → Formats lists every format, lets you switch the optional ones (APNG, AVIF, JPEG XL, HEIC, RAW) off and on at once without a restart, and explains the patents of HEIC.
- The information panel shows the EXIF, GPS, color profile and XMP of HEIC, AVIF, JPEG XL and RAW files, as far as each format stores them (the table is in `docs/formats.md`; the profile of JPEG XL is not read yet).
- `docs/formats.md`: the table of formats, the limits and the known restrictions.

### Changed
- Every format can be left out of the build with a tag (`noapng`, `noavif`, `nojxl`, `noheic`, `noraw`); with all of them the program is 2.8 MiB smaller.
- The parsers written in PixGo have fuzz tests; `scripts/fuzz-all.sh` runs every target.
- The licenses and patent grants of the new libraries are listed in `NOTICE` and shipped in `third_party/`.
- The list of releases in the Linux metadata is complete.

### Notes
- Color profiles, wide gamut (Display P3) and HDR curves of AVIF, HEIC and JPEG XL are not applied yet; the picture is shown as sRGB. See `docs/formats.md`.

## [0.4.2] - 2026-10-14

### Changed
- Graphics engine updated to v3.35.1.
- Opening an image from the file manager while PixGo runs brings its window to the front and restores it if it is minimized, now through the window activation of the engine. On Wayland and X11 the activation token of the launcher is passed to the running window, so the compositor lets it come forward; before, only Windows was covered.

## [0.4.1] - 2026-10-13

### Changed
- The text of the interface follows the size of the system font: 9 pt (Segoe UI) on Windows, including the "Text size" setting of the accessibility options, and the font of the GTK settings on Linux (10 pt when there are none). It used to be about 20 % smaller. The gallery, the settings, the information panel, the title, the slideshow bar, tooltips and notifications adapt their layout to the size, so nothing is cut or overlapped.
- A different size can be forced with `pixgo -font-size 10.5` or the environment variable `PIXGO_FONT_PT`.

## [0.4.0] - 2026-10-13

### Added
- Full screen (F11, Alt+Enter, Esc to leave). The picture fills the screen; a compact bar with play, the position, the progress to the next picture, the slideshow menu and the exit button appears when the mouse moves and hides after 2.5 seconds together with the cursor.
- Slideshow: F5 starts it (entering full screen if needed), Space pauses and resumes, the arrows and the wheel switch pictures by hand and restart the countdown. The gear on the bar sets the interval (1, 3, 5, 10 or 30 seconds), looping and shuffling; shuffling shows every picture once per round. It ends on the last picture when looping is off.
- Settings page (Ctrl+, or the context menu) with the sections General, View, Formats, Slideshow, Performance, Language and About. Changes apply at once, no restart: theme, language, background of transparent images, mouse wheel, order of files, cache size and the rest. The Formats section lists the supported formats and sets PixGo as the default viewer (Windows opens Default apps), Performance clears the thumbnail cache, About shows the version, the engine, the licenses and checks for updates.
- Smooth zoom (View section); it is off while the system asks for reduced animation.
- Animations of GIF and WebP pause while the window is not active.

### Changed
- Licenses of every component that ships in the program (Open Sans, golang.org/x/sys, golang.org/x/text, go-text/typesetting) are now listed in `NOTICE` and kept in `third_party/`.
- Removed the unused option `pixelGridAbove400` from `settings.json`; the file keeps working, the key is ignored.

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
