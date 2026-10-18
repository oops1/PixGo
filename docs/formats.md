# Image formats

PixGo recognizes a file by its signature, not by its extension. The extension only breaks a tie when two decoders claim the same bytes (for example a TIFF-based RAW file against a plain TIFF).

| Format | Extensions | Read | Write | Animation | Alpha | Build tag to remove it |
|---|---|---|---|---|---|---|
| JPEG | `.jpg` `.jpeg` `.jpe` `.jfif` | yes, also in parts above the pixel limit | yes, quality 1–100, or lossless rotation and crop of a JPEG | no | no | – |
| PNG | `.png` | yes | yes | no | yes | – |
| APNG | `.apng`, animated `.png` | yes | no | yes | yes | `noapng` |
| GIF | `.gif` | yes | no | yes | yes | – |
| BMP | `.bmp` `.dib` | yes | yes | no | yes | – |
| TIFF | `.tif` `.tiff` | yes (also BigTIFF), also in parts above the pixel limit | no | no | yes | – |
| WebP | `.webp` | yes, lossy and lossless | yes, lossy with quality 1–100 or lossless | yes | yes | – |
| SVG | `.svg` `.svgz` | yes, drawn once, up to 1024 pixels on the long side | no | no | yes | – |
| ICO, CUR | `.ico` `.cur` | yes, the largest picture of the file | no | no | yes | – |
| AVIF | `.avif` | yes, 8 to 12 bits, image sequences | no | yes | yes | `noavif` |
| JPEG XL | `.jxl` | yes, lossless and lossy, animation | no | yes | yes | `nojxl` |
| HEIC, HEIF | `.heic` `.heif` `.hif` | yes, including tiled photos of phones | no | sequences | yes | `noheic` |
| RAW | `.cr2` `.cr3` `.nef` `.arw` `.dng` `.orf` `.rw2` `.pef` `.raf` | DNG: developed from the sensor data; the others: embedded preview only | no | no | no | `noraw` |

"Read" means that the format is shown by the viewer, the gallery and the thumbnails. "Write" is what **Save as** offers.

An optional format (APNG, AVIF, JPEG XL, HEIC, RAW) can also be switched off at run time in **Settings → Formats**; files of a switched-off format disappear from the folder list at once.

## RAW and DNG

- **DNG is developed.** A DNG file is built from the data of the sensor, not from the JPEG preview. The window does not say "Preview" for it, the information panel says "DNG", and the size is the size of the picture after the crop of the file (`DefaultCropOrigin`, `DefaultCropSize`, inside the `ActiveArea`). **Settings → Formats → Develop DNG files** switches the development off; the preview stored in the file is shown then. The default is on.
- **Other RAW formats** (CR2, CR3, NEF, ARW, ORF, RW2, PEF, RAF) show the largest embedded JPEG preview, marked "Preview". Their sensor data is not read.
- **What is read.** The raw image can be in IFD0 or in a SubIFD. Color filter array (`PhotometricInterpretation` 32803) with a 2 by 2 Bayer pattern (RGGB, BGGR, GRBG, GBRG) or linear raw (34892) with three samples. 8 to 16 bits per sample. Strips and tiles. Compression: none (also packed 10, 12 and 14 bits), lossless JPEG (ITU T.81 process 14, predictors 1 to 7, restart intervals, one to four components per row, so also the DNG layout of two components per row), Deflate (`Compression` 8 and 32946) with no predictor, the horizontal predictor 2 and the integer predictors 34892 and 34893.
- **What is applied.** `LinearizationTable`, `BlackLevel` (with `BlackLevelRepeatDim`, `BlackLevelDeltaH` and `BlackLevelDeltaV`), `WhiteLevel`, `ActiveArea`, the crop, `AsShotNeutral` or `AsShotWhiteXY`, `ColorMatrix1` and `ColorMatrix2` with `CalibrationIlluminant1` and `CalibrationIlluminant2` (interpolated by the color temperature of the white balance, as in the DNG specification), `CameraCalibration1` and `CameraCalibration2`, `AnalogBalance`, `ForwardMatrix1` and `ForwardMatrix2`, `BaselineExposure` (from -4 to +4 EV) and `Orientation`.
- **Pipeline.** Linearization, black and white level, white balance, demosaicing with the Malvar-He-Cutler gradient-corrected filter (five by five pixels), camera color to XYZ with the D50 white, Bradford adaptation to linear sRGB, exposure, a highlight shoulder, the sRGB transfer curve. The shoulder is the identity up to 0.75 of the white and rolls the values above it off so that the clipped white of the sensor ends at pure white, also after a positive exposure. The result is a picture in sRGB; with color management on, the viewer keeps its 16 bits until the conversion to the display.
- **First picture.** For the first, reduced picture the mosaic is binned instead of demosaiced (one Bayer quad becomes one pixel at the scale 2, a plain average at 4 and 8), which is much faster than the full development. The full development follows.
- **Not applied.** Lens corrections, gain maps and the other opcode lists, `DefaultScale` (non-square pixels), the noise profile, camera profiles (`ProfileHueSatMap` and the tone curves), and the highlight reconstruction beyond the clipping to white. The colors are those of the matrices of the file; they are not the rendering of any other raw converter.
- **When a DNG file is shown as a preview anyway.** Floating point samples, the floating point predictors 34894 and 34895, lossy JPEG compression (34892), planar layouts, four-color or other than 2 by 2 Bayer patterns (X-Trans, Quad Bayer), staggered layouts, a file without `ColorMatrix1`, and any file in which the sensor data cannot be read to the end. A file that has no usable preview then fails with a message. A DNG file larger than the pixel limit is shown as its preview too.

