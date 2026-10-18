package app

import (
	"context"
	"errors"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/printing"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	pixprint "github.com/oops1/PixGo/internal/print"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/printdlg"
)

func TestPrintCommandsAreRegisteredWithCtrlP(t *testing.T) {
	var print, export *Spec
	for i := range Specs {
		switch Specs[i].ID {
		case CmdPrint:
			print = &Specs[i]
		case CmdExportPDF:
			export = &Specs[i]
		}
	}
	if print == nil || export == nil {
		t.Fatal("the commands are not in the table")
	}
	if len(print.Keys) != 1 || print.Keys[0] != commands.C(commands.Ctrl, 'P') {
		t.Errorf("print keys %v", print.Keys)
	}
	if print.Title != "cmd.print" || export.Title != "cmd.exportPdf" {
		t.Errorf("titles %q %q", print.Title, export.Title)
	}
	r := newPNGPrintRig(t, 600, 400)
	if got := r.app.Commands.ForChord(commands.C(commands.Ctrl, 'P')); got == nil || got.ID != CmdPrint {
		t.Fatalf("Ctrl+P is bound to %v", got)
	}
}

func TestPrintEntriesAreInTheMenuAndEnabledWithAPicture(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	for _, l := range i18n.Supported() {
		i18n.SetLang(l)
		for _, key := range []string{"cmd.print", "cmd.exportPdf"} {
			found := false
			for _, it := range r.app.menuItems() {
				if it.Text == i18n.T(key) {
					found = true
					if it.Disabled {
						t.Errorf("%s: %s is disabled with a picture", l, key)
					}
				}
			}
			if !found {
				t.Errorf("%s: no menu entry %s", l, key)
			}
		}
	}
	i18n.SetLang("ru")
	order := menuIDs(r.app)
	at := slices.Index(order, CmdPrint)
	if at < 0 || order[at+1] != CmdExportPDF || order[at-1] != CmdSaveAs {
		t.Fatalf("menu order %v", order)
	}
}

func TestPrintIsDisabledWithoutAPictureAndInTheGallery(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	for _, id := range []commands.ID{CmdPrint, CmdExportPDF} {
		if a.Commands.Get(id).IsEnabled() {
			t.Errorf("%s is enabled without a picture", id)
		}
	}
	r := newPNGPrintRig(t, 600, 400)
	for _, id := range []commands.ID{CmdPrint, CmdExportPDF} {
		if !r.app.Commands.Get(id).IsEnabled() {
			t.Errorf("%s is disabled with a picture", id)
		}
	}
	r.app.OpenGallery()
	r.settle()
	for _, id := range []commands.ID{CmdPrint, CmdExportPDF} {
		if r.app.Commands.Get(id).IsEnabled() {
			t.Errorf("%s is enabled on the gallery page", id)
		}
	}
}

func TestPrintDialogOpensWithThePrintersAndThePictureSize(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	r.openPrint()
	m := r.app.printDlg.Model()
	if m.Picture().W != 600 || m.Picture().H != 400 {
		t.Fatalf("picture %+v", m.Picture())
	}
	if m.Printer() != "Photo Inkjet" || m.PDF() {
		t.Fatalf("printer %q pdf %v", m.Printer(), m.PDF())
	}
	if got := m.Destinations(); len(got) != 3 || got[0] != "Office Laser" {
		t.Fatalf("destinations %v", got)
	}
	if m.Env().Preview == nil || m.Env().Preview.Bounds().Dx() != 480 {
		t.Fatalf("preview %v", m.Env().Preview.Bounds())
	}
	if !r.app.ModalOpen() {
		t.Fatal("the dialog must be tracked as modal")
	}
	if r.fake.callCount("print") != 0 {
		t.Fatal("opening the dialog must not print")
	}
	r.press(widget.KeyEscape)
	eventually(t, r.app, "the dialog closes", func() bool { return r.app.printDlg == nil })
	if r.app.ModalOpen() {
		t.Fatal("the modal flag is still set")
	}
}

func TestPictureDensityComesFromThePNGHeader(t *testing.T) {
	r := newPrintRig(t, "photo.png", func(t *testing.T, p string) { writePNGWithDPI(t, p, 600, 400, 150) }, nil)
	r.openPrint()
	if got := r.app.printDlg.Model().Picture().DPI; got < 149.5 || got > 150.5 {
		t.Fatalf("dpi %v", got)
	}
}

