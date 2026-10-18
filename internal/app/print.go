package app

import (
	"context"
	"errors"
	"image"
	"path/filepath"
	"strings"
	"time"

	"github.com/oops1/headless-gui/v3/printing"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/edit"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/platform"
	pixprint "github.com/oops1/PixGo/internal/print"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/printdlg"
	"github.com/oops1/PixGo/internal/ui/window"
)

const (
	printThumbSide  = 480
	pdfJPEGQuality  = 95
	systemDialogDPI = 72

	printCreator = "PixGo"
)

var (
	contextCanceled  = context.Canceled
	printerListLimit = 4 * time.Second
	printDPI         = pixprint.DefaultDPI
)

type printSource struct {
	src   pixprint.Source
	pic   pixprint.Picture
	thumb *image.NRGBA
	name  string
	path  string
}

type printCapture struct {
	path  string
	name  string
	build func(ctx contextT) (pixprint.Source, error)
	dpi   func(ctx contextT) float64
}

func (a *App) canPrint() bool {
	return a.page == window.PageViewer && a.hasImage() && a.printDlg == nil && !a.printBusy &&
		a.saveDlg == nil && a.resizeDlg == nil && !a.slideshowActive()
}

func (a *App) printPicture() { a.openPrint(false) }

func (a *App) exportPDF() { a.openPrint(true) }

func (a *App) openPrint(export bool) {
	if !a.canPrint() {
		return
	}
	capture, refusal := a.capturePrint()
	if capture == nil {
		a.Root.Toast(i18n.T(refusal))
		return
	}
	a.printBusy = true
	svc := a.printSvc
	a.Ctl.Async("print.prepare", func(ctx contextT) (any, error) {
		return a.preparePrint(ctx, capture, svc, export)
	}, func(v any, err error) {
		a.printBusy = false
		if err != nil {
			if !errors.Is(err, contextCanceled) {
				a.errorBox(i18n.T(printErrorKey(err)))
			}
			return
		}
		prep, _ := v.(*printPrepared)
		if prep == nil || a.printDlg != nil || !a.hasFile() && !a.editing {
			return
		}
		a.showPrintDialog(prep, export)
	})
}

type printPrepared struct {
	source   *printSource
	printers []printdlg.Printer
}

func (a *App) preparePrint(ctx contextT, c *printCapture, svc printService, export bool) (*printPrepared, error) {
	src, err := c.build(ctx)
	if err != nil {
		return nil, err
	}
	size := src.Size()
	if size.X < 1 || size.Y < 1 {
		return nil, pixprint.ErrNoPicture
	}
	ps := &printSource{src: src, name: c.name, path: c.path, pic: pixprint.Picture{W: size.X, H: size.Y}}
	if c.dpi != nil {
		ps.pic.DPI = c.dpi(ctx)
	}
	k := min(1, float64(printThumbSide)/float64(max(size.X, size.Y)))
	tw, th := max(1, int(float64(size.X)*k+.5)), max(1, int(float64(size.Y)*k+.5))
	thumb, err := src.Render(ctx, image.Rectangle{Max: size}, tw, th)
	if err != nil {
		return nil, err
	}
	ps.thumb = thumb
	out := &printPrepared{source: ps}
	if !export {
		out.printers = listPrinters(ctx, svc)
	}
	return out, nil
}

func listPrinters(ctx contextT, svc printService) []printdlg.Printer {
	type answer struct {
		list []printing.Printer
		err  error
	}
	ch := make(chan answer, 1)
	go func() {
		l, err := svc.Printers()
		ch <- answer{l, err}
	}()
	var got answer
	select {
	case got = <-ch:
	case <-ctx.Done():
		return nil
	case <-time.After(printerListLimit):
		return nil
	}
	if got.err != nil {
		return nil
	}
	out := make([]printdlg.Printer, 0, len(got.list))
	for _, p := range got.list {
		if p.Name == "" {
			continue
		}
		out = append(out, printdlg.Printer{Name: p.Name, Description: p.Description, Default: p.Default})
	}
	return out
}

func (a *App) printInitial(export bool) printdlg.Values {
	st := a.Settings.Get()
	opts := pixprint.Options{
		Paper:       st.PrintPaper,
		Orientation: pixprint.ParseOrientation(st.PrintOrientation),
		Placement:   pixprint.ParsePlacement(st.PrintPlacement),
		Margin:      pixprint.ParseMargin(st.PrintMargins),
		Unit:        pixprint.ParseUnit(st.PrintUnit),
		Keep:        true,
	}.Normalize()
	v := printdlg.Values{Options: opts, Copies: 1}
	if export || st.PrintDestination == settings.PrintDestinationPDF {
		v.PDF = true
	} else {
		v.Printer = st.PrintDestination
	}
	return v
}

