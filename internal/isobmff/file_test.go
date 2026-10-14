package isobmff_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/isobmff"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

const (
	alphaURN = "urn:mpeg:mpegB:cicp:systems:auxiliary:alpha"
	payload  = "0123456789abcdef"
)

func av1Still(width, height uint32, extra ...isotest.Prop) isotest.File {
	props := append([]isotest.Prop{isotest.Ispe(width, height), isotest.AV1C(false, false, false), isotest.Pixi(8, 8, 8)}, extra...)
	return isotest.File{
		Major:      "avif",
		Compatible: []string{"avif", "mif1", "miaf"},
		Primary:    1,
		Items:      []isotest.Item{{ID: 1, Type: "av01", Data: []byte(payload), Props: props}},
	}
}

func open(t testing.TB, f isotest.File) *isobmff.File {
	t.Helper()
	data := f.Bytes()
	file, err := isobmff.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func openBytes(data []byte) (*isobmff.File, error) {
	return isobmff.Open(context.Background(), bytes.NewReader(data), int64(len(data)))
}

func requireKind(t *testing.T, err, kind error) {
	t.Helper()
	if !errors.Is(err, kind) {
		t.Fatalf("error %v is not %v", err, kind)
	}
}

func TestOpenReadsBrandsItemsAndProperties(t *testing.T) {
	file := open(t, av1Still(33, 21, isotest.Colr(), isotest.Irot(1)))
	if file.Brands.Major != "avif" || !file.HasMeta || file.Mini || len(file.Tracks) != 0 {
		t.Fatalf("%+v", file.Brands)
	}
	item := file.PrimaryItem()
	if item == nil || item.ID != 1 || item.Type != "av01" || item.Hidden {
		t.Fatalf("primary %+v", item)
	}
	if got := file.FirstItem("grid", "av01"); got != item {
		t.Fatalf("first item %+v", got)
	}
	if file.FirstItem("hvc1") != nil {
		t.Fatal("unexpected hvc1 item")
	}
	ispe := file.Property(item, "ispe")
	if ispe == nil || ispe.Width != 33 || ispe.Height != 21 {
		t.Fatalf("ispe %+v", ispe)
	}
	if av1c := file.Property(item, "av1C"); av1c == nil || av1c.AV1.BitDepth() != 8 {
		t.Fatalf("av1C %+v", av1c)
	}
	if irot := file.Property(item, "irot"); irot == nil || irot.Angle != 1 {
		t.Fatalf("irot %+v", irot)
	}
	if file.Property(item, "clap") != nil || file.Property(nil, "ispe") != nil {
		t.Fatal("phantom property")
	}
	data, err := file.ReadItem(item, 1<<20)
	if err != nil || string(data) != payload {
		t.Fatalf("data %q %v", data, err)
	}
	if file.Size() != int64(len(av1Still(33, 21, isotest.Colr(), isotest.Irot(1)).Bytes())) {
		t.Fatal("size")
	}
}

func TestOpenParsesItemFlagsAndContentTypes(t *testing.T) {
	f := av1Still(2, 2)
	f.Items = append(f.Items,
		isotest.Item{ID: 2, Type: "Exif", Data: []byte("Exif"), Hidden: true},
		isotest.Item{ID: 3, Type: "mime", Data: []byte("<x/>"), ContentType: "application/rdf+xml"},
	)
	file := open(t, f)
	if item := file.Items[2]; item == nil || !item.Hidden || item.Type != "Exif" {
		t.Fatalf("%+v", item)
	}
	if item := file.Items[3]; item == nil || item.ContentType != "application/rdf+xml" {
		t.Fatalf("%+v", item)
	}
	if len(file.Order) != 3 || file.Order[0] != 1 || file.Order[2] != 3 {
		t.Fatalf("order %v", file.Order)
	}
}

func TestReadItemHandlesEveryConstructionMethod(t *testing.T) {
	f := av1Still(2, 2)
	f.Items = append(f.Items,
		isotest.Item{ID: 2, Type: "grid", Data: []byte("inline-data"), InIdat: true},
		isotest.Item{ID: 3, Type: "free", NoData: true},
	)
	file := open(t, f)
	data, err := file.ReadItem(file.Items[2], 1<<10)
	if err != nil || string(data) != "inline-data" {
		t.Fatalf("idat item: %q %v", data, err)
	}
	data, err = file.ReadItem(file.Items[1], 4)
	if err != nil || string(data) != payload[:4] {
		t.Fatalf("limited read: %q %v", data, err)
	}
	if _, err := file.ReadItem(file.Items[3], 1<<10); err == nil {
		t.Fatal("item without extents was read")
	}
	file.Items[1].Method = 2
	if _, err := file.ReadItem(file.Items[1], 1<<10); !errors.Is(err, codec.ErrUnsupported) {
		t.Fatalf("method 2: %v", err)
	}
	file.Items[1].Method = 0
	file.Items[1].Extents = []isobmff.Extent{{Offset: 1 << 40, Length: 4}}
	if _, err := file.ReadItem(file.Items[1], 1<<10); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("far extent: %v", err)
	}
	file.Items[1].Extents = []isobmff.Extent{{Offset: 0, Length: 1 << 40}}
	if _, err := file.ReadItem(file.Items[1], 1<<10); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("long extent: %v", err)
	}
	file.Items[1].BaseOffset = ^uint64(0)
	file.Items[1].Extents = []isobmff.Extent{{Offset: 8, Length: 4}}
	if _, err := file.ReadItem(file.Items[1], 1<<10); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("overflowing base: %v", err)
	}
	file.Items[2].Extents = []isobmff.Extent{{Offset: 1 << 20, Length: 2}}
	if _, err := file.ReadItem(file.Items[2], 1<<10); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("far idat extent: %v", err)
	}
	file.Items[2].Extents = []isobmff.Extent{{Offset: 2, Length: 0}}
	if data, err := file.ReadItem(file.Items[2], 1<<10); err != nil || string(data) != "line-data" {
		t.Fatalf("open ended idat extent: %q %v", data, err)
	}
}