func TestUnknownDensityStaysUnknown(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	r.openPrint()
	if got := r.app.printDlg.Model().Picture().DPI; got != 0 {
		t.Fatalf("dpi %v", got)
	}
}

func TestPrintSendsOnePageToThePreselectedPrinter(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	r.fullResolution()
	r.openPrint()
	r.confirm()
	eventually(t, r.app, "the job reaches the printer", func() bool { return r.fake.printedCount() == 1 })
	r.waitIdle()
	got := r.fake.lastPrinted()
	if got.target.Printer != "Photo Inkjet" || got.target.Copies != 1 {
		t.Fatalf("target %+v", got.target)
	}
	if got.job.Name != "photo.png" {
		t.Fatalf("job name %q", got.job.Name)
	}
	if len(got.job.Pages) != 1 {
		t.Fatalf("%d pages", len(got.job.Pages))
	}
	w, h := printing.PagePixels(printing.A4, printing.Landscape, 300)
	if sz := got.job.Pages[0].Bounds().Size(); sz != image.Pt(w, h) {
		t.Fatalf("page %v, want A4 landscape %dx%d", sz, w, h)
	}
	if got.job.Setup.DPI != 300 || got.job.Setup.Orientation != printing.Landscape || got.job.Setup.Paper.Name != "A4" {
		t.Fatalf("setup %+v", got.job.Setup)
	}
	if err := got.job.Validate(); err != nil {
		t.Fatal(err)
	}
	r.waitToast(i18n.T("toast.printed"))
}

func TestPrintedPageHasThePictureCentredOnWhitePaper(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	r.openPrint()
	r.confirm()
	eventually(t, r.app, "the job reaches the printer", func() bool { return r.fake.printedCount() == 1 })
	r.waitIdle()
	page := r.fake.lastPrinted().job.Pages[0].(*image.RGBA)
	b := page.Bounds()
	if c := page.RGBAAt(2, 2); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("the corner is %v, not paper white", c)
	}
	mid := page.RGBAAt(b.Dx()/2, b.Dy()/2)
	want := gradientNRGBA(600, 400).NRGBAAt(300, 200)
	if absDiff(mid.R, want.R) > 6 || absDiff(mid.G, want.G) > 6 || absDiff(mid.B, want.B) > 6 {
		t.Errorf("the centre is %v, the picture has %v", mid, want)
	}
	margin := pixprint.MMToPx(6, printDPI)
	if c := page.RGBAAt(margin/2, b.Dy()/2); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("inside the margin: %v", c)
	}
	if c := page.RGBAAt(margin+6, b.Dy()/2); c == (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("the picture must start at the margin: %v", c)
	}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestCopiesAndPaperReachThePrinter(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	r.openPrint()
	form := r.app.printDlg
	form.Model().SetCopies(3)
	form.Model().SetPaper(pixprint.Letter.ID)
	form.Model().SetOrientation(pixprint.OrientPortrait)
	form.Model().SetMargin(pixprint.MarginNone)
	r.confirm()
	eventually(t, r.app, "the job reaches the printer", func() bool { return r.fake.printedCount() == 1 })
	got := r.fake.lastPrinted()
	if got.target.Copies != 3 {
		t.Fatalf("copies %d", got.target.Copies)
	}
	w, h := printing.PagePixels(printing.Letter, printing.Portrait, printDPI)
	if sz := got.job.Pages[0].Bounds().Size(); sz != image.Pt(w, h) {
		t.Fatalf("page %v, want %dx%d", sz, w, h)
	}
	if got.job.Setup.Paper.Name != "Letter" || got.job.Setup.Orientation != printing.Portrait {
		t.Fatalf("%+v", got.job.Setup)
	}
}

