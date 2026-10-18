package app

import (
	"bytes"
	"context"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	jpegcodec "github.com/oops1/PixGo/internal/codec/jpeg"
	pngcodec "github.com/oops1/PixGo/internal/codec/png"
	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/testutil"
	uisettings "github.com/oops1/PixGo/internal/ui/settings"
)

type stubFormat struct {
	id   codec.ID
	name string
	exts []string
}

func (s stubFormat) ID() codec.ID         { return s.id }
func (s stubFormat) Name() string         { return s.name }
func (s stubFormat) Extensions() []string { return s.exts }
func (s stubFormat) Sniff(head []byte) bool {
	return bytes.HasPrefix(head, []byte("STUB-"+string(s.id)))
}
func (s stubFormat) Probe(context.Context, io.ReaderAt, int64, codec.Limits) (codec.Info, error) {
	return codec.Info{Format: s.id, FormatName: s.name, Width: 4, Height: 4}, nil
}
func (s stubFormat) Decode(context.Context, io.ReaderAt, int64, codec.Options) (*codec.Image, error) {
	info := codec.Info{Format: s.id, FormatName: s.name, Width: 4, Height: 4}
	return &codec.Image{Info: info, Frames: []codec.Frame{{Pix: image.NewNRGBA(image.Rect(0, 0, 4, 4))}}}, nil
}

var stubExtension = map[codec.ID]string{
	codec.APNG: ".apng", codec.AVIF: ".avif", codec.JXL: ".jxl", codec.HEIC: ".heic", codec.RAW: ".cr2",
}

func stubFor(id codec.ID) stubFormat {
	ext := stubExtension[id]
	return stubFormat{id: id, name: strings.ToUpper(string(id)), exts: []string{ext}}
}

func formatsRegistry(optional ...codec.ID) *codec.Registry {
	reg := codec.NewRegistry()
	reg.RegisterDecoder(jpegcodec.Decoder{})
	reg.RegisterDecoder(pngcodec.Decoder{})
	for _, id := range optional {
		reg.RegisterDecoder(stubFor(id))
	}
	return reg
}

func formatsApp(t *testing.T, reg *codec.Registry, mutate func(*settings.Settings)) *App {
	t.Helper()
	i18n.SetLang("ru")
	t.Setenv("PIXGO_CONFIG_DIR", t.TempDir())
	t.Setenv("PIXGO_STATE_DIR", t.TempDir())
	t.Setenv("PIXGO_CACHE_DIR", t.TempDir())
	st := settings.Defaults()
	st.Theme = settings.ThemeDark
	if mutate != nil {
		mutate(&st)
	}
	eng := testutil.NewEngine(770, 504)
	a := New(eng, Options{Settings: settings.NewMemory(st), Registry: reg})
	eng.SetRootFullCanvas(a.Root)
	t.Cleanup(a.Shutdown)
	t.Cleanup(func() { i18n.SetLang("ru") })
	return a
}

func eventually(t *testing.T, a *App, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.Eng.Flush()
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("not reached in time: %s", what)
}

func rowByID(rows []uisettings.Format, id string) (uisettings.Format, bool) {
	for _, r := range rows {
		if r.ID == id {
			return r, true
		}
	}
	return uisettings.Format{}, false
}

func TestFormatRowsListEveryRegisteredDecoderWithItsNameAndExtensions(t *testing.T) {
	a := formatsApp(t, formatsRegistry(), nil)

	rows := a.ensureSettings().Formats()

	jpeg, ok := rowByID(rows, "jpeg")
	if !ok || jpeg.Name != "JPEG" || strings.Join(jpeg.Extensions, " ") != "JPG JPEG JPE JFIF" || !jpeg.Available || !jpeg.Enabled || jpeg.Optional {
		t.Fatalf("jpeg row = %+v", jpeg)
	}
	if png, ok := rowByID(rows, "png"); !ok || png.Name != "PNG" || png.Optional {
		t.Fatalf("png row = %+v", png)
	}
}

func TestDecodersSwitchedOffStillGetARow(t *testing.T) {
	reg := formatsRegistry(codec.AVIF)
	reg.SetEnabled(codec.JPEG, false)
	a := formatsApp(t, reg, nil)

	rows := a.ensureSettings().Formats()

	if _, ok := rowByID(rows, "jpeg"); !ok {
		t.Fatal("a registered but switched-off decoder lost its row")
	}
}

