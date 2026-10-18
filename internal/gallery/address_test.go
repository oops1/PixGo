package gallery

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/i18n"
)

type addressFixture struct {
	root   string
	folder string
	file   string
}

func newAddressFixture(t *testing.T) addressFixture {
	t.Helper()
	root := t.TempDir()
	folder := mkdir(t, filepath.Join(root, "sub dir"))
	file := touch(t, filepath.Join(folder, "pic.jpg"), 1, time.Time{})
	return addressFixture{root: root, folder: folder, file: file}
}

func TestParseAddressResolvesFolders(t *testing.T) {
	setLang(t, "ru")
	fx := newAddressFixture(t)
	sep := string(os.PathSeparator)
	cases := []struct {
		name string
		text string
		home string
		want Location
	}{
		{"plain path", fx.folder, "", Folder(fx.folder)},
		{"surrounding spaces", "  " + fx.folder + "  ", "", Folder(fx.folder)},
		{"double quotes as copied from the shell", `"` + fx.folder + `"`, "", Folder(fx.folder)},
		{"quotes and spaces", `  "` + fx.folder + `"  `, "", Folder(fx.folder)},
		{"unbalanced quote while typing", `"` + fx.folder, "", Folder(fx.folder)},
		{"single quotes", "'" + fx.folder + "'", "", Folder(fx.folder)},
		{"trailing separator", fx.folder + sep, "", Folder(fx.folder)},
		{"redundant segments", fx.root + sep + "sub dir" + sep + "x" + sep + "..", "", Folder(fx.folder)},
		{"tilde alone", "~", fx.root, Folder(fx.root)},
		{"tilde with separator", "~" + sep + "sub dir", fx.root, Folder(fx.folder)},
		{"tilde with slash", "~/sub dir", fx.root, Folder(fx.folder)},
		{"all images by its name", "Все изображения", "", Location{Kind: KindAll}},
		{"recent by its name", "Недавние", "", Location{Kind: KindRecent}},
		{"favorites in another case", "  избранные ", "", Location{Kind: KindFavorites}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseAddress(tc.text, tc.home)
			if err != nil {
				t.Fatal(err)
			}
			if got.Loc != tc.want || got.File != "" {
				t.Fatalf("got %+v, want folder %+v", got, tc.want)
			}
		})
	}
}

