# Image formats

PixGo recognizes a file by its signature, not by its extension. The extension only breaks a tie when two decoders claim the same bytes (for example a TIFF-based RAW file against a plain TIFF).

| Format | Extensions | Read | Write | Animation | Alpha | Build tag to remove it |
|---|---|---|---|---|---|---|
| JPEG | `.jpg` `.jpeg` `.jpe` `.jfif` | yes, also in parts above the pixel limit | yes, quality 1–100, or lossless rotation of a JPEG | no | no | – |
| PNG | `.png` | yes | yes | no | yes | – |
| APNG | `.apng`, animated `.png` | yes | no | yes | yes | `noapng` |
| GIF | `.gif` | yes | no | yes | yes | – |
| BMP | `.bmp` `.dib` | yes | yes | no | yes | – |
| TIFF | `.tif` `.tiff` | yes (also BigTIFF), also in parts above the pixel limit | no | no | yes | – |
| WebP | `.webp` | yes, lossy and lossless | yes, lossy with quality 1–100 or lossless | yes | yes | – |
| SVG | `.svg` `.svgz` | yes, drawn at the size of the window | no | no | yes | – |
| ICO, CUR | `.ico` `.cur` | yes, the size that fits the window best | no | no | yes | – |
| AVIF | `.avif` | yes, 8 to 12 bits, image sequences | no | yes | yes | `noavif` |
| JPEG XL | `.jxl` | yes, lossless and lossy, animation | no | yes | yes | `nojxl` |
| HEIC, HEIF | `.heic` `.heif` `.hif` | yes, including tiled photos of phones | no | sequences | yes | `noheic` |
| RAW | `.cr2` `.cr3` `.nef` `.arw` `.dng` `.orf` `.rw2` `.pef` `.raf` | embedded preview only | no | no | no | `noraw` |

"Read" means that the format is shown by the viewer, the gallery and the thumbnails. "Write" is what **Save as** offers.

An optional format (APNG, AVIF, JPEG XL, HEIC, RAW) can also be switched off at run time in **Settings → Formats**; files of a switched-off format disappear from the folder list at once.

## Color

- Pixels are shown as 8-bit sRGB. Files with 10, 12 or 16 bits per channel are reduced to 8 bits.
- Embedded ICC profiles, wide gamuts (Display P3, Rec. 2020) and HDR transfer curves (PQ, HLG) are **not** applied yet. Such pictures are shown with their numbers read as sRGB, so colors can look slightly different from the original. Color management is planned for 1.2.
- The gain map and the depth map of phone photos are ignored; the base picture is shown.

## Metadata

The information panel reads the camera data (EXIF), the GPS position, the color profile (ICC) and the XMP description from the file itself, after the picture is shown. The table says which of these blocks PixGo reads in each format, when the file stores them.

| Format | EXIF and GPS | ICC profile | XMP |
|---|---|---|---|
| JPEG | yes | yes | yes |
| PNG, APNG | yes (`eXIf`) | yes (`iCCP`) | yes (`iTXt`) |
| WebP | yes | yes | yes |
| TIFF | yes | yes | yes |
| AVIF, HEIC, HEIF | yes (the `Exif` item) | yes (the `colr` box of the type `prof` or `rICC`) | yes (the `mime` item `application/rdf+xml`) |
| JPEG XL | yes (the `Exif` box) | no | yes (the `xml ` box) |
| RAW: CR2, NEF, ARW, DNG, ORF, RW2, PEF | yes (IFD0, Exif IFD, GPS IFD) | only if the file keeps it in IFD0 (some DNG) | only if the file keeps it in IFD0 (most DNG) |
| RAW: CR3 | yes (the boxes `CMT1`, `CMT2`, `CMT4`) | no | yes (the `uuid` box of the XMP) |
| RAW: RAF | yes (from the JPEG preview of the file) | if the preview has it | if the preview has it |
| GIF, BMP, ICO, CUR, SVG | no | no | no |

- **AVIF, HEIC.** The profile is the one of the main picture, or of the first tile of a tiled picture. A file that names no main picture shows no profile. When a file holds several `Exif` or XMP items, the one that describes the main picture is used.
- **JPEG XL.** The profile is part of the coded picture and is not read. A metadata box that is compressed with Brotli (`brob`) is not read either, so such a file shows no EXIF or XMP. A plain codestream without the container has no metadata at all.
- **RAW.** The maker notes are not read, so the lens and the settings that only the maker stores are missing; the lens is shown when the standard EXIF tag has it. The size of the sensor is not read: the size of the file is the size of the preview. The format is told from the first bytes of the file, not from the extension.
- A block that is cut off or damaged is skipped; what was read before it is still shown.