func TestReadItemConcatenatesExtentsAndStopsAtTheLimit(t *testing.T) {
	file := open(t, av1Still(2, 2))
	item := file.Items[1]
	item.Extents = []isobmff.Extent{{Offset: item.Extents[0].Offset, Length: 4}, {Offset: item.Extents[0].Offset + 8, Length: 4}}
	data, err := file.ReadItem(item, 1<<10)
	if err != nil || string(data) != "0123"+"89ab" {
		t.Fatalf("%q %v", data, err)
	}
	data, err = file.ReadItem(item, 6)
	if err != nil || string(data) != "0123"+"89" {
		t.Fatalf("limited: %q %v", data, err)
	}
	item.Extents[0].Length = 0
	data, err = file.ReadItem(item, 1<<10)
	if err != nil || len(data) < 8 {
		t.Fatalf("open ended extent: %d %v", len(data), err)
	}
}

func TestOpenRejectsMalformedContainers(t *testing.T) {
	valid := av1Still(8, 8).Bytes()
	metaAt := bytes.Index(valid, []byte("meta")) - 4
	tests := []struct {
		name string
		data []byte
		kind error
	}{
		{"empty", nil, codec.ErrCorrupt},
		{"shorter than a box", valid[:12], codec.ErrCorrupt},
		{"first box is not ftyp", append(isotest.Box("free", []byte("12345678")), valid...), codec.ErrCorrupt},
		{"ftyp too short", append(isotest.Box("ftyp", []byte("avif")), valid[8:]...), codec.ErrCorrupt},
		{"ftyp too large", isotest.Box("ftyp", make([]byte, 8000)), codec.ErrTooLarge},
		{"box larger than the file", append(append([]byte{}, valid[:metaAt]...), append(isotest.U32(1<<30), []byte("meta")...)...), codec.ErrCorrupt},
		{"large box larger than the file", append(append([]byte{}, valid[:metaAt]...), append(isotest.U32(1), append([]byte("meta"), isotest.U64(1<<50)...)...)...), codec.ErrCorrupt},
		{"large box header cut off", append(append([]byte{}, valid[:metaAt]...), append(isotest.U32(1), []byte("meta1234")...)...), codec.ErrCorrupt},
		{"box smaller than its header", append(append([]byte{}, valid[:metaAt]...), append(isotest.U32(4), []byte("meta12345678")...)...), codec.ErrCorrupt},
		{"meta over the cap", append(append([]byte{}, valid[:metaAt]...), isotest.Box("meta", make([]byte, isobmff.MaxHeaderBytes+1))...), codec.ErrTooLarge},
		{"meta without a header", append(append([]byte{}, valid[:metaAt]...), isotest.Box("meta", []byte{0, 0})...), codec.ErrCorrupt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := openBytes(tt.data)
			requireKind(t, err, tt.kind)
		})
	}
}

