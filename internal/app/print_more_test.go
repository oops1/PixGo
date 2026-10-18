package app

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/oops1/headless-gui/v3/printing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	pixprint "github.com/oops1/PixGo/internal/print"
	"github.com/oops1/PixGo/internal/settings"
)

func newTiledPrintRig(t *testing.T, w, h int) (*printRig, *regionFake) {
	t.Helper()
	fake := &regionFake{bigFake: &bigFake{}}
	r := newPrintRig(t, "huge.bigf", func(t *testing.T, p string) { writeBig(t, filepath.Dir(p), "huge", w, h) },
		func(s *settings.Settings) { s.MaxPixels = 4; s.Preload = 0 }, fake)
	old := printDPI
	printDPI = 150
	t.Cleanup(func() { printDPI = old })
	return r, fake
}

func TestTiledPictureIsPrintedFromAReducedLevel(t *testing.T) {
	r, fake := newTiledPrintRig(t, 8000, 6000)
	if r.app.Ctl.Decoded() != nil {
		t.Fatal("the test needs a tiled picture")
	}
	baseline := fake.openReaders()
	r.openPrint()
	m := r.app.printDlg.Model()
	if m.Picture().W != 8000 || m.Picture().H != 6000 {
		t.Fatalf("the dialog must know the full size: %+v", m.Picture())
	}
	fake.mu.Lock()
	thumbCalls := len(fake.regions)
	fake.mu.Unlock()
	if thumbCalls == 0 {
		t.Fatal("the preview must come through the tile reader")
	}
	m.SetMargin(pixprint.MarginNone)
	fake.mu.Lock()
	fake.regions = nil
	fake.mu.Unlock()
	r.confirm()
	eventually(t, r.app, "the job reaches the printer", func() bool { return r.fake.printedCount() == 1 })
	r.waitIdle()
	fake.mu.Lock()
	calls := append([]regionCall(nil), fake.regions...)
	fake.mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("%d region reads, want 1: %+v", len(calls), calls)
	}
	got := calls[0]
	if got.shift != 2 {
		t.Errorf("shift %d: an 8000x6000 picture on a 1754x1240 sheet needs level 2", got.shift)
	}
	sheetW, sheetH := printing.PagePixels(printing.A4, printing.Landscape, printDPI)
	pagePixels := sheetW * sheetH
	if got.rect.Dx()*got.rect.Dy() > 4*pagePixels {
		t.Errorf("the level read is %v, more than 4x the %d page pixels", got.rect, pagePixels)
	}
	if got.rect.Dx() < 1500 || got.rect.Dy() < 1100 {
		t.Errorf("the level %v is smaller than the area on the page", got.rect)
	}
	page := r.fake.lastPrinted().job.Pages[0]
	if page.Bounds().Dx() != sheetW || page.Bounds().Dy() != sheetH {
		t.Fatalf("page %v", page.Bounds())
	}
	if got := fake.openReaders(); got != baseline {
		t.Fatalf("%d tile readers open, %d before printing: the print path leaked one", got, baseline)
	}
}

func TestTiledFillReadsOnlyTheVisiblePart(t *testing.T) {
	r, fake := newTiledPrintRig(t, 8000, 6000)
	r.openPrint()
	m := r.app.printDlg.Model()
	m.SetPlacement(pixprint.PlaceFill)
	m.SetMargin(pixprint.MarginNone)
	fake.mu.Lock()
	fake.regions = nil
	fake.mu.Unlock()
	r.confirm()
	eventually(t, r.app, "the job reaches the printer", func() bool { return r.fake.printedCount() == 1 })
	r.waitIdle()
	fake.mu.Lock()
	calls := append([]regionCall(nil), fake.regions...)
	fake.mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("%d region reads", len(calls))
	}
	full := image.Rect(0, 0, 8000>>uint(calls[0].shift), 6000>>uint(calls[0].shift))
	if calls[0].rect.Dy() >= full.Dy() {
		t.Errorf("a cropped fill must not read the whole height: %v of %v", calls[0].rect, full)
	}
}

func TestPrintSetupMatchesTheEngineSheetForEveryPaper(t *testing.T) {
	for _, p := range pixprint.Papers {
		for _, land := range []bool{false, true} {
			for _, m := range pixprint.Margins {
				o := pixprint.Options{Paper: p.ID, Margin: m, Orientation: pixprint.OrientPortrait}
				if land {
					o.Orientation = pixprint.OrientLandscape
				}
				page, err := o.Layout(pixprint.Picture{W: 300, H: 200}, 300)
				if err != nil {
					t.Fatal(err)
				}
				setup := printSetup(o, page)
				w, h := setup.SheetSize()
				if image.Pt(w, h) != page.Sheet {
					t.Errorf("%s landscape=%v: engine sheet %dx%d, page %v", p.ID, land, w, h, page.Sheet)
				}
				if err := setup.Validate(); err != nil {
					t.Errorf("%s landscape=%v margin %s: %v", p.ID, land, m.ID(), err)
				}
				if cr := setup.ContentRect(); cr != page.Content {
					t.Errorf("%s/%s: engine content %v, ours %v", p.ID, m.ID(), cr, page.Content)
				}
			}
		}
	}
}

