package isobmff

import (
	"context"
	"io"
)

const (
	MaxHeaderBytes   = 32 << 20
	maxTopLevelBoxes = 1 << 16
	maxItems         = 1 << 17
	maxProperties    = 1 << 17
	maxReferences    = 1 << 18
	maxTracks        = 64
	maxSamples       = 1 << 24
	maxBrandBytes    = 1 << 12
	contextInterval  = 64
)

type Extent struct {
	Offset uint64
	Length uint64
}

type Association struct {
	Index     int
	Essential bool
}

type Item struct {
	ID           uint32
	Type         string
	ContentType  string
	Hidden       bool
	Method       uint8
	BaseOffset   uint64
	Extents      []Extent
	Associations []Association
}

type Reference struct {
	Type string
	From uint32
	To   []uint32
}

type Track struct {
	Handler string
	Samples int
	Width   int
	Height  int
}

type File struct {
	Brands       Brands
	Items        map[uint32]*Item
	Order        []uint32
	PrimaryID    uint32
	HasPrimaryID bool
	Properties   []Property
	References   []Reference
	Tracks       []Track
	Mini         bool
	HasMeta      bool

	source io.ReaderAt
	size   int64
	idat   []byte
}

func Open(ctx context.Context, r io.ReaderAt, size int64) (*File, error) {
	f := &File{source: r, size: size, Items: map[uint32]*Item{}}
	if size < boxHeaderSize+8 {
		return nil, corrupt("heif: %d bytes is too short", size)
	}
	var position int64
	for boxes := 0; position+boxHeaderSize <= size; boxes++ {
		if boxes >= maxTopLevelBoxes {
			return nil, corrupt("heif: more than %d top level boxes", maxTopLevelBoxes)
		}
		if boxes%contextInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		box, err := f.readTopLevelBox(position)
		if err != nil {
			return nil, err
		}
		if boxes == 0 && box.kind != "ftyp" {
			return nil, corrupt("heif: first box is %q, not ftyp", box.kind)
		}
		if err := f.visitTopLevel(box); err != nil {
			return nil, err
		}
		position += int64(box.total)
	}
	if f.Brands.Major == "" {
		return nil, corrupt("heif: no ftyp box")
	}
	return f, nil
}

type topLevelBox struct {
	boxHeader
	start int64
}

func (f *File) readTopLevelBox(position int64) (topLevelBox, error) {
	var head [largeBoxHeaderSize]byte
	window := min(int64(len(head)), f.size-position)
	if err := readAtInto(f.source, head[:window], position, f.size); err != nil {
		return topLevelBox{}, err
	}
	header, err := parseBoxHeader(head[:window], uint64(f.size-position))
	if err != nil {
		return topLevelBox{}, err
	}
	return topLevelBox{boxHeader: header, start: position}, nil
}

func (f *File) payload(box topLevelBox, limit int64) ([]byte, error) {
	length := int64(box.total) - int64(box.headLen)
	if length > limit {
		return nil, tooLarge("heif: %s box of %d bytes", box.kind, length)
	}
	return readAt(f.source, box.start+int64(box.headLen), length, f.size)
}

func (f *File) visitTopLevel(box topLevelBox) error {
	switch box.kind {
	case "ftyp":
		if f.Brands.Major != "" {
			return nil
		}
		payload, err := f.payload(box, maxBrandBytes)
		if err != nil {
			return err
		}
		brands, ok := parseBrands(payload)
		if !ok {
			return corrupt("heif: ftyp box of %d bytes", len(payload))
		}
		f.Brands = brands
	case "meta":
		if f.HasMeta {
			return nil
		}
		payload, err := f.payload(box, MaxHeaderBytes)
		if err != nil {
			return err
		}
		if err := f.parseMeta(payload); err != nil {
			return err
		}
		f.HasMeta = true
	case "moov":
		if len(f.Tracks) > 0 {
			return nil
		}
		payload, err := f.payload(box, MaxHeaderBytes)
		if err != nil {
			return err
		}
		return f.parseMovie(payload)
	case "mini":
		f.Mini = true
	}
	return nil
}

func (f *File) Size() int64 { return f.size }

func (f *File) PrimaryItem() *Item {
	if !f.HasPrimaryID {
		return nil
	}
	return f.Items[f.PrimaryID]
}

func (f *File) FirstItem(kinds ...string) *Item {
	for _, id := range f.Order {
		item := f.Items[id]
		for _, kind := range kinds {
			if item.Type == kind {
				return item
			}
		}
	}
	return nil
}

func (f *File) ReferencesFrom(kind string, from uint32) []uint32 {
	for _, ref := range f.References {
		if ref.Type == kind && ref.From == from {
			return ref.To
		}
	}
	return nil
}

func (f *File) Property(item *Item, kind string) *Property {
	if item == nil {
		return nil
	}
	for _, assoc := range item.Associations {
		if assoc.Index < len(f.Properties) && f.Properties[assoc.Index].Type == kind {
			return &f.Properties[assoc.Index]
		}
	}
	return nil
}

func (f *File) PropertiesOf(item *Item, kind string) []*Property {
	if item == nil {
		return nil
	}
	var out []*Property
	for _, assoc := range item.Associations {
		if assoc.Index < len(f.Properties) && f.Properties[assoc.Index].Type == kind {
			out = append(out, &f.Properties[assoc.Index])
		}
	}
	return out
}

func (f *File) UnknownEssential(item *Item, known []string) string {
	for _, assoc := range item.Associations {
		if !assoc.Essential {
			continue
		}
		if assoc.Index >= len(f.Properties) {
			return "missing"
		}
		kind := f.Properties[assoc.Index].Type
		if !containsString(known, kind) {
			return kind
		}
	}
	return ""
}

func containsString(list []string, value string) bool {
	for _, entry := range list {
		if entry == value {
			return true
		}
	}
	return false
}

func (f *File) ReadItem(item *Item, limit int64) ([]byte, error) {
	var out []byte
	for _, extent := range item.Extents {
		if int64(len(out)) >= limit {
			break
		}
		offset := item.BaseOffset + extent.Offset
		if offset < item.BaseOffset {
			return nil, corrupt("heif: item %d extent offset overflows", item.ID)
		}
		var data []byte
		switch item.Method {
		case 0:
			length := int64(extent.Length)
			if extent.Length == 0 {
				length = f.size - int64(offset)
			}
			if offset > uint64(f.size) || length < 0 || uint64(length) > uint64(f.size)-offset {
				return nil, corrupt("heif: item %d extent lies outside the file", item.ID)
			}
			length = min(length, limit-int64(len(out)))
			buf, err := readAt(f.source, int64(offset), length, f.size)
			if err != nil {
				return nil, err
			}
			data = buf
		case 1:
			length := extent.Length
			if length == 0 && offset <= uint64(len(f.idat)) {
				length = uint64(len(f.idat)) - offset
			}
			if offset > uint64(len(f.idat)) || length > uint64(len(f.idat))-offset {
				return nil, corrupt("heif: item %d extent lies outside idat", item.ID)
			}
			length = min(length, uint64(limit)-uint64(len(out)))
			data = f.idat[offset : offset+length]
		default:
			return nil, unsupported("heif: item %d uses construction method %d", item.ID, item.Method)
		}
		out = append(out, data...)
	}
	if len(out) == 0 {
		return nil, corrupt("heif: item %d has no data", item.ID)
	}
	return out, nil
}