func TestOpenRejectsMalformedMetaChildren(t *testing.T) {
	broken := func(children ...[]byte) []byte {
		data := isotest.File{Major: "avif", NoMeta: true}.Bytes()
		return append(data, isotest.Box("meta", append([][]byte{isotest.U32(0)}, children...)...)...)
	}
	cases := []struct {
		name string
		data []byte
	}{
		{"pitm cut", broken(isotest.FullBox("pitm", 0, 0, []byte{0}))},
		{"iinf cut", broken(isotest.FullBox("iinf", 0, 0, []byte{0}))},
		{"infe cut", broken(isotest.FullBox("iinf", 0, 0, isotest.U16(1), isotest.FullBox("infe", 2, 0, isotest.U16(1))))},
		{"iloc bad offset size", broken(isotest.FullBox("iloc", 1, 0, []byte{0x34, 0x00}, isotest.U16(0)))},
		{"iloc cut", broken(isotest.FullBox("iloc", 1, 0, []byte{0x44}))},
		{"iloc entry cut", broken(isotest.FullBox("iloc", 1, 0, []byte{0x44, 0x00}, isotest.U16(2), isotest.U16(1)))},
		{"iloc extent cut", broken(isotest.FullBox("iloc", 1, 0, []byte{0x44, 0x00}, isotest.U16(1), isotest.U16(1), isotest.U16(0), isotest.U16(0), isotest.U16(1), isotest.U32(1)))},
		{"iref cut", broken(isotest.Box("iref", []byte{0}))},
		{"iref entry cut", broken(isotest.FullBox("iref", 0, 0, isotest.Box("dimg", isotest.U16(1), isotest.U16(3), isotest.U16(2))))},
		{"ipma cut", broken(isotest.Box("iprp", isotest.FullBox("ipma", 0, 0, []byte{0})))},
		{"ipma entry cut", broken(isotest.Box("iprp", isotest.FullBox("ipma", 0, 0, isotest.U32(1), isotest.U16(1), []byte{3, 1})))},
		{"ipma entry without association count", broken(isotest.Box("iprp", isotest.FullBox("ipma", 0, 0, isotest.U32(1), isotest.U16(1))))},
		{"ipco ispe cut", broken(isotest.Box("iprp", isotest.Box("ipco", isotest.FullBox("ispe", 0, 0, isotest.U32(1)))))},
		{"ipco av1C marker", broken(isotest.Box("iprp", isotest.Box("ipco", isotest.Box("av1C", []byte{0x01, 0, 0, 0}))))},
		{"ipco av1C cut", broken(isotest.Box("iprp", isotest.Box("ipco", isotest.Box("av1C", []byte{0x81}))))},
		{"ipco hvcC version", broken(isotest.Box("iprp", isotest.Box("ipco", isotest.Box("hvcC", []byte{2, 0, 0, 0}))))},
		{"ipco pixi cut", broken(isotest.Box("iprp", isotest.Box("ipco", isotest.FullBox("pixi", 0, 0, []byte{3, 8}))))},
		{"ipco clap cut", broken(isotest.Box("iprp", isotest.Box("ipco", isotest.Box("clap", []byte{0, 0, 0, 1}))))},
		{"ipco auxC without terminator", broken(isotest.Box("iprp", isotest.Box("ipco", isotest.FullBox("auxC", 0, 0, []byte("urn")))))},
		{"child box overrunning meta", broken(append(isotest.U32(400), []byte("iinfxxxx")...))},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := openBytes(tt.data)
			if err == nil || !(errors.Is(err, codec.ErrCorrupt) || errors.Is(err, codec.ErrTooLarge)) {
				t.Fatalf("error %v", err)
			}
		})
	}
}

