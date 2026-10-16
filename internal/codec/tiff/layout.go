package tiff

import (
	"encoding/binary"
	"fmt"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

const (
	compressionNone       = 1
	compressionLZW        = 5
	compressionDeflate    = 8
	compressionDeflateOld = 32946
	compressionPackBits   = 32773

	photometricWhiteIsZero = 0
	photometricBlackIsZero = 1
	photometricRGB         = 2
	photometricPalette     = 3

	predictorNone       = 1
	predictorHorizontal = 2
)

var maxBandBytes = 2 << 20

const (
	maxChunks       = 1 << 20
	maxTileSide     = 1 << 16
	maxStripWidth   = 1 << 21
	maxPaletteWords = 3 * 256
	maxDimension    = 1<<31 - 1
	tileAlignment   = 16
	nativeShift     = 3
)

type pixelMode uint8

const (
	modeIndexed pixelMode = iota + 1
	modeGray16
	modeRGB8
	modeRGB16
	modeRGBA8
	modeRGBA16
)

type layout struct {
	width, height  int
	tiled          bool
	chunkW, chunkH int
	across, down   int
	compression    int
	predictor      bool
	bits, samples  int
	mode           pixelMode
	invert         bool
	associated     bool
	littleEndian   bool
	rowBytes       int
	lut            [256][4]uint8
	lutLen         int
	info           codec.Info
	offsets        []uint64
	counts         []uint64
}

func unsupportedf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

func classify(f *file) (*layout, string, error) {
	lay := &layout{littleEndian: f.order == binary.LittleEndian}
	width, ok, err := f.first(tagImageWidth)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", guard.Corrupt("tiff: no ImageWidth")
	}
	height, ok, err := f.first(tagImageLength)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", guard.Corrupt("tiff: no ImageLength")
	}
	if width == 0 || height == 0 {
		return nil, "", guard.Corrupt("tiff: zero-size image")
	}
	if width > maxDimension || height > maxDimension {
		return nil, "", guard.TooLarge("tiff: %dx%d pixels", width, height)
	}
	lay.width, lay.height = int(width), int(height)
	lay.info = codec.Info{
		Format:      codec.TIFF,
		FormatName:  "TIFF",
		Width:       lay.width,
		Height:      lay.height,
		Frames:      1,
		Orientation: codec.OrientNormal,
	}
	if e, present := f.entries[tagOrientation]; present && e.typ == typeShort {
		if values, err := f.uints(e, 1); err == nil && len(values) == 1 && values[0] >= 1 && values[0] <= 8 {
			lay.info.Orientation = codec.Orientation(values[0])
		}
	}
	why, err := classifyPixels(f, lay)
	if err != nil || why != "" {
		return lay, why, err
	}
	why, err = classifyGeometry(f, lay)
	return lay, why, err
}

