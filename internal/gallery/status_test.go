package gallery

import (
	"slices"
	"testing"
)

func TestStatusTextMatchesTheMockup(t *testing.T) {
	setLang(t, "ru")
	sel := Item{Name: "mountain.jpg", Size: 9_154_600}
	if got, want := StatusText(12, &sel, 6000, 4000), "Выбрано: 1 элемент • 8,73 МБ • 6000 × 4000"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStatusText(t *testing.T) {
	file := Item{Name: "a.jpg", Size: 2048}
	dir := Item{Name: "sub", Dir: true}
	cases := []struct {
		name  string
		lang  string
		count int
		sel   *Item
		w, h  int
		want  string
	}{
		{"nothing selected in Russian", "ru", 12, nil, 0, 0, "Элементов: 12"},
		{"nothing selected with a large count", "ru", 5000, nil, 0, 0, "Элементов: 5 000"},
		{"nothing selected in an empty folder", "ru", 0, nil, 0, 0, "Элементов: 0"},
		{"nothing selected in English", "en", 1234, nil, 0, 0, "Items: 1,234"},
		{"file with dimensions", "ru", 3, &file, 800, 600, "Выбрано: 1 элемент • 2,00 КБ • 800 × 600"},
		{"file without dimensions yet", "ru", 3, &file, 0, 0, "Выбрано: 1 элемент • 2,00 КБ"},
		{"only width known is not shown", "ru", 3, &file, 800, 0, "Выбрано: 1 элемент • 2,00 КБ"},
		{"only height known is not shown", "ru", 3, &file, 0, 600, "Выбрано: 1 элемент • 2,00 КБ"},
		{"file in English", "en", 3, &file, 800, 600, "Selected: 1 item • 2.00 KB • 800 × 600"},
		{"empty file", "en", 1, &Item{Name: "z.png"}, 1, 1, "Selected: 1 item • 0 B • 1 × 1"},
		{"folder", "ru", 3, &dir, 800, 600, "Выбрано: 1 элемент • Папка"},
		{"folder in English", "en", 3, &dir, 0, 0, "Selected: 1 item • Folder"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setLang(t, tc.lang)
			if got := StatusText(tc.count, tc.sel, tc.w, tc.h); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusPartsSplitTheSegmentsForColumnLayout(t *testing.T) {
	setLang(t, "ru")
	sel := Item{Name: "a.jpg", Size: 1 << 20}
	got := StatusParts(1, &sel, 10, 20)
	if want := []string{"Выбрано: 1 элемент", "1,00 МБ", "10 × 20"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := StatusParts(4, nil, 0, 0); len(got) != 1 {
		t.Fatalf("no selection must be a single segment, got %q", got)
	}
}

func TestEmptyText(t *testing.T) {
	cases := []struct {
		name        string
		lang        string
		loc         Location
		unavailable bool
		want        string
	}{
		{"empty folder", "ru", Folder("x"), false, "В этой папке нет изображений"},
		{"empty recent", "ru", Location{Kind: KindRecent}, false, "Нет недавних файлов"},
		{"empty favorites", "ru", Location{Kind: KindFavorites}, false, "Нет избранных файлов"},
		{"empty all images", "ru", Location{Kind: KindAll}, false, "Изображения не найдены"},
		{"unavailable folder", "ru", Folder("x"), true, "Папка недоступна"},
		{"unavailable wins over the place", "ru", Location{Kind: KindRecent}, true, "Папка недоступна"},
		{"empty folder in English", "en", Folder("x"), false, "No images in this folder"},
		{"empty recent in English", "en", Location{Kind: KindRecent}, false, "No recent files"},
		{"empty favorites in English", "en", Location{Kind: KindFavorites}, false, "No favorite files"},
		{"empty all images in English", "en", Location{Kind: KindAll}, false, "No images found"},
		{"unavailable in English", "en", Folder("x"), true, "This folder is unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setLang(t, tc.lang)
			if got := EmptyText(tc.loc, tc.unavailable); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