func TestEveryOptionalFormatShowsEvenWhenTheBuildLacksIt(t *testing.T) {
	a := formatsApp(t, formatsRegistry(), nil)

	rows := a.ensureSettings().Formats()

	for _, id := range settings.OptionalFormats {
		r, ok := rowByID(rows, id)
		if !ok {
			t.Fatalf("no row for %s", id)
		}
		if !r.Optional || r.Available || r.Note != "" || r.Name == "" || len(r.Extensions) == 0 {
			t.Errorf("%s row = %+v", id, r)
		}
	}
}

func TestOptionalFormatNamesAreLocalised(t *testing.T) {
	tests := []struct {
		lang string
		want map[string]string
	}{
		{"ru", map[string]string{"apng": "Анимированный PNG (APNG)", "avif": "AVIF", "jxl": "JPEG XL", "heic": "HEIC / HEIF", "raw": "RAW — встроенный просмотр"}},
		{"en", map[string]string{"apng": "Animated PNG (APNG)", "avif": "AVIF", "jxl": "JPEG XL", "heic": "HEIC / HEIF", "raw": "RAW — embedded preview"}},
	}
	for _, tc := range tests {
		t.Run(tc.lang, func(t *testing.T) {
			a := formatsApp(t, formatsRegistry(codec.AVIF, codec.RAW), nil)
			i18n.SetLang(tc.lang)

			rows := a.ensureSettings().Formats()

			for id, want := range tc.want {
				if r, _ := rowByID(rows, id); r.Name != want {
					t.Errorf("%s: name %q, want %q", id, r.Name, want)
				}
			}
		})
	}
}

func TestOptionalRowAvailabilityComesFromTheRegistryAndSwitchFromTheSettings(t *testing.T) {
	tests := []struct {
		name          string
		registered    []codec.ID
		off           []string
		id            string
		wantAvailable bool
		wantEnabled   bool
	}{
		{"built in and on", []codec.ID{codec.AVIF}, nil, "avif", true, true},
		{"built in and switched off", []codec.ID{codec.AVIF}, []string{"avif"}, "avif", true, false},
		{"missing from the build, on by default", nil, nil, "jxl", false, true},
		{"missing from the build and off", nil, []string{"jxl"}, "jxl", false, false},
		{"neighbour is not touched", []codec.ID{codec.AVIF, codec.JXL}, []string{"avif"}, "jxl", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := formatsApp(t, formatsRegistry(tc.registered...), func(s *settings.Settings) {
				for _, id := range tc.off {
					s.Formats[id] = settings.Format{Enabled: false}
				}
			})

			r, _ := rowByID(a.ensureSettings().Formats(), tc.id)

			if r.Available != tc.wantAvailable || r.Enabled != tc.wantEnabled || !r.Optional {
				t.Fatalf("row = %+v, want available %v enabled %v", r, tc.wantAvailable, tc.wantEnabled)
			}
		})
	}
}

func TestEnabledColumnReadsTheSettingsNotTheRegistry(t *testing.T) {
	reg := formatsRegistry(codec.AVIF)
	reg.SetEnabled(codec.AVIF, false)
	a := formatsApp(t, reg, nil)
	reg.SetEnabled(codec.AVIF, false)

	r, _ := rowByID(a.ensureSettings().Formats(), "avif")

	if !r.Enabled {
		t.Fatal("the row must show what the user chose, not the transient registry state")
	}
}

func TestPatentAndPreviewNotesAppearOnlyForPresentFormats(t *testing.T) {
	tests := []struct {
		name       string
		registered []codec.ID
		id         string
		wantKey    string
	}{
		{"heic present", []codec.ID{codec.HEIC}, "heic", "settings.format.heic.note"},
		{"raw present", []codec.ID{codec.RAW}, "raw", "settings.format.raw.note"},
		{"heic missing", nil, "heic", ""},
		{"raw missing", nil, "raw", ""},
		{"avif present has no note", []codec.ID{codec.AVIF}, "avif", ""},
		{"jxl present has no note", []codec.ID{codec.JXL}, "jxl", ""},
		{"apng present has no note", []codec.ID{codec.APNG}, "apng", ""},
	}
	for _, lang := range i18n.Supported() {
		for _, tc := range tests {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				a := formatsApp(t, formatsRegistry(tc.registered...), nil)
				i18n.SetLang(lang)
				want := ""
				if tc.wantKey != "" {
					want = i18n.T(tc.wantKey)
				}

				r, _ := rowByID(a.ensureSettings().Formats(), tc.id)

				if r.Note != want {
					t.Fatalf("note = %q, want %q", r.Note, want)
				}
			})
		}
	}
}