func TestCancelPrintsNothingAndKeepsTheSettings(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	before := r.app.Settings.Get()
	r.openPrint()
	r.app.printDlg.Model().SetPaper(pixprint.A3.ID)
	r.press(widget.KeyEscape)
	eventually(t, r.app, "the dialog closes", func() bool { return r.app.printDlg == nil })
	r.settle()
	if r.fake.callCount("print") != 0 || r.fake.callCount("pdf") != 0 {
		t.Fatalf("calls %v", r.fake.calls)
	}
	after := r.app.Settings.Get()
	if after.PrintPaper != before.PrintPaper || after.PrintDestination != before.PrintDestination {
		t.Fatalf("settings changed: %+v", after)
	}
	if r.app.printBusy {
		t.Fatal("the app stays busy after a cancelled dialog")
	}
	r.app.Commands.Run(CmdPrint)
	r.waitDialog()
}

func TestTheLastChoicesAreRememberedAndRestored(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	r.openPrint()
	m := r.app.printDlg.Model()
	m.SetDestIndex(0)
	m.SetPaper(pixprint.Photo1.ID)
	m.SetOrientation(pixprint.OrientLandscape)
	m.SetPlacement(pixprint.PlaceFill)
	m.SetMargin(pixprint.MarginNormal)
	m.SetUnit(pixprint.UnitInch)
	r.confirm()
	eventually(t, r.app, "the job reaches the printer", func() bool { return r.fake.printedCount() == 1 })
	r.waitIdle()
	st := r.app.Settings.Get()
	if st.PrintDestination != "Office Laser" || st.PrintPaper != "10x15" || st.PrintOrientation != "landscape" ||
		st.PrintPlacement != "fill" || st.PrintMargins != "normal" || st.PrintUnit != "in" {
		t.Fatalf("%+v", st)
	}
	r.openPrint()
	o := r.app.printDlg.Model().Options()
	if r.app.printDlg.Model().Printer() != "Office Laser" || o.Paper != "10x15" || o.Orientation != pixprint.OrientLandscape ||
		o.Placement != pixprint.PlaceFill || o.Margin != pixprint.MarginNormal || o.Unit != pixprint.UnitInch {
		t.Fatalf("restored %q %+v", r.app.printDlg.Model().Printer(), o)
	}
}

func TestARememberedPrinterThatIsGoneFallsBackToTheDefault(t *testing.T) {
	r := newPrintRig(t, "photo.png", func(t *testing.T, p string) { writePNGFile(t, p, 60, 40) },
		func(s *settings.Settings) { s.PrintDestination = "Printer Not There" })
	r.openPrint()
	if got := r.app.printDlg.Model().Printer(); got != "Photo Inkjet" {
		t.Fatalf("printer %q", got)
	}
}

func TestNoPrintersLeavesOnlyPDF(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	r.fake.printers, r.fake.printersErr = nil, printing.ErrNoPrintService
	r.openPrint()
	m := r.app.printDlg.Model()
	if !m.PDF() || len(m.Destinations()) != 1 {
		t.Fatalf("pdf=%v %v", m.PDF(), m.Destinations())
	}
}

func TestASlowPrinterListDoesNotBlockTheDialog(t *testing.T) {
	old := printerListLimit
	printerListLimit = 60 * time.Millisecond
	t.Cleanup(func() { printerListLimit = old })
	r := newPNGPrintRig(t, 600, 400)
	gate := make(chan struct{})
	r.fake.printersGate = gate
	t.Cleanup(func() { close(gate) })
	r.openPrint()
	if !r.app.printDlg.Model().PDF() {
		t.Fatal("without a printer list the dialog offers PDF")
	}
}

func TestPrintersWithoutANameAreIgnored(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	r.fake.printers = []printing.Printer{{Name: ""}, {Name: "Only", Default: true}}
	r.openPrint()
	if got := r.app.printDlg.Model().Destinations(); len(got) != 2 || got[0] != "Only" {
		t.Fatalf("%v", got)
	}
}

func TestPrinterErrorsAreShownLocalised(t *testing.T) {
	for _, c := range []struct {
		err error
		key string
	}{
		{printing.ErrNoPrinter, "error.printNoPrinter"},
		{printing.ErrNoPrintService, "error.printNoService"},
		{printing.ErrUnsupported, "error.printUnsupported"},
		{errors.New("spooler stopped"), "error.printFailed"},
	} {
		r := newPNGPrintRig(t, 60, 40)
		r.fake.printErr = c.err
		r.openPrint()
		r.confirm()
		r.waitIdle()
		r.settle()
		if !r.app.ModalsOpen() {
			t.Errorf("%v: no error box", c.err)
		}
		if got := printErrorKey(c.err); got != c.key {
			t.Errorf("%v: key %q, want %q", c.err, got, c.key)
		}
		if r.app.Root.ToastText() == i18n.T("toast.printed") {
			t.Errorf("%v: an error must not end with the success toast", c.err)
		}
	}
}