## Orientation

- EXIF orientation is applied by the viewer after decoding (the setting **Apply EXIF orientation** in Settings → View). The decoder only reports it.
- HEIF and AVIF use their own `irot` and `imir` boxes; the EXIF orientation of the same file is ignored so that a picture is never turned twice.
- JPEG XL applies its orientation inside the decoder.

## Lossless rotation

When the picture on the screen comes from a JPEG file and you save it as JPEG, the **Save options** dialog has the switch **Lossless** (on by default). With the switch on, PixGo does not decode and encode the picture again: it takes the stored DCT coefficients of the original file, moves and mirrors the blocks the way you turned the picture (this includes the EXIF orientation that the viewer applied), and writes them out. Every coefficient of the original is kept, so the picture suffers no new compression loss (a decoder may differ from the original in the last digit of a few samples, because its rounding is not symmetric). The quality slider is off in this mode. The choice is remembered in the setting `saveJpegLossless` of `settings.json`.

- **What is written.** A baseline file with Huffman tables that are optimized for the new order of the blocks, so the file is usually a little smaller; restart markers are dropped. All APPn markers (JFIF, EXIF, ICC profile, XMP, Adobe) and comments are kept in their order.
- **What is fixed in the metadata.** EXIF orientation becomes 1, the pixel dimensions (`PixelXDimension`, `PixelYDimension`, `ImageWidth`, `ImageLength`) follow the new size, `XResolution` and `YResolution` swap places when the picture is turned by 90 degrees, and `tiff:Orientation` of the XMP packet becomes 1. The small preview picture inside the EXIF block would show the old position, so it is removed whenever the picture or its orientation changes. The multi-picture block (`MPF`) is removed too, because the extra pictures behind the main one are not copied.
- **Which files.** Baseline and extended sequential JPEG with 8-bit samples: gray, YCbCr with any subsampling, CMYK and YCCK. Progressive, arithmetic-coded, 12-bit, lossless files and files that code the components in several scans are not changed this way: the switch is not offered for them and the picture is saved the usual way, with the chosen quality.
- **Edges.** A JPEG is coded in blocks of 8 by 8 pixels; with chroma subsampling the units are 16 by 16 pixels (up to 32 by 16). A turn or a mirror can only move whole units. When the size of the picture is not a multiple of the unit on the edge that moves to the other side, that partial unit is cut off, as `jpegtran -trim` does: less than 16 pixels (less than 32 for 4:1:1 and 4:1:0 files), and PixGo tells you in the notice after saving. The rule for each operation:

| Operation | Width must be a multiple of the unit | Height must be a multiple of the unit |
|---|---|---|
| Mirror left to right | yes | no |
| Mirror top to bottom | no | yes |
| Turn right by 90 degrees | no | yes |
| Turn by 180 degrees | yes | yes |
| Turn left by 90 degrees | yes | no |
| Transpose | no | no |
| Transverse | yes | yes |

- **Memory.** The coefficients of the whole picture are kept in memory while the file is written, about 3 bytes per pixel for 4:2:0 and 6 bytes for 4:4:4 (a 100 Mpx photo needs roughly 300 MB). A picture above 268 Mpx is saved the usual way.
- **When it falls back.** If the original file was changed or removed after it was opened, or cannot be read in this way, PixGo saves the picture that is on the screen with the quality of the dialog, without a message.

## Animation

- All frames are composed on the canvas of the file with the disposal and blending rules of the format.
- A delay of 10 ms or less is shown as 100 ms for WebP and as 10 ms for APNG, like web browsers do. The frame rate never goes above **Animation frames per second** in Settings → Performance, and animations stop while the window is not active.
- The number of repeats stored in the file is respected; 0 means forever.

## Limits

Every decoder checks the size of the file and the number of pixels **before** it allocates memory, so a tiny file that claims a huge picture is refused.

| Limit | Default | Where to change |
|---|---|---|
| Pixels per picture that is decoded whole | 100 million, and not more than 1/16 of the cache size in pixels (64 million with the default cache of 1 GB) | Settings → Performance |
| Pixels per picture that is shown in parts (JPEG, TIFF) | 1 000 million | – |
| Size of a file | 512 MB | – |
| Frames of an animation | 10 000 | – |
| Pixels of a single frame | 400 million | – |

