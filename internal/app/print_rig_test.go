package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/printing"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	codecall "github.com/oops1/PixGo/internal/codec/all"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/settings"
)

type printedJob struct {
	target printing.Target
	job    printing.Job
}

type fakePrinting struct {
	mu sync.Mutex

	printers     []printing.Printer
	printersErr  error
	printersGate chan struct{}

	printed  []printedJob
	printErr error
	gate     chan struct{}

	system    bool
	sysTarget printing.Target
	sysErr    error
	sysJobs   []printing.Job

	pdfs   []string
	pdfErr error
	calls  []string
}

func (f *fakePrinting) note(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakePrinting) Printers() ([]printing.Printer, error) {
	f.note("printers")
	if f.printersGate != nil {
		<-f.printersGate
	}
	return f.printers, f.printersErr
}

func (f *fakePrinting) Print(t printing.Target, job printing.Job) error {
	f.note("print")
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.printErr != nil {
		return f.printErr
	}
	f.printed = append(f.printed, printedJob{t, job})
	return nil
}

func (f *fakePrinting) HasSystemDialog() bool { return f.system }

func (f *fakePrinting) SystemDialog(job printing.Job) (printing.Target, error) {
	f.note("system")
	f.mu.Lock()
	f.sysJobs = append(f.sysJobs, job)
	f.mu.Unlock()
	return f.sysTarget, f.sysErr
}

func (f *fakePrinting) SavePDF(path string, job printing.Job, opt printing.PDFOptions) error {
	f.note("pdf")
	f.mu.Lock()
	f.pdfs = append(f.pdfs, path)
	err := f.pdfErr
	f.mu.Unlock()
	if err != nil {
		return err
	}
	return printing.SavePDF(path, job, opt)
}

func (f *fakePrinting) printedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.printed)
}

func (f *fakePrinting) lastPrinted() printedJob {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.printed[len(f.printed)-1]
}

func (f *fakePrinting) callCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == name {
			n++
		}
	}
	return n
}

func twoPrinters() []printing.Printer {
	return []printing.Printer{
		{Name: "Office Laser", Accepting: true},
		{Name: "Photo Inkjet", Accepting: true, Default: true},
	}
}

type recordingSaveDialog struct {
	mu   sync.Mutex
	path string
	opts []platform.SaveOptions
}

func (d *recordingSaveDialog) Save(o platform.SaveOptions) (string, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.opts = append(d.opts, o)
	return d.path, d.path != "", nil
}

func (d *recordingSaveDialog) asked() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.opts)
}

func (d *recordingSaveDialog) last() platform.SaveOptions {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.opts[len(d.opts)-1]
}

type printRig struct {
	t      *testing.T
	app    *App
	fake   *fakePrinting
	dialog *recordingSaveDialog
	dir    string
	src    string
}

func newPrintRig(t *testing.T, name string, write func(*testing.T, string), mutate func(*settings.Settings), extra ...codec.Decoder) *printRig {
	t.Helper()
	reg := codec.NewRegistry()
	codecall.Register(reg)
	for _, d := range extra {
		reg.RegisterDecoder(d)
	}
	a := formatsApp(t, reg, mutate)
	r := &printRig{t: t, app: a, fake: &fakePrinting{printers: twoPrinters()}, dialog: &recordingSaveDialog{}}
	a.printSvc = r.fake
	oldDPI := printDPI
	printDPI = 100
	t.Cleanup(func() { printDPI = oldDPI })
	a.Platform = &platform.Platform{SaveDialog: r.dialog}
	r.dir = t.TempDir()
	r.src = filepath.Join(r.dir, name)
	write(t, r.src)
	a.OpenFiles([]string{r.src})
	waitName(t, a, name)
	a.Root.Refresh()
	r.settle()
	return r
}

func (r *printRig) fullResolution() {
	r.t.Helper()
	old := printDPI
	printDPI = 300
	r.t.Cleanup(func() { printDPI = old })
}

func newPNGPrintRig(t *testing.T, w, h int) *printRig {
	return newPrintRig(t, "photo.png", func(t *testing.T, p string) { writePNGFile(t, p, w, h) }, nil)
}

func (r *printRig) settle() {
	for i := 0; i < 4; i++ {
		r.app.Eng.Flush()
	}
	r.app.Eng.RenderOnce()
}

func (r *printRig) press(code widget.KeyCode) {
	r.app.Eng.SendKeyEvent(widget.KeyEvent{Code: code, Pressed: true})
	r.app.Eng.SendKeyEvent(widget.KeyEvent{Code: code, Pressed: false})
}

func (r *printRig) typeText(s string) {
	for _, ch := range s {
		code := widget.KeyUnknown
		if ch >= '0' && ch <= '9' {
			code = widget.KeyCode(ch)
		}
		r.app.Eng.SendKeyEvent(widget.KeyEvent{Code: code, Rune: ch, Pressed: true})
		r.app.Eng.SendKeyEvent(widget.KeyEvent{Code: code, Rune: ch, Pressed: false})
	}
}