## Color

Color management is on by default (**Settings, View, Color management**). With it off, every picture is shown as 1.1 showed it: the numbers are read as sRGB and reduced to 8 bits.

- **What is applied.** The color description of the file: an embedded ICC profile (JPEG, PNG, APNG, WebP, TIFF, AVIF, HEIC, JPEG XL) or the CICP code points of the file (PNG `cICP`, the `nclx` box of AVIF and HEIC, the color encoding of JPEG XL). When both are present the ICC profile wins, as in browsers; a profile that cannot be read falls back to the CICP description, and then to sRGB. A plain sRGB description (primaries 1, transfer 13, or an sRGB profile) is the identity: the output is byte-identical to the input.
- **ICC profiles.** Version 2 and 4. RGB matrix and curve profiles (`rXYZ`, `gXYZ`, `bXYZ`, `rTRC`, `gTRC`, `bTRC`; curves as `curv` gamma or table and `para` types 0 to 4), gray profiles (`kTRC`), and LUT profiles (`A2B0` to `A2B2`, `B2A0` to `B2A2` as `mft1`, `mft2`, `mAB ` and `mBA ` with curves, matrix and CLUT, PCS XYZ or Lab). Every offset and size is checked; a profile of a class other than display, input, output or color space, or of a color space other than RGB, gray and CMYK, is ignored. The default intent is relative colorimetric with black point compensation (it uses `A2B1`, else the matrix, else `A2B0`); perceptual uses `A2B0` when the profile has it. The D50 PCS and the `chad` tag are read; the colorants are used as stored.
- **CICP.** Primaries BT.709 (1), BT.470 M (4), BT.601 (5, 6), SMPTE 240M (7), generic film (8), BT.2020 (9), DCI P3 (11), Display P3 (12), EBU 3213 (22). Transfers BT.709 / BT.601 (1, 6, 7, 14, 15, inverse of the OETF), gamma 2.2 (4), 2.8 (5), 2.6 (17), linear (8), sRGB (13), PQ (16) and HLG (18). The matrix coefficients are applied by the decoders. A code point that is not in the list leaves the picture as sRGB.
- **The conversion.** For matrix profiles on both sides: source curve, 3 by 3 matrix through the D50 PCS (the white points are adapted with the Bradford matrix), inverse curve of the display, all through tables; colors outside the display gamut are clipped per channel. For LUT profiles: a 33 by 33 by 33 table with tetrahedral interpolation is built from the exact pipeline (17 to the 4th for CMYK). Pictures with more than 8 bits (PNG and TIFF with 16 bits, AVIF, HEIC and JPEG XL with 10, 12 or 16 bits) are decoded to 16 bits per channel, converted to the display in one pass and only then reduced to 8 bits, with an ordered 4 by 4 dither so smooth gradients do not band. Tiled pictures convert the overview and every tile with the same transform; animations convert every frame; thumbnails use the same conversion, and their cache files are kept apart from the unmanaged ones.
- **Display profile.** The destination is sRGB, or with **Display profile: System** the profile of the monitor the window is on (Windows: `GetICMProfileW` of that monitor; it is looked up again at most once per second, so moving the window to another monitor changes the colors of the next picture and a changed profile reloads the pictures). A profile that cannot be read, a profile that is not an RGB profile, and every other system fall back to sRGB; PixGo does not talk to colord on Linux.
- **HDR (PQ and HLG).** Pictures with a PQ or HLG transfer are mapped to the SDR display. PQ is decoded to nits; HLG goes through the BT.2100 OOTF for a 1000-nit display (system gamma 1.2). The luminance is then compressed on the largest channel, so the hue is kept: values up to the **reference white of 203 nits** are unchanged (203 nits is SDR white, PQ code 0.5806 and HLG 75 %), and the range from 203 nits to the 1000 nits assumed for the source is rolled into 203 to 300 nits, the assumed peak of an SDR display, with the ITU-R BT.2390 spline (the knee is placed at the reference white and the first slope is limited so the curve stays monotone). The result is divided by 203 nits, converted from the primaries of the picture to those of the display, and colors outside the display gamut are brought back by desaturating them toward their luminance instead of clipping each channel. Highlights above the reference white still end at white on an 8-bit display. The picture is not tone mapped by its content: the same curve is used for every picture.
- **CMYK JPEG.** A CMYK or YCCK JPEG (it must have the Adobe marker, as before) with an embedded CMYK ICC profile is converted with that profile. With color management on, the decoder does not convert the four channels itself: it returns them as ink values, 255 meaning full ink, exactly what the standard library puts into `image.CMYK` (the Adobe inversion is undone, YCCK is turned into CMY first), and the color stage converts them with the `A2B` table of the profile (a 17 by 17 by 17 by 17 table with tetrahedral interpolation, relative colorimetric with black point compensation) straight to the display profile. The reduced decodes that are shown first for big pictures (1/2, 1/4, 1/8), thumbnails, and the tiles of CMYK JPEG files above the pixel limit use the same conversion; above 1/8 the tile reader averages the ink channels before converting. Without a profile, with a profile that is not CMYK or has no `A2B` table, and with color management off, the simple conversion of the decoder (each of C, M, Y scaled by the black channel) is used, as in 1.2. While a picture is converted, the ink samples and the result are in memory together (8 bytes per pixel instead of 4); the loader reserves this. The CMYK profile is replaced by the display profile when the picture is saved, like any other converted profile.
- **JPEG XL above white.** The library returns decoded samples as floating point numbers and `DecodeFloat` does not clamp them, so PixGo reads those instead of the 16-bit pictures of `DecodeAll` (which clip to 0..1) when color management is on and the file is HDR: a VarDCT or XYB file with a PQ, HLG or linear transfer, or a modular file with 32-bit floating point samples and a linear, sRGB, BT.709 or gamma transfer, with no animation, no embedded profile and no spot colors. The library does not apply the PQ and HLG curves to XYB data, it returns linear light in units of the `intensity_target` of the file, so PixGo multiplies by that target to get nits (PQ and HLG files: the target is the luminance of 1.0; HLG is not given its OOTF), and re-encodes the result as PQ in 16 bits with the primaries of the file (transfer 16 is reported to the color stage, which tone maps it like any PQ picture). For linear and the other relative transfers 1.0 is the SDR white: with the default target of 255 nits 1.0 becomes the 203-nit reference white (the scale is `target * 203 / 255`), so SDR data looks the same as before and values above 1.0 become highlights above the reference white that the tone mapping rolls off instead of clipping. A linear file that names a target above 255 nits is taken literally (1.0 is that many nits). Other JPEG XL files, and every file with color management off, are decoded as in 1.2.
- **Information panel.** The color card shows the profile with its origin ("Display P3 (embedded)", "(CICP)"), "HDR: PQ, tone mapped", the bit depth of pictures that were converted from more than 8 bits, and the display profile when it is not sRGB.
- **Speed.** On the development machine (20 threads) a 24-megapixel picture is converted in 21 ms from Display P3 to sRGB (8 bits, in place), 23 ms from 16 bits, 40 ms from 16-bit LUT profile, 42 ms from PQ and 77 ms from HLG; building a transform takes 1 to 2 ms.
- **Saving.** A picture whose pixels were converted is saved without the embedded ICC profile of the source, because the saved pixels are the displayed sRGB ones. A lossless JPEG copy keeps the original bytes and the profile.
- **Not done.** `gAMA` and `cHRM` chunks of PNG are not used. Absolute colorimetric and saturation intents, named color profiles and device link profiles are not supported. The gain map and the depth map of phone photos are ignored; the base picture is shown. HDR JPEG XL animations and files with spot color channels keep the clamped 16-bit result of the decoding library.

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
- **RAW.** The maker notes are not read, so the lens and the settings that only the maker stores are missing; the lens is shown when the standard EXIF tag has it. The size of the sensor is not read: the size of a RAW file is the size of its preview, except for a developed DNG, where it is the size of the developed picture. The format is told from the first bytes of the file, not from the extension.
- A block that is cut off or damaged is skipped; what was read before it is still shown.