Files above a limit are reported with a message and never crash the viewer.

## Large pictures

A JPEG or a TIFF that has more pixels than may be decoded whole is not refused. PixGo reads it in parts:

- The whole picture is decoded once at 1/4 or 1/8 size (at most 16 million pixels) and shown at once. This overview stays in memory.
- When the zoom needs more detail than the overview has, the visible area is decoded in tiles of 256 by 256 pixels and drawn over the overview. While a tile is not ready, the overview shows through; panning back finds the tiles in a cache.
- JPEG: the entropy stream is indexed during the overview pass, so a tile is decoded without reading the rest of the picture (about 0.5 ms for a tile of 24 Mpx files). Only baseline (sequential) JPEG can be read by region. A **progressive** JPEG has to keep every coefficient for that, so above the limit it opens at 1/8 detail only; below the limit it is decoded whole as before.
- TIFF: only the strips or tiles that cover the area are read and decompressed. Layouts that the TIFF reader of PixGo does not handle (planar, CCITT, JPEG inside TIFF, CMYK, Lab, YCbCr, 32-bit float) are refused above the limit.
- Memory: the cache size of Settings → Performance is one budget for the cached pictures, the tiles of big pictures and the decodes that are running. A 200 Mpx JPEG needs about 130 MB when it is open and 300 to 400 MB while you pan it in a 1080p window ([perf.md](perf.md)).
- Other formats (PNG, GIF, BMP, WebP, AVIF, JPEG XL, HEIC, RAW previews, SVG) above the limit are refused with a message.
- Saving: a picture that is shown in parts or at reduced size cannot be saved as a pixel copy. The lossless JPEG copy (see above) works from the file, so a rotated copy of a huge JPEG can still be saved.
- The gallery makes thumbnails of big JPEG and TIFF files from the reduced decoding.

## HEIC and patents

HEIC and HEIF use the HEVC (H.265) video codec, which is covered by patents of several patent pools and of individual owners. PixGo contains a software HEVC decoder under the MIT license but does not grant any license to these patents. If you distribute PixGo or use it commercially, make sure that you have the rights you need. Support for HEIC can be left out of the build with the tag `noheic` or switched off in Settings → Formats.

AV1 (AVIF), WebP (VP8) and JPEG XL come with patent grants of their authors; the texts are in `third_party/` and ship with every package.

## What each decoder does not do

- **AVIF.** An animated file that has no `meta` box (some encoders write only `moov`) is refused as unsupported.
- **JPEG XL.** An animation that carries an embedded ICC profile or a permuted table of contents is refused as unsupported. The level 10 profile is refused. To show the first frame of an animation, all frames are decoded.
- **HEIC.** Overlays (`iovl`), derived images (`iden`) and the `mini` brand are refused as unsupported.
- **RAW.** Only the JPEG preview that is stored in the file is shown, the largest one. The picture of the sensor is not developed, and the window says "Preview". A DNG file without a JPEG preview cannot be shown. The preview of Canon CR3 is looked for in the first 16 MiB of the data.
- **WebP.** A lossless animation made by some encoders has no alpha flag in its header; the picture is still decoded with its alpha, only the information panel says that there is none.

## Build options

```bash
CGO_ENABLED=0 go build -tags "noapng noavif nojxl noheic noraw" ./cmd/pixgo
```

- Every tag removes the decoder and the third-party code it needs from the program. With `noavif noheic noraw` the binary is about 2.8 MiB smaller (AVIF 1.7, HEIC 0.6, JPEG XL 0.5).
- `-tags noasm` builds the codec libraries in pure Go on every platform, without the assembly of amd64 and arm64.

## Safety

- The decoders of the third-party libraries (WebP, AVIF, JPEG XL, HEIC) are wrapped so that a panic in the calling goroutine becomes "corrupt file". The libraries start helper goroutines for big pictures; PixGo keeps them single-threaded by default, because a panic in a helper goroutine could not be caught and would end the program.
- The parsers and decoders that are written in PixGo (the JPEG decoder and the lossless JPEG transform, the TIFF reader, APNG, RAW, BMP, ICO, GIF, the container of WebP, the ISO box reader for AVIF and HEIC, EXIF, the tile manager) have fuzz tests. `scripts/fuzz-all.sh 10m` runs every target for ten minutes.
