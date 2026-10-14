package isobmff

import (
	"image"

	"github.com/oops1/PixGo/internal/codec"
)

const (
	MaxDimension      = 1 << 20
	gridDescriptorMax = 12
	wideFrameCost     = 3
)

type Codec uint8

const (
	CodecOther Codec = iota
	CodecAV1
	CodecHEVC
)

func codecOf(itemType string) Codec {
	switch itemType {
	case "av01":
		return CodecAV1
	case "hvc1", "hvc2":
		return CodecHEVC
	}
	return CodecOther
}

type Transform struct {
	Clap     *[8]uint32
	Rotation uint8
	Mirrored bool
	Axis     uint8
}

func (t Transform) Orientation() codec.Orientation {
	orientation := codec.OrientNormal
	switch t.Rotation {
	case 1:
		orientation = codec.OrientRotate270
	case 2:
		orientation = codec.OrientRotate180
	case 3:
		orientation = codec.OrientRotate90
	}
	if t.Mirrored {
		if t.Axis == 0 {
			orientation = orientation.FlipV()
		} else {
			orientation = orientation.FlipH()
		}
	}
	return orientation
}

func (t Transform) Crop(width, height int) (image.Rectangle, bool) {
	full := image.Rect(0, 0, width, height)
	if t.Clap == nil {
		return full, true
	}
	clap := t.Clap
	widthNum, widthDen := int64(int32(clap[0])), int64(int32(clap[1]))
	heightNum, heightDen := int64(int32(clap[2])), int64(int32(clap[3]))
	horizNum, horizDen := int64(int32(clap[4])), int64(int32(clap[5]))
	vertNum, vertDen := int64(int32(clap[6])), int64(int32(clap[7]))
	if widthDen <= 0 || heightDen <= 0 || horizDen <= 0 || vertDen <= 0 || widthNum < 0 || heightNum < 0 {
		return image.Rectangle{}, false
	}
	if widthNum%widthDen != 0 || heightNum%heightDen != 0 {
		return image.Rectangle{}, false
	}
	cropWidth, cropHeight := widthNum/widthDen, heightNum/heightDen
	numX := int64(width)*horizDen + 2*horizNum - cropWidth*horizDen
	numY := int64(height)*vertDen + 2*vertNum - cropHeight*vertDen
	if numX%(2*horizDen) != 0 || numY%(2*vertDen) != 0 {
		return image.Rectangle{}, false
	}
	x, y := numX/(2*horizDen), numY/(2*vertDen)
	if x < 0 || y < 0 || cropWidth <= 0 || cropHeight <= 0 || x+cropWidth > int64(width) || y+cropHeight > int64(height) {
		return image.Rectangle{}, false
	}
	return image.Rect(int(x), int(y), int(x+cropWidth), int(y+cropHeight)), true
}

type Grid struct {
	Rows       int
	Cols       int
	Width      int
	Height     int
	TileWidth  int
	TileHeight int
	Tiles      []uint32
}

type Picture struct {
	Item       *Item
	Codec      Codec
	ItemType   string
	Width      int
	Height     int
	Transform  Transform
	Alpha      bool
	BitDepth   int
	Monochrome bool
	Grid       *Grid
}

func (p Picture) BitsPerPixel() int {
	samples := 3
	if p.Monochrome {
		samples = 1
	}
	if p.Alpha {
		samples++
	}
	return samples * p.BitDepth
}

func (p Picture) FrameLimits(lim codec.Limits) codec.Limits {
	if p.BitDepth > 8 {
		lim.MaxFramePixels /= wideFrameCost
	}
	return lim
}

func (p Picture) Cropped() (image.Rectangle, bool) { return p.Transform.Crop(p.Width, p.Height) }

func (p Picture) StoredSize() (int, int, bool) {
	rect, ok := p.Cropped()
	return rect.Dx(), rect.Dy(), ok
}

func (p Picture) Check(lim codec.Limits) error {
	if err := lim.Check(p.Width, p.Height); err != nil {
		return err
	}
	if p.Grid == nil {
		return nil
	}
	if err := lim.Check(p.Grid.Width, p.Grid.Height); err != nil {
		return err
	}
	tilePixels := int64(len(p.Grid.Tiles)) * int64(p.Grid.TileWidth) * int64(p.Grid.TileHeight)
	if tilePixels > lim.MaxFramePixels {
		return tooLarge("heif: grid tiles hold %d pixels in total", tilePixels)
	}
	return nil
}

func (f *File) AlphaItem(id uint32, alphaURNs []string) *Item {
	for _, ref := range f.References {
		if ref.Type != "auxl" || len(ref.To) == 0 || ref.To[0] != id {
			continue
		}
		item := f.Items[ref.From]
		if item == nil {
			continue
		}
		if aux := f.Property(item, "auxC"); aux != nil && containsString(alphaURNs, aux.AuxType) {
			return item
		}
	}
	return nil
}

