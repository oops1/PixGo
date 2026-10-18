package printdlg

import (
	"context"
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/print"
)

const (
	MinCopies = 1
	MaxCopies = 99

	maxCopiesDigits = 2
	maxSizeChars    = 7
	maxDecimals     = 2
	lowDPI          = 150
)

type Printer struct {
	Name        string
	Description string
	Default     bool
}

type Env struct {
	Pic      print.Picture
	Preview  *image.NRGBA
	Printers []Printer
	Export   bool
	System   bool
}

type Values struct {
	PDF     bool
	Printer string
	Copies  int
	Options print.Options
}

type Info struct {
	Err      error
	WidthMM  float64
	HeightMM float64
	DPI      float64
	Known    bool
	Cut      bool
	Low      bool
}

type Preview struct {
	Page  print.Page
	Sheet *image.RGBA
	Err   error
}

type previewKey struct {
	opts print.Options
	dpi  int
}

type Model struct {
	env    Env
	v      Values
	copies string
	wText  string
	hText  string

	pv    Preview
	pvKey previewKey
	pvOK  bool
}

func NewModel(env Env, v Values) *Model {
	m := &Model{env: env}
	v.Options = v.Options.Normalize()
	if env.Export {
		v.PDF = true
	}
	if !v.PDF {
		v.Printer = m.resolvePrinter(v.Printer)
		if v.Printer == "" {
			v.PDF = true
		}
	}
	if v.PDF {
		v.Printer = ""
	}
	if v.Copies < MinCopies || v.Copies > MaxCopies {
		v.Copies = MinCopies
	}
	m.v = v
	m.copies = strconv.Itoa(v.Copies)
	if m.v.Options.Placement == print.PlaceCustom && !m.v.Options.CustomValid() {
		m.initCustom()
	}
	m.syncTexts()
	return m
}

func (m *Model) resolvePrinter(want string) string {
	for _, p := range m.env.Printers {
		if p.Name == want && want != "" {
			return want
		}
	}
	for _, p := range m.env.Printers {
		if p.Default {
			return p.Name
		}
	}
	if len(m.env.Printers) > 0 {
		return m.env.Printers[0].Name
	}
	return ""
}

func (m *Model) Env() Env { return m.env }

func (m *Model) Picture() print.Picture { return m.env.Pic }

func (m *Model) Options() print.Options { return m.v.Options }

func (m *Model) PDF() bool { return m.v.PDF }

func (m *Model) Printer() string { return m.v.Printer }

func (m *Model) Destinations() []string {
	out := make([]string, 0, len(m.env.Printers)+1)
	for _, p := range m.env.Printers {
		out = append(out, p.Name)
	}
	return append(out, i18n.T("print.dest.pdf"))
}

func (m *Model) DestIndex() int {
	if !m.v.PDF {
		for i, p := range m.env.Printers {
			if p.Name == m.v.Printer {
				return i
			}
		}
	}
	return len(m.env.Printers)
}

func (m *Model) SetDestIndex(i int) {
	if i < 0 || i > len(m.env.Printers) {
		return
	}
	if i == len(m.env.Printers) {
		m.v.PDF, m.v.Printer = true, ""
		return
	}
	m.v.PDF, m.v.Printer = false, m.env.Printers[i].Name
}

func (m *Model) SetPrinter(name string) {
	for i, p := range m.env.Printers {
		if p.Name == name {
			m.SetDestIndex(i)
			return
		}
	}
}

func (m *Model) Copies() int { return m.v.Copies }

func (m *Model) CopiesText() string { return m.copies }

func (m *Model) SetCopies(n int) {
	m.v.Copies = min(max(n, MinCopies), MaxCopies)
	m.copies = strconv.Itoa(m.v.Copies)
}

func (m *Model) SetCopiesText(s string) {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' && b.Len() < maxCopiesDigits {
			b.WriteRune(r)
		}
	}
	m.copies = b.String()
	n, err := strconv.Atoi(m.copies)
	if err == nil && n >= MinCopies && n <= MaxCopies {
		m.v.Copies = n
	}
}

func (m *Model) copiesValid() bool {
	n, err := strconv.Atoi(m.copies)
	return err == nil && n >= MinCopies && n <= MaxCopies
}

func (m *Model) SetPaper(id string) {
	m.v.Options.Paper = id
	m.v.Options = m.v.Options.Normalize()
}

func (m *Model) SetOrientation(o print.Orientation) {
	m.v.Options.Orientation = o
	m.v.Options = m.v.Options.Normalize()
}

func (m *Model) SetMargin(g print.Margin) {
	m.v.Options.Margin = g
	m.v.Options = m.v.Options.Normalize()
}

func (m *Model) SetPlacement(p print.Placement) {
	m.v.Options.Placement = p
	m.v.Options = m.v.Options.Normalize()
	if p == print.PlaceCustom && !m.v.Options.CustomValid() {
		m.initCustom()
		m.syncTexts()
	}
}

func (m *Model) initCustom() {
	w, h := m.v.Options.FitSizeMM(m.env.Pic)
	if w <= 0 || h <= 0 {
		w, h = m.v.Options.ActualSizeMM(m.env.Pic)
	}
	m.v.Options.CustomW, m.v.Options.CustomH = print.ClampCustomMM(w, h)
	if m.v.Options.Keep {
		m.v.Options.CustomH = clampMM(print.KeepRatio(m.env.Pic, m.v.Options.CustomW, true))
	}
}

func clampMM(v float64) float64 {
	return math.Min(math.Max(v, print.MinCustomMM), print.MaxCustomMM)
}

func (m *Model) SetUnit(u print.Unit) {
	m.v.Options.Unit = u
	m.v.Options = m.v.Options.Normalize()
	m.syncTexts()
}

