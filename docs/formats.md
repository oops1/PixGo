# Image formats

PixGo recognizes a file by its signature, not by its extension. The extension only breaks a tie when two decoders claim the same bytes (for example a TIFF-based RAW file against a plain TIFF).

| Format | Extensions | Read | Write | Animation | Alpha | Build tag to remove it |
|---|---|---|---|---|---|---|
| JPEG | `.jpg` `.jpeg` `.jpe` `.jfif` | yes | yes, quality 1–100 | no | no | – |
| PNG | `.png` | yes | yes | no | yes | – |
| APNG | `.apng`, animated `.png` | yes | no | yes | yes | `noapng` |
| GIF | `.gif` | yes | no | yes | yes | – |
| BMP | `.bmp` `.dib` | yes | yes | no | yes | – |
| TIFF | `.tif` `.tiff` | yes | no | no | yes | – |
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

## Animation

- All frames are composed on the canvas of the file with the disposal and blending rules of the format.
- A delay of 10 ms or less is shown as 100 ms for WebP and as 10 ms for APNG, like web browsers do. The frame rate never goes above **Animation frames per second** in Settings → Performance, and animations stop while the window is not active.
- The number of repeats stored in the file is respected; 0 means forever.

## Limits

Every decoder checks the size of the file and the number of pixels **before** it allocates memory, so a tiny file that claims a huge picture is refused.

| Limit | Default | Where to change |
|---|---|---|
| Pixels per picture | 100 million | Settings → Performance |
| Size of a file | 512 MB | – |
| Frames of an animation | 10 000 | – |
| Pixels of a single frame | 400 million | – |

Files above a limit are reported with a message and never crash the viewer.

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
- The parsers that are written in PixGo (APNG, RAW, BMP, ICO, GIF, the container of WebP, the ISO box reader for AVIF and HEIC, EXIF) have fuzz tests. `scripts/fuzz-all.sh 10m` runs every target for ten minutes.