func (a *App) rememberPrint(v printdlg.Values, export bool) {
	a.Settings.Update(func(s *settings.Settings) {
		o := v.Options
		s.PrintPaper = o.Paper
		s.PrintOrientation = o.Orientation.ID()
		s.PrintPlacement = o.Placement.ID()
		s.PrintMargins = o.Margin.ID()
		s.PrintUnit = o.Unit.ID()
		switch {
		case export:
		case v.PDF:
			s.PrintDestination = settings.PrintDestinationPDF
		default:
			s.PrintDestination = v.Printer
		}
	})
}

func (a *App) showPrintDialog(prep *printPrepared, export bool) {
	ps := prep.source
	svc := a.printSvc
	env := printdlg.Env{Pic: ps.pic, Preview: ps.thumb, Printers: prep.printers, Export: export}
	var opts []printdlg.Option
	if !export && svc.HasSystemDialog() {
		env.System = true
		opts = append(opts, printdlg.WithSystemDialog(func(v printdlg.Values, apply func(string, int)) {
			a.systemDialog(ps, v, apply)
		}))
	}
	a.sysTarget = nil
	a.modalOpen.Store(true)
	a.printDlg = printdlg.Show(a.Eng, env, a.printInitial(export), func(v printdlg.Values, ok bool) {
		a.modalOpen.Store(false)
		a.Eng.Post(func() {
			a.printDlg = nil
			a.focusPage()
			if !ok {
				return
			}
			a.rememberPrint(v, export)
			a.runPrint(ps, v, export)
		})
	}, opts...)
}

func (a *App) runPrint(ps *printSource, v printdlg.Values, export bool) {
	if v.PDF {
		base := strings.TrimSuffix(ps.name, filepath.Ext(ps.name))
		a.printBusy = true
		a.askPDFPath(filepath.Dir(ps.path), base+".pdf", func(path string) {
			a.writePDF(ps, v.Options, path)
		}, func() {
			a.printBusy = false
			a.Root.Refresh()
		})
		return
	}
	a.sendToPrinter(ps, v)
}

func printingPaper(p pixprint.Paper) printing.Paper {
	switch p.ID {
	case pixprint.A4.ID:
		return printing.A4
	case pixprint.A5.ID:
		return printing.A5
	case pixprint.A3.ID:
		return printing.A3
	case pixprint.Letter.ID:
		return printing.Letter
	case pixprint.Legal.ID:
		return printing.Legal
	}
	return printing.CustomPaper(p.ID+"cm", p.ShortMM, p.LongMM)
}

func printSetup(o pixprint.Options, page pixprint.Page) printing.PageSetup {
	setup := printing.PageSetup{
		Paper:   printingPaper(o.PaperValue()),
		Margins: printing.UniformMargins(o.Margin.MM()),
		DPI:     page.DPI,
	}
	if page.Landscape {
		setup.Orientation = printing.Landscape
	}
	return setup
}

func (a *App) buildJob(ctx contextT, ps *printSource, o pixprint.Options) (printing.Job, error) {
	page, err := o.Layout(ps.pic, printDPI)
	if err != nil {
		return printing.Job{}, err
	}
	sheet, err := pixprint.Render(ctx, ps.src, page)
	if err != nil {
		return printing.Job{}, err
	}
	return printing.Job{Name: ps.name, Setup: printSetup(o, page), Pages: []image.Image{sheet}}, nil
}

func (a *App) sendToPrinter(ps *printSource, v printdlg.Values) {
	target := printing.Target{Printer: v.Printer, Copies: v.Copies}
	if st := a.sysTarget; st != nil && st.Printer == v.Printer {
		target = *st
		target.Copies = v.Copies
		target.FromPage, target.ToPage = 0, 0
	}
	svc := a.printSvc
	a.printBusy = true
	a.Root.Toast(i18n.T("toast.printPreparing"))
	a.Ctl.Async("print", func(ctx contextT) (any, error) {
		job, err := a.buildJob(ctx, ps, v.Options)
		if err != nil {
			return nil, err
		}
		a.Eng.Post(func() { a.Root.Toast(i18n.T("toast.printSending")) })
		return nil, svc.Print(target, job)
	}, func(_ any, err error) {
		a.printBusy = false
		a.Root.Refresh()
		switch {
		case err == nil:
			a.Root.Toast(i18n.T("toast.printed"))
		case errors.Is(err, printing.ErrCanceled), errors.Is(err, contextCanceled):
			a.Root.Toast("")
		default:
			a.Root.Toast("")
			a.errorBox(i18n.T(printErrorKey(err)))
		}
	})
}