func TestCancelledSystemPrintIsSilent(t *testing.T) {
	r := newPNGPrintRig(t, 60, 40)
	r.fake.printErr = printing.ErrCanceled
	r.openPrint()
	r.confirm()
	r.waitIdle()
	r.settle()
	if r.app.ModalsOpen() {
		t.Fatal("a cancelled job must not show an error")
	}
	if r.app.Root.ToastText() != "" {
		t.Fatalf("toast %q", r.app.Root.ToastText())
	}
}

func TestPrintingRunsInTheBackgroundAndBlocksASecondJob(t *testing.T) {
	r := newPNGPrintRig(t, 60, 40)
	gate := make(chan struct{})
	r.fake.gate = gate
	r.openPrint()
	r.confirm()
	eventually(t, r.app, "the printer is called", func() bool { return r.fake.callCount("print") == 1 })
	if !r.app.printBusy {
		t.Fatal("the app must be busy while the printer works")
	}
	if r.app.Commands.Get(CmdPrint).IsEnabled() || r.app.Commands.Get(CmdExportPDF).IsEnabled() {
		t.Fatal("the commands must be disabled while a job runs")
	}
	r.app.Commands.Run(CmdPrint)
	r.settle()
	if r.app.printDlg != nil {
		t.Fatal("a second dialog opened during a job")
	}
	close(gate)
	r.waitIdle()
	r.waitToast(i18n.T("toast.printed"))
	if !r.app.Commands.Get(CmdPrint).IsEnabled() {
		t.Fatal("the command must come back")
	}
}

func TestSystemDialogChoiceIsUsedForThePrintJob(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	r.fake.system = true
	r.fake.sysTarget = printing.Target{Printer: "Office Laser", Copies: 4, FromPage: 2, ToPage: 3, Collate: true}
	r.openPrint()
	if r.app.printDlg.Rect(printdlg.PartSystem).Empty() {
		t.Fatal("the system dialog button is missing")
	}
	r.app.Eng.SendMouseMove(0, 0)
	cx, cy := centreOf(r.app.printDlg.Rect(printdlg.PartSystem))
	r.app.Eng.SendMouseMove(cx, cy)
	r.app.Eng.SendMouseButton(cx, cy, widget.MouseLeft, true)
	r.app.Eng.SendMouseButton(cx, cy, widget.MouseLeft, false)
	eventually(t, r.app, "the system dialog is called", func() bool { return r.fake.callCount("system") == 1 })
	eventually(t, r.app, "the choice reaches the dialog", func() bool { return r.app.printDlg.Model().Printer() == "Office Laser" })
	if r.app.printDlg.Model().Copies() != 4 {
		t.Fatalf("copies %d", r.app.printDlg.Model().Copies())
	}
	job := r.fake.sysJobs[0]
	if err := job.Validate(); err != nil || job.Name != "photo.png" || len(job.Pages) != 1 {
		t.Fatalf("the system dialog got an invalid job: %v %+v", err, job)
	}
	r.app.printDlg.Model().SetCopies(5)
	px, py := centreOf(r.app.printDlg.Rect(printdlg.PartPrint))
	r.app.Eng.SendMouseMove(px, py)
	r.app.Eng.SendMouseButton(px, py, widget.MouseLeft, true)
	r.app.Eng.SendMouseButton(px, py, widget.MouseLeft, false)
	eventually(t, r.app, "the job reaches the printer", func() bool { return r.fake.printedCount() == 1 })
	got := r.fake.lastPrinted().target
	if got.Printer != "Office Laser" || got.Copies != 5 || got.FromPage != 0 || got.ToPage != 0 || !got.Collate {
		t.Fatalf("target %+v", got)
	}
}

func centreOf(rc image.Rectangle) (int, int) {
	return (rc.Min.X + rc.Max.X) / 2, (rc.Min.Y + rc.Max.Y) / 2
}

