package isobmff_test

import (
	"testing"

	"github.com/oops1/PixGo/internal/isobmff"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

func infeItem(id uint32, kind string) isotest.Item {
	return isotest.Item{ID: id, Type: kind, Data: []byte{1, 2, 3}}
}

func headOf(f isotest.File) []byte {
	data := f.Bytes()
	if len(data) > 512 {
		return data[:512]
	}
	return data
}

func TestClassifyDecidesByBrandsAndItemTypes(t *testing.T) {
	filler := make([]isotest.Item, 0, 80)
	for i := 0; i < 80; i++ {
		filler = append(filler, infeItem(uint32(100+i), "Exif"))
	}
	tests := []struct {
		name string
		file isotest.File
		want isobmff.Family
	}{
		{"avif major brand", isotest.File{Major: "avif", Compatible: []string{"mif1", "miaf"}}, isobmff.FamilyAVIF},
		{"avis major brand", isotest.File{Major: "avis", Compatible: []string{"msf1"}}, isobmff.FamilyAVIF},
		{"generic major with avif compatible", isotest.File{Major: "mif1", Compatible: []string{"avif", "miaf"}}, isobmff.FamilyAVIF},
		{"AVIF profile brand alone", isotest.File{Major: "mif1", Compatible: []string{"MA1B"}}, isobmff.FamilyAVIF},
		{"heic major brand", isotest.File{Major: "heic", Compatible: []string{"mif1", "heic"}}, isobmff.FamilyHEIC},
		{"heix major brand", isotest.File{Major: "heix", Compatible: []string{"mif1"}}, isobmff.FamilyHEIC},
		{"hevc sequence brand", isotest.File{Major: "hevc", Compatible: []string{"msf1"}}, isobmff.FamilyHEIC},
		{"generic major with heic compatible", isotest.File{Major: "mif1", Compatible: []string{"heic", "miaf"}}, isobmff.FamilyHEIC},
		{"both families, major avif", isotest.File{Major: "avif", Compatible: []string{"heic"}}, isobmff.FamilyAVIF},
		{"both families, major heic", isotest.File{Major: "heic", Compatible: []string{"avif"}}, isobmff.FamilyHEIC},
		{"generic brands only without hints", isotest.File{Major: "mif1", Compatible: []string{"miaf"}}, isobmff.FamilyUndetermined},
		{"generic brands, av01 item decides", isotest.File{Major: "mif1", Compatible: []string{"miaf"}, Primary: 1, Items: []isotest.Item{infeItem(1, "av01")}}, isobmff.FamilyAVIF},
		{"generic brands, hvc1 item decides", isotest.File{Major: "mif1", Compatible: []string{"miaf"}, Primary: 1, Items: []isotest.Item{infeItem(1, "hvc1")}}, isobmff.FamilyHEIC},
		{"generic brands, hvc2 item decides", isotest.File{Major: "msf1", Compatible: []string{"miaf"}, Primary: 1, Items: []isotest.Item{infeItem(1, "hvc2")}}, isobmff.FamilyHEIC},
		{"generic brands, av1C property decides", isotest.File{Major: "mif1", Items: []isotest.Item{{ID: 1, Type: "grid", Data: []byte{0}, Props: []isotest.Prop{isotest.AV1C(false, false, false)}}}}, isobmff.FamilyAVIF},
		{"generic brands, hvcC property decides", isotest.File{Major: "mif1", Items: []isotest.Item{{ID: 1, Type: "grid", Data: []byte{0}, Props: []isotest.Prop{isotest.HVCC(1, 8)}}}}, isobmff.FamilyHEIC},
		{"generic brands, both kinds of item", isotest.File{Major: "mif1", Items: []isotest.Item{infeItem(1, "av01"), infeItem(2, "hvc1")}}, isobmff.FamilyUndetermined},
		{"generic brands, evidence beyond the window", isotest.File{Major: "mif1", Items: append(append([]isotest.Item{}, filler...), infeItem(1, "av01"))}, isobmff.FamilyUndetermined},
		{"mp4 file", isotest.File{Major: "isom", Compatible: []string{"mp42"}}, isobmff.FamilyNone},
		{"canon raw", isotest.File{Major: "crx ", Compatible: []string{"isom"}}, isobmff.FamilyNone},
		{"jxl brand", isotest.File{Major: "jxl "}, isobmff.FamilyNone},
		{"quicktime", isotest.File{Major: "qt  "}, isobmff.FamilyNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isobmff.Classify(headOf(tt.file)); got != tt.want {
				t.Fatalf("classified as %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClassifyRejectsHeadsThatAreNotFileTypeBoxes(t *testing.T) {
	valid := isotest.File{Major: "avif"}.Bytes()
	tests := []struct {
		name string
		head []byte
	}{
		{"empty", nil},
		{"too short", valid[:11]},
		{"png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")},
		{"box that is not ftyp", isotest.Box("free", []byte("avifavifavif"))},
		{"ftyp declared smaller than its fixed fields", isotest.Box("ftyp", []byte("avif"))},
		{"jxl container signature", []byte("\x00\x00\x00\x0cJXL \x0d\x0a\x87\x0a\x00\x00\x00\x14ftypjxl \x00\x00\x00\x00jxl ")},
		{"large ftyp truncated", append([]byte{0, 0, 0, 1, 'f', 't', 'y', 'p'}, 0, 0)},
		{"ftyp size below header", append([]byte{0, 0, 0, 12, 'f', 't', 'y', 'p'}, "avifavif"...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isobmff.Classify(tt.head); got != isobmff.FamilyNone {
				t.Fatalf("classified as %d", got)
			}
		})
	}
}

func TestClassifyAcceptsEveryBoxSizeForm(t *testing.T) {
	body := append([]byte("avif"), isotest.U32(0)...)
	body = append(body, "mif1"...)
	tests := map[string][]byte{
		"plain":      isotest.Box("ftyp", body),
		"large size": isotest.LargeBox("ftyp", body),
		"open ended": isotest.OpenEndedBox("ftyp", body),
	}
	for name, head := range tests {
		if got := isobmff.Classify(head); got != isobmff.FamilyAVIF {
			t.Fatalf("%s: classified as %d", name, got)
		}
	}
}

func TestClassifyTolerateTruncatedWindow(t *testing.T) {
	full := isotest.File{Major: "mif1", Compatible: []string{"miaf"}, Primary: 1, Items: []isotest.Item{infeItem(1, "hvc1")}}.Bytes()
	for cut := 1; cut < len(full); cut += 3 {
		got := isobmff.Classify(full[:cut])
		if got != isobmff.FamilyNone && got != isobmff.FamilyUndetermined && got != isobmff.FamilyHEIC {
			t.Fatalf("cut %d: classified as %d", cut, got)
		}
	}
}

func TestBrandsExposeMajorAndCompatibleBrands(t *testing.T) {
	head := isotest.File{Major: "heic", Minor: 7, Compatible: []string{"mif1", "miaf"}}.Bytes()
	brands, ok := isobmff.ReadBrands(head)
	if !ok || brands.Major != "heic" || brands.Minor != 7 || len(brands.Compatible) != 2 {
		t.Fatalf("%+v %v", brands, ok)
	}
	if !brands.Contains("heic") || !brands.Contains("miaf") || brands.Contains("avif") {
		t.Fatalf("contains: %+v", brands)
	}
	if all := brands.All(); len(all) != 3 || all[0] != "heic" {
		t.Fatalf("all: %v", all)
	}
	if brands.Family() != isobmff.FamilyHEIC {
		t.Fatal("family")
	}
	if _, ok := isobmff.ReadBrands([]byte("short")); ok {
		t.Fatal("short head accepted")
	}
}
