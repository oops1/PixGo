package meta

import (
	"math"
	"strings"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	tagMake                uint16 = 0x010f
	tagModel               uint16 = 0x0110
	tagSoftware            uint16 = 0x0131
	tagDateTime            uint16 = 0x0132
	tagExposureTime        uint16 = 0x829a
	tagFNumber             uint16 = 0x829d
	tagExposureProgram     uint16 = 0x8822
	tagISO                 uint16 = 0x8827
	tagISOSpeed            uint16 = 0x8833
	tagOffsetTime          uint16 = 0x9010
	tagOffsetTimeOriginal  uint16 = 0x9011
	tagOffsetTimeDigitized uint16 = 0x9012
	tagDateTimeOriginal    uint16 = 0x9003
	tagDateTimeDigitized   uint16 = 0x9004
	tagMeteringMode        uint16 = 0x9207
	tagFlash               uint16 = 0x9209
	tagFocalLength         uint16 = 0x920a
	tagSubSecTime          uint16 = 0x9290
	tagSubSecTimeOriginal  uint16 = 0x9291
	tagSubSecTimeDigitized uint16 = 0x9292
	tagColorSpace          uint16 = 0xa001
	tagPixelXDimension     uint16 = 0xa002
	tagPixelYDimension     uint16 = 0xa003
	tagWhiteBalance        uint16 = 0xa403
	tagFocalLengthIn35mm   uint16 = 0xa405
	tagLensModel           uint16 = 0xa434
	gpsTagLatitudeRef      uint16 = 0x0001
	gpsTagLatitude         uint16 = 0x0002
	gpsTagLongitudeRef     uint16 = 0x0003
	gpsTagLongitude        uint16 = 0x0004
	gpsTagAltitudeRef      uint16 = 0x0005
	gpsTagAltitude         uint16 = 0x0006
	gpsTagTimeStamp        uint16 = 0x0007
	gpsTagDateStamp        uint16 = 0x001d
	maxISO                        = 1 << 24
	isoOverflow                   = 0xffff
	maxAbsAltitude                = 1e7
	maxTagTextBytes               = 1 << 12
	maxZoneOffset                 = 18 * time.Hour
)

type tagSet struct{ dirs [2]*exif.Directory }

func (s tagSet) find(tag uint16) (exif.Entry, bool) {
	for _, d := range s.dirs {
		if e, ok := d.Find(tag); ok {
			return e, true
		}
	}
	return exif.Entry{}, false
}

func (s tagSet) text(tag uint16) string {
	e, ok := s.find(tag)
	if !ok || !byteLike(e) {
		return ""
	}
	raw, ok := e.RawMax(maxTagTextBytes)
	if !ok {
		return ""
	}
	return decodeText(raw)
}

func (s tagSet) uint(tag uint16) (uint32, bool) {
	e, ok := s.find(tag)
	if !ok {
		return 0, false
	}
	if v, ok := e.Uint(0); ok {
		return v, true
	}
	if v, ok := e.Int(0); ok && v >= 0 {
		return uint32(v), true
	}
	return 0, false
}

func (s tagSet) number(tag uint16, limit int) int {
	v, ok := s.uint(tag)
	if !ok || uint64(v) > uint64(limit) {
		return 0
	}
	return int(v)
}

func (s tagSet) rational(tag uint16) Rational {
	e, ok := s.find(tag)
	if !ok {
		return Rational{}
	}
	r, _ := entryRational(e, 0)
	return r
}

func entryRational(e exif.Entry, index int) (Rational, bool) {
	switch e.Type {
	case exif.TypeShort, exif.TypeLong, exif.TypeByte:
		v, ok := e.Uint(index)
		return Rational{Num: int64(v), Den: 1}, ok
	}
	return rationalOnly(e, index)
}

func rationalOnly(e exif.Entry, index int) (Rational, bool) {
	switch e.Type {
	case exif.TypeRational:
		n, d, ok := e.Rational(index)
		return Rational{Num: int64(n), Den: int64(d)}, ok
	case exif.TypeSRational:
		n, d, ok := e.SRational(index)
		return Rational{Num: int64(n), Den: int64(d)}, ok
	}
	return Rational{}, false
}

func extractTags(tree exif.Tree) (*Exif, *GPS) {
	var e *Exif
	if tree.IFD0 != nil {
		e = extractExif(tree)
	}
	return e, extractGPS(tree.GPS)
}