func TestSystemDialogIsOfferedOnlyWhenTheSystemHasOne(t *testing.T) {
	r := newPNGPrintRig(t, 60, 40)
	r.openPrint()
	if !r.app.printDlg.Rect(printdlg.PartSystem).Empty() {
		t.Fatal("no system dialog on this platform, no button")
	}
}

func TestCancelledSystemDialogChangesNothing(t *testing.T) {
	r := newPNGPrintRig(t, 60, 40)
	r.fake.system = true
	r.fake.sysErr = printing.ErrCanceled
	r.openPrint()
	r.app.printDlg.Model().SetCopies(2)
	cx, cy := centreOf(r.app.printDlg.Rect(printdlg.PartSystem))
	r.app.Eng.SendMouseMove(cx, cy)
	r.app.Eng.SendMouseButton(cx, cy, widget.MouseLeft, true)
	r.app.Eng.SendMouseButton(cx, cy, widget.MouseLeft, false)
	eventually(t, r.app, "the system dialog is called", func() bool { return r.fake.callCount("system") == 1 })
	r.settle()
	if r.app.printDlg.Model().Copies() != 2 || r.app.ModalsOpen() && r.app.printDlg == nil {
		t.Fatal("a cancelled system dialog must change nothing")
	}
	if r.app.sysTarget != nil {
		t.Fatal("no target must be kept")
	}
}

func TestExportToPDFAsksForTheFileAndWritesAOnePagePDF(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	r.fullResolution()
	out := filepath.Join(r.dir, "result.pdf")
	r.dialog.path = out
	r.openExport()
	if r.app.printDlg.Widget().Title != i18n.T("print.titleExport") {
		t.Fatalf("title %q", r.app.printDlg.Widget().Title)
	}
	if !r.app.printDlg.Model().PDF() || !r.app.printDlg.Rect(printdlg.PartPrinter).Empty() {
		t.Fatal("the export dialog has no printer")
	}
	r.confirm()
	data := r.waitFile(out)
	r.waitIdle()
	f := parsePDF(t, data)
	if f.pages != 1 {
		t.Fatalf("%d pages", f.pages)
	}
	if !within(f.mediaW, 841.9, 0.2) || !within(f.mediaH, 595.2, 0.2) {
		t.Fatalf("page %.2f x %.2f pt, want A4 landscape", f.mediaW, f.mediaH)
	}
	if f.imgW != 3508 || f.imgH != 2480 {
		t.Fatalf("image %dx%d", f.imgW, f.imgH)
	}
	if f.colour != "/DeviceRGB" {
		t.Fatalf("colour space %s", f.colour)
	}
	if f.title != "photo.png" || f.creator != "PixGo" {
		t.Fatalf("title %q creator %q", f.title, f.creator)
	}
	img := f.decode(t)
	mid := color.NRGBAModel.Convert(img.At(f.imgW/2, f.imgH/2)).(color.NRGBA)
	want := gradientNRGBA(600, 400).NRGBAAt(300, 200)
	if absDiff(mid.R, want.R) > 12 || absDiff(mid.G, want.G) > 12 || absDiff(mid.B, want.B) > 12 {
		t.Fatalf("the centre of the PDF page is %v, the picture has %v", mid, want)
	}
	r.waitToast(i18n.T("toast.pdfSaved", "result.pdf"))
	if r.fake.callCount("print") != 0 {
		t.Fatal("exporting must not print")
	}
}

func TestPDFFileDialogStartsInThePictureFolderWithTheRightName(t *testing.T) {
	r := newPNGPrintRig(t, 60, 40)
	r.dialog.path = ""
	r.openExport()
	r.confirm()
	eventually(t, r.app, "the file dialog is asked", func() bool { return r.dialog.asked() == 1 })
	o := r.dialog.last()
	if o.Name != "photo.pdf" || o.Dir != r.dir {
		t.Fatalf("%+v", o)
	}
	if len(o.Filters) != 1 || o.Filters[0].Patterns[0] != "*.pdf" || o.Filters[0].Name != i18n.T("print.pdfFilter") {
		t.Fatalf("filters %+v", o.Filters)
	}
	if o.Title != i18n.T("print.titleExport") {
		t.Fatalf("title %q", o.Title)
	}
	r.settle()
	time.Sleep(20 * time.Millisecond)
	r.settle()
	if r.fake.callCount("pdf") != 0 {
		t.Fatal("cancelling the file dialog must write nothing")
	}
	if r.app.printBusy {
		t.Fatal("the app stays busy")
	}
}

