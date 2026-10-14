package isobmff

type AV1Config struct {
	HighBitDepth bool
	TwelveBit    bool
	Monochrome   bool
}

func (c AV1Config) BitDepth() int {
	switch {
	case c.TwelveBit:
		return 12
	case c.HighBitDepth:
		return 10
	}
	return 8
}

type HEVCConfig struct {
	ChromaFormat   uint8
	BitDepthLuma   uint8
	BitDepthChroma uint8
}

type Property struct {
	Type     string
	Payload  []byte
	Width    uint32
	Height   uint32
	Angle    uint8
	Axis     uint8
	Clap     [8]uint32
	Channels []uint8
	AuxType  string
	AV1      AV1Config
	HEVC     HEVCConfig
}

func (b *metaBuilder) parseProperties(body []byte) error {
	return eachBox(body, func(kind string, payload []byte) error {
		if len(b.file.Properties) >= maxProperties {
			return tooLarge("heif: more than %d properties", maxProperties)
		}
		property, err := parseProperty(kind, payload)
		if err != nil {
			return err
		}
		b.file.Properties = append(b.file.Properties, property)
		return nil
	})
}

func parseProperty(kind string, payload []byte) (Property, error) {
	property := Property{Type: kind, Payload: payload}
	c := newCursor(payload)
	switch kind {
	case "ispe":
		c.fullBox()
		property.Width = c.u32()
		property.Height = c.u32()
	case "pixi":
		c.fullBox()
		count := int(c.u8())
		for i := 0; i < count && !c.failed; i++ {
			property.Channels = append(property.Channels, c.u8())
		}
	case "irot":
		property.Angle = c.u8() & 3
	case "imir":
		property.Axis = c.u8() & 1
	case "clap":
		for i := range property.Clap {
			property.Clap[i] = c.u32()
		}
	case "auxC":
		c.fullBox()
		property.AuxType = c.cString()
	case "av1C":
		marker := c.u8()
		if !c.failed && (marker>>7 != 1 || marker&0x7f != 1) {
			return property, corrupt("heif: av1C marker %#x", marker)
		}
		c.skip(1)
		flags := c.u8()
		property.AV1 = AV1Config{
			HighBitDepth: flags>>6&1 != 0,
			TwelveBit:    flags>>5&1 != 0,
			Monochrome:   flags>>4&1 != 0,
		}
	case "hvcC":
		if version := c.u8(); !c.failed && version != 1 {
			return property, corrupt("heif: hvcC version %d", version)
		}
		c.skip(12)
		c.skip(2)
		c.skip(1)
		property.HEVC.ChromaFormat = c.u8() & 3
		property.HEVC.BitDepthLuma = c.u8()&7 + 8
		property.HEVC.BitDepthChroma = c.u8()&7 + 8
	}
	if c.failed {
		return property, corrupt("heif: truncated %s property", kind)
	}
	return property, nil
}
