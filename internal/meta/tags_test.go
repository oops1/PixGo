package meta

import (
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

var le = enc{orders[0].order}
var be = enc{orders[1].order}

func readTIFFParts(t *testing.T, e enc, p tiffParts) *Metadata {
	t.Helper()
	return mustRead(t, e.tiff(p), codec.TIFF)
}

func TestGarbageTagsAreIgnored(t *testing.T) {
	for _, e := range []enc{le, be} {
		m := readTIFFParts(t, e, tiffParts{
			ifd0: []tentry{
				e.short(0x010f, 1, 2, 3),
				e.long(0x0110, 7),
				e.short(0x0112, 9),
				e.rational(0x0131, 1, 2),
			},
			exif: []tentry{
				e.ascii(0x829a, "fast"),
				e.long(0x829d, 4),
				e.ascii(0x8827, "100"),
				e.long(0x9209, 0x70000),
				e.rational(0x920a, 5, 0),
				e.ascii(0x9003, "yesterday"),
				e.short(0xa002, 1),
			},
			gps: []tentry{
				e.ascii(0x0002, "north"),
				e.ascii(0x0004, "east"),
			},
		})
		if m.GPS != nil {
			t.Fatalf("GPS from garbage: %+v", m.GPS)
		}
		x := m.Exif
		if x == nil {
			t.Fatal("FNumber stored as LONG is lenient and must survive")
		}
		if x.Make != "" || x.Model != "" || x.Software != "" || x.Orientation != 0 || x.ISO != 0 || x.Flash != nil {
			t.Fatalf("garbage leaked: %+v", *x)
		}
		if x.Exposure.Valid() || x.FocalLength.Valid() || !x.TakenAt.IsZero() || x.PixelWidth != 1 {
			t.Fatalf("garbage leaked: %+v", *x)
		}
	}
}

func TestZeroDenominatorsLeaveNoExif(t *testing.T) {
	for _, e := range []enc{le, be} {
		m := readTIFFParts(t, e, tiffParts{
			exif: []tentry{
				e.rational(0x829a, 1, 0),
				e.rational(0x829d, 0, 0),
				e.rational(0x920a, 24, 0),
			},
		})
		if m.Exif != nil {
			t.Fatalf("unexpected %+v", *m.Exif)
		}
	}
}

func TestEmptyMetadataYieldsNilSections(t *testing.T) {
	m := readTIFFParts(t, le, tiffParts{ifd0: []tentry{le.long(0x0100, 10), le.long(0x0101, 20)}})
	if m.Exif != nil || m.GPS != nil || m.ICC != nil || m.XMP != nil || m.Width != 10 || m.Height != 20 {
		t.Fatalf("%+v", m)
	}
}

func TestExposureStoredAsSignedRationalAndFocalAsShort(t *testing.T) {
	for _, e := range []enc{le, be} {
		m := readTIFFParts(t, e, tiffParts{exif: []tentry{
			e.srational(0x829a, 1, 8),
			e.srational(0x829d, 28, 10),
			e.short(0x920a, 50),
		}})
		x := m.Exif
		if x == nil || x.Exposure.String() != "1/8" || x.FNumber.FNumberString() != "2.8" || x.FocalLength.String() != "50" {
			t.Fatalf("%+v", x)
		}
	}
}

func TestExposureFoundInIFD0WhenExifDirectoryMissesIt(t *testing.T) {
	m := readTIFFParts(t, le, tiffParts{
		ifd0: []tentry{le.rational(0x829a, 1, 60)},
		exif: []tentry{le.short(0x8827, 200)},
	})
	if m.Exif == nil || m.Exif.Exposure.String() != "1/60" || m.Exif.ISO != 200 {
		t.Fatalf("%+v", m.Exif)
	}
}

func TestMakeFoundInExifDirectoryAsFallback(t *testing.T) {
	m := readTIFFParts(t, le, tiffParts{
		ifd0: []tentry{le.short(0x0112, 3)},
		exif: []tentry{le.ascii(0x010f, "Sony")},
	})
	if m.Exif == nil || m.Exif.Make != "Sony" || m.Exif.Orientation != codec.OrientRotate180 {
		t.Fatalf("%+v", m.Exif)
	}
}

func TestISOVariants(t *testing.T) {
	cases := []struct {
		name  string
		entry []tentry
		want  int
	}{
		{"short", []tentry{le.short(0x8827, 400)}, 400},
		{"long", []tentry{le.long(0x8827, 800)}, 800},
		{"two values", []tentry{le.short(0x8827, 640, 1)}, 640},
		{"overflow with ISOSpeed", []tentry{le.short(0x8827, 65535), le.long(0x8833, 51200)}, 51200},
		{"missing with ISOSpeed", []tentry{le.long(0x8833, 3200)}, 3200},
		{"overflow alone", []tentry{le.short(0x8827, 65535)}, 0},
		{"absurd", []tentry{le.long(0x8827, 0xfffffff0)}, 0},
	}
	for _, c := range cases {
		m := readTIFFParts(t, le, tiffParts{exif: append(c.entry, le.short(0xa001, 1))})
		if m.Exif == nil || m.Exif.ISO != c.want {
			t.Fatalf("%s: %+v", c.name, m.Exif)
		}
	}
}

func TestDecodeFlash(t *testing.T) {
	cases := []struct {
		code  uint32
		fired bool
		mode  FlashMode
		ret   FlashReturn
		red   bool
		none  bool
	}{
		{0x00, false, FlashModeUnknown, FlashReturnNone, false, false},
		{0x01, true, FlashModeUnknown, FlashReturnNone, false, false},
		{0x05, true, FlashModeUnknown, FlashReturnNotDetected, false, false},
		{0x07, true, FlashModeUnknown, FlashReturnDetected, false, false},
		{0x09, true, FlashModeCompulsoryOn, FlashReturnNone, false, false},
		{0x10, false, FlashModeCompulsoryOff, FlashReturnNone, false, false},
		{0x18, false, FlashModeAuto, FlashReturnNone, false, false},
		{0x19, true, FlashModeAuto, FlashReturnNone, false, false},
		{0x20, false, FlashModeUnknown, FlashReturnNone, false, true},
		{0x41, true, FlashModeUnknown, FlashReturnNone, true, false},
		{0x5f, true, FlashModeAuto, FlashReturnDetected, true, false},
	}
	for _, c := range cases {
		f := decodeFlash(c.code)
		if f.Code != int(c.code) || f.Fired != c.fired || f.Mode != c.mode || f.Return != c.ret || f.RedEye != c.red || f.NoFunction != c.none {
			t.Fatalf("%#x: %+v", c.code, *f)
		}
	}
	m := readTIFFParts(t, le, tiffParts{exif: []tentry{le.short(0x9209, 0)}})
	if m.Exif == nil || m.Exif.Flash == nil || m.Exif.Flash.Fired {
		t.Fatalf("flash 0 must be reported as present and not fired: %+v", m.Exif)
	}
}

func TestParseExifTime(t *testing.T) {
	valid := map[string]time.Time{
		"2024:09:14 18:27:03":     time.Date(2024, 9, 14, 18, 27, 3, 0, time.UTC),
		"2024-09-14 18:27:03":     time.Date(2024, 9, 14, 18, 27, 3, 0, time.UTC),
		"2024-09-14T18:27:03":     time.Date(2024, 9, 14, 18, 27, 3, 0, time.UTC),
		"  2024:02:29 00:00:00  ": time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
		"1999:12:31 23:59:59":     time.Date(1999, 12, 31, 23, 59, 59, 0, time.UTC),
	}
	for in, want := range valid {
		got, ok := parseExifTime(in)
		if !ok || !got.Equal(want) {
			t.Fatalf("%q: %v %v", in, got, ok)
		}
	}
	for _, in := range []string{
		"", "   ", "0000:00:00 00:00:00", "    :  :     :  :  ", "2024:02:30 10:00:00",
		"2023:02:29 10:00:00", "2024:13:01 10:00:00", "2024:00:10 10:00:00", "2024:01:00 10:00:00",
		"2024:01:01 24:00:00", "2024:01:01 10:60:00", "2024:01:01 10:00:60", "2024:09:14",
		"2024:09:14 18:27", "abcd:ef:gh ij:kl:mn", "2024.09.14 18:27:03", "2024:09:14x18:27:03",
	} {
		if got, ok := parseExifTime(in); ok {
			t.Fatalf("%q accepted as %v", in, got)
		}
	}
}

func TestTakenAtFallbacksAndAbsence(t *testing.T) {
	m := readTIFFParts(t, le, tiffParts{
		ifd0: []tentry{le.ascii(0x0132, "2020:01:02 03:04:05")},
		exif: []tentry{le.ascii(0x9003, "0000:00:00 00:00:00")},
	})
	want := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if m.Exif == nil || !m.Exif.TakenAt.Equal(want) || m.Exif.HasTakenZone {
		t.Fatalf("%+v", m.Exif)
	}
	m = readTIFFParts(t, le, tiffParts{
		ifd0: []tentry{le.ascii(0x0132, "    :  :     :  :  ")},
		exif: []tentry{le.ascii(0x9004, "2021:05:06 07:08:09"), le.ascii(0x9012, "-05:30")},
	})
	if m.Exif == nil || !m.Exif.HasTakenZone || !m.Exif.TakenAt.Equal(time.Date(2021, 5, 6, 12, 38, 9, 0, time.UTC)) {
		t.Fatalf("digitized fallback: %+v", m.Exif)
	}
	m = readTIFFParts(t, le, tiffParts{exif: []tentry{le.short(0x8827, 100), le.ascii(0x9003, "    :  :     :  :  ")}})
	if m.Exif == nil || !m.Exif.TakenAt.IsZero() || m.Exif.HasTakenZone {
		t.Fatalf("blank date must be absent: %+v", m.Exif)
	}
}

func TestTakenAtZoneAndSubSeconds(t *testing.T) {
	cases := []struct {
		name    string
		extra   []tentry
		hasZone bool
		offset  int
		nanos   int
	}{
		{"none", nil, false, 0, 0},
		{"east", []tentry{le.ascii(0x9011, "+05:30")}, true, 5*3600 + 1800, 0},
		{"west", []tentry{le.ascii(0x9011, "-05:00")}, true, -5 * 3600, 0},
		{"garbage zone", []tentry{le.ascii(0x9011, "MSK")}, false, 0, 0},
		{"huge zone", []tentry{le.ascii(0x9011, "+23:00")}, false, 0, 0},
		{"bad minutes", []tentry{le.ascii(0x9011, "+03:75")}, false, 0, 0},
		{"subsec short", []tentry{le.ascii(0x9291, "5")}, false, 0, 500_000_000},
		{"subsec long", []tentry{le.ascii(0x9291, "123456")}, false, 0, 123_456_000},
		{"subsec bad", []tentry{le.ascii(0x9291, "x1")}, false, 0, 0},
		{"subsec too long", []tentry{le.ascii(0x9291, "1234567890")}, false, 0, 0},
	}
	for _, c := range cases {
		entries := append([]tentry{le.ascii(0x9003, "2024:09:14 18:27:03")}, c.extra...)
		m := readTIFFParts(t, le, tiffParts{exif: entries})
		x := m.Exif
		if x == nil || x.HasTakenZone != c.hasZone {
			t.Fatalf("%s: %+v", c.name, x)
		}
		_, off := x.TakenAt.Zone()
		if off != c.offset || x.TakenAt.Nanosecond() != c.nanos {
			t.Fatalf("%s: offset %d nanos %d", c.name, off, x.TakenAt.Nanosecond())
		}
		wall := time.Date(2024, 9, 14, 18, 27, 3, c.nanos, time.UTC)
		got := time.Date(x.TakenAt.Year(), x.TakenAt.Month(), x.TakenAt.Day(), x.TakenAt.Hour(), x.TakenAt.Minute(), x.TakenAt.Second(), x.TakenAt.Nanosecond(), time.UTC)
		if !got.Equal(wall) {
			t.Fatalf("%s: wall clock %v", c.name, x.TakenAt)
		}
	}
}

func TestDecodeText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Canon", "Canon"},
		{"NUL padded", "Canon\x00\x00\x00\x00", "Canon"},
		{"NUL then junk", "Canon\x00garbage", "Canon"},
		{"spaces", "  Canon EOS  \x00", "Canon EOS"},
		{"latin1", "Fuj\xedfilm", "Fujífilm"},
		{"windows1252", "Ivan\x92s \x93lens\x94 \x80", "Ivan’s “lens” €"},
		{"undefined 1252", "a\x81b", "ab"},
		{"valid utf8", "Canön ЕОS", "Canön ЕОS"},
		{"controls", "a\x01b\tc\nd", "ab c d"},
		{"blank", " \x00 ", ""},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := decodeText([]byte(c.in)); got != c.want {
			t.Fatalf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	long := make([]byte, 100000)
	for i := range long {
		long[i] = 'x'
	}
	if got := decodeText(long); len(got) != maxTextBytes {
		t.Fatalf("long text %d bytes", len(got))
	}
}

func TestNonUTF8AndUnterminatedASCIIThroughRead(t *testing.T) {
	m := readTIFFParts(t, be, tiffParts{ifd0: []tentry{
		be.asciiRaw(0x010f, []byte("Fuj\xedfilm\x00\x00")),
		be.asciiRaw(0x0110, []byte("X-T5")),
		be.bytesVal(0x0131, []byte("Capture One\x00")...),
	}})
	x := m.Exif
	if x == nil || x.Make != "Fujífilm" || x.Model != "X-T5" || x.Software != "Capture One" {
		t.Fatalf("%+v", x)
	}
}

func TestGPSHemispheresAndAltitude(t *testing.T) {
	cases := []struct {
		name     string
		entries  []tentry
		lat, lon float64
		alt      float64
		hasAlt   bool
	}{
		{"south west", []tentry{
			le.ascii(0x0001, "S"), le.rational(0x0002, 33, 1, 51, 1, 0, 1),
			le.ascii(0x0003, "W"), le.rational(0x0004, 151, 1, 12, 1, 0, 1),
		}, -(33 + 51.0/60), -(151 + 12.0/60), 0, false},
		{"missing refs are positive", []tentry{
			le.rational(0x0002, 10, 1, 30, 1, 0, 1), le.rational(0x0004, 20, 1, 0, 1, 36, 1),
		}, 10.5, 20 + 36.0/3600, 0, false},
		{"decimal degrees", []tentry{
			le.ascii(0x0001, "N"), le.rational(0x0002, 5575586, 100000),
			le.ascii(0x0003, "E"), le.rational(0x0004, 3761736, 100000),
		}, 55.75586, 37.61736, 0, false},
		{"degrees and minutes", []tentry{
			le.ascii(0x0001, "N"), le.rational(0x0002, 40, 1, 3030, 100),
			le.ascii(0x0003, "W"), le.rational(0x0004, 74, 1, 0, 1),
		}, 40 + 30.3/60, -74, 0, false},
		{"below sea level", []tentry{
			le.ascii(0x0001, "N"), le.rational(0x0002, 31, 1, 30, 1, 0, 1),
			le.ascii(0x0003, "E"), le.rational(0x0004, 35, 1, 30, 1, 0, 1),
			le.bytesVal(0x0005, 1), le.rational(0x0006, 4305, 10),
		}, 31.5, 35.5, -430.5, true},
		{"altitude ref missing", []tentry{
			le.ascii(0x0001, "N"), le.rational(0x0002, 1, 1, 0, 1, 0, 1),
			le.ascii(0x0003, "E"), le.rational(0x0004, 2, 1, 0, 1, 0, 1),
			le.rational(0x0006, 12, 1),
		}, 1, 2, 12, true},
		{"signed altitude without ref", []tentry{
			le.ascii(0x0001, "N"), le.rational(0x0002, 1, 1, 0, 1, 0, 1),
			le.ascii(0x0003, "E"), le.rational(0x0004, 2, 1, 0, 1, 0, 1),
			le.srational(0x0006, -9, 2),
		}, 1, 2, -4.5, true},
		{"zero altitude denominator", []tentry{
			le.ascii(0x0001, "N"), le.rational(0x0002, 1, 1, 0, 1, 0, 1),
			le.ascii(0x0003, "E"), le.rational(0x0004, 2, 1, 0, 1, 0, 1),
			le.rational(0x0006, 12, 0),
		}, 1, 2, 0, false},
		{"garbage ref letters", []tentry{
			le.ascii(0x0001, "?"), le.rational(0x0002, 1, 1, 0, 1, 0, 1),
			le.ascii(0x0003, "x"), le.rational(0x0004, 2, 1, 0, 1, 0, 1),
		}, 1, 2, 0, false},
	}
	for _, c := range cases {
		for _, e := range []enc{le, be} {
			swapped := make([]tentry, len(c.entries))
			for i, en := range c.entries {
				swapped[i] = reencode(e, en)
			}
			m := readTIFFParts(t, e, tiffParts{gps: swapped})
			g := m.GPS
			if g == nil || !approx(g.Lat, c.lat) || !approx(g.Lon, c.lon) || g.HasAlt != c.hasAlt || (c.hasAlt && !approx(g.Alt, c.alt)) {
				t.Fatalf("%s: %+v", c.name, g)
			}
			if !g.Time.IsZero() {
				t.Fatalf("%s: time without stamps %v", c.name, g.Time)
			}
		}
	}
}

func reencode(e enc, en tentry) tentry {
	if e.o == le.o {
		return en
	}
	out := en
	out.data = append([]byte(nil), en.data...)
	switch en.typ {
	case 3:
		for i := 0; i+2 <= len(out.data); i += 2 {
			out.data[i], out.data[i+1] = out.data[i+1], out.data[i]
		}
	case 4, 5, 9, 10:
		for i := 0; i+4 <= len(out.data); i += 4 {
			out.data[i], out.data[i+1], out.data[i+2], out.data[i+3] = out.data[i+3], out.data[i+2], out.data[i+1], out.data[i]
		}
	}
	return out
}

func TestGPSRejectsInvalidPositions(t *testing.T) {
	good := func() []tentry {
		return []tentry{
			le.ascii(0x0001, "N"), le.rational(0x0002, 55, 1, 45, 1, 0, 1),
			le.ascii(0x0003, "E"), le.rational(0x0004, 37, 1, 37, 1, 0, 1),
		}
	}
	if m := readTIFFParts(t, le, tiffParts{gps: good()}); m.GPS == nil {
		t.Fatal("control sample must parse")
	}
	cases := map[string][]tentry{
		"latitude over 90":      {le.ascii(0x0001, "N"), le.rational(0x0002, 91, 1, 0, 1, 0, 1), le.ascii(0x0003, "E"), le.rational(0x0004, 37, 1, 0, 1, 0, 1)},
		"longitude over 180":    {le.ascii(0x0001, "N"), le.rational(0x0002, 55, 1, 0, 1, 0, 1), le.ascii(0x0003, "E"), le.rational(0x0004, 181, 1, 0, 1, 0, 1)},
		"zero denominator":      {le.ascii(0x0001, "N"), le.rational(0x0002, 55, 1, 45, 0, 0, 1), le.ascii(0x0003, "E"), le.rational(0x0004, 37, 1, 0, 1, 0, 1)},
		"missing longitude":     {le.ascii(0x0001, "N"), le.rational(0x0002, 55, 1, 0, 1, 0, 1)},
		"missing latitude":      {le.ascii(0x0003, "E"), le.rational(0x0004, 37, 1, 0, 1, 0, 1)},
		"four components":       {le.rational(0x0002, 1, 1, 2, 1, 3, 1, 4, 1), le.rational(0x0004, 1, 1, 2, 1, 3, 1)},
		"wrong type":            {le.short(0x0002, 1, 2, 3), le.short(0x0004, 1, 2, 3)},
		"null island":           {le.ascii(0x0001, "N"), le.rational(0x0002, 0, 1, 0, 1, 0, 1), le.ascii(0x0003, "E"), le.rational(0x0004, 0, 1, 0, 1, 0, 1)},
		"empty":                 {},
		"only altitude present": {le.rational(0x0006, 100, 1)},
	}
	for name, entries := range cases {
		m := readTIFFParts(t, le, tiffParts{ifd0: []tentry{le.ascii(0x010f, "X")}, gps: append([]tentry{le.bytesVal(0x0000, 2, 3, 0, 0)}, entries...)})
		if m.GPS != nil {
			t.Fatalf("%s: %+v", name, m.GPS)
		}
	}
}

func TestGPSTimeStampValidation(t *testing.T) {
	base := []tentry{
		le.ascii(0x0001, "N"), le.rational(0x0002, 1, 1, 0, 1, 0, 1),
		le.ascii(0x0003, "E"), le.rational(0x0004, 2, 1, 0, 1, 0, 1),
	}
	cases := []struct {
		name  string
		extra []tentry
		want  time.Time
	}{
		{"valid", []tentry{le.rational(0x0007, 15, 1, 27, 1, 3, 1), le.ascii(0x001d, "2024:09:14")}, time.Date(2024, 9, 14, 15, 27, 3, 0, time.UTC)},
		{"fractional seconds", []tentry{le.rational(0x0007, 1, 1, 2, 1, 35, 10), le.ascii(0x001d, "2024:01:01")}, time.Date(2024, 1, 1, 1, 2, 3, 500_000_000, time.UTC)},
		{"no date", []tentry{le.rational(0x0007, 15, 1, 27, 1, 3, 1)}, time.Time{}},
		{"no time", []tentry{le.ascii(0x001d, "2024:09:14")}, time.Time{}},
		{"zero denominator", []tentry{le.rational(0x0007, 15, 0, 27, 1, 3, 1), le.ascii(0x001d, "2024:09:14")}, time.Time{}},
		{"hour 25", []tentry{le.rational(0x0007, 25, 1, 0, 1, 0, 1), le.ascii(0x001d, "2024:09:14")}, time.Time{}},
		{"bad date", []tentry{le.rational(0x0007, 1, 1, 0, 1, 0, 1), le.ascii(0x001d, "2024:02:31")}, time.Time{}},
		{"short date", []tentry{le.rational(0x0007, 1, 1, 0, 1, 0, 1), le.ascii(0x001d, "2024")}, time.Time{}},
	}
	for _, c := range cases {
		m := readTIFFParts(t, le, tiffParts{gps: append(append([]tentry(nil), base...), c.extra...)})
		if m.GPS == nil || !m.GPS.Time.Equal(c.want) {
			t.Fatalf("%s: %+v", c.name, m.GPS)
		}
	}
}