func extractExif(tree exif.Tree) *Exif {
	primary := tagSet{dirs: [2]*exif.Directory{tree.IFD0, tree.Exif}}
	detail := tagSet{dirs: [2]*exif.Directory{tree.Exif, tree.IFD0}}
	x := &Exif{
		Make:            primary.text(tagMake),
		Model:           primary.text(tagModel),
		Software:        primary.text(tagSoftware),
		Lens:            detail.text(tagLensModel),
		Exposure:        detail.rational(tagExposureTime),
		FNumber:         detail.rational(tagFNumber),
		FocalLength:     detail.rational(tagFocalLength),
		FocalLength35:   detail.number(tagFocalLengthIn35mm, math.MaxUint16),
		ExposureProgram: detail.number(tagExposureProgram, math.MaxUint16),
		MeteringMode:    detail.number(tagMeteringMode, math.MaxUint16),
		WhiteBalance:    detail.number(tagWhiteBalance, math.MaxUint16),
		ColorSpace:      detail.number(tagColorSpace, math.MaxUint16),
		PixelWidth:      detail.number(tagPixelXDimension, maxDimension),
		PixelHeight:     detail.number(tagPixelYDimension, maxDimension),
	}
	if !x.Exposure.Valid() {
		x.Exposure = Rational{}
	}
	if !x.FNumber.Valid() {
		x.FNumber = Rational{}
	}
	if !x.FocalLength.Valid() {
		x.FocalLength = Rational{}
	}
	x.ISO = isoValue(detail)
	if v, ok := detail.uint(tagFlash); ok && v <= math.MaxUint16 {
		x.Flash = decodeFlash(v)
	}
	if v, ok := tree.IFD0.Find(exif.TagOrientation); ok {
		if n, ok := v.Uint(0); ok && n >= uint32(codec.OrientNormal) && n <= uint32(codec.OrientRotate270) {
			x.Orientation = codec.Orientation(n)
		}
	}
	x.TakenAt, x.HasTakenZone = takenAt(detail)
	if *x == (Exif{}) {
		return nil
	}
	return x
}

func isoValue(s tagSet) int {
	iso := s.number(tagISO, maxISO)
	if iso == 0 || iso == isoOverflow {
		if alt := s.number(tagISOSpeed, maxISO); alt > 0 {
			return alt
		}
	}
	if iso == isoOverflow {
		return 0
	}
	return iso
}

func decodeFlash(code uint32) *Flash {
	f := &Flash{
		Code:       int(code),
		Fired:      code&1 != 0,
		RedEye:     code&0x40 != 0,
		NoFunction: code&0x20 != 0,
	}
	switch code >> 1 & 3 {
	case 2:
		f.Return = FlashReturnNotDetected
	case 3:
		f.Return = FlashReturnDetected
	}
	switch code >> 3 & 3 {
	case 1:
		f.Mode = FlashModeCompulsoryOn
	case 2:
		f.Mode = FlashModeCompulsoryOff
	case 3:
		f.Mode = FlashModeAuto
	}
	return f
}

type timeTags struct{ stamp, subSec, offset uint16 }

var timeSources = [...]timeTags{
	{tagDateTimeOriginal, tagSubSecTimeOriginal, tagOffsetTimeOriginal},
	{tagDateTimeDigitized, tagSubSecTimeDigitized, tagOffsetTimeDigitized},
	{tagDateTime, tagSubSecTime, tagOffsetTime},
}

func takenAt(s tagSet) (time.Time, bool) {
	for _, src := range timeSources {
		t, ok := parseExifTime(s.text(src.stamp))
		if !ok {
			continue
		}
		if nanos, ok := parseSubSec(s.text(src.subSec)); ok {
			t = t.Add(time.Duration(nanos))
		}
		if zone, ok := parseZoneOffset(s.text(src.offset)); ok {
			loc := time.FixedZone("", int(zone/time.Second))
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
			return t, true
		}
		return t, false
	}
	return time.Time{}, false
}

func digits(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

func parseExifTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 19 {
		return time.Time{}, false
	}
	isDateSep := func(c byte) bool { return c == ':' || c == '-' || c == '/' }
	if !isDateSep(s[4]) || !isDateSep(s[7]) || (s[10] != ' ' && s[10] != 'T') || s[13] != ':' || s[16] != ':' {
		return time.Time{}, false
	}
	var v [6]int
	for i, span := range [6][2]int{{0, 4}, {5, 7}, {8, 10}, {11, 13}, {14, 16}, {17, 19}} {
		n, ok := digits(s[span[0]:span[1]])
		if !ok {
			return time.Time{}, false
		}
		v[i] = n
	}
	if v[0] == 0 || v[1] < 1 || v[1] > 12 || v[2] < 1 || v[2] > 31 || v[3] > 23 || v[4] > 59 || v[5] > 59 {
		return time.Time{}, false
	}
	t := time.Date(v[0], time.Month(v[1]), v[2], v[3], v[4], v[5], 0, time.UTC)
	if t.Day() != v[2] {
		return time.Time{}, false
	}
	return t, true
}