func TestNoteTextsSayWhatTheUserAsked(t *testing.T) {
	i18n.SetLang("ru")
	t.Cleanup(func() { i18n.SetLang("ru") })

	heic := i18n.T("settings.format.heic.note")
	raw := i18n.T("settings.format.raw.note")

	if heic != "HEIC использует видеокодек HEVC, на который заявлены патенты. PixGo только читает такие файлы. Если это важно для вас, отключите формат." {
		t.Errorf("heic note: %q", heic)
	}
	if raw != "Файлы DNG проявляются из данных сенсора. Остальные форматы RAW показывают встроенный в файл предварительный просмотр." {
		t.Errorf("raw note: %q", raw)
	}
}

func TestBuiltInFormatsComeFirstAndOptionalOnesFollowInSettingsOrder(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.RAW, codec.APNG), nil)

	rows := a.ensureSettings().Formats()

	var optional []string
	seenOptional := false
	for _, r := range rows {
		if r.Optional {
			seenOptional = true
			optional = append(optional, r.ID)
		} else if seenOptional {
			t.Fatalf("built-in %s listed after the optional ones", r.ID)
		}
	}
	if strings.Join(optional, ",") != strings.Join(settings.OptionalFormats, ",") {
		t.Fatalf("optional order %v, want %v", optional, settings.OptionalFormats)
	}
	for i := 1; i < len(rows) && !rows[i].Optional; i++ {
		if rows[i-1].Name > rows[i].Name {
			t.Errorf("built-in rows out of order: %q before %q", rows[i-1].Name, rows[i].Name)
		}
	}
}

func TestEverySettingsOptionalFormatIsDescribedForThePage(t *testing.T) {
	for _, id := range settings.OptionalFormats {
		info, ok := optionalFormats[id]
		if !ok {
			t.Errorf("%s has no description", id)
			continue
		}
		if i18n.T(info.nameKey) == info.nameKey || len(info.extensions) == 0 {
			t.Errorf("%s: name key %q or extensions %v are not usable", id, info.nameKey, info.extensions)
		}
		if info.noteKey != "" && i18n.T(info.noteKey) == info.noteKey {
			t.Errorf("%s: note key %q is not in the catalogue", id, info.noteKey)
		}
	}
	if len(optionalFormats) != len(settings.OptionalFormats) {
		t.Errorf("%d descriptions for %d optional formats", len(optionalFormats), len(settings.OptionalFormats))
	}
}

func TestUnknownOptionalIDFallsBackToItsUpperCaseName(t *testing.T) {
	old := settings.OptionalFormats
	settings.OptionalFormats = append(append([]string(nil), old...), "xyz")
	t.Cleanup(func() { settings.OptionalFormats = old })
	a := formatsApp(t, formatsRegistry(), nil)

	r, ok := rowByID(a.ensureSettings().Formats(), "xyz")

	if !ok || r.Name != "XYZ" || r.Available || !r.Optional {
		t.Fatalf("row = %+v ok=%v", r, ok)
	}
}

type folderFiles struct {
	dir   string
	jpg1  string
	avif  string
	jpg2  string
	heic  string
	other string
}