func TestParseAddressOfFileReturnsItsFolderAndThePath(t *testing.T) {
	fx := newAddressFixture(t)
	got, err := ParseAddress(`"`+fx.file+`"`, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Loc != Folder(fx.folder) || got.File != fx.file {
		t.Fatalf("got %+v", got)
	}
}

func TestParseAddressRecognizesVirtualPlacesInTheCurrentLanguage(t *testing.T) {
	setLang(t, "en")
	cases := []struct {
		text string
		want Location
	}{
		{"All images", Location{Kind: KindAll}},
		{"recent", Location{Kind: KindRecent}},
		{"FAVORITES", Location{Kind: KindFavorites}},
	}
	for _, tc := range cases {
		got, err := ParseAddress(tc.text, "")
		if err != nil || got.Loc != tc.want {
			t.Errorf("ParseAddress(%q) = %+v, %v", tc.text, got, err)
		}
	}
}

func TestParseAddressRoundTripsDisplay(t *testing.T) {
	fx := newAddressFixture(t)
	for _, lang := range i18n.Supported() {
		setLang(t, lang)
		for _, loc := range []Location{Folder(fx.folder), {Kind: KindAll}, {Kind: KindRecent}, {Kind: KindFavorites}} {
			got, err := ParseAddress(loc.Display(), "")
			if err != nil || got.Loc != loc {
				t.Errorf("%s: ParseAddress(%q) = %+v, %v; want %+v", lang, loc.Display(), got, err, loc)
			}
		}
	}
}

func TestParseAddressErrors(t *testing.T) {
	fx := newAddressFixture(t)
	missing := filepath.Join(fx.root, "missing")
	cases := []struct {
		name     string
		text     string
		home     string
		wantErr  error
		wantPath string
	}{
		{"empty", "", "", ErrEmpty, ""},
		{"blanks", "   \t ", "", ErrEmpty, ""},
		{"only quotes", `""`, "", ErrEmpty, ""},
		{"missing folder", missing, "", ErrNotFound, missing},
		{"missing under an existing file", filepath.Join(fx.file, "x"), "", nil, ""},
		{"relative path", "photos", "", ErrNotFound, "photos"},
		{"dot", ".", "", ErrNotFound, "."},
		{"tilde without a home", "~", "", ErrNotFound, "~"},
		{"tilde of another user is not expanded", "~someone", fx.root, ErrNotFound, "~someone"},
		{"unix variable is not expanded", "$HOME", "", ErrNotFound, "$HOME"},
		{"windows variable is not expanded", "%USERPROFILE%", "", ErrNotFound, "%USERPROFILE%"},
		{"variable inside an absolute path is not expanded", filepath.Join(fx.root, "$HOME"), "", ErrNotFound, filepath.Join(fx.root, "$HOME")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseAddress(tc.text, tc.home)
			if err == nil {
				t.Fatalf("expected an error, got %+v", got)
			}
			if tc.wantErr == nil {
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != (Address{}) {
				t.Fatalf("an error must come with a zero address, got %+v", got)
			}
			var ae *AddressError
			if tc.wantPath != "" && (!errors.As(err, &ae) || ae.Path != tc.wantPath) {
				t.Fatalf("err = %v, want an AddressError for %q", err, tc.wantPath)
			}
		})
	}
}

func TestParseAddressKeepsOtherStatErrors(t *testing.T) {
	fx := newAddressFixture(t)
	_, err := ParseAddress(fx.root+string(os.PathSeparator)+"bad\x00name", "")
	var ae *AddressError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v", err)
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("an invalid name is not a missing path: %v", err)
	}
	if !strings.Contains(ae.Error(), "bad") || errors.Unwrap(ae) == nil {
		t.Fatalf("AddressError = %q, unwrap %v", ae.Error(), errors.Unwrap(ae))
	}
}

func TestParseAddressExpandsBareDriveLetterOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive letters exist only on Windows")
	}
	vol := filepath.VolumeName(t.TempDir())
	got, err := ParseAddress(vol, "")
	if err != nil || got.Loc != Folder(vol+`\`) {
		t.Fatalf("ParseAddress(%q) = %+v, %v", vol, got, err)
	}
}

func TestAddressForRejectsEntriesThatAreNeitherFoldersNorFiles(t *testing.T) {
	cases := []struct {
		name    string
		mode    fs.FileMode
		wantErr error
	}{
		{"folder", fs.ModeDir, nil},
		{"regular file", 0o644, nil},
		{"device", fs.ModeDevice, ErrNotDirectory},
		{"named pipe", fs.ModeNamedPipe, ErrNotDirectory},
		{"socket", fs.ModeSocket, ErrNotDirectory},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "x")
			got, err := addressFor(path, tc.mode)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil && got != (Address{}) {
				t.Fatalf("got %+v with an error", got)
			}
			if tc.wantErr == nil && got.Loc.IsZero() {
				t.Fatal("no location")
			}
		})
	}
}

func TestAddressErrorText(t *testing.T) {
	cases := []struct {
		name string
		lang string
		err  error
		want string
	}{
		{"no error", "ru", nil, ""},
		{"empty address", "ru", ErrEmpty, ""},
		{"not found", "ru", &AddressError{Path: "D:/x", Err: ErrNotFound}, "Не удаётся найти «D:/x»"},
		{"not found in English", "en", &AddressError{Path: "D:/x", Err: ErrNotFound}, `Cannot find "D:/x"`},
		{"not a folder", "ru", &AddressError{Path: "/dev/null", Err: ErrNotDirectory}, "Не удалось открыть «/dev/null»"},
		{"other system error", "en", &AddressError{Path: "/root", Err: fs.ErrPermission}, `Cannot open "/root"`},
		{"unrelated error", "ru", errors.New("boom"), "Не удалось выполнить операцию"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setLang(t, tc.lang)
			if got := AddressErrorText(tc.err); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
