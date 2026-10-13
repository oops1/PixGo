//go:build !windows

package platform

import "os"

const defaultBodyPt = 10.0

type osFonts struct{}

func NewSystemFonts() SystemFonts { return osFonts{} }

func (osFonts) BodyPt() float64 {
	if pt, ok := envFontPt(os.Getenv); ok {
		return pt
	}
	if pt, ok := gtkFontPt(gtkSettingsFiles()); ok {
		return pt
	}
	return defaultBodyPt
}