## Metadata when saving

Pictures that PixGo encodes again (Save as, and every save from the edit mode) keep their metadata. The blocks are taken from the file on disk, not from the decoded picture, and written to the new file in the form that its format uses.

| Target | EXIF | ICC profile | XMP |
|---|---|---|---|
| JPEG | `APP1` `Exif` segment | `APP2` `ICC_PROFILE` segments of up to 65 519 bytes each | `APP1` segment with the XMP namespace |
| PNG | `eXIf` chunk | `iCCP` chunk, zlib-compressed | `iTXt` chunk with the keyword `XML:com.adobe.xmp` |
| WebP | `EXIF` chunk | `ICCP` chunk | `XMP ` chunk |
| BMP | no | no | no |

A WebP file with metadata gets the extended header (`VP8X`) with the flags for the alpha channel, the profile, the EXIF block and the XMP packet; a file without metadata is written in the plain form as before. The blocks are placed in the order the format asks for: header, profile, picture, EXIF, XMP.

- **What is kept.** All camera tags, the maker note, the lens, the date, the color profile and the XMP description are copied byte for byte, except the entries listed below. Metadata is read from JPEG, PNG, APNG, WebP, TIFF, HEIC, AVIF, JPEG XL and RAW; for TIFF and RAW only the descriptive tags are taken (make, model, orientation, software, date, artist, copyright, resolution), the Exif and GPS directories, and not the tags that describe the strips and tiles of the sensor data or the maker note.
- **What is changed.** The orientation is set to 1 (the saved picture is already turned), `ImageWidth`, `ImageLength`, `PixelXDimension` and `PixelYDimension` become the new size, but only the entries that the original had. In the XMP packet `tiff:Orientation`, `tiff:ImageWidth`, `tiff:ImageLength`, `exif:PixelXDimension` and `exif:PixelYDimension` change in the same way. The preview picture inside the EXIF block is removed, because it shows the old picture. The EXIF structure is written again with all offsets recalculated; a maker note that has no header of its own (Canon and others, whose inner offsets count from the start of the EXIF block) stays at the same offset in the block so that its inner references remain valid.
- **GPS.** The option **Remove location information (GPS)** in the save options removes the GPS directory from the EXIF block and the `exif:GPS*` properties from the XMP packet. Nothing else is touched. For a lossless copy the option works the same way.
- **What does not fit.** A JPEG segment holds up to 65 533 bytes. When the EXIF block of a picture is larger, the maker note is dropped first; when the block still does not fit, or when it cannot be read, it is left out of the file and PixGo tells you about it after saving. An XMP packet above the limit is left out too (extended XMP is not written); a profile above 16.7 MB is left out. The picture itself is always saved.