func (r *printRig) waitDialog() {
	r.t.Helper()
	eventually(r.t, r.app, "the print dialog opens", func() bool { return r.app.printDlg != nil })
	r.settle()
}

func (r *printRig) openPrint() {
	r.t.Helper()
	r.app.Commands.Run(CmdPrint)
	r.waitDialog()
}

func (r *printRig) openExport() {
	r.t.Helper()
	r.app.Commands.Run(CmdExportPDF)
	r.waitDialog()
}

func (r *printRig) waitIdle() {
	r.t.Helper()
	eventually(r.t, r.app, "the print job is finished", func() bool { return !r.app.printBusy && r.app.printDlg == nil })
}

func (r *printRig) confirm() {
	r.t.Helper()
	r.press(widget.KeyEnter)
	eventually(r.t, r.app, "the dialog closes", func() bool { return r.app.printDlg == nil })
}

func (r *printRig) waitToast(want string) {
	r.t.Helper()
	eventually(r.t, r.app, "the toast "+want, func() bool { return r.app.Root.ToastText() == want })
}

func (r *printRig) waitFile(path string) []byte {
	r.t.Helper()
	var data []byte
	eventually(r.t, r.app, "the file "+filepath.Base(path), func() bool {
		b, err := os.ReadFile(path)
		data = b
		return err == nil && len(b) > 0
	})
	return data
}

func writePNGWithDPI(t *testing.T, path string, w, h int, dpi float64) {
	t.Helper()
	writePNGFile(t, path, w, h)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ppm := uint32(dpi/0.0254 + .5)
	body := make([]byte, 9)
	binary.BigEndian.PutUint32(body[0:], ppm)
	binary.BigEndian.PutUint32(body[4:], ppm)
	body[8] = 1
	chunk := binary.BigEndian.AppendUint32(nil, 9)
	chunk = append(chunk, "pHYs"...)
	chunk = append(chunk, body...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(append([]byte("pHYs"), body...)))
	at := 8 + 25
	out := append(append(append([]byte{}, data[:at]...), chunk...), data[at:]...)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

type pdfFacts struct {
	pages    int
	mediaW   float64
	mediaH   float64
	imgW     int
	imgH     int
	colour   string
	filter   string
	title    string
	creator  string
	imageRaw []byte
}

var (
	rePDFPage   = regexp.MustCompile(`/Type /Page /Parent`)
	rePDFMedia  = regexp.MustCompile(`/MediaBox \[0 0 ([0-9.]+) ([0-9.]+)\]`)
	rePDFImage  = regexp.MustCompile(`/Subtype /Image /Width (\d+) /Height (\d+) /ColorSpace (/\w+) /BitsPerComponent 8 /Filter (/\w+)[^>]*?/Length (\d+) >>\nstream\n`)
	rePDFTitle  = regexp.MustCompile(`/Title \(([^)]*)\)`)
	rePDFCreate = regexp.MustCompile(`/Creator \(([^)]*)\)`)
)

func parsePDF(t *testing.T, data []byte) pdfFacts {
	t.Helper()
	var f pdfFacts
	if !bytes.HasPrefix(data, []byte("%PDF-")) || !bytes.HasSuffix(bytes.TrimSpace(data), []byte("%%EOF")) {
		t.Fatalf("not a PDF: %.20q ... %.12q", data, data[max(0, len(data)-12):])
	}
	f.pages = len(rePDFPage.FindAll(data, -1))
	if m := rePDFMedia.FindSubmatch(data); m != nil {
		f.mediaW, _ = strconv.ParseFloat(string(m[1]), 64)
		f.mediaH, _ = strconv.ParseFloat(string(m[2]), 64)
	}
	if loc := rePDFImage.FindSubmatchIndex(data); loc != nil {
		m := rePDFImage.FindSubmatch(data)
		f.imgW, _ = strconv.Atoi(string(m[1]))
		f.imgH, _ = strconv.Atoi(string(m[2]))
		f.colour, f.filter = string(m[3]), string(m[4])
		n, _ := strconv.Atoi(string(m[5]))
		f.imageRaw = data[loc[1] : loc[1]+n]
	}
	if m := rePDFTitle.FindSubmatch(data); m != nil {
		f.title = string(m[1])
	}
	if m := rePDFCreate.FindSubmatch(data); m != nil {
		f.creator = string(m[1])
	}
	return f
}

func (f pdfFacts) decode(t *testing.T) image.Image {
	t.Helper()
	if f.filter != "/DCTDecode" {
		t.Fatalf("filter %s", f.filter)
	}
	img, err := jpeg.Decode(bytes.NewReader(f.imageRaw))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func within(a, b, tol float64) bool {
	d := a - b
	return d <= tol && d >= -tol
}

var _ = context.Background
var _ = time.Second
