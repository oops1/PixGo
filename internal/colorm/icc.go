package colorm

import (
	"crypto/sha256"
	"encoding/binary"
	"strings"
	"unicode/utf16"

	"github.com/oops1/PixGo/internal/codec"
)

const (
	iccHeaderSize = 128
	iccMaxTags    = 1024
	MaxICCBytes   = 16 << 20
	iccMaxDesc    = 256
)

type Space uint8

const (
	SpaceRGB Space = iota
	SpaceGray
	SpaceCMYK
)

type kind uint8

const (
	kindMatrix kind = iota
	kindLUT
)

type Origin uint8

const (
	OriginNone Origin = iota
	OriginEmbedded
	OriginCICP
)

type Profile struct {
	Description string
	Major       uint8
	Minor       uint8
	Class       string
	Space       Space
	PCSLab      bool
	Hash        [32]byte

	kind      kind
	colorants Mat3
	trc       [3]*Curve
	a2b       [4]*lutPipe
	b2a       [4]*lutPipe
	wtpt      *Vec3
	bkpt      *Vec3
	chad      *Mat3
	raw       []byte
	cicp      *codec.CICP
	hdr       hdrKind
	gray      bool
	builtin   bool
}

func (p *Profile) Channels() int {
	if p.Space == SpaceCMYK {
		return 4
	}
	if p.Space == SpaceGray {
		return 1
	}
	return 3
}

func (p *Profile) CICP() *codec.CICP { return p.cicp }

func (p *Profile) Adaptation() (Mat3, bool) {
	if p.chad == nil {
		return Mat3{}, false
	}
	return *p.chad, true
}

func (p *Profile) MediaWhite() (Vec3, bool) {
	if p.wtpt == nil {
		return Vec3{}, false
	}
	return *p.wtpt, true
}

type tagEntry struct{ off, size int }

type tagTable map[string]tagEntry

func (t tagTable) data(b []byte, sig string) ([]byte, bool) {
	e, ok := t[sig]
	if !ok {
		return nil, false
	}
	return b[e.off : e.off+e.size], true
}

func ParseICC(data []byte) (*Profile, error) {
	if len(data) < iccHeaderSize+4 || len(data) > MaxICCBytes {
		return nil, ErrProfile
	}
	if string(data[36:40]) != "acsp" {
		return nil, ErrProfile
	}
	if size := int(binary.BigEndian.Uint32(data[0:4])); size >= iccHeaderSize+4 && size <= len(data) {
		data = data[:size]
	} else if size > len(data) {
		return nil, ErrProfile
	}
	p := &Profile{Major: data[8], Minor: data[9] >> 4, Class: string(data[12:16]), Hash: sha256.Sum256(data), raw: append([]byte(nil), data...)}
	switch p.Class {
	case "mntr", "scnr", "prtr", "spac":
	default:
		return nil, ErrUnsupported
	}
	switch string(data[16:20]) {
	case "RGB ":
		p.Space = SpaceRGB
	case "GRAY":
		p.Space = SpaceGray
	case "CMYK":
		p.Space = SpaceCMYK
	default:
		return nil, ErrUnsupported
	}
	switch string(data[20:24]) {
	case "XYZ ":
	case "Lab ":
		p.PCSLab = true
	default:
		return nil, ErrProfile
	}
	if p.Major < 2 || p.Major > 4 {
		return nil, ErrUnsupported
	}
	count := int(binary.BigEndian.Uint32(data[iccHeaderSize:]))
	if count < 0 || count > iccMaxTags || count > (len(data)-iccHeaderSize-4)/12 {
		return nil, ErrProfile
	}
	tags := make(tagTable, count)
	for i := 0; i < count; i++ {
		e := data[iccHeaderSize+4+12*i:]
		sig := string(e[0:4])
		off, size := int64(binary.BigEndian.Uint32(e[4:])), int64(binary.BigEndian.Uint32(e[8:]))
		if size < 8 || off < iccHeaderSize || off+size > int64(len(data)) {
			return nil, ErrProfile
		}
		if _, dup := tags[sig]; !dup {
			tags[sig] = tagEntry{int(off), int(size)}
		}
	}
	if b, ok := tags.data(data, "desc"); ok {
		p.Description = readText(b)
	}
	if b, ok := tags.data(data, "wtpt"); ok {
		if v, ok := readXYZ(b); ok {
			p.wtpt = &v
		}
	}
	if b, ok := tags.data(data, "bkpt"); ok {
		if v, ok := readXYZ(b); ok {
			p.bkpt = &v
		}
	}
	if b, ok := tags.data(data, "chad"); ok {
		if m, ok := readS15Matrix(b); ok {
			p.chad = &m
		}
	}
	if b, ok := tags.data(data, "cicp"); ok && len(b) >= 12 && string(b[:4]) == "cicp" {
		p.cicp = &codec.CICP{Primaries: b[8], Transfer: b[9], Matrix: b[10], FullRange: b[11] != 0}
		p.hdr = hdrKindOf(p.cicp.Transfer)
	}
	pcs := string(data[20:24])
	for i, sigs := range [...][2]string{{"A2B0", "B2A0"}, {"A2B1", "B2A1"}, {"A2B2", "B2A2"}} {
		if b, ok := tags.data(data, sigs[0]); ok {
			if lp, err := parseLUT(b, true, pcs); err == nil && lp.in == p.Channels() && lp.out == 3 {
				p.a2b[i] = lp
			}
		}
		if b, ok := tags.data(data, sigs[1]); ok {
			if lp, err := parseLUT(b, false, pcs); err == nil && lp.out == p.Channels() && lp.in == 3 {
				p.b2a[i] = lp
			}
		}
	}
	hasLUT := p.a2b[0] != nil || p.a2b[1] != nil || p.a2b[2] != nil
	switch p.Space {
	case SpaceCMYK:
		if !hasLUT {
			return nil, ErrUnsupported
		}
		p.kind = kindLUT
	case SpaceGray:
		b, ok := tags.data(data, "kTRC")
		if !ok {
			return nil, ErrUnsupported
		}
		c, _, err := parseCurveElement(b)
		if err != nil {
			return nil, err
		}
		p.trc = [3]*Curve{c, c, c}
		third := d50
		p.colorants = Mat3{third[0] / 3, third[0] / 3, third[0] / 3, third[1] / 3, third[1] / 3, third[1] / 3, third[2] / 3, third[2] / 3, third[2] / 3}
		p.gray = true
		p.kind = kindMatrix
	default:
		if err := p.readMatrixShaper(data, tags); err != nil {
			if !hasLUT {
				return nil, err
			}
			p.kind = kindLUT
		}
	}
	return p, nil
}