func parseSubSec(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 9 {
		return 0, false
	}
	n, ok := digits(s)
	if !ok {
		return 0, false
	}
	nanos := int64(n)
	for i := len(s); i < 9; i++ {
		nanos *= 10
	}
	return nanos, true
}

func parseZoneOffset(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if len(s) != 6 || (s[0] != '+' && s[0] != '-') || s[3] != ':' {
		return 0, false
	}
	h, okH := digits(s[1:3])
	m, okM := digits(s[4:6])
	if !okH || !okM || m > 59 {
		return 0, false
	}
	d := time.Duration(h)*time.Hour + time.Duration(m)*time.Minute
	if d > maxZoneOffset {
		return 0, false
	}
	if s[0] == '-' {
		d = -d
	}
	return d, true
}

func gpsCoordinate(dir *exif.Directory, valueTag, refTag uint16, limit float64) (float64, bool) {
	e, ok := dir.Find(valueTag)
	if !ok || e.Count < 1 || e.Count > 3 {
		return 0, false
	}
	var value float64
	scale := 1.0
	for i := 0; i < int(e.Count); i++ {
		r, ok := rationalOnly(e, i)
		if !ok || !r.Valid() {
			return 0, false
		}
		value += r.Float() / scale
		scale *= 60
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	if ref, ok := dir.Find(refTag); ok && byteLike(ref) {
		if raw, ok := ref.RawMax(maxTagTextBytes); ok && len(raw) > 0 {
			switch raw[0] {
			case 'S', 's', 'W', 'w':
				value = -math.Abs(value)
			}
		}
	}
	if math.Abs(value) > limit {
		return 0, false
	}
	return value, true
}

func extractGPS(dir *exif.Directory) *GPS {
	if dir == nil {
		return nil
	}
	lat, ok := gpsCoordinate(dir, gpsTagLatitude, gpsTagLatitudeRef, 90)
	if !ok {
		return nil
	}
	lon, ok := gpsCoordinate(dir, gpsTagLongitude, gpsTagLongitudeRef, 180)
	if !ok {
		return nil
	}
	if lat == 0 && lon == 0 {
		return nil
	}
	g := &GPS{Lat: lat, Lon: lon}
	if e, ok := dir.Find(gpsTagAltitude); ok {
		if r, ok := rationalOnly(e, 0); ok && r.Valid() {
			alt := math.Abs(r.Float())
			if ref, ok := dir.Find(gpsTagAltitudeRef); ok {
				if v, ok := ref.Uint(0); ok && v == 1 {
					alt = -alt
				}
			} else if r.Float() < 0 {
				alt = -alt
			}
			if alt <= maxAbsAltitude && alt >= -maxAbsAltitude {
				g.Alt, g.HasAlt = alt, true
			}
		}
	}
	g.Time = gpsTime(dir)
	return g
}

func gpsTime(dir *exif.Directory) time.Time {
	stamp, ok := dir.Find(gpsTagTimeStamp)
	if !ok || stamp.Count != 3 {
		return time.Time{}
	}
	var v [3]float64
	for i := range v {
		r, ok := rationalOnly(stamp, i)
		if !ok || !r.Valid() {
			return time.Time{}
		}
		v[i] = r.Float()
	}
	if v[0] < 0 || v[0] >= 24 || v[1] < 0 || v[1] >= 60 || v[2] < 0 || v[2] >= 61 {
		return time.Time{}
	}
	dateEntry, ok := dir.Find(gpsTagDateStamp)
	if !ok || !byteLike(dateEntry) {
		return time.Time{}
	}
	raw, ok := dateEntry.RawMax(maxTagTextBytes)
	if !ok {
		return time.Time{}
	}
	date := decodeText(raw)
	if len(date) != 10 {
		return time.Time{}
	}
	y, okY := digits(date[0:4])
	mo, okM := digits(date[5:7])
	d, okD := digits(date[8:10])
	if !okY || !okM || !okD || y == 0 || mo < 1 || mo > 12 || d < 1 || d > 31 {
		return time.Time{}
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	if t.Day() != d {
		return time.Time{}
	}
	secs := time.Duration(v[0])*time.Hour + time.Duration(v[1])*time.Minute + time.Duration(v[2]*float64(time.Second))
	return t.Add(secs)
}