func TestSettingsListsMatchThePrintPackage(t *testing.T) {
	var papers, orients, places, margins []string
	for _, p := range pixprint.Papers {
		papers = append(papers, p.ID)
	}
	for _, v := range pixprint.Orientations {
		orients = append(orients, v.ID())
	}
	for _, v := range pixprint.Placements {
		places = append(places, v.ID())
	}
	for _, v := range pixprint.Margins {
		margins = append(margins, v.ID())
	}
	units := []string{pixprint.UnitCM.ID(), pixprint.UnitInch.ID()}
	for _, c := range []struct {
		name      string
		got, want []string
	}{
		{"papers", settings.PrintPapers, papers},
		{"orientations", settings.PrintOrientations, orients},
		{"placements", settings.PrintPlacements, places},
		{"margins", settings.PrintMargins, margins},
		{"units", settings.PrintUnits, units},
	} {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("%s: settings %v, print %v", c.name, c.got, c.want)
		}
	}
	d := pixprint.DefaultOptions()
	st := settings.Defaults()
	if st.PrintPaper != d.Paper || st.PrintOrientation != d.Orientation.ID() || st.PrintPlacement != d.Placement.ID() ||
		st.PrintMargins != d.Margin.ID() || st.PrintUnit != d.Unit.ID() {
		t.Errorf("the defaults differ: %+v vs %+v", st, d)
	}
}

func TestPrintMessagesExistInEveryLanguage(t *testing.T) {
	keys := []string{
		"cmd.print", "cmd.exportPdf", "print.title", "print.titleExport", "print.pdfFilter",
		"toast.printPreparing", "toast.printSending", "toast.printed", "toast.pdfSaved",
		"error.printNoPrinter", "error.printNoService", "error.printUnsupported", "error.printFailed", "error.printTooLarge", "error.pdfFailed",
	}
	for _, l := range i18n.Supported() {
		have := i18n.Keys(l)
		for _, k := range keys {
			if !slices.Contains(have, k) {
				t.Errorf("%s lacks %s", l, k)
			}
		}
	}
}

func TestPrintErrorKeys(t *testing.T) {
	for _, c := range []struct {
		err error
		key string
	}{
		{printing.ErrNoPrinter, "error.printNoPrinter"},
		{fmt.Errorf("wrapped: %w", printing.ErrNoPrintService), "error.printNoService"},
		{printing.ErrUnsupported, "error.printUnsupported"},
		{pixprint.ErrTooLarge, "error.printTooLarge"},
		{codec.ErrTooLarge, "error.printTooLarge"},
		{printing.ErrBadSetup, "error.printTooLarge"},
		{errors.New("other"), "error.printFailed"},
		{nil, "error.printFailed"},
	} {
		if got := printErrorKey(c.err); got != c.key {
			t.Errorf("%v: %q, want %q", c.err, got, c.key)
		}
	}
}

func TestPrintingPaperMapsKnownSizesAndCustomOnes(t *testing.T) {
	for _, c := range []struct {
		p    pixprint.Paper
		want printing.Paper
	}{
		{pixprint.A4, printing.A4}, {pixprint.A5, printing.A5}, {pixprint.A3, printing.A3},
		{pixprint.Letter, printing.Letter}, {pixprint.Legal, printing.Legal},
	} {
		if got := printingPaper(c.p); got != c.want {
			t.Errorf("%s: %+v", c.p.ID, got)
		}
	}
	got := printingPaper(pixprint.Photo1)
	if got.ShortMM != 100 || got.LongMM != 150 || got.Name != "10x15cm" {
		t.Errorf("%+v", got)
	}
	if got.PWGName() == "" {
		t.Error("no PWG name")
	}
}

func TestDefaultAppUsesTheEnginePrinterAndTestsDoNotReachIt(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	if _, ok := a.printSvc.(enginePrinting); !ok {
		t.Fatalf("the default service is %T", a.printSvc)
	}
	r := newPNGPrintRig(t, 60, 40)
	if _, ok := r.app.printSvc.(*fakePrinting); !ok {
		t.Fatalf("the rig service is %T", r.app.printSvc)
	}
}

func TestOversizedCustomPrintIsRefusedWithAMessageNotACrash(t *testing.T) {
	r := newPrintRig(t, "logo.svg", func(t *testing.T, p string) { writeSVGFile(t, p) }, nil)
	r.fullResolution()
	r.openPrint()
	m := r.app.printDlg.Model()
	m.SetPaper(pixprint.A3.ID)
	m.SetPlacement(pixprint.PlaceCustom)
	m.SetKeep(false)
	m.SetWidthText("200")
	m.SetHeightText("200")
	r.confirm()
	r.waitIdle()
	r.settle()
	if !r.app.ModalsOpen() {
		t.Fatal("a 2 m square vector print at 300 dpi is over the limit and must say so")
	}
	if r.fake.printedCount() != 0 {
		t.Fatal("nothing may be printed")
	}
}

func TestNothingHappensWithoutAPicture(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	a.printSvc = &fakePrinting{}
	a.Commands.Run(CmdPrint)
	a.Eng.Flush()
	if a.printDlg != nil || a.printBusy {
		t.Fatal("nothing to print")
	}
}

func TestTransparentPicturePrintsOnPaperWhite(t *testing.T) {
	r := newPrintRig(t, "alpha.png", func(t *testing.T, p string) {
		img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
		writeNRGBA(t, p, img)
	}, nil)
	r.openPrint()
	r.confirm()
	eventually(t, r.app, "the job reaches the printer", func() bool { return r.fake.printedCount() == 1 })
	page := r.fake.lastPrinted().job.Pages[0].(*image.RGBA)
	b := page.Bounds()
	if c := page.RGBAAt(b.Dx()/2, b.Dy()/2); c != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("a fully transparent picture prints as white paper, got %v", c)
	}
}

func writeNRGBA(t *testing.T, path string, img *image.NRGBA) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