func TestOpenCapsTheNumberOfItemsAndBoxes(t *testing.T) {
	many := isotest.File{Major: "avif", Compatible: []string{"avif"}}
	for i := 0; i < 70000; i++ {
		many.Extra = append(many.Extra, isotest.Box("free"))
	}
	_, err := openBytes(many.Bytes())
	requireKind(t, err, codec.ErrCorrupt)

	var items []byte
	for id := uint32(1); id <= 1<<17+1; id++ {
		items = append(items, isotest.FullBox("infe", 3, 0, isotest.U32(id), []byte{0, 0}, []byte("Exif"), []byte{0})...)
	}
	data := isotest.File{Major: "avif", NoMeta: true}.Bytes()
	data = append(data, isotest.Box("meta", isotest.U32(0), isotest.FullBox("iinf", 1, 0, isotest.U32(1<<17+1), items))...)
	_, err = openBytes(data)
	requireKind(t, err, codec.ErrTooLarge)
}

func TestOpenCapsLocationsAssociationsReferencesAndProperties(t *testing.T) {
	prefix := isotest.File{Major: "avif", NoMeta: true}.Bytes()
	var locations []byte
	for id := uint32(1); id <= 1<<17+1; id++ {
		locations = append(locations, isotest.U32(id)...)
		locations = append(locations, isotest.U16(0)...)
		locations = append(locations, isotest.U16(0)...)
		locations = append(locations, isotest.U16(0)...)
	}
	iloc := isotest.FullBox("iloc", 2, 0, []byte{0x00, 0x00}, isotest.U32(1<<17+1), locations)
	_, err := openBytes(append(append([]byte{}, prefix...), isotest.Box("meta", isotest.U32(0), iloc)...))
	requireKind(t, err, codec.ErrTooLarge)

	var assoc []byte
	for id := uint32(1); id <= 1<<17+1; id++ {
		assoc = append(assoc, isotest.U32(id)...)
		assoc = append(assoc, 1, 1)
	}
	ipma := isotest.FullBox("ipma", 1, 0, isotest.U32(1<<17+1), assoc)
	_, err = openBytes(append(append([]byte{}, prefix...), isotest.Box("meta", isotest.U32(0), isotest.Box("iprp", ipma))...))
	requireKind(t, err, codec.ErrTooLarge)

	var refs []byte
	for i := 0; i < 1<<18+1; i++ {
		refs = append(refs, isotest.Box("dimg", isotest.U16(1), isotest.U16(0))...)
	}
	_, err = openBytes(append(append([]byte{}, prefix...), isotest.Box("meta", isotest.U32(0), isotest.FullBox("iref", 0, 0, refs))...))
	requireKind(t, err, codec.ErrTooLarge)

	var props []byte
	for i := 0; i < 1<<17+1; i++ {
		props = append(props, isotest.Box("free")...)
	}
	_, err = openBytes(append(append([]byte{}, prefix...), isotest.Box("meta", isotest.U32(0), isotest.Box("iprp", isotest.Box("ipco", props)))...))
	requireKind(t, err, codec.ErrTooLarge)
}