func (f *File) describeGrid(item *Item) (*Grid, error) {
	data, err := f.ReadItem(item, gridDescriptorMax)
	if err != nil {
		return nil, err
	}
	c := newCursor(data)
	c.u8()
	flags := c.u8()
	grid := &Grid{Rows: int(c.u8()) + 1, Cols: int(c.u8()) + 1}
	if flags&1 != 0 {
		grid.Width, grid.Height = int(c.u32()), int(c.u32())
	} else {
		grid.Width, grid.Height = int(c.u16()), int(c.u16())
	}
	if c.failed || grid.Width <= 0 || grid.Height <= 0 || grid.Width > MaxDimension || grid.Height > MaxDimension {
		return nil, corrupt("heif: grid descriptor of item %d", item.ID)
	}
	grid.Tiles = f.ReferencesFrom("dimg", item.ID)
	if len(grid.Tiles) != grid.Rows*grid.Cols {
		return nil, corrupt("heif: grid %dx%d has %d tiles", grid.Cols, grid.Rows, len(grid.Tiles))
	}
	for i, id := range grid.Tiles {
		tile := f.Items[id]
		if tile == nil {
			return nil, corrupt("heif: grid tile %d is missing", id)
		}
		ispe := f.Property(tile, "ispe")
		if ispe == nil || ispe.Width == 0 || ispe.Height == 0 || ispe.Width > MaxDimension || ispe.Height > MaxDimension {
			return nil, corrupt("heif: grid tile %d has no usable size", id)
		}
		if i == 0 {
			grid.TileWidth, grid.TileHeight = int(ispe.Width), int(ispe.Height)
			continue
		}
		if tile.Type != f.Items[grid.Tiles[0]].Type || int(ispe.Width) != grid.TileWidth || int(ispe.Height) != grid.TileHeight {
			return nil, corrupt("heif: grid tiles differ in kind or size")
		}
	}
	if grid.TileWidth*grid.Cols < grid.Width || grid.TileHeight*grid.Rows < grid.Height {
		return nil, corrupt("heif: grid tiles do not cover %dx%d", grid.Width, grid.Height)
	}
	return grid, nil
}

func (f *File) Describe(item *Item, alphaURNs []string) (Picture, error) {
	picture := Picture{Item: item, ItemType: item.Type}
	ispe := f.Property(item, "ispe")
	if ispe == nil {
		return picture, corrupt("heif: item %d has no ispe property", item.ID)
	}
	if ispe.Width == 0 || ispe.Height == 0 || ispe.Width > MaxDimension || ispe.Height > MaxDimension {
		return picture, corrupt("heif: item %d is %dx%d", item.ID, ispe.Width, ispe.Height)
	}
	picture.Width, picture.Height = int(ispe.Width), int(ispe.Height)
	configItem := item
	if item.Type == "grid" {
		grid, err := f.describeGrid(item)
		if err != nil {
			return picture, err
		}
		if picture.Width > grid.Width || picture.Height > grid.Height {
			return picture, corrupt("heif: item %dx%d is larger than its %dx%d grid", picture.Width, picture.Height, grid.Width, grid.Height)
		}
		picture.Grid = grid
		configItem = f.Items[grid.Tiles[0]]
		picture.ItemType = "grid of " + configItem.Type
	}
	picture.Codec = codecOf(configItem.Type)
	picture.BitDepth = 8
	if config := f.Property(configItem, "av1C"); config != nil {
		picture.BitDepth = config.AV1.BitDepth()
		picture.Monochrome = config.AV1.Monochrome
	} else if config := f.Property(configItem, "hvcC"); config != nil {
		picture.BitDepth = int(config.HEVC.BitDepthLuma)
		picture.Monochrome = config.HEVC.ChromaFormat == 0
	} else if pixel := f.Property(configItem, "pixi"); pixel != nil && len(pixel.Channels) > 0 {
		picture.BitDepth = int(pixel.Channels[0])
		picture.Monochrome = len(pixel.Channels) == 1
	}
	if clap := f.Property(item, "clap"); clap != nil {
		values := clap.Clap
		picture.Transform.Clap = &values
	}
	if irot := f.Property(item, "irot"); irot != nil {
		picture.Transform.Rotation = irot.Angle
	}
	if imir := f.Property(item, "imir"); imir != nil {
		picture.Transform.Mirrored = true
		picture.Transform.Axis = imir.Axis
	}
	if _, _, ok := picture.StoredSize(); !ok {
		return picture, corrupt("heif: clean aperture does not fit the %dx%d picture", picture.Width, picture.Height)
	}
	picture.Alpha = f.AlphaItem(item.ID, alphaURNs) != nil
	if !picture.Alpha && picture.Grid != nil {
		picture.Alpha = true
		for _, id := range picture.Grid.Tiles {
			if f.AlphaItem(id, alphaURNs) == nil {
				picture.Alpha = false
				break
			}
		}
	}
	return picture, nil
}