func TestPDFExtensionIsAddedWhenTheUserLeavesItOut(t *testing.T) {
	r := newPNGPrintRig(t, 60, 40)
	r.dialog.path = filepath.Join(r.dir, "named")
	r.openExport()
	r.confirm()
	data := r.waitFile(filepath.Join(r.dir, "named.pdf"))
	if parsePDF(t, data).pages != 1 {
		t.Fatal("not a one page PDF")
	}
}

func TestSavingAsPDFFromThePrintDialogUsesTheSamePath(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	out := filepath.Join(r.dir, "via-print.pdf")
	r.dialog.path = out
	r.openPrint()
	r.app.printDlg.Model().SetDestIndex(len(r.app.printDlg.Model().Destinations()) - 1)
	r.app.printDlg.Model().SetPaper(pixprint.A5.ID)
	r.confirm()
	data := r.waitFile(out)
	f := parsePDF(t, data)
	if !within(f.mediaW, 595.2, 0.3) || !within(f.mediaH, 419.5, 0.3) {
		t.Fatalf("A5 landscape expected, got %.2f x %.2f", f.mediaW, f.mediaH)
	}
	if r.app.Settings.Get().PrintDestination != settings.PrintDestinationPDF {
		t.Fatalf("destination %q", r.app.Settings.Get().PrintDestination)
	}
	if r.fake.callCount("print") != 0 {
		t.Fatal("PDF must not go to a printer")
	}
}

func TestExportDoesNotChangeTheRememberedPrinter(t *testing.T) {
	r := newPNGPrintRig(t, 60, 40)
	r.dialog.path = filepath.Join(r.dir, "x.pdf")
	r.app.Settings.Update(func(s *settings.Settings) { s.PrintDestination = "Office Laser" })
	r.openExport()
	r.app.printDlg.Model().SetPaper(pixprint.A3.ID)
	r.confirm()
	r.waitFile(filepath.Join(r.dir, "x.pdf"))
	r.waitIdle()
	st := r.app.Settings.Get()
	if st.PrintDestination != "Office Laser" || st.PrintPaper != "a3" {
		t.Fatalf("%+v", st)
	}
}

func TestPDFWriteFailureIsReported(t *testing.T) {
	r := newPNGPrintRig(t, 60, 40)
	r.dialog.path = filepath.Join(r.dir, "x.pdf")
	r.fake.pdfErr = errors.New("disk full")
	r.openExport()
	r.confirm()
	r.waitIdle()
	r.settle()
	if !r.app.ModalsOpen() {
		t.Fatal("no error box")
	}
	if _, err := os.Stat(filepath.Join(r.dir, "x.pdf")); err == nil {
		t.Fatal("a failed export left a file")
	}
}

func TestEditedPictureIsPrintedNotTheOriginal(t *testing.T) {
	r := newPrintRig(t, "photo.png", func(t *testing.T, p string) { writePNGFile(t, p, 640, 480) }, nil)
	r.app.Commands.Run(CmdEditStart)
	r.settle()
	if !r.app.Editing() {
		t.Fatal("no edit mode")
	}
	r.app.Editor.SetOutputSize(320, 240)
	r.app.Commands.Run(CmdPrint)
	r.waitDialog()
	m := r.app.printDlg.Model()
	if m.Picture().W != 320 || m.Picture().H != 240 {
		t.Fatalf("the dialog sees %+v, not the edited 320x240", m.Picture())
	}
	m.SetPaper(pixprint.A5.ID)
	r.confirm()
	eventually(t, r.app, "the job reaches the printer", func() bool { return r.fake.printedCount() == 1 })
	if !r.app.Editing() {
		t.Fatal("printing must not leave the edit mode")
	}
}

func TestPrintIsAllowedInTheEditMode(t *testing.T) {
	r := newPNGPrintRig(t, 640, 480)
	r.app.Commands.Run(CmdEditStart)
	r.settle()
	for _, id := range []commands.ID{CmdPrint, CmdExportPDF} {
		if !r.app.Commands.Get(id).IsEnabled() {
			t.Errorf("%s is disabled in the edit mode", id)
		}
	}
}