func writeStub(t *testing.T, path string, id codec.ID) {
	t.Helper()
	if err := os.WriteFile(path, []byte("STUB-"+string(id)+" payload"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mixedFolder(t *testing.T) folderFiles {
	t.Helper()
	dir := t.TempDir()
	f := folderFiles{dir: dir, jpg1: filepath.Join(dir, "a.jpg"), avif: filepath.Join(dir, "b.avif"), jpg2: filepath.Join(dir, "c.jpg"), heic: filepath.Join(dir, "d.heic")}
	writeBigJPEG(t, f.jpg1)
	writeBigJPEG(t, f.jpg2)
	writeStub(t, f.avif, codec.AVIF)
	writeStub(t, f.heic, codec.HEIC)
	return f
}

func folderCount(a *App) int {
	if n := a.Ctl.Navigator(); n != nil {
		return n.Count()
	}
	return 0
}

func setFormat(a *App, id string, on bool) {
	a.Settings.Update(func(s *settings.Settings) { s.Formats[id] = settings.Format{Enabled: on} })
}

func TestSwitchingAFormatOffRemovesItsFilesFromTheOpenFolderAndBack(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.AVIF, codec.HEIC), nil)
	f := mixedFolder(t)
	a.OpenFiles([]string{f.jpg1})
	waitName(t, a, "a.jpg")
	if got := folderCount(a); got != 4 {
		t.Fatalf("folder starts with %d files, want 4", got)
	}

	setFormat(a, "avif", false)
	eventually(t, a, "AVIF files leave the folder", func() bool { return folderCount(a) == 3 })

	if a.Ctl.env.Registry.IsEnabled(codec.AVIF) || a.Ctl.env.Registry.Supports(f.avif) {
		t.Error("the registry still serves AVIF")
	}
	if !a.Ctl.env.Registry.Supports(f.heic) {
		t.Error("HEIC must stay on")
	}

	setFormat(a, "avif", true)
	eventually(t, a, "AVIF files come back", func() bool { return folderCount(a) == 4 })

	if !a.Ctl.env.Registry.Supports(f.avif) {
		t.Error("the registry does not serve AVIF again")
	}
}

func TestStepsSkipTheFilesOfAFormatSwitchedOff(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.AVIF), nil)
	f := mixedFolder(t)
	a.OpenFiles([]string{f.jpg1})
	waitName(t, a, "a.jpg")
	setFormat(a, "avif", false)
	eventually(t, a, "folder shrinks", func() bool { return folderCount(a) == 2 })

	a.Ctl.Step(1)

	waitName(t, a, "c.jpg")
}

func TestFileOpenedByNameStaysOnScreenWhenItsFormatIsSwitchedOff(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.AVIF), nil)
	f := mixedFolder(t)
	extra := filepath.Join(f.dir, "e.avif")
	writeStub(t, extra, codec.AVIF)
	a.OpenFiles([]string{f.avif})
	waitName(t, a, "b.avif")
	if got := folderCount(a); got != 4 {
		t.Fatalf("folder starts with %d files, want 4", got)
	}

	setFormat(a, "avif", false)

	eventually(t, a, "other AVIF files leave the folder", func() bool { return folderCount(a) == 3 })
	if a.Ctl.Name() != "b.avif" || a.Ctl.Status() != StatusReady {
		t.Fatalf("the file the user asked for was taken away: %q status %d", a.Ctl.Name(), a.Ctl.Status())
	}
}

func TestSwitchingOffTheFormatOfTheShownNeighbourMovesToAnotherFile(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.AVIF), nil)
	f := mixedFolder(t)
	a.OpenFiles([]string{f.jpg1})
	waitName(t, a, "a.jpg")
	a.Ctl.Step(1)
	waitName(t, a, "b.avif")

	setFormat(a, "avif", false)

	eventually(t, a, "another file is shown", func() bool { return a.Ctl.Name() != "b.avif" && a.Ctl.Status() == StatusReady })
}

func TestSwitchingOffTheOnlyFormatInAFolderLeavesAnEmptyViewerNotACrash(t *testing.T) {
	reg := codec.NewRegistry()
	reg.RegisterDecoder(stubFor(codec.AVIF))
	a := formatsApp(t, reg, nil)
	dir := t.TempDir()
	writeStub(t, filepath.Join(dir, "only.avif"), codec.AVIF)
	if err := a.Ctl.Open(dir); err != nil {
		t.Fatal(err)
	}
	eventually(t, a, "the file is shown", func() bool { return a.Ctl.Status() == StatusReady })

	setFormat(a, "avif", false)

	eventually(t, a, "the viewer is empty", func() bool { return a.Ctl.Status() == StatusEmpty })
}

