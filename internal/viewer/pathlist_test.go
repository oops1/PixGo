package viewer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func p(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

func TestPathListAddMovesToFront(t *testing.T) {
	root := t.TempDir()
	a, b, c := filepath.Join(root, "a"), filepath.Join(root, "b"), filepath.Join(root, "c")
	l := NewPathList(0)
	l.Add(a)
	l.Add(b)
	l.Add(c)
	if !slices.Equal(l.Items(), []string{c, b, a}) {
		t.Fatalf("got %v", l.Items())
	}
	l.Add(a)
	if !slices.Equal(l.Items(), []string{a, c, b}) || l.Len() != 3 {
		t.Fatalf("got %v", l.Items())
	}
	l.Add("")
	if l.Len() != 3 {
		t.Fatal("empty path must be ignored")
	}
}

func TestPathListLimit(t *testing.T) {
	root := t.TempDir()
	l := NewPathList(3)
	var all []string
	for _, n := range []string{"1", "2", "3", "4", "5"} {
		path := filepath.Join(root, n)
		all = append(all, path)
		l.Add(path)
	}
	if want := []string{all[4], all[3], all[2]}; !slices.Equal(l.Items(), want) {
		t.Fatalf("got %v, want %v", l.Items(), want)
	}
	l.SetLimit(2)
	if l.Len() != 2 || l.Limit() != 2 {
		t.Fatalf("len %d limit %d", l.Len(), l.Limit())
	}
	l.SetLimit(0)
	l.Add(all[0])
	l.Add(all[1])
	l.Add(all[2])
	if l.Len() != 5 {
		t.Fatalf("limit 0 must mean unlimited, len %d", l.Len())
	}
}

func TestPathListRemoveContainsToggle(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	l := NewPathList(0)
	l.Add(a)
	l.Add(b)
	if !l.Contains(a) || l.Contains(filepath.Join(root, "zzz")) {
		t.Fatal("Contains")
	}
	if !l.Remove(a) || l.Remove(a) || l.Contains(a) {
		t.Fatal("Remove")
	}
	if !l.Toggle(a) || !l.Contains(a) || l.Items()[0] != a {
		t.Fatalf("Toggle on: %v", l.Items())
	}
	if l.Toggle(a) || l.Contains(a) {
		t.Fatal("Toggle off")
	}
	if l.Toggle("") {
		t.Fatal("Toggle of an empty path")
	}
	l.Clear()
	if l.Len() != 0 || len(l.Items()) != 0 {
		t.Fatal("Clear")
	}
}

func TestPathListCaseSensitivityFollowsPlatform(t *testing.T) {
	root := t.TempDir()
	lower := filepath.Join(root, "photo.jpg")
	upper := filepath.Join(root, "PHOTO.JPG")
	l := NewPathList(0)
	l.Add(lower)
	if got := l.Contains(upper); got != foldPaths {
		t.Fatalf("Contains(upper) = %v, want %v", got, foldPaths)
	}
	l.Add(upper)
	if foldPaths {
		if l.Len() != 1 || l.Items()[0] != upper {
			t.Fatalf("got %v", l.Items())
		}
		if !l.Remove(lower) || l.Len() != 0 {
			t.Fatal("Remove must ignore case")
		}
	} else if l.Len() != 2 {
		t.Fatalf("got %v", l.Items())
	}
}

func TestPathListItemsIsACopy(t *testing.T) {
	l := NewPathList(0)
	path := p(t, "a")
	l.Add(path)
	items := l.Items()
	items[0] = "changed"
	if l.Items()[0] != path {
		t.Fatal("Items must return a copy")
	}
}

func TestPathListJSONRoundTrip(t *testing.T) {
	root := t.TempDir()
	l := NewPathList(5)
	l.Add(filepath.Join(root, "a"))
	l.Add(filepath.Join(root, "b"))
	data, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var back PathList
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(back.Items(), l.Items()) || back.Limit() != 5 {
		t.Fatalf("got %v limit %d from %s", back.Items(), back.Limit(), data)
	}
	empty, _ := json.Marshal(NewPathList(0))
	if !strings.Contains(string(empty), `"items":[]`) {
		t.Fatalf("empty list must encode as an array: %s", empty)
	}
}

func TestPathListUnmarshalAcceptsBareArrayAndCleansInput(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	raw, _ := json.Marshal([]string{a, "", b, a, filepath.Join(root, "x", "..", "b")})
	var l PathList
	if err := json.Unmarshal(raw, &l); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(l.Items(), []string{a, b}) {
		t.Fatalf("got %v", l.Items())
	}
	object := `{"limit":1,"items":["` + strings.ReplaceAll(a, `\`, `\\`) + `","` + strings.ReplaceAll(b, `\`, `\\`) + `"]}`
	var limited PathList
	if err := json.Unmarshal([]byte(object), &limited); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(limited.Items(), []string{a}) {
		t.Fatalf("limit must trim on load: %v", limited.Items())
	}
	if err := json.Unmarshal([]byte(`{"items": 5}`), &l); err == nil {
		t.Fatal("expected a type error")
	}
}

func TestPathListLoadSave(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "state", "favorites.json")
	missing, err := Load(file)
	if err != nil || missing.Len() != 0 {
		t.Fatalf("missing file must give an empty list: %v %v", missing.Items(), err)
	}
	l := NewPathList(200)
	l.Add(filepath.Join(root, "one.jpg"))
	l.Add(filepath.Join(root, "two.jpg"))
	var wrotePath string
	var wrote []byte
	err = l.Save(file, func(path string, data []byte) error {
		wrotePath, wrote = path, data
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, data, 0o644)
	})
	if err != nil || wrotePath != file || !json.Valid(wrote) {
		t.Fatalf("save: %v %q %s", err, wrotePath, wrote)
	}
	back, err := Load(file)
	if err != nil || !slices.Equal(back.Items(), l.Items()) || back.Limit() != 200 {
		t.Fatalf("load: %v %v %d", err, back.Items(), back.Limit())
	}
}

func TestPathListSavePropagatesWriterError(t *testing.T) {
	want := errors.New("disk full")
	if err := NewPathList(0).Save("x", func(string, []byte) error { return want }); err != want {
		t.Fatalf("got %v", err)
	}
	if err := NewPathList(0).Save("x", nil); !errors.Is(err, ErrNoWriter) {
		t.Fatalf("got %v", err)
	}
}

func TestPathListLoadBadFiles(t *testing.T) {
	dir := t.TempDir()
	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := Load(corrupt)
	if err == nil || l == nil || l.Len() != 0 {
		t.Fatalf("corrupt: %v %v", l, err)
	}
	blank := filepath.Join(dir, "blank.json")
	if err := os.WriteFile(blank, []byte("  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if l, err := Load(blank); err != nil || l.Len() != 0 {
		t.Fatalf("blank: %v %v", l, err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("loading a directory must fail")
	}
}

func TestPathListConcurrentUse(t *testing.T) {
	root := t.TempDir()
	l := NewPathList(10)
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				path := filepath.Join(root, strconv.Itoa((g*7+i)%15))
				switch i % 6 {
				case 0, 1:
					l.Add(path)
				case 2:
					l.Toggle(path)
				case 3:
					l.Remove(path)
				case 4:
					l.Contains(path)
					l.Items()
				default:
					if data, err := json.Marshal(l); err == nil {
						var back PathList
						json.Unmarshal(data, &back)
					}
				}
			}
		}(g)
	}
	wg.Wait()
	if l.Len() > 10 {
		t.Fatalf("limit exceeded: %d", l.Len())
	}
}
