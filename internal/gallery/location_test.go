package gallery

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocationEqual(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		a, b Location
		want bool
	}{
		{"same folder", Folder(dir), Folder(dir), true},
		{"trailing separator", Folder(dir + string(filepath.Separator)), Folder(dir), true},
		{"different folders", Folder(filepath.Join(dir, "a")), Folder(filepath.Join(dir, "b")), false},
		{"folder case follows platform", Folder(strings.ToUpper(dir)), Folder(strings.ToLower(dir)), runtime.GOOS == "windows"},
		{"same virtual place", Location{Kind: KindRecent}, Location{Kind: KindRecent}, true},
		{"virtual place ignores path", Location{Kind: KindAll, Path: "x"}, Location{Kind: KindAll, Path: "y"}, true},
		{"different virtual places", Location{Kind: KindRecent}, Location{Kind: KindFavorites}, false},
		{"folder against virtual place", Folder(dir), Location{Kind: KindAll}, false},
		{"zero values", Location{}, Location{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Equal(tc.b); got != tc.want {
				t.Fatalf("Equal = %v, want %v", got, tc.want)
			}
			if got := tc.b.Equal(tc.a); got != tc.want {
				t.Fatalf("Equal is not symmetric: %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLocationIsZero(t *testing.T) {
	cases := []struct {
		name string
		loc  Location
		want bool
	}{
		{"empty folder", Location{}, true},
		{"folder with path", Folder("x"), false},
		{"virtual place", Location{Kind: KindAll}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.loc.IsZero(); got != tc.want {
				t.Fatalf("IsZero = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLocationDisplayIsLocalized(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		lang string
		loc  Location
		want string
	}{
		{"ru", Folder(dir), dir},
		{"ru", Location{Kind: KindAll}, "Все изображения"},
		{"ru", Location{Kind: KindRecent}, "Недавние"},
		{"ru", Location{Kind: KindFavorites}, "Избранные"},
		{"en", Location{Kind: KindAll}, "All images"},
		{"en", Location{Kind: KindRecent}, "Recent"},
		{"en", Location{Kind: KindFavorites}, "Favorites"},
		{"en", Folder(dir), dir},
		{"en", Location{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.lang+"/"+tc.want, func(t *testing.T) {
			setLang(t, tc.lang)
			if got := tc.loc.Display(); got != tc.want {
				t.Fatalf("Display = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLocationParent(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "a", "b")
	root := filepath.VolumeName(dir) + string(filepath.Separator)
	cases := []struct {
		name   string
		loc    Location
		want   Location
		wantOK bool
	}{
		{"nested folder", Folder(child), Folder(filepath.Join(dir, "a")), true},
		{"folder with trailing separator", Folder(child + string(filepath.Separator)), Folder(filepath.Join(dir, "a")), true},
		{"first level under the root", Folder(filepath.Join(root, "top")), Folder(root), true},
		{"filesystem root", Folder(root), Location{}, false},
		{"empty path", Folder(""), Location{}, false},
		{"relative single segment", Folder("a"), Location{}, false},
		{"all images", Location{Kind: KindAll}, Location{}, false},
		{"recent", Location{Kind: KindRecent}, Location{}, false},
		{"favorites", Location{Kind: KindFavorites}, Location{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.loc.Parent()
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("Parent = %+v, %v; want %+v, %v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestWithin(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name       string
		path, root string
		want       bool
	}{
		{"same path", dir, dir, true},
		{"direct child", filepath.Join(dir, "a"), dir, true},
		{"deep descendant", filepath.Join(dir, "a", "b", "c"), dir, true},
		{"sibling with common prefix", dir + "x", dir, false},
		{"parent is not within child", dir, filepath.Join(dir, "a"), false},
		{"empty path", "", dir, false},
		{"empty root", dir, "", false},
		{"trailing separator on the path", filepath.Join(dir, "a") + string(filepath.Separator), dir, true},
		{"case follows platform", strings.ToUpper(filepath.Join(dir, "a")), dir, runtime.GOOS == "windows"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := within(tc.path, tc.root); got != tc.want {
				t.Fatalf("within(%q, %q) = %v, want %v", tc.path, tc.root, got, tc.want)
			}
		})
	}
}

func TestLocationParentOfNetworkShareStopsAtTheShareRoot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("UNC paths exist only on Windows")
	}
	cases := []struct {
		name   string
		loc    Location
		want   Location
		wantOK bool
	}{
		{"share root has no parent", Folder(`\\server\share`), Location{}, false},
		{"share root with trailing separator", Folder(`\\server\share\`), Location{}, false},
		{"folder on the share", Folder(`\\server\share\photos`), Folder(`\\server\share\`), true},
		{"drive without a separator is a root", Folder("C:"), Location{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.loc.Parent()
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("Parent = %+v, %v; want %+v, %v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
