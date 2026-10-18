package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	module = "github.com/oops1/PixGo/"
	engine = "github.com/oops1/headless-gui"
)

var engineFree = []string{
	"internal/codec",
	"internal/imaging",
	"internal/render",
	"internal/meta",
	"internal/isobmff",
	"internal/viewer",
	"internal/jobs",
	"internal/settings",
	"internal/i18n",
	"internal/commands",
	"internal/fsutil",
	"internal/thumbs",
	"internal/updates",
	"internal/gallery",
	"internal/jpegxform",
	"internal/jpegdec",
	"internal/par",
	"internal/edit",
	"internal/colorm",
	"internal/rawdev",
}

var vendorCodecs = map[string]string{
	"github.com/gen2brain/vpx":   "internal/codec/webp",
	"github.com/gen2brain/gav1d": "internal/codec/avif",
	"github.com/gen2brain/jxl":   "internal/codec/jxl",
	"github.com/gen2brain/h265":  "internal/codec/heic",
}

var engineAllowedInCodec = map[string]bool{
	"internal/codec/svg": true,
}

func root(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

type fileImports struct {
	pkgDir  string
	file    string
	imports []string
	isTest  bool
}

func scan(t *testing.T) []fileImports {
	t.Helper()
	base := root(t)
	var out []fileImports
	err := filepath.WalkDir(filepath.Join(base, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		rel, _ := filepath.Rel(base, filepath.Dir(path))
		fi := fileImports{pkgDir: filepath.ToSlash(rel), file: filepath.Base(path), isTest: strings.HasSuffix(path, "_test.go")}
		for _, im := range f.Imports {
			fi.imports = append(fi.imports, strings.Trim(im.Path.Value, `"`))
		}
		out = append(out, fi)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func under(dir, prefix string) bool {
	return dir == prefix || strings.HasPrefix(dir, prefix+"/")
}

func TestModelPackagesDoNotImportEngine(t *testing.T) {
	for _, fi := range scan(t) {
		for _, prefix := range engineFree {
			if !under(fi.pkgDir, prefix) || engineAllowedInCodec[fi.pkgDir] {
				continue
			}
			for _, im := range fi.imports {
				if strings.HasPrefix(im, engine) {
					t.Errorf("%s/%s imports %s", fi.pkgDir, fi.file, im)
				}
			}
		}
	}
}

func TestVendorCodecsStayInTheirPackage(t *testing.T) {
	for _, fi := range scan(t) {
		for _, im := range fi.imports {
			for vendor, home := range vendorCodecs {
				if strings.HasPrefix(im, vendor) && !under(fi.pkgDir, home) {
					t.Errorf("%s/%s imports %s outside %s", fi.pkgDir, fi.file, im, home)
				}
			}
		}
	}
}

func TestDependenciesPointDownward(t *testing.T) {
	forbidden := map[string][]string{
		"internal/codec":     {"internal/ui", "internal/app", "internal/viewer", "internal/imaging", "internal/render"},
		"internal/isobmff":   {"internal/ui", "internal/app", "internal/viewer", "internal/imaging", "internal/render"},
		"internal/meta":      {"internal/ui", "internal/app", "internal/viewer", "internal/imaging", "internal/render"},
		"internal/render":    {"internal/ui", "internal/app", "internal/imaging", "internal/viewer"},
		"internal/imaging":   {"internal/ui", "internal/app", "internal/viewer"},
		"internal/viewer":    {"internal/ui", "internal/app", "internal/imaging"},
		"internal/jobs":      {"internal/ui", "internal/app", "internal/codec"},
		"internal/settings":  {"internal/ui", "internal/app"},
		"internal/i18n":      {"internal/ui", "internal/app"},
		"internal/updates":   {"internal/ui", "internal/app"},
		"internal/gallery":   {"internal/ui", "internal/app"},
		"internal/jpegxform": {"internal/ui", "internal/app", "internal/viewer", "internal/imaging", "internal/render"},
		"internal/jpegdec":   {"internal/ui", "internal/app", "internal/viewer", "internal/imaging", "internal/render", "internal/codec"},
		"internal/par":       {"internal/ui", "internal/app", "internal/viewer", "internal/imaging", "internal/render", "internal/codec", "internal/thumbs", "internal/settings"},
		"internal/edit":      {"internal/ui", "internal/app", "internal/viewer", "internal/render"},
		"internal/colorm":    {"internal/ui", "internal/app", "internal/viewer", "internal/imaging", "internal/render", "internal/thumbs", "internal/settings"},
		"internal/rawdev":    {"internal/ui", "internal/app", "internal/viewer", "internal/imaging", "internal/render", "internal/thumbs", "internal/settings", "internal/codec", "internal/meta"},
	}
	for _, fi := range scan(t) {
		if fi.isTest {
			continue
		}
		for from, bad := range forbidden {
			if !under(fi.pkgDir, from) {
				continue
			}
			for _, im := range fi.imports {
				for _, b := range bad {
					if strings.HasPrefix(im, module+b) {
						t.Errorf("%s/%s imports %s", fi.pkgDir, fi.file, im)
					}
				}
			}
		}
	}
}

func TestNoCgo(t *testing.T) {
	for _, fi := range scan(t) {
		for _, im := range fi.imports {
			if im == "C" {
				t.Errorf("%s/%s uses cgo", fi.pkgDir, fi.file)
			}
		}
	}
}

func TestNoCommentsInSources(t *testing.T) {
	base := root(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "third_party", "testdata", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			return perr
		}
		for _, g := range f.Comments {
			for _, c := range g.List {
				if strings.HasPrefix(c.Text, "//go:") || strings.HasPrefix(c.Text, "//line ") {
					continue
				}
				rel, _ := filepath.Rel(base, path)
				t.Errorf("%s:%d: comment %q", filepath.ToSlash(rel), fset.Position(c.Pos()).Line, c.Text)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