func classifyPixels(f *file, lay *layout) (string, error) {
	bits := []uint64{1}
	if e, ok := f.entries[tagBitsPerSample]; ok {
		if e.count == 0 || e.count > 16 {
			return unsupportedf("%d samples per pixel", e.count), nil
		}
		values, err := f.uints(e, 16)
		if err != nil {
			return "", err
		}
		bits = values
	}
	lay.samples = len(bits)
	lay.bits = int(bits[0])
	total := 0
	for _, b := range bits {
		total += int(b)
	}
	lay.info.BitDepth = total
	if declared, ok, err := f.first(tagSamplesPerPixel); err != nil {
		return "", err
	} else if ok && declared != uint64(lay.samples) {
		return unsupportedf("SamplesPerPixel %d with %d BitsPerSample values", declared, lay.samples), nil
	}
	if planar, ok, err := f.first(tagPlanar); err != nil {
		return "", err
	} else if ok && planar != 1 && lay.samples > 1 {
		return unsupportedf("planar configuration %d", planar), nil
	}
	if e, ok := f.entries[tagSampleFormat]; ok {
		values, err := f.uints(e, 16)
		if err != nil {
			return "", err
		}
		for _, v := range values {
			if v != 1 {
				return unsupportedf("sample format %d", v), nil
			}
		}
	}
	compression, ok, err := f.first(tagCompression)
	if err != nil {
		return "", err
	}
	if !ok {
		compression = compressionNone
	}
	switch compression {
	case compressionNone, compressionLZW, compressionDeflate, compressionPackBits:
		lay.compression = int(compression)
	case compressionDeflateOld:
		lay.compression = compressionDeflate
	default:
		return unsupportedf("compression %d", compression), nil
	}
	predictor, ok, err := f.first(tagPredictor)
	if err != nil {
		return "", err
	}
	if ok && predictor != predictorNone {
		if predictor != predictorHorizontal {
			return unsupportedf("predictor %d", predictor), nil
		}
		lay.predictor = true
	}
	photometric, _, err := f.first(tagPhotometric)
	if err != nil {
		return "", err
	}
	switch photometric {
	case photometricWhiteIsZero, photometricBlackIsZero:
		if lay.samples != 1 {
			return unsupportedf("%d samples in a grayscale image", lay.samples), nil
		}
		lay.invert = photometric == photometricWhiteIsZero
		switch lay.bits {
		case 1, 2, 4, 8:
			lay.mode = modeIndexed
			lay.buildGrayTable()
		case 16:
			lay.mode = modeGray16
		default:
			return unsupportedf("%d bits per sample", lay.bits), nil
		}
	case photometricPalette:
		if lay.samples != 1 {
			return unsupportedf("%d samples in a palette image", lay.samples), nil
		}
		switch lay.bits {
		case 1, 2, 4, 8:
		default:
			return unsupportedf("%d bits per palette index", lay.bits), nil
		}
		lay.mode = modeIndexed
		if err := lay.readPalette(f); err != nil {
			return "", err
		}
	case photometricRGB:
		for _, b := range bits {
			if b != bits[0] {
				return unsupportedf("mixed sample depths"), nil
			}
		}
		if lay.bits != 8 && lay.bits != 16 {
			return unsupportedf("%d bits per RGB sample", lay.bits), nil
		}
		switch lay.samples {
		case 3:
			lay.mode = modeRGB8
			if lay.bits == 16 {
				lay.mode = modeRGB16
			}
		case 4:
			extra, _, err := f.first(tagExtraSamples)
			if err != nil {
				return "", err
			}
			if extra != 1 && extra != 2 {
				return unsupportedf("extra sample kind %d", extra), nil
			}
			lay.associated = extra == 1
			lay.info.HasAlpha = true
			lay.mode = modeRGBA8
			if lay.bits == 16 {
				lay.mode = modeRGBA16
			}
		default:
			return unsupportedf("%d samples in an RGB image", lay.samples), nil
		}
	default:
		return unsupportedf("photometric interpretation %d", photometric), nil
	}
	if lay.predictor && lay.bits != 8 && lay.bits != 16 {
		return unsupportedf("horizontal predictor with %d bits per sample", lay.bits), nil
	}
	if fill, ok, err := f.first(tagFillOrder); err != nil {
		return "", err
	} else if ok && fill == 2 && lay.bits < 8 {
		return unsupportedf("reversed fill order"), nil
	}
	return "", nil
}

func (lay *layout) buildGrayTable() {
	lay.lutLen = 1 << lay.bits
	maxValue := lay.lutLen - 1
	for v := 0; v < lay.lutLen; v++ {
		g := uint8(v * 255 / maxValue)
		if lay.invert {
			g = 255 - g
		}
		lay.lut[v] = [4]uint8{g, g, g, 255}
	}
}

func (lay *layout) readPalette(f *file) error {
	e, ok := f.entries[tagColorMap]
	if !ok {
		return guard.Corrupt("tiff: palette image without ColorMap")
	}
	if e.count == 0 || e.count > maxPaletteWords || e.count%3 != 0 {
		return guard.Corrupt("tiff: ColorMap with %d values", e.count)
	}
	words, err := f.uints(e, maxPaletteWords)
	if err != nil {
		return err
	}
	colors := len(words) / 3
	lay.lutLen = colors
	for i := 0; i < colors; i++ {
		lay.lut[i] = [4]uint8{uint8(words[i] >> 8), uint8(words[colors+i] >> 8), uint8(words[2*colors+i] >> 8), 255}
	}
	return nil
}

