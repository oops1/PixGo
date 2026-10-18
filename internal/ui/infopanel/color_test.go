package infopanel

import (
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/colorm"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/meta"
)

func colorRows(m Model) map[string]string {
	rows := map[string]string{}
	for _, c := range m.Cards {
		if c.Kind != CardColor {
			continue
		}
		for _, r := range c.Rows {
			rows[r.Label] = r.Value
		}
	}
	return rows
}

func TestColorCardShowsProfileHDRAndDepth(t *testing.T) {
	i18n.SetLang("ru")
	t.Cleanup(func() { i18n.SetLang("ru") })
	in := Input{Path: "a.avif", Info: codec.Info{Width: 10, Height: 10, BitDepth: 10}}
	in.Color = colorm.Report{Managed: true, Profile: "Display P3", Origin: colorm.OriginEmbedded}
	rows := colorRows(BuildModel(in))
	if rows["Цветовой профиль"] != "Display P3 (встроенный)" {
		t.Fatalf("%v", rows)
	}
	in.Color = colorm.Report{Managed: true, Profile: "BT.2020 / PQ", Origin: colorm.OriginCICP, HDR: "PQ", ToneMapped: true, Deep: true, Converted: true, Display: "Monitor"}
	rows = colorRows(BuildModel(in))
	if rows["HDR"] != "PQ, тональная компрессия" || rows["Глубина"] != "10 бит" || rows["Цветовой профиль"] != "BT.2020 / PQ (CICP)" || rows["Профиль экрана"] != "Monitor" {
		t.Fatalf("%v", rows)
	}
	i18n.SetLang("en")
	rows = colorRows(BuildModel(in))
	if rows["HDR"] != "PQ, tone mapped" || rows["Bit depth"] != "10 bit" {
		t.Fatalf("%v", rows)
	}
}

func TestColorCardWithoutManagementKeepsTheOldRows(t *testing.T) {
	i18n.SetLang("en")
	md := &meta.Metadata{ICC: &meta.ICC{Description: "sRGB IEC61966-2.1", ColorSpace: "RGB"}}
	rows := colorRows(BuildModel(Input{Path: "a.jpg", Meta: md}))
	if rows["ICC profile"] != "sRGB IEC61966-2.1" || rows["Color space"] != "RGB" || len(rows) != 2 {
		t.Fatalf("%v", rows)
	}
	managed := colorRows(BuildModel(Input{Path: "a.jpg", Meta: md, Color: colorm.Report{Managed: true, Profile: "sRGB IEC61966-2.1", Origin: colorm.OriginEmbedded}}))
	if managed["ICC profile"] != "" || managed["Color profile"] != "sRGB IEC61966-2.1 (embedded)" || managed["Color space"] != "RGB" {
		t.Fatalf("%v", managed)
	}
	if len(colorRows(BuildModel(Input{Path: "a.png", Color: colorm.Report{Managed: true}}))) != 0 {
		t.Fatal("an untagged picture has no colour card")
	}
}