func (a *App) writePDF(ps *printSource, o pixprint.Options, path string) {
	svc := a.printSvc
	a.printBusy = true
	a.Root.Toast(i18n.T("toast.printPreparing"))
	a.Ctl.Async("print", func(ctx contextT) (any, error) {
		job, err := a.buildJob(ctx, ps, o)
		if err != nil {
			return nil, err
		}
		return nil, svc.SavePDF(path, job, printing.PDFOptions{Title: ps.name, Creator: printCreator, JPEGQuality: pdfJPEGQuality})
	}, func(_ any, err error) {
		a.printBusy = false
		a.Root.Refresh()
		switch {
		case err == nil:
			a.Root.Toast(i18n.T("toast.pdfSaved", filepath.Base(path)))
		case errors.Is(err, contextCanceled):
			a.Root.Toast("")
		default:
			a.Root.Toast("")
			key := printErrorKey(err)
			if key == "error.printFailed" {
				key = "error.pdfFailed"
			}
			a.errorBox(i18n.T(key))
		}
	})
}

func (a *App) systemDialog(ps *printSource, v printdlg.Values, apply func(string, int)) {
	svc := a.printSvc
	o := v.Options
	a.Ctl.Async("print.system", func(ctx contextT) (any, error) {
		page, err := o.Layout(ps.pic, systemDialogDPI)
		if err != nil {
			return nil, err
		}
		blank := image.NewRGBA(image.Rectangle{Max: page.Sheet})
		job := printing.Job{Name: ps.name, Setup: printSetup(o, page), Pages: []image.Image{blank}}
		t, err := svc.SystemDialog(job)
		return t, err
	}, func(res any, err error) {
		switch {
		case err == nil:
		case errors.Is(err, printing.ErrCanceled), errors.Is(err, contextCanceled):
			return
		default:
			a.errorBox(i18n.T(printErrorKey(err)))
			return
		}
		t, ok := res.(printing.Target)
		if !ok {
			return
		}
		a.sysTarget = &t
		apply(t.Printer, t.Copies)
	})
}

func printErrorKey(err error) string {
	switch {
	case errors.Is(err, printing.ErrNoPrinter):
		return "error.printNoPrinter"
	case errors.Is(err, printing.ErrNoPrintService):
		return "error.printNoService"
	case errors.Is(err, printing.ErrUnsupported):
		return "error.printUnsupported"
	case errors.Is(err, pixprint.ErrTooLarge), errors.Is(err, codec.ErrTooLarge), errors.Is(err, edit.ErrSize),
		errors.Is(err, printing.ErrBadSetup):
		return "error.printTooLarge"
	}
	return "error.printFailed"
}

func (a *App) askPDFPath(dir, name string, done func(path string), cancelled func()) {
	finish := func(p string) {
		if p == "" {
			cancelled()
			return
		}
		if !strings.EqualFold(filepath.Ext(p), ".pdf") {
			p += ".pdf"
		}
		done(p)
	}
	if a.Platform != nil && a.Platform.SaveDialog != nil {
		opts := platform.SaveOptions{
			Title:       i18n.T("print.titleExport"),
			Dir:         dir,
			Name:        name,
			FilterIndex: 1,
			Filters:     []platform.FileFilter{{Name: i18n.T("print.pdfFilter"), Patterns: []string{"*.pdf"}}},
		}
		a.Ctl.Async("savePdfDialog", func(ctx contextT) (any, error) {
			p, ok, err := a.Platform.SaveDialog.Save(opts)
			if err != nil || !ok {
				return "", err
			}
			return p, nil
		}, func(v any, err error) {
			if p, _ := v.(string); err == nil && p != "" {
				finish(p)
				return
			}
			if errors.Is(err, platform.ErrUnsupported) {
				a.pdfEngineDialog(dir, name, finish)
				return
			}
			cancelled()
		})
		return
	}
	a.pdfEngineDialog(dir, name, finish)
}

func (a *App) pdfEngineDialog(dir, name string, finish func(string)) {
	widget.NewMessageBox(a.Eng).ShowSaveFile(widget.FileDialogOptions{
		Title:       i18n.T("print.titleExport"),
		StartDir:    dir,
		InitialName: name,
		Filters:     []widget.FileFilter{{Label: i18n.T("print.pdfFilter"), Exts: []string{".pdf"}}},
	}, func(path string, ok bool) {
		if ok {
			finish(path)
			return
		}
		finish("")
	})
}

type tileCloser struct {
	codec.TileReader
	src *codec.Source
}

func (t tileCloser) Close() error {
	err := t.TileReader.Close()
	if e := t.src.Close(); err == nil {
		err = e
	}
	return err
}