func TestOpenReadsMetaAfterMediaData(t *testing.T) {
	f := av1Still(4, 4)
	whole := f.Bytes()
	metaStart := bytes.Index(whole, []byte("meta")) - 4
	mdatStart := bytes.Index(whole, []byte("mdat")) - 4
	meta := whole[metaStart:mdatStart]
	file, err := openBytes(append(append(append([]byte{}, whole[:metaStart]...), whole[mdatStart:]...), meta...))
	if err != nil {
		t.Fatal(err)
	}
	if file.PrimaryItem() == nil {
		t.Fatal("no primary item")
	}
}

func TestOpenKeepsTheFirstBoxOfEachKindAndIgnoresUUIDBoxes(t *testing.T) {
	f := av1Still(4, 4)
	f.Extra = [][]byte{
		isotest.Box("ftyp", []byte("heic"), isotest.U32(0)),
		isotest.Box("uuid", make([]byte, 40)),
		isotest.Box("meta", isotest.U32(0)),
		isotest.Box("free", []byte("padding")),
	}
	file := open(t, f)
	if file.Brands.Major != "avif" || file.PrimaryItem() == nil {
		t.Fatalf("%+v", file.Brands)
	}
}

func TestOpenSeesMinimizedImageBoxes(t *testing.T) {
	data := isotest.File{Major: "avif", NoMeta: true, Extra: [][]byte{isotest.Box("mini", make([]byte, 16))}}.Bytes()
	file, err := openBytes(data)
	if err != nil || !file.Mini || file.HasMeta {
		t.Fatalf("%+v %v", file, err)
	}
}

func TestOpenStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data := av1Still(4, 4).Bytes()
	if _, err := isobmff.Open(ctx, bytes.NewReader(data), int64(len(data))); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}

func TestOpenFailsOnShortReads(t *testing.T) {
	data := av1Still(4, 4).Bytes()
	if _, err := isobmff.Open(context.Background(), bytes.NewReader(data[:len(data)/2]), int64(len(data))); err == nil {
		t.Fatal("short reader accepted")
	}
}

func TestUnknownEssentialReportsTheFirstUnsupportedProperty(t *testing.T) {
	f := av1Still(4, 4, isotest.Unknown("rloc", false), isotest.Unknown("lsel", true), isotest.Unknown("a1lx", false))
	file := open(t, f)
	item := file.PrimaryItem()
	if got := file.UnknownEssential(item, []string{"ispe", "av1C", "pixi"}); got != "lsel" {
		t.Fatalf("got %q", got)
	}
	if got := file.UnknownEssential(item, []string{"ispe", "av1C", "pixi", "lsel"}); got != "" {
		t.Fatalf("got %q", got)
	}
	item.Associations = append(item.Associations, isobmff.Association{Index: 9999, Essential: true})
	if got := file.UnknownEssential(item, []string{"ispe", "av1C", "pixi", "lsel"}); got != "missing" {
		t.Fatalf("got %q", got)
	}
}

func TestReferencesFromFindsByTypeAndSource(t *testing.T) {
	f := av1Still(4, 4)
	f.Items = append(f.Items, isotest.Item{ID: 2, Type: "Exif", Data: []byte("x")})
	f.Refs = []isotest.Ref{{Type: "cdsc", From: 2, To: []uint32{1}}, {Type: "dimg", From: 7, To: []uint32{1, 2, 3}}}
	file := open(t, f)
	if got := file.ReferencesFrom("dimg", 7); len(got) != 3 || got[2] != 3 {
		t.Fatalf("%v", got)
	}
	if file.ReferencesFrom("dimg", 2) != nil || file.ReferencesFrom("cdsc", 7) != nil {
		t.Fatal("wrong reference matched")
	}
	if got := file.ReferencesFrom("cdsc", 2); len(got) != 1 || got[0] != 1 {
		t.Fatalf("%v", got)
	}
}

func TestPlayCountFollowsTheGIFConvention(t *testing.T) {
	for loop, want := range map[int]int{0: 0, -1: 1, 1: 2, 4: 5} {
		if got := isobmff.PlayCount(loop); got != want {
			t.Errorf("PlayCount(%d) = %d, want %d", loop, got, want)
		}
	}
}
