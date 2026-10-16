# Known limitations

This page lists what PixGo does not do, or does only in part, as of 1.0.0. Each item says what happens instead. Details of the formats are in [formats.md](formats.md), the numbers of speed and memory in [perf.md](perf.md), the keyboard and screen reader notes in [accessibility.md](accessibility.md).

## Color

- Embedded ICC profiles, wide gamuts (Display P3, Rec. 2020) and HDR curves (PQ, HLG) are not applied. A picture is shown with its numbers read as sRGB, so some colors differ slightly from the original.
- Pictures with 10, 12 or 16 bits per channel are reduced to 8 bits.
- The gain map and the depth map of phone photos are ignored; the base picture is shown.

## Formats

- RAW files show the JPEG preview stored in the file, marked "Preview". The sensor data is not developed. A DNG file without a JPEG preview cannot be shown.
- HEIC: overlays, derived images and the `mini` brand are refused as unsupported. HEIC needs the HEVC codec, which is covered by patents; PixGo grants no license to them. Build with the tag `noheic` or switch HEIC off in Settings → Formats if this matters to you.
- AVIF: an animated file that has no `meta` box is refused as unsupported.
- JPEG XL: an animation with an embedded ICC profile or a permuted table of contents is refused; the color profile of a still picture and metadata compressed with Brotli are not read.
- Animations of WebP, GIF, APNG, AVIF and JPEG XL play, but are never faster than the frame rate set in Settings → Performance and pause while the window is not active.
- Save as writes JPEG, PNG, BMP and WebP. AVIF, JPEG XL, HEIC, TIFF and animated files cannot be written.
- Saving a picture in a new encoding does not carry over the camera data (EXIF), the color profile or the XMP packet. Only the lossless JPEG copy keeps them.
- SVG is drawn once, up to 1024 pixels on the long side, and then scaled like any other picture, so it becomes soft when zoomed far beyond that. Save as writes that drawing.

## Large pictures

- JPEG and TIFF pictures above the decoding limit open in parts, up to 1000 million pixels. Other formats above the limit are refused with a message.
- A progressive JPEG above the limit opens at 1/8 of its size only: reading a region of such a file would need every coefficient in memory.
- A TIFF that is planar, compressed with CCITT or JPEG, or stored as CMYK, Lab, YCbCr or 32-bit float is refused above the limit; below it the older reader decodes it whole.
- A picture that is shown in parts, or at reduced size while the full picture is still loading, cannot be saved as a pixel copy or copied as an image. A lossless JPEG copy can still be saved.
- Panning a 200 Mpx picture in a 3840 by 2160 window runs at about 30 frames per second; most of the time of a frame is spent in the graphics engine.

## Editing

- PixGo views and converts; it does not crop, resize or change colors.
- Lossless rotation works for baseline JPEG files only. When the size of the picture is not a multiple of the block (8 to 32 pixels, depending on the chroma subsampling), the partial edge is cut off and a notice says so. Progressive, arithmetic-coded and 12-bit files are saved the usual way, re-encoded with the chosen quality.

## Windows and Linux

- Opening a file from the file manager while PixGo runs brings the window forward. On Wayland the compositor decides; without the `xdg_activation_v1` protocol the window cannot be raised, and a minimized window is not detected.
- A file can be dropped onto the window, but not dragged out of it.
- On Windows only files on drives that have a recycle bin go there. For other places (removable and network drives, very long paths) PixGo asks whether to delete the file permanently.
- The portable archive for Windows does not register itself in Default apps; the installer does (its optional step **Associate PixGo with image files**).
- The list of files in the folder is read when a picture is opened and again when the window becomes active, not continuously.
- The window opens with the size it had last time, but where the system places it: its position and the maximized state are saved and not yet restored, because the window cannot be moved before it is shown without a visible jump.
- Copy image needs a clipboard that carries pictures. Where it carries only text and lists of files, use Copy file or Copy path.
- Printing is not provided.
- The update check reads the public release feed of the project on GitHub and nothing else. It can be turned off in Settings or for every user of a machine with the environment variable `PIXGO_NO_UPDATE_CHECK=1`.
- macOS is not supported.

## Screen readers

- A screen reader can read every element, but cannot press or focus a single control on the pages that PixGo draws itself; the request goes to the page as a whole. Use the keyboard. The full list is in [accessibility.md](accessibility.md).

## Display scale

- PixGo is per-monitor DPI aware on Windows and draws at the real pixel size at 100 % to 200 %. Moving the window between monitors of different scale is handled by the graphics engine; it has not been tried on every combination of displays.

## Files and settings

- `settings.json` has a schema version. Keys that a version does not know are kept when the file is rewritten, so an older and a newer version can share one file.
- The thumbnail cache is kept on disk and limited by Settings → Performance; clearing it is safe.
