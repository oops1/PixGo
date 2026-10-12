package platform

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestEncodeTrashPath(t *testing.T) {
	for _, p := range []string{
		"/home/u/photo.jpg",
		"/home/u/a b/c%d.jpg",
		"/home/u/Фото/снимок №1.jpg",
		"/home/u/new\nline",
		"/home/u/#hash?q=1&x",
		"/home/u/.hidden~",
		"relative/p",
	} {
		enc := encodeTrashPath(p)
		for _, r := range enc {
			if r > 0x7e || r < 0x21 {
				t.Errorf("%q: encoded form %q has a raw byte", p, enc)
			}
		}
		if strings.ContainsAny(enc, "\n ?#&") {
			t.Errorf("%q: unescaped delimiter in %q", p, enc)
		}
		dec, ok := decodeTrashPath(enc)
		if !ok || dec != p {
			t.Errorf("round trip %q -> %q -> %q", p, enc, dec)
		}
		if u, err := url.PathUnescape(enc); err != nil || u != p {
			t.Errorf("url.PathUnescape(%q) = %q, %v", enc, u, err)
		}
	}
	if encodeTrashPath("/a b") != "/a%20b" || encodeTrashPath("é") != "%C3%A9" {
		t.Error("encoding")
	}
	if _, ok := decodeTrashPath("%zz"); ok {
		t.Error("bad escape accepted")
	}
	if _, ok := decodeTrashPath("abc%4"); ok {
		t.Error("truncated escape accepted")
	}
}

var infoRE = regexp.MustCompile(`^\[Trash Info\]\nPath=[^\n]+\nDeletionDate=\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\n$`)

func TestTrashInfoText(t *testing.T) {
	when := time.Date(2026, 10, 11, 9, 5, 7, 0, time.Local)
	got := trashInfoText("/home/u/снимок 1.jpg", when)
	if !infoRE.MatchString(got) {
		t.Fatalf("format: %q", got)
	}
	want := "[Trash Info]\nPath=/home/u/%D1%81%D0%BD%D0%B8%D0%BC%D0%BE%D0%BA%201.jpg\nDeletionDate=2026-10-11T09:05:07\n"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestTrashCandidate(t *testing.T) {
	for _, c := range []struct {
		base string
		n    int
		want string
	}{
		{"a.jpg", 1, "a.jpg"},
		{"a.jpg", 2, "a.2.jpg"},
		{"archive.tar.gz", 3, "archive.tar.3.gz"},
		{"noext", 2, "noext.2"},
		{".hidden", 2, ".hidden.2"},
	} {
		if got := trashCandidate(c.base, c.n); got != c.want {
			t.Errorf("trashCandidate(%q, %d) = %q, want %q", c.base, c.n, got, c.want)
		}
	}
}

func TestWithin(t *testing.T) {
	for _, c := range []struct {
		parent, path string
		want         bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b", "/a/b/c", true},
		{"/a/b", "/a/bc", false},
		{"/a/b", "/a", false},
		{"/a/b", "/x/b/c", false},
	} {
		if got := within(c.parent, c.path); got != c.want {
			t.Errorf("within(%q, %q) = %v", c.parent, c.path, got)
		}
	}
}