func TestRotatedPictureIsPrintedAsOnScreen(t *testing.T) {
	r := newPNGPrintRig(t, 600, 400)
	r.app.Commands.Run(CmdRotateRight)
	r.settle()
	r.openPrint()
	m := r.app.printDlg.Model()
	if m.Picture().W != 400 || m.Picture().H != 600 {
		t.Fatalf("a rotated picture is %+v", m.Picture())
	}
	r.confirm()
	eventually(t, r.app, "the job reaches the printer", func() bool { return r.fake.printedCount() == 1 })
	got := r.fake.lastPrinted().job
	if got.Setup.Orientation != printing.Portrait {
		t.Fatalf("a portrait picture needs a portrait sheet, got %v", got.Setup.Orientation)
	}
}

func TestExifOrientationIsAppliedToThePrint(t *testing.T) {
	r := newPrintRig(t, "photo.jpg", func(t *testing.T, p string) { writeOrientedJPEG(t, p, 80, 40, 6) }, nil)
	r.openPrint()
	m := r.app.printDlg.Model()
	if m.Picture().W != 40 || m.Picture().H != 80 {
		t.Fatalf("a picture stored 80x40 with orientation 6 is shown 40x80, got %+v", m.Picture())
	}
}

func TestAnimatedPictureIsPrintedFromTheCurrentFrame(t *testing.T) {
	r := newPrintRig(t, "anim.gif", func(t *testing.T, p string) { writeAnimatedGIF(t, p) }, nil)
	r.openPrint()
	m := r.app.printDlg.Model()
	if m.Picture().W != 32 || m.Picture().H != 32 {
		t.Fatalf("%+v", m.Picture())
	}
	r.confirm()
	eventually(t, r.app, "the job reaches the printer", func() bool { return r.fake.printedCount() == 1 })
	page := r.fake.lastPrinted().job.Pages[0]
	if w, h := printing.PagePixels(printing.A4, printing.Portrait, printDPI); page.Bounds().Dx() != w || page.Bounds().Dy() != h {
		t.Fatalf("a square picture goes on a portrait sheet, got %v", page.Bounds())
	}
}

func TestSVGIsDrawnAtThePrintResolution(t *testing.T) {
	r := newPrintRig(t, "logo.svg", func(t *testing.T, p string) { writeSVGFile(t, p) }, nil)
	r.openPrint()
	m := r.app.printDlg.Model()
	if m.Picture().W != 64 || m.Picture().H != 48 || m.Picture().DPI != 96 {
		t.Fatalf("%+v", m.Picture())
	}
	m.SetPlacement(pixprint.PlaceFit)
	m.SetMargin(pixprint.MarginNone)
	r.confirm()
	eventually(t, r.app, "the job reaches the printer", func() bool { return r.fake.printedCount() == 1 })
	r.waitIdle()
	page := r.fake.lastPrinted().job.Pages[0].(*image.RGBA)
	b := page.Bounds()
	if c := page.RGBAAt(b.Dx()/2, b.Dy()/2); c.R < 240 || c.G > 20 || c.B > 20 {
		t.Fatalf("the centre of the rectangle is %v, not red", c)
	}
	edge := page.RGBAAt(b.Dx()/2+b.Dx()*45/100, b.Dy()/2)
	if edge.R < 200 || edge.G > 60 {
		t.Fatalf("a vector edge near the border of a 3508 px wide print must be sharp red, got %v", edge)
	}
}

type regionCall struct {
	shift int
	rect  image.Rectangle
}

type regionFake struct {
	*bigFake
	mu      sync.Mutex
	regions []regionCall
}

type recordingReader struct {
	codec.TileReader
	owner *regionFake
}

func (f *regionFake) OpenTiled(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.TileReader, error) {
	rd, err := f.bigFake.OpenTiled(ctx, r, size, lim)
	if err != nil {
		return nil, err
	}
	return recordingReader{TileReader: rd, owner: f}, nil
}

func (r recordingReader) Region(ctx context.Context, shift int, rc image.Rectangle) (*image.NRGBA, error) {
	r.owner.mu.Lock()
	r.owner.regions = append(r.owner.regions, regionCall{shift, rc})
	r.owner.mu.Unlock()
	return r.TileReader.Region(ctx, shift, rc)
}
