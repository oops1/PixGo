package gallery

import (
	"slices"
	"testing"
)

func TestFormatOfCollapsesSynonyms(t *testing.T) {
	cases := []struct {
		name string
		file string
		want string
	}{
		{"jpg", "a.jpg", "JPEG"},
		{"jpeg", "a.jpeg", "JPEG"},
		{"jpe", "a.jpe", "JPEG"},
		{"jfif", "a.jfif", "JPEG"},
		{"upper case jpeg", "A.JPG", "JPEG"},
		{"tif", "a.tif", "TIFF"},
		{"tiff", "a.tiff", "TIFF"},
		{"svg", "a.svg", "SVG"},
		{"svgz", "a.svgz", "SVG"},
		{"dib", "a.dib", "BMP"},
		{"bmp", "a.bmp", "BMP"},
		{"heif", "a.heif", "HEIC"},
		{"heic", "a.heic", "HEIC"},
		{"png", "a.png", "PNG"},
		{"mixed case webp", "a.WebP", "WEBP"},
		{"last extension wins", "archive.tar.gif", "GIF"},
		{"no extension", "README", ""},
		{"trailing dot", "a.", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatOf(tc.file); got != tc.want {
				t.Fatalf("FormatOf(%q) = %q, want %q", tc.file, got, tc.want)
			}
		})
	}
}

func TestListingFormatsAreSortedUniqueAndIgnoreFolders(t *testing.T) {
	cases := []struct {
		name  string
		items []Item
		want  []string
	}{
		{"empty listing", nil, nil},
		{"only folders", []Item{{Name: "x.jpg", Dir: true}, {Name: "y", Dir: true}}, nil},
		{"synonyms are merged", []Item{{Name: "a.jpg"}, {Name: "b.JPEG"}, {Name: "c.jfif"}, {Name: "d.tif"}, {Name: "e.tiff"}}, []string{"JPEG", "TIFF"}},
		{"sorted alphabetically", []Item{{Name: "a.webp"}, {Name: "b.png"}, {Name: "c.gif"}, {Name: "d.bmp"}}, []string{"BMP", "GIF", "PNG", "WEBP"}},
		{"files without extension are skipped", []Item{{Name: "README"}, {Name: "a.png"}}, []string{"PNG"}},
		{"folders do not contribute", []Item{{Name: "photos.png", Dir: true}, {Name: "a.gif"}}, []string{"GIF"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Listing{Items: tc.items}.Formats()
			if !slices.Equal(got, tc.want) {
				t.Fatalf("Formats = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestListingHasFormatAcceptsAnySpelling(t *testing.T) {
	l := Listing{Items: []Item{{Name: "a.jpeg"}, {Name: "b.png"}}}
	cases := []struct {
		format string
		want   bool
	}{
		{"JPEG", true},
		{"jpg", true},
		{".JPG", true},
		{"PNG", true},
		{"gif", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := l.HasFormat(tc.format); got != tc.want {
			t.Errorf("HasFormat(%q) = %v, want %v", tc.format, got, tc.want)
		}
	}
}

func TestFilterFormatKeepsFoldersAndMatchingFiles(t *testing.T) {
	items := []Item{
		{Name: "folder", Dir: true},
		{Name: "dir.png", Dir: true},
		{Name: "a.jpg"},
		{Name: "b.JPEG"},
		{Name: "c.png"},
		{Name: "d.tif"},
		{Name: "README"},
	}
	cases := []struct {
		name   string
		format string
		want   []string
	}{
		{"empty format keeps everything", "", []string{"folder", "dir.png", "a.jpg", "b.JPEG", "c.png", "d.tif", "README"}},
		{"blank format keeps everything", "  ", []string{"folder", "dir.png", "a.jpg", "b.JPEG", "c.png", "d.tif", "README"}},
		{"canonical name matches synonyms", "JPEG", []string{"folder", "dir.png", "a.jpg", "b.JPEG"}},
		{"synonym spelling matches the same files", "jpg", []string{"folder", "dir.png", "a.jpg", "b.JPEG"}},
		{"png", "PNG", []string{"folder", "dir.png", "c.png"}},
		{"tiff by synonym", "tif", []string{"folder", "dir.png", "d.tif"}},
		{"absent format leaves only folders", "GIF", []string{"folder", "dir.png"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FilterFormat(items, tc.format)
			if !slices.Equal(names(got), tc.want) {
				t.Fatalf("got %v, want %v", names(got), tc.want)
			}
		})
	}
}

func TestFilterFormatDoesNotModifyItsInput(t *testing.T) {
	items := []Item{{Name: "a.jpg"}, {Name: "b.png"}}
	FilterFormat(items, "PNG")
	if !slices.Equal(names(items), []string{"a.jpg", "b.png"}) {
		t.Fatalf("input changed: %v", names(items))
	}
	if got := FilterFormat(nil, "PNG"); len(got) != 0 {
		t.Fatalf("nil input produced %v", got)
	}
}

func TestListingTruncatedTextNamesTheLimit(t *testing.T) {
	cases := []struct {
		name string
		lang string
		l    Listing
		want string
	}{
		{"not truncated", "ru", Listing{Limit: 5000}, ""},
		{"truncated in Russian", "ru", Listing{Truncated: true, Limit: 5000}, "Показаны не все файлы (лимит: 5 000)"},
		{"truncated in English", "en", Listing{Truncated: true, Limit: 5000}, "Not all files are shown (limit: 5,000)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setLang(t, tc.lang)
			if got := tc.l.TruncatedText(); got != tc.want {
				t.Fatalf("TruncatedText = %q, want %q", got, tc.want)
			}
		})
	}
}
