package isobmff

import (
	"encoding/binary"
	"slices"
)

var (
	avifBrands    = []string{"avif", "avis", "MA1A", "MA1B"}
	heicBrands    = []string{"heic", "heix", "hevc", "hevx", "heim", "heis", "hevm", "hevs"}
	genericBrands = []string{"mif1", "msf1", "miaf"}
)

type Family uint8

const (
	FamilyNone Family = iota
	FamilyAVIF
	FamilyHEIC
	FamilyUndetermined
)

type Brands struct {
	Major      string
	Minor      uint32
	Compatible []string
}

func (b Brands) All() []string {
	return append([]string{b.Major}, b.Compatible...)
}

func (b Brands) Contains(brand string) bool {
	return b.Major == brand || slices.Contains(b.Compatible, brand)
}

func (b Brands) containsAny(set []string) bool {
	for _, brand := range set {
		if b.Contains(brand) {
			return true
		}
	}
	return false
}

func parseBrands(payload []byte) (Brands, bool) {
	if len(payload) < 8 {
		return Brands{}, false
	}
	brands := Brands{Major: string(payload[:4]), Minor: binary.BigEndian.Uint32(payload[4:8])}
	for rest := payload[8:]; len(rest) >= 4; rest = rest[4:] {
		brands.Compatible = append(brands.Compatible, string(rest[:4]))
	}
	return brands, true
}

func readFtyp(head []byte) (Brands, int, bool) {
	if len(head) < boxHeaderSize+8 || string(head[4:8]) != "ftyp" {
		return Brands{}, 0, false
	}
	total := uint64(binary.BigEndian.Uint32(head[:4]))
	headLen := uint64(boxHeaderSize)
	switch total {
	case 1:
		if len(head) < largeBoxHeaderSize+8 {
			return Brands{}, 0, false
		}
		total = binary.BigEndian.Uint64(head[8:16])
		headLen = largeBoxHeaderSize
	case 0:
		total = uint64(len(head))
	}
	if total < headLen+8 {
		return Brands{}, 0, false
	}
	end := min(total, uint64(len(head)))
	brands, ok := parseBrands(head[headLen:end])
	if !ok {
		return Brands{}, 0, false
	}
	if total > uint64(len(head)) {
		return brands, len(head), true
	}
	return brands, int(total), true
}

func ReadBrands(head []byte) (Brands, bool) {
	brands, _, ok := readFtyp(head)
	return brands, ok
}

type headHints struct {
	av1  bool
	hevc bool
}

func hintsFromItemType(kind string, hints *headHints) {
	switch kind {
	case "av01", "av1C":
		hints.av1 = true
	case "hvc1", "hvc2", "hvcC":
		hints.hevc = true
	}
}

func scanHeadHints(data []byte) headHints {
	var hints headHints
	eachBoxLenient(data, func(kind string, payload []byte) bool {
		if kind == "meta" && len(payload) >= fullBoxHeaderSize {
			scanMetaHints(payload[fullBoxHeaderSize:], &hints)
		}
		return true
	})
	return hints
}

func scanMetaHints(data []byte, hints *headHints) {
	eachBoxLenient(data, func(kind string, payload []byte) bool {
		switch kind {
		case "iinf":
			scanItemInfoHints(payload, hints)
		case "iprp":
			eachBoxLenient(payload, func(kind string, payload []byte) bool {
				if kind == "ipco" {
					eachBoxLenient(payload, func(kind string, _ []byte) bool {
						hintsFromItemType(kind, hints)
						return true
					})
				}
				return true
			})
		}
		return true
	})
}

func scanItemInfoHints(payload []byte, hints *headHints) {
	c := newCursor(payload)
	version, _ := c.fullBox()
	if version == 0 {
		c.skip(2)
	} else {
		c.skip(4)
	}
	if c.failed {
		return
	}
	eachBoxLenient(c.rest(), func(kind string, entry []byte) bool {
		if kind != "infe" {
			return true
		}
		e := newCursor(entry)
		version, _ := e.fullBox()
		if version < 2 {
			return true
		}
		if version == 2 {
			e.skip(2)
		} else {
			e.skip(4)
		}
		e.skip(2)
		itemType := e.fourCC()
		if !e.failed {
			hintsFromItemType(itemType, hints)
		}
		return true
	})
}

func (b Brands) Family() Family {
	avif := b.containsAny(avifBrands)
	heic := b.containsAny(heicBrands)
	switch {
	case avif && !heic:
		return FamilyAVIF
	case heic && !avif:
		return FamilyHEIC
	case avif && heic:
		switch {
		case slices.Contains(avifBrands, b.Major):
			return FamilyAVIF
		case slices.Contains(heicBrands, b.Major):
			return FamilyHEIC
		}
		return FamilyUndetermined
	case b.containsAny(genericBrands):
		return FamilyUndetermined
	}
	return FamilyNone
}

func Classify(head []byte) Family {
	brands, ftypEnd, ok := readFtyp(head)
	if !ok {
		return FamilyNone
	}
	family := brands.Family()
	if family != FamilyUndetermined {
		return family
	}
	hints := scanHeadHints(head[ftypEnd:])
	switch {
	case hints.av1 && !hints.hevc:
		return FamilyAVIF
	case hints.hevc && !hints.av1:
		return FamilyHEIC
	}
	return FamilyUndetermined
}