func TestSwitchingAFormatAppliesWhileTheSettingsPageIsOpen(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.AVIF), nil)
	f := mixedFolder(t)
	a.OpenFiles([]string{f.jpg1})
	waitName(t, a, "a.jpg")
	a.OpenSettings()
	act := a.ensureSettings().actions()

	act.Update(func(s *settings.Settings) { s.Formats["avif"] = settings.Format{Enabled: false} })

	eventually(t, a, "folder behind the page shrinks", func() bool { return folderCount(a) == 2 })
	rows := a.ensureSettings().Formats()
	if r, _ := rowByID(rows, "avif"); r.Enabled || !r.Available {
		t.Fatalf("avif row after the switch = %+v", r)
	}
}

func TestGalleryBehindTheSettingsPageRelistsAfterAFormatSwitch(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.AVIF), nil)
	f := mixedFolder(t)
	a.OpenGalleryAt(model.Folder(f.dir), "")
	listed := func() []string {
		var out []string
		if a.gal != nil {
			for _, it := range a.gal.Items() {
				out = append(out, it.Name)
			}
		}
		return out
	}
	eventually(t, a, "folder listed", func() bool { return !a.gal.loading && len(listed()) == 3 })
	a.OpenSettings()

	setFormat(a, "avif", false)

	eventually(t, a, "AVIF leaves the gallery", func() bool { return !a.gal.loading && strings.Join(listed(), ",") == "a.jpg,c.jpg" })
	setFormat(a, "avif", true)
	eventually(t, a, "AVIF returns to the gallery", func() bool { return !a.gal.loading && len(listed()) == 3 })
}

func TestGalleryThatIsClosedForgetsItsCachedListingOnAFormatSwitch(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.AVIF), nil)
	f := mixedFolder(t)
	a.OpenGalleryAt(model.Folder(f.dir), "")
	eventually(t, a, "folder listed", func() bool { return !a.gal.loading && len(a.gal.Items()) == 3 })
	a.CloseGallery()

	setFormat(a, "avif", false)
	a.Eng.Flush()
	time.Sleep(20 * time.Millisecond)
	a.Eng.Flush()
	a.OpenGalleryAt(model.Folder(f.dir), "")

	eventually(t, a, "the new listing has no AVIF", func() bool { return !a.gal.loading && len(a.gal.Items()) == 2 })
}

func TestFormatSwitchBeforeTheGalleryWasEverOpenedIsHarmless(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.AVIF), nil)

	setFormat(a, "avif", false)
	a.Eng.Flush()

	if a.gal != nil || a.Ctl.env.Registry.IsEnabled(codec.AVIF) {
		t.Fatalf("gallery %v, avif enabled %v", a.gal, a.Ctl.env.Registry.IsEnabled(codec.AVIF))
	}
}

func TestUnrelatedSettingsDoNotRescanTheFolder(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.AVIF), nil)
	f := mixedFolder(t)
	a.OpenFiles([]string{f.jpg1})
	waitName(t, a, "a.jpg")
	writeBigJPEG(t, filepath.Join(f.dir, "e.jpg"))

	a.Settings.Update(func(s *settings.Settings) { s.Preload = 2 })
	a.Settings.Update(func(s *settings.Settings) { s.SaveJPEGQuality = 50 })
	for i := 0; i < 5; i++ {
		a.Eng.Flush()
		time.Sleep(10 * time.Millisecond)
	}

	if got := folderCount(a); got != 3 {
		t.Fatalf("folder has %d entries, an unrelated change must not rescan it (want 3)", got)
	}
}

func TestRepeatedSwitchWithTheSameValueDoesNotRescan(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.AVIF), nil)
	f := mixedFolder(t)
	a.OpenFiles([]string{f.jpg1})
	waitName(t, a, "a.jpg")
	writeBigJPEG(t, filepath.Join(f.dir, "e.jpg"))

	setFormat(a, "avif", true)
	for i := 0; i < 5; i++ {
		a.Eng.Flush()
		time.Sleep(10 * time.Millisecond)
	}

	if got := folderCount(a); got != 3 {
		t.Fatalf("folder has %d entries after a no-op switch, want 3", got)
	}
}

func TestSwitchingAnUnregisteredFormatTouchesNothingButTheSetting(t *testing.T) {
	a := formatsApp(t, formatsRegistry(), nil)
	f := mixedFolder(t)
	a.OpenFiles([]string{f.jpg1})
	waitName(t, a, "a.jpg")

	setFormat(a, "jxl", false)
	for i := 0; i < 3; i++ {
		a.Eng.Flush()
		time.Sleep(5 * time.Millisecond)
	}

	if a.Settings.Get().FormatEnabled("jxl") || folderCount(a) != 2 {
		t.Fatalf("setting %v, folder %d", a.Settings.Get().FormatEnabled("jxl"), folderCount(a))
	}
}

