package exif

const (
	thumbCompressionTag uint16 = 0x0103
	thumbOffsetTag      uint16 = 0x0201
	thumbLengthTag      uint16 = 0x0202
)

const (
	thumbCompressionOldJPEG uint32 = 6
	thumbCompressionJPEG    uint32 = 7
	thumbMinBytes                  = 4
)

func JPEGThumbnail(tiff []byte) ([]byte, bool) {
	r, err := NewReader(tiff)
	if err != nil {
		return nil, false
	}
	tree, err := r.Tree()
	if err != nil || tree.IFD1 == nil {
		return nil, false
	}
	if e, ok := tree.IFD1.Find(thumbCompressionTag); ok {
		v, ok := e.Uint(0)
		if !ok || (v != thumbCompressionOldJPEG && v != thumbCompressionJPEG) {
			return nil, false
		}
	}
	offsetEntry, ok := tree.IFD1.Find(thumbOffsetTag)
	if !ok {
		return nil, false
	}
	lengthEntry, ok := tree.IFD1.Find(thumbLengthTag)
	if !ok {
		return nil, false
	}
	offset, ok := offsetEntry.Uint(0)
	if !ok {
		return nil, false
	}
	length, ok := lengthEntry.Uint(0)
	if !ok || length < thumbMinBytes {
		return nil, false
	}
	if uint64(offset) > uint64(len(tiff)) || uint64(length) > uint64(len(tiff))-uint64(offset) {
		return nil, false
	}
	data := tiff[offset : uint64(offset)+uint64(length) : uint64(offset)+uint64(length)]
	if data[0] != 0xff || data[1] != 0xd8 {
		return nil, false
	}
	return data, true
}