func classifyGeometry(f *file, lay *layout) (string, error) {
	tileWidth, tiled, err := f.first(tagTileWidth)
	if err != nil {
		return "", err
	}
	if tiled && tileWidth != 0 {
		tileLength, _, err := f.first(tagTileLength)
		if err != nil {
			return "", err
		}
		if tileLength == 0 {
			return "", guard.Corrupt("tiff: zero tile length")
		}
		if tileWidth > maxTileSide || tileLength > maxTileSide {
			return unsupportedf("%dx%d tiles", tileWidth, tileLength), nil
		}
		if tileWidth%tileAlignment != 0 || tileLength%tileAlignment != 0 {
			return unsupportedf("%dx%d tiles are not multiples of %d", tileWidth, tileLength, tileAlignment), nil
		}
		if (int64(tileWidth)-int64(lay.width) > 16 || int64(tileLength)-int64(lay.height) > 16) && (tileWidth > 1024 || tileLength > 1024) {
			return "", guard.Corrupt("tiff: %dx%d tiles in a %dx%d image", tileWidth, tileLength, lay.width, lay.height)
		}
		lay.tiled = true
		lay.chunkW, lay.chunkH = int(tileWidth), int(tileLength)
		lay.across = (lay.width + lay.chunkW - 1) / lay.chunkW
		lay.down = (lay.height + lay.chunkH - 1) / lay.chunkH
	} else {
		rows, ok, err := f.first(tagRowsPerStrip)
		if err != nil {
			return "", err
		}
		if !ok || rows == 0 || rows > uint64(lay.height) {
			rows = uint64(lay.height)
		}
		if lay.width > maxStripWidth {
			return unsupportedf("strips %d pixels wide", lay.width), nil
		}
		lay.chunkW, lay.chunkH = lay.width, int(rows)
		lay.across = 1
		lay.down = (lay.height + lay.chunkH - 1) / lay.chunkH
	}
	if int64(lay.across)*int64(lay.down) > maxChunks {
		return unsupportedf("%d strips or tiles", int64(lay.across)*int64(lay.down)), nil
	}
	lay.rowBytes = rowBytesFor(lay, lay.chunkW)
	return "", nil
}

func rowBytesFor(lay *layout, pixels int) int {
	return int((int64(pixels)*int64(lay.samples)*int64(lay.bits) + 7) / 8)
}

func (lay *layout) chunkCount() int { return lay.across * lay.down }

func (lay *layout) chunkRect(index int) (x0, y0, validW, validH int) {
	x0 = (index % lay.across) * lay.chunkW
	y0 = (index / lay.across) * lay.chunkH
	return x0, y0, min(lay.chunkW, lay.width-x0), min(lay.chunkH, lay.height-y0)
}

func (lay *layout) bandRows() int {
	rows := maxBandBytes / (lay.chunkW * 4)
	if lay.tiled {
		rows &^= 7
		rows = max(rows, 8)
	}
	rows = max(rows, 1)
	return min(rows, lay.chunkH)
}

func (lay *layout) minChunkBytes(validW, validH int) int64 {
	return int64(validH-1)*int64(lay.rowBytes) + int64(rowBytesFor(lay, validW))
}

func (lay *layout) maxExpansion() int64 {
	switch lay.compression {
	case compressionPackBits:
		return 64
	case compressionLZW:
		return 3000
	case compressionDeflate:
		return 1100
	}
	return 1
}

func (f *file) loadTables(lay *layout) error {
	count := lay.chunkCount()
	offsetTag, countTag := uint16(tagStripOffsets), uint16(tagStripByteCounts)
	if lay.tiled {
		offsetTag, countTag = tagTileOffsets, tagTileByteCounts
	}
	var err error
	if lay.offsets, err = f.table(offsetTag, count); err != nil {
		return err
	}
	if lay.counts, err = f.table(countTag, count); err != nil {
		return err
	}
	expansion := lay.maxExpansion()
	for i := 0; i < count; i++ {
		_, _, validW, validH := lay.chunkRect(i)
		offset, length := lay.offsets[i], lay.counts[i]
		if length == 0 || offset > uint64(f.size) || length > uint64(f.size)-offset {
			return guard.Corrupt("tiff: chunk %d lies outside the file", i)
		}
		need := lay.minChunkBytes(validW, validH)
		if expansion == 1 && int64(length) < need {
			return guard.Corrupt("tiff: chunk %d holds %d bytes, %d needed", i, length, need)
		}
		if expansion > 1 && need > int64(length)*expansion+1024 {
			return guard.Corrupt("tiff: chunk %d cannot expand from %d to %d bytes", i, length, need)
		}
	}
	return nil
}

func (f *file) table(tag uint16, count int) ([]uint64, error) {
	e, ok := f.entries[tag]
	if !ok {
		return nil, guard.Corrupt("tiff: no strip or tile table %#x", tag)
	}
	if e.count < uint64(count) {
		return nil, guard.Corrupt("tiff: table %#x has %d values, %d needed", tag, e.count, count)
	}
	return f.uints(e, uint64(count))
}
