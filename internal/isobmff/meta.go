package isobmff

type locationEntry struct {
	method uint8
	base   uint64
	extent []Extent
}

type metaBuilder struct {
	file      *File
	locations map[uint32]locationEntry
	links     map[uint32][]Association
}

func (f *File) parseMeta(payload []byte) error {
	c := newCursor(payload)
	c.fullBox()
	if c.failed {
		return corrupt("heif: meta box has no header")
	}
	b := &metaBuilder{file: f, locations: map[uint32]locationEntry{}, links: map[uint32][]Association{}}
	err := eachBox(c.rest(), func(kind string, body []byte) error {
		switch kind {
		case "pitm":
			return b.parsePrimary(body)
		case "iinf":
			return b.parseItemInfo(body)
		case "iloc":
			return b.parseLocations(body)
		case "iref":
			return b.parseReferences(body)
		case "idat":
			f.idat = body
		case "iprp":
			return eachBox(body, func(kind string, body []byte) error {
				switch kind {
				case "ipco":
					return b.parseProperties(body)
				case "ipma":
					return b.parseAssociations(body)
				}
				return nil
			})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for id, item := range f.Items {
		if location, ok := b.locations[id]; ok {
			item.Method, item.BaseOffset, item.Extents = location.method, location.base, location.extent
		}
		item.Associations = b.links[id]
	}
	return nil
}

func (b *metaBuilder) parsePrimary(body []byte) error {
	c := newCursor(body)
	version, _ := c.fullBox()
	if version == 0 {
		b.file.PrimaryID = uint32(c.u16())
	} else {
		b.file.PrimaryID = c.u32()
	}
	if c.failed {
		return corrupt("heif: truncated pitm box")
	}
	b.file.HasPrimaryID = true
	return nil
}

func (b *metaBuilder) parseItemInfo(body []byte) error {
	c := newCursor(body)
	version, _ := c.fullBox()
	if version == 0 {
		c.skip(2)
	} else {
		c.skip(4)
	}
	if c.failed {
		return corrupt("heif: truncated iinf box")
	}
	return eachBox(c.rest(), func(kind string, entry []byte) error {
		if kind != "infe" {
			return nil
		}
		e := newCursor(entry)
		version, flags := e.fullBox()
		if version < 2 {
			return nil
		}
		var id uint32
		if version == 2 {
			id = uint32(e.u16())
		} else {
			id = e.u32()
		}
		e.skip(2)
		itemType := e.fourCC()
		if e.failed {
			return corrupt("heif: truncated infe box")
		}
		item := b.file.Items[id]
		if item == nil {
			if len(b.file.Items) >= maxItems {
				return tooLarge("heif: more than %d items", maxItems)
			}
			item = &Item{ID: id}
			b.file.Items[id] = item
			b.file.Order = append(b.file.Order, id)
		}
		item.Type = itemType
		item.Hidden = flags&1 != 0
		if itemType == "mime" {
			e.cString()
			if contentType := e.cString(); !e.failed {
				item.ContentType = contentType
			}
		}
		return nil
	})
}

func validFieldSize(n int) bool { return n == 0 || n == 4 || n == 8 }

func (b *metaBuilder) parseLocations(body []byte) error {
	c := newCursor(body)
	version, _ := c.fullBox()
	sizes := c.u8()
	offsetSize, lengthSize := int(sizes>>4), int(sizes&0xf)
	sizes = c.u8()
	baseSize, indexSize := int(sizes>>4), int(sizes&0xf)
	if version < 1 {
		indexSize = 0
	}
	for _, n := range []int{offsetSize, lengthSize, baseSize, indexSize} {
		if !validFieldSize(n) {
			return corrupt("heif: iloc field size %d", n)
		}
	}
	var count uint32
	if version < 2 {
		count = uint32(c.u16())
	} else {
		count = c.u32()
	}
	if c.failed {
		return corrupt("heif: truncated iloc box")
	}
	for i := uint32(0); i < count; i++ {
		var id uint32
		if version < 2 {
			id = uint32(c.u16())
		} else {
			id = c.u32()
		}
		var entry locationEntry
		if version >= 1 {
			entry.method = uint8(c.u16() & 0xf)
		}
		c.skip(2)
		entry.base = c.unsigned(baseSize)
		extents := int(c.u16())
		for j := 0; j < extents && !c.failed; j++ {
			c.skip(indexSize)
			offset := c.unsigned(offsetSize)
			length := c.unsigned(lengthSize)
			if c.failed {
				break
			}
			entry.extent = append(entry.extent, Extent{Offset: offset, Length: length})
		}
		if c.failed {
			return corrupt("heif: truncated iloc entry")
		}
		if _, known := b.locations[id]; !known && len(b.locations) >= maxItems {
			return tooLarge("heif: more than %d item locations", maxItems)
		}
		b.locations[id] = entry
	}
	return nil
}

func (b *metaBuilder) parseReferences(body []byte) error {
	c := newCursor(body)
	version, _ := c.fullBox()
	if c.failed {
		return corrupt("heif: truncated iref box")
	}
	return eachBox(c.rest(), func(kind string, entry []byte) error {
		e := newCursor(entry)
		ref := Reference{Type: kind}
		if version == 0 {
			ref.From = uint32(e.u16())
		} else {
			ref.From = e.u32()
		}
		count := int(e.u16())
		for i := 0; i < count && !e.failed; i++ {
			if version == 0 {
				ref.To = append(ref.To, uint32(e.u16()))
			} else {
				ref.To = append(ref.To, e.u32())
			}
		}
		if e.failed {
			return corrupt("heif: truncated %s reference", kind)
		}
		if len(b.file.References) >= maxReferences {
			return tooLarge("heif: more than %d references", maxReferences)
		}
		b.file.References = append(b.file.References, ref)
		return nil
	})
}

func (b *metaBuilder) parseAssociations(body []byte) error {
	c := newCursor(body)
	version, flags := c.fullBox()
	count := c.u32()
	if c.failed {
		return corrupt("heif: truncated ipma box")
	}
	for i := uint32(0); i < count; i++ {
		var id uint32
		if version < 1 {
			id = uint32(c.u16())
		} else {
			id = c.u32()
		}
		n := int(c.u8())
		for j := 0; j < n; j++ {
			var index int
			var essential bool
			if flags&1 != 0 {
				value := c.u16()
				index, essential = int(value&0x7fff), value&0x8000 != 0
			} else {
				value := c.u8()
				index, essential = int(value&0x7f), value&0x80 != 0
			}
			if c.failed {
				return corrupt("heif: truncated ipma entry")
			}
			if index == 0 {
				continue
			}
			if _, known := b.links[id]; !known && len(b.links) >= maxItems {
				return tooLarge("heif: more than %d associated items", maxItems)
			}
			b.links[id] = append(b.links[id], Association{Index: index - 1, Essential: essential})
		}
		if c.failed {
			return corrupt("heif: truncated ipma entry")
		}
	}
	return nil
}
