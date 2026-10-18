package colorm

import (
	"image"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func systemProfiles(t *testing.T) []string {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("the real profiles shipped with Windows are only read there")
	}
	dir := filepath.Join(os.Getenv("SystemRoot"), "System32", "spool", "drivers", "color")
	var files []string
	for _, pat := range []string{"*.icm", "*.icc", "*.ICC"} {
		m, _ := filepath.Glob(filepath.Join(dir, pat))
		files = append(files, m...)
	}
	if len(files) == 0 {
		t.Skip("no system profiles")
	}
	return files
}

func TestWindowsSystemProfilesParseAndConvert(t *testing.T) {
	for _, f := range systemProfiles(t) {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		p, err := ParseICC(data)
		if err != nil {
			continue
		}
		for _, in := range []Intent{RelativeColorimetric, Perceptual} {
			tf, err := NewTransform(p, SRGB(), Options{Intent: in})
			if err != nil {
				t.Errorf("%s: %v", filepath.Base(f), err)
				continue
			}
			if p.Channels() == 4 {
				c := image.NewCMYK(image.Rect(0, 0, 2, 1))
				c.Pix[4+3] = 255
				o := image.NewNRGBA(c.Rect)
				if err := tf.ConvertCMYK(o, c); err != nil {
					t.Fatal(err)
				}
				if o.Pix[0] < 240 || o.Pix[4] > 40 || o.Pix[5] > 40 || o.Pix[6] > 40 {
					t.Errorf("%s: SWOP-like white %v black %v", filepath.Base(f), o.Pix[:3], o.Pix[4:7])
				}
				continue
			}
			img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
			copy(img.Pix, []uint8{255, 255, 255, 255, 0, 0, 0, 255})
			tf.Apply(img)
			if img.Pix[0] < 200 || img.Pix[1] < 200 || img.Pix[2] < 200 || img.Pix[4] > 20 {
				t.Errorf("%s: white %v black %v", filepath.Base(f), img.Pix[:3], img.Pix[4:7])
			}
		}
		if p.Description == "sRGB IEC61966-2.1" {
			tf, _ := NewTransform(p, SRGB(), Options{})
			if !tf.Identity() {
				t.Errorf("%s: the Microsoft sRGB profile must be the identity", filepath.Base(f))
			}
		}
	}
}