## Orientation

- EXIF orientation is applied by the viewer after decoding (the setting **Apply EXIF orientation** in Settings → View). The decoder only reports it.
- HEIF and AVIF use their own `irot` and `imir` boxes; the EXIF orientation of the same file is ignored so that a picture is never turned twice.
- JPEG XL applies its orientation inside the decoder.

## Lossless rotation and crop

When the picture on the screen comes from a JPEG file and you save it as JPEG, the **Save options** dialog has the switch **Lossless** (on by default). With the switch on, PixGo does not decode and encode the picture again: it takes the stored DCT coefficients of the original file, moves and mirrors the blocks the way you turned the picture (this includes the EXIF orientation that the viewer applied), keeps only the blocks of the crop rectangle when there is one, and writes them out. A resize cannot be lossless: with a new size the switch is not offered and the picture is encoded again with the chosen quality. Every coefficient of the original is kept, so the picture suffers no new compression loss (a decoder may differ from the original in the last digit of a few samples, because its rounding is not symmetric). The quality slider is off in this mode. The choice is remembered in the setting `saveJpegLossless` of `settings.json`.

- **What is written.** A baseline file with Huffman tables that are optimized for the new order of the blocks, so the file is usually a little smaller; restart markers are dropped. All APPn markers (JFIF, EXIF, ICC profile, XMP, Adobe) and comments are kept in their order.
- **Crop.** The crop rectangle is given in the pixels of the picture as it looks after the turn. Only whole blocks can be cut out, so the top-left corner moves left and up to the nearest unit of the output (8 by 8 pixels, 16 by 16 and so on, the same units as below, with width and height swapped when the picture is turned by 90 degrees), and the bottom-right corner stays where it is, so the result always covers the area you chose and is at most 15 pixels (31 for 4:1:1 and 4:1:0 files) larger on the left and on the top. This is what `jpegtran -crop` does. The edit mode shows the snapped rectangle before you save; the program asks for it with `jpegxform.CropFor`. A rectangle that is partly outside the picture is clipped to it; a rectangle that is completely outside is refused. Cutting out a part of a picture needs no whole units on an edge that is not in the crop: the partial unit at the edge described below only matters when the crop reaches into it.
- **What is fixed in the metadata.** EXIF orientation becomes 1, the pixel dimensions (`PixelXDimension`, `PixelYDimension`, `ImageWidth`, `ImageLength`) follow the new size (the size of the crop), `XResolution` and `YResolution` swap places when the picture is turned by 90 degrees, and `tiff:Orientation` of the XMP packet becomes 1. The small preview picture inside the EXIF block would show the old picture, so it is removed whenever the picture, its orientation or its size changes (for a crop or for the option that removes the location, the EXIF block and the XMP packet are written again as described in *Metadata when saving*). The multi-picture block (`MPF`) is removed too, because the extra pictures behind the main one are not copied.
- **Which files.** Baseline and extended sequential JPEG with 8-bit samples: gray, YCbCr with any subsampling, CMYK and YCCK. Progressive, arithmetic-coded, 12-bit, lossless files and files that code the components in several scans are not changed this way: the switch is not offered for them and the picture is saved the usual way, with the chosen quality.
- **Edges.** A JPEG is coded in blocks of 8 by 8 pixels; with chroma subsampling the units are 16 by 16 pixels (up to 32 by 16). A turn or a mirror can only move whole units. When the size of the picture is not a multiple of the unit on the edge that moves to the other side, that partial unit is cut off, as `jpegtran -trim` does: less than 16 pixels (less than 32 for 4:1:1 and 4:1:0 files), and PixGo tells you in the notice after saving. With a crop, nothing is cut off when the rectangle stays clear of that partial unit. The rule for each operation:

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
- **RAW.** Only DNG files are developed (see the section RAW and DNG, which also lists what is not applied). The other formats show only the JPEG preview that is stored in the file, the largest one, and the window says "Preview". A DNG file that cannot be developed, or when the development is switched off, is shown as its preview; one without a JPEG preview then cannot be shown. The preview of Canon CR3 is looked for in the first 16 MiB of the data.
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
