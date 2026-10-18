package app

import (
	"github.com/oops1/headless-gui/v3/printing"
)

type printService interface {
	Printers() ([]printing.Printer, error)
	Print(t printing.Target, job printing.Job) error
	HasSystemDialog() bool
	SystemDialog(job printing.Job) (printing.Target, error)
	SavePDF(path string, job printing.Job, opt printing.PDFOptions) error
}

type enginePrinting struct{}

func (enginePrinting) Printers() ([]printing.Printer, error) { return printing.Printers() }

func (enginePrinting) Print(t printing.Target, job printing.Job) error {
	return printing.Print(t, job)
}

func (enginePrinting) HasSystemDialog() bool { return printing.HasPrintDialog() }

func (enginePrinting) SystemDialog(job printing.Job) (printing.Target, error) {
	return printing.PrintDialog(0, job)
}

func (enginePrinting) SavePDF(path string, job printing.Job, opt printing.PDFOptions) error {
	return printing.SavePDF(path, job, opt)
}
