# Known limitations

This page lists what PixGo does not do, or does only in part, as of 1.1.0. Each item says what happens instead. Details of the formats are in [formats.md](formats.md), the numbers of speed and memory in [perf.md](perf.md), the keyboard and screen reader notes in [accessibility.md](accessibility.md).

## Color

- Color management applies embedded ICC profiles and CICP descriptions and tone maps PQ and HLG pictures to the SDR display (see [formats.md](formats.md#color)). It is not a calibrated pipeline: highlights above the 203-nit reference white of an HDR picture end at white, the same curve is used for every picture, and the display stays 8-bit SDR.
- Only sRGB, or the profile of the monitor the window is on (Windows), is the destination. On Linux, and when the monitor profile cannot be read, it is sRGB. A saved copy of a converted picture carries the profile it was converted to: none for sRGB, the monitor profile otherwise (a lossless JPEG copy keeps the original file and its profile).
- CMYK profiles of CMYK JPEG files are not used; those pictures keep the simple conversion of the decoder. JPEG XL values above 1.0 are clamped. `gAMA` and `cHRM` of PNG are ignored. Only relative colorimetric (with black point compensation) and perceptual intents exist.
- With **Color management** off, pictures are shown as in 1.1: numbers read as sRGB and reduced to 8 bits.
- The gain map and the depth map of phone photos are ignored; the base picture is shown.

## Formats

- DNG files are developed from the sensor data (2 by 2 Bayer and linear raw, lossless JPEG, Deflate and uncompressed data); lens corrections, gain maps and camera profiles are not applied, so the colors are not those of other raw converters. DNG files with floating point samples, X-Trans or Quad Bayer sensors, or lossy compression show their embedded preview. Other RAW formats (CR2, CR3, NEF, ARW, ORF, RW2, PEF, RAF) show the JPEG preview stored in the file, marked "Preview"; their sensor data is not developed. A file whose preview is needed but absent cannot be shown.
- HEIC: overlays, derived images and the `mini` brand are refused as unsupported. HEIC needs the HEVC codec, which is covered by patents; PixGo grants no license to them. Build with the tag `noheic` or switch HEIC off in Settings → Formats if this matters to you.
- AVIF: an animated file that has no `meta` box is refused as unsupported.
- JPEG XL: an animation with an embedded ICC profile or a permuted table of contents is refused; the color profile of a still picture and metadata compressed with Brotli are not read.
- Animations of WebP, GIF, APNG, AVIF and JPEG XL play, but are never faster than the frame rate set in Settings → Performance and pause while the window is not active.
- Save as writes JPEG, PNG, BMP and WebP. AVIF, JPEG XL, HEIC, TIFF and animated files cannot be written.
- Saving carries the camera data (EXIF), the color profile and the XMP packet into JPEG, PNG and WebP files; BMP has no place for them. A block that does not fit the container (a JPEG segment holds 64 KB) is left out and a notice says so. The thumbnail inside the EXIF is removed, and the orientation is set to normal because the pixels are already turned.
- SVG is drawn once, up to 1024 pixels on the long side, and then scaled like any other picture, so it becomes soft when zoomed far beyond that. Save as writes that drawing.

## Large pictures

- JPEG and TIFF pictures above the decoding limit open in parts, up to 1000 million pixels. Other formats above the limit are refused with a message.
- A progressive JPEG above the limit opens at 1/8 of its size only: reading a region of such a file would need every coefficient in memory.
- A TIFF that is planar, compressed with CCITT or JPEG, or stored as CMYK, Lab, YCbCr or 32-bit float is refused above the limit; below it the older reader decodes it whole.
- A picture that is shown in parts, or at reduced size while the full picture is still loading, cannot be saved as a pixel copy or copied as an image. A lossless JPEG copy can still be saved.

## Editing

- PixGo crops, resizes, rotates and flips; it does not change colors, retouch or draw.
- Animated pictures and SVG cannot be edited. A picture that is still loading cannot be edited either.
- A picture shown in parts (above the decoding limit) can be cropped when the chosen area is not larger than the pixel limit, or when it is resized; the area is read from the file at full detail, and a resize reads the best level that fits. A larger area without a resize is refused with a message; a lossless JPEG crop has no such limit.
- Formats that PixGo cannot write (AVIF, JPEG XL, HEIC, TIFF, GIF, RAW, ICO) are saved as PNG when you press **Save a copy**; **Save** (replace the original) is off for them.
- Editing the picture while another picture is opened (for example from the file manager) drops the unsaved changes without asking.
- Lossless rotation and crop work for baseline JPEG files only. A crop moves its top-left corner to the grid of blocks (up to 15 pixels, 31 for some subsampling), so the lossless result can be slightly larger than the frame; the dashed frame in the editor shows the real one. When the size of the picture is not a multiple of the block (8 to 32 pixels, depending on the chroma subsampling), the partial edge is cut off and a notice says so. Progressive, arithmetic-coded and 12-bit files are saved the usual way, re-encoded with the chosen quality.

## Windows and Linux

- Opening a file from the file manager while PixGo runs brings the window forward. On Wayland the compositor decides; without the `xdg_activation_v1` protocol the window cannot be raised, and a minimized window is not detected.
- A file can be dropped onto the window, but not dragged out of it.
- On Windows only files on drives that have a recycle bin go there. For other places (removable and network drives, very long paths) PixGo asks whether to delete the file permanently.
- The portable archive for Windows does not register itself in Default apps; the installer does (its optional step **Associate PixGo with image files**).
- The list of files in the folder is read when a picture is opened and again when the window becomes active, not continuously.
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
