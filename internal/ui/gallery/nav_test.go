package gallery

import (
	"image"
	"testing"

	model "github.com/oops1/PixGo/internal/gallery"
)

func TestSidebarRowsMatchTheMockupBaselines(t *testing.T) {
	rows := buildNavRows(mockSidebar(), navMetricsFor(1))
	top := headerHeight + 1
	want := []int{69, 92, 115, 145, 169, 191, 213, 234, 256, 287, 312, 335, 361}

	if len(rows) != len(want) {
		t.Fatalf("%d rows", len(rows))
	}
	for i, r := range rows {
		if got := r.baseline(top); got != want[i] {
			t.Errorf("row %d (%s): baseline %d, want %d", i, r.entry.Label, got, want[i])
		}
	}
}

func TestSidebarIconsMatchTheCalibratedPositions(t *testing.T) {
	rows := buildNavRows(mockSidebar(), navMetricsFor(1))
	top := headerHeight + 1
	want := map[string]image.Point{
		"images": {20, 57}, "recent": {20, 80}, "heart-small": {20, 102},
		"desktop": {23, 158}, "pictures": {23, 179}, "video": {23, 200}, "documents": {23, 222}, "download": {23, 244},
		"drive-system": {23, 299}, "drive": {23, 322}, "network": {22, 348},
	}

	for _, r := range rows {
		if r.entry.Header {
			continue
		}
		if got := r.iconPos(top); got != want[r.entry.Icon] {
			t.Errorf("%s at %v, want %v", r.entry.Icon, got, want[r.entry.Icon])
		}
	}
}

func TestSelectedSidebarPillMatchesTheMockup(t *testing.T) {
	rows := buildNavRows(mockSidebar(), navMetricsFor(1))

	got := rows[11].rect(headerHeight+1, sidebarWidth)

	if want := image.Rect(8, 319, 146, 343); got != want {
		t.Fatalf("pill = %v, want %v", got, want)
	}
}

func TestSidebarHitTestFindsTheNearestEnabledRow(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	top := r.page.geo.sidebar.Min.Y

	for _, row := range r.page.rows {
		y := top + int(row.center)
		got := r.page.navHit(30, y)
		wantNav := !row.entry.Header && !row.entry.Disabled
		if wantNav != (got.kind == hitNav && got.index == row.index) {
			t.Errorf("%s at y=%d: %+v", row.entry.Label, y, got)
		}
	}
	if got := r.page.navHit(30, top+4); got.kind != hitNone {
		t.Errorf("empty space above the first row: %+v", got)
	}
	if got := r.page.navHit(3, top+int(r.page.rows[0].center)); got.kind != hitNone {
		t.Errorf("left margin: %+v", got)
	}
	if got := r.page.navHit(148, top+int(r.page.rows[0].center)); got.kind != hitNone {
		t.Errorf("right margin: %+v", got)
	}
}

func TestSidebarTextColumnsDependOnTheSection(t *testing.T) {
	rows := buildNavRows(mockSidebar(), navMetricsFor(1))

	if rows[0].textX() != navTextTop || rows[4].textX() != navTextSection || rows[3].textX() != navHeaderX {
		t.Fatalf("columns: %d %d %d", rows[0].textX(), rows[4].textX(), rows[3].textX())
	}
}

func TestUnknownIconsGetTheDefaultNudge(t *testing.T) {
	rows := buildNavRows([]model.Entry{{Label: "x", Icon: "mystery"}}, navMetricsFor(1))

	if got := rows[0].iconPos(0); got.Y != int(navFirstCenter+.5)+defaultNavNudge.Y {
		t.Fatalf("pos = %v", got)
	}
	if buildNavRows(nil, navMetricsFor(1)) != nil || navContentHeight(nil) != 0 {
		t.Fatal("an empty sidebar has no rows")
	}
}

func TestSidebarRowPitchFollowsTheMockup(t *testing.T) {
	cases := []struct {
		name      string
		prev, cur model.Entry
		want      float64
	}{
		{"plain items", model.Entry{Icon: "images"}, model.Entry{Icon: "recent"}, 23},
		{"item to header", model.Entry{Icon: "heart-small"}, model.Entry{Header: true}, 30},
		{"folder to header", model.Entry{Icon: "download"}, model.Entry{Header: true}, 31},
		{"header to folder", model.Entry{Header: true}, model.Entry{Icon: "desktop"}, 24},
		{"header to drive", model.Entry{Header: true}, model.Entry{Icon: "drive"}, 25},
		{"folders", model.Entry{Icon: "desktop"}, model.Entry{Icon: "pictures"}, 21.75},
		{"drive to network", model.Entry{Icon: "drive"}, model.Entry{Icon: "network", Disabled: true}, 26},
	}
	for _, c := range cases {
		if got := navGap(c.prev, c.cur); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