func formatRowOnThePage(t *testing.T, a *App, name string) widget.AccessInfo {
	t.Helper()
	for _, info := range a.set.page.AccessChildren() {
		if info.Name == name && info.Role == widget.RoleCheckBox {
			return info
		}
	}
	t.Fatalf("no switch named %q on the page", name)
	return widget.AccessInfo{}
}

func clickAt(a *App, x, y int) {
	p := a.set.page
	p.OnMouseMove(x, y)
	p.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
	p.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false, Clicks: 1})
}

func TestSwitchOnTheFormatsPageReachesTheRegistryAndTheFolder(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.AVIF, codec.HEIC), nil)
	f := mixedFolder(t)
	a.OpenFiles([]string{f.jpg1})
	waitName(t, a, "a.jpg")
	a.OpenSettings()
	a.set.page.SetSection(uisettings.SectionFormats)
	a.Root.Refresh()
	a.Eng.Flush()
	row := formatRowOnThePage(t, a, i18n.T("settings.format.avif"))

	clickAt(a, row.Bounds.Max.X-4, row.Bounds.Min.Y+8)

	eventually(t, a, "AVIF is off in the registry", func() bool { return !a.Ctl.env.Registry.IsEnabled(codec.AVIF) })
	eventually(t, a, "AVIF files left the folder", func() bool { return folderCount(a) == 3 })
	if a.Settings.Get().FormatEnabled("avif") {
		t.Fatal("the setting was not stored")
	}
	if got := formatRowOnThePage(t, a, i18n.T("settings.format.avif")); contains(got.States, widget.StateChecked) {
		t.Fatalf("the switch still looks on: %v", got.States)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestFormatsPageShowsTheHEICAndRAWNotes(t *testing.T) {
	a := formatsApp(t, formatsRegistry(codec.HEIC, codec.RAW), nil)
	a.OpenSettings()
	a.set.page.SetSection(uisettings.SectionFormats)
	a.Root.Refresh()
	a.Eng.Flush()

	var labels []string
	for _, info := range a.set.page.AccessChildren() {
		if info.Role == widget.RoleLabel {
			labels = append(labels, info.Name)
		}
	}

	joined := strings.Join(labels, "\n")
	for _, key := range []string{"settings.format.heic.note", "settings.format.raw.note"} {
		first := strings.Fields(i18n.T(key))[0]
		if !strings.Contains(joined, first) {
			t.Errorf("the page does not show %s (looked for %q)", key, first)
		}
	}
}

func TestSeveralDecodersOfOneOptionalFormatShareASingleRow(t *testing.T) {
	reg := formatsRegistry()
	reg.RegisterDecoder(stubFormat{id: codec.RAW, name: "RAW A", exts: []string{".cr2", ".nef"}})
	reg.RegisterDecoder(stubFormat{id: codec.RAW, name: "RAW B", exts: []string{".nef", ".arw"}})
	a := formatsApp(t, reg, nil)

	var rows []uisettings.Format
	for _, r := range a.ensureSettings().Formats() {
		if r.ID == "raw" {
			rows = append(rows, r)
		}
	}

	if len(rows) != 1 || !rows[0].Available || strings.Join(rows[0].Extensions, " ") != "CR2 NEF ARW" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestSwitchingAnOptionalFormatOffHidesEveryDecoderOfIt(t *testing.T) {
	reg := formatsRegistry()
	reg.RegisterDecoder(stubFormat{id: codec.RAW, name: "RAW A", exts: []string{".cr2"}})
	reg.RegisterDecoder(stubFormat{id: codec.RAW, name: "RAW B", exts: []string{".arw"}})
	a := formatsApp(t, reg, nil)

	setFormat(a, "raw", false)
	eventually(t, a, "RAW is off", func() bool { return !reg.IsEnabled(codec.RAW) })

	if reg.Supports("a.cr2") || reg.Supports("b.arw") || !reg.Supports("c.jpg") {
		t.Fatal("the switch must act on the format id, not on one decoder")
	}
}