func (m *Model) SetKeep(on bool) {
	m.v.Options.Keep = on
	if on && m.v.Options.CustomValid() {
		m.v.Options.CustomH = clampMM(print.KeepRatio(m.env.Pic, m.v.Options.CustomW, true))
		m.syncTexts()
	}
}

func (m *Model) WidthText() string { return m.wText }

func (m *Model) HeightText() string { return m.hText }

func (m *Model) syncTexts() {
	o := m.v.Options
	if o.CustomW > 0 {
		m.wText = formatSize(o.CustomW, o.Unit)
	} else {
		m.wText = ""
	}
	if o.CustomH > 0 {
		m.hText = formatSize(o.CustomH, o.Unit)
	} else {
		m.hText = ""
	}
}

func formatSize(mm float64, u print.Unit) string {
	v := math.Round(mm/u.MM()*100) / 100
	s := strconv.FormatFloat(v, 'f', maxDecimals, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return strings.Replace(s, ".", i18n.CurrentLocale().Decimal, 1)
}

func CleanDecimal(s string) string {
	var b strings.Builder
	sep, frac := false, 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			if sep {
				if frac >= maxDecimals {
					continue
				}
				frac++
			}
			if b.Len() < maxSizeChars {
				b.WriteRune(r)
			}
		case (r == '.' || r == ',') && !sep && b.Len() < maxSizeChars:
			sep = true
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ParseSize(s string, u print.Unit) (float64, bool) {
	s = strings.Replace(strings.TrimSpace(s), ",", ".", 1)
	if s == "" || s == "." {
		return 0, false
	}
	dots := 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r == '.' && dots == 0:
			dots++
		default:
			return 0, false
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return 0, false
	}
	return v * u.MM(), true
}

func (m *Model) SetWidthText(s string) {
	m.wText = CleanDecimal(s)
	mm, ok := ParseSize(m.wText, m.v.Options.Unit)
	if !ok {
		m.v.Options.CustomW = 0
		return
	}
	m.v.Options.CustomW = math.Min(mm, 1e6)
	if m.v.Options.Keep {
		m.v.Options.CustomH = math.Min(print.KeepRatio(m.env.Pic, mm, true), 1e6)
		m.hText = formatSize(m.v.Options.CustomH, m.v.Options.Unit)
	}
}

func (m *Model) SetHeightText(s string) {
	m.hText = CleanDecimal(s)
	mm, ok := ParseSize(m.hText, m.v.Options.Unit)
	if !ok {
		m.v.Options.CustomH = 0
		return
	}
	m.v.Options.CustomH = math.Min(mm, 1e6)
	if m.v.Options.Keep {
		m.v.Options.CustomW = math.Min(print.KeepRatio(m.env.Pic, mm, false), 1e6)
		m.wText = formatSize(m.v.Options.CustomW, m.v.Options.Unit)
	}
}

func (m *Model) CustomActive() bool { return m.v.Options.Placement == print.PlaceCustom }

func (m *Model) SizeValid(width bool) bool {
	mm := m.v.Options.CustomH
	if width {
		mm = m.v.Options.CustomW
	}
	return mm >= print.MinCustomMM && mm <= print.MaxCustomMM
}

func (m *Model) Valid() bool {
	if !m.v.PDF && !m.copiesValid() {
		return false
	}
	if m.CustomActive() && !m.v.Options.CustomValid() {
		return false
	}
	_, err := m.Page()
	return err == nil
}

func (m *Model) CopiesValid() bool { return m.v.PDF || m.copiesValid() }

func (m *Model) Values() Values {
	v := m.v
	if v.PDF {
		v.Printer = ""
		v.Copies = MinCopies
	}
	return v
}

func (m *Model) Page() (print.Page, error) {
	return m.v.Options.Layout(m.env.Pic, print.DefaultDPI)
}

func (m *Model) Info() Info {
	pg, err := m.Page()
	if err != nil {
		return Info{Err: err}
	}
	w, h := pg.DestMM()
	return Info{
		WidthMM: w, HeightMM: h,
		DPI: pg.PrintDPI, Known: m.env.Pic.Known(),
		Cut: pg.Cut, Low: pg.PrintDPI < lowDPI,
	}
}

func (m *Model) Sheet(maxW, maxH int) Preview {
	o := m.v.Options
	land := o.Landscape(m.env.Pic)
	wmm, hmm := o.PaperValue().Size(land)
	if maxW < 1 || maxH < 1 {
		return Preview{Err: print.ErrNoRoom}
	}
	perMM := math.Min(float64(maxW)/wmm, float64(maxH)/hmm)
	dpi := max(1, int(math.Floor(perMM*25.4)))
	key := previewKey{opts: o, dpi: dpi}
	if m.pvOK && m.pvKey == key {
		return m.pv
	}
	pg, err := o.Layout(m.env.Pic, dpi)
	pv := Preview{Page: pg, Err: err}
	if err != nil {
		frame := o
		frame.Placement = print.PlaceFit
		if fp, ferr := frame.Layout(m.env.Pic, dpi); ferr == nil {
			fp.Dest, fp.Crop = image.Rectangle{}, image.Rectangle{}
			pv.Page = fp
		}
	}
	if err == nil && m.env.Preview != nil && !m.env.Preview.Bounds().Empty() {
		src := print.Proxy{Source: print.ImageSource{Img: m.env.Preview}, Full: image.Pt(m.env.Pic.W, m.env.Pic.H)}
		if sheet, rerr := print.Render(context.Background(), src, pg); rerr == nil {
			pv.Sheet = sheet
		}
	}
	m.pv, m.pvKey, m.pvOK = pv, key, true
	return pv
}