func (p *Profile) readMatrixShaper(data []byte, tags tagTable) error {
	var m Mat3
	for i, sig := range [...]string{"rXYZ", "gXYZ", "bXYZ"} {
		b, ok := tags.data(data, sig)
		if !ok {
			return ErrUnsupported
		}
		v, ok := readXYZ(b)
		if !ok {
			return ErrProfile
		}
		m[i], m[3+i], m[6+i] = v[0], v[1], v[2]
	}
	for i, sig := range [...]string{"rTRC", "gTRC", "bTRC"} {
		b, ok := tags.data(data, sig)
		if !ok {
			return ErrUnsupported
		}
		c, _, err := parseCurveElement(b)
		if err != nil {
			return err
		}
		p.trc[i] = c
	}
	if _, ok := m.Inverse(); !ok || !m.finite() {
		return ErrProfile
	}
	p.colorants = m
	p.kind = kindMatrix
	return nil
}

func readXYZ(b []byte) (Vec3, bool) {
	if len(b) < 20 || string(b[:4]) != "XYZ " {
		return Vec3{}, false
	}
	return Vec3{s15f16(b, 8), s15f16(b, 12), s15f16(b, 16)}, true
}

func readS15Matrix(b []byte) (Mat3, bool) {
	if len(b) < 44 || string(b[:4]) != "sf32" {
		return Mat3{}, false
	}
	var m Mat3
	for i := range m {
		m[i] = s15f16(b, 8+4*i)
	}
	return m, m.finite()
}

func readText(b []byte) string {
	if len(b) < 12 {
		return ""
	}
	switch string(b[:4]) {
	case "desc":
		n := int(be32(b, 8))
		if n <= 0 || 12+n > len(b) {
			return ""
		}
		return cleanText(string(b[12 : 12+n]))
	case "text":
		return cleanText(string(b[8:]))
	case "mluc":
		count := int(be32(b, 8))
		recSize := int(be32(b, 12))
		if count <= 0 || count > 64 || recSize < 12 || 16+count*recSize > len(b) {
			return ""
		}
		pick := -1
		for i := 0; i < count; i++ {
			r := b[16+i*recSize:]
			if string(r[:4]) == "enUS" {
				pick = i
				break
			}
			if pick < 0 {
				pick = i
			}
		}
		r := b[16+pick*recSize:]
		length, off := int(be32(r, 4)), int(be32(r, 8))
		if length < 0 || off < 0 || off+length > len(b) || length%2 != 0 {
			return ""
		}
		u := make([]uint16, length/2)
		for i := range u {
			u[i] = be16(b, off+2*i)
		}
		return cleanText(string(utf16.Decode(u)))
	}
	return ""
}

func cleanText(s string) string {
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if len(s) > iccMaxDesc {
		s = s[:iccMaxDesc]
		for len(s) > 0 && s[len(s)-1] >= 0x80 {
			s = s[:len(s)-1]
		}
	}
	return s
}

func (p *Profile) Bytes() []byte {
	if p == nil {
		return nil
	}
	return append([]byte(nil), p.raw...)
}
