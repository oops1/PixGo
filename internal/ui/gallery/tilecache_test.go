package gallery

import (
	"image"
	"image/color"
	"testing"
)

func TestCoverRectCropsTheLongAxisAroundTheCentre(t *testing.T) {
	cases := []struct {
		name string
		src  image.Rectangle
		w, h int
		want image.Rectangle
	}{
		{"wide source into 4:3", image.Rect(0, 0, 200, 100), 96, 72, image.Rect(33, 0, 167, 100)},
		{"tall source into 4:3", image.Rect(0, 0, 100, 200), 96, 72, image.Rect(0, 63, 100, 138)},
		{"same ratio keeps everything", image.Rect(0, 0, 96, 72), 96, 72, image.Rect(0, 0, 96, 72)},
		{"offset source", image.Rect(10, 20, 210, 120), 96, 72, image.Rect(43, 20, 177, 120)},
	}
	for _, c := range cases {
		if got := coverRect(c.src, c.w, c.h); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRenderTileRoundsCornersAndKeepsTheCentre(t *testing.T) {
	green := color.RGBA{20, 180, 40, 255}

	tile := renderTile(solid(200, 100, green), 95, 71, 6)

	if got := tile.RGBAAt(47, 35); got != green {
		t.Fatalf("centre = %v", got)
	}
	if got := tile.RGBAAt(0, 0); got.A != 0 {
		t.Fatalf("top-left corner alpha = %d", got.A)
	}
	if got := tile.RGBAAt(94, 70); got.A != 0 {
		t.Fatalf("bottom-right corner alpha = %d", got.A)
	}
	if got := tile.RGBAAt(47, 0); got.A != 255 {
		t.Fatalf("straight edge alpha = %d", got.A)
	}
	if edge, inner := tile.RGBAAt(47, 0), tile.RGBAAt(47, 6); edge.R <= inner.R {
		t.Fatalf("the outline must lighten the edge: %v vs %v", edge, inner)
	}
}

func TestRenderTileSurvivesEmptySources(t *testing.T) {
	if tile := renderTile(image.NewRGBA(image.Rectangle{}), 40, 30, 6); tile.Bounds().Dx() != 40 {
		t.Fatalf("bounds = %v", tile.Bounds())
	}
	if tile := renderTile(solid(10, 10, color.RGBA{1, 2, 3, 255}), 0, 0, 6); !tile.Bounds().Empty() {
		t.Fatalf("bounds = %v", tile.Bounds())
	}
}

func TestTileCacheReusesTheBakedTileForTheSameSource(t *testing.T) {
	c := newTileCache()
	src := solid(64, 48, color.RGBA{9, 9, 9, 255})

	a := c.get(src, 95, 71, 6)
	b := c.get(src, 95, 71, 6)

	if a != b {
		t.Fatal("the same source and size must give the same tile")
	}
	if other := c.get(src, 80, 60, 6); other == a || c.size() != 2 {
		t.Fatalf("a different size is a different tile, size = %d", c.size())
	}
	if other := c.get(solid(64, 48, color.RGBA{9, 9, 9, 255}), 95, 71, 6); other == a {
		t.Fatal("a different source image is a different tile")
	}
}

func TestTileCacheEvictsTheLeastRecentlyUsed(t *testing.T) {
	c := newTileCache()
	first := solid(8, 8, color.RGBA{1, 1, 1, 255})
	firstTile := c.get(first, 8, 8, 2)
	for i := 0; i < tileCacheEntries; i++ {
		c.get(solid(8, 8, color.RGBA{2, 2, 2, 255}), 8, 8, 2)
	}

	if c.size() != tileCacheEntries {
		t.Fatalf("size = %d", c.size())
	}
	if again := c.get(first, 8, 8, 2); again == firstTile {
		t.Fatal("the oldest tile should have been evicted")
	}
}

func TestTileCacheTouchKeepsRecentTilesAlive(t *testing.T) {
	c := newTileCache()
	keep := solid(8, 8, color.RGBA{1, 1, 1, 255})
	keepTile := c.get(keep, 8, 8, 2)
	for i := 0; i < tileCacheEntries-1; i++ {
		c.get(solid(8, 8, color.RGBA{2, 2, 2, 255}), 8, 8, 2)
	}
	c.get(keep, 8, 8, 2)
	c.get(solid(8, 8, color.RGBA{3, 3, 3, 255}), 8, 8, 2)

	if c.get(keep, 8, 8, 2) != keepTile {
		t.Fatal("a recently used tile must survive")
	}
}

func TestTileCacheIsBoundedInBytes(t *testing.T) {
	c := newTileCache()

	for i := 0; i < 8; i++ {
		c.get(solid(16, 16, color.RGBA{uint8(i), 4, 4, 255}), 1800, 1800, 6)
	}

	if c.bytes > tileCacheBytes || c.size() >= 8 || c.size() == 0 {
		t.Fatalf("bytes = %d, size = %d", c.bytes, c.size())
	}
}

func TestTileCacheResetForgetsEverything(t *testing.T) {
	c := newTileCache()
	c.get(solid(8, 8, color.RGBA{1, 1, 1, 255}), 8, 8, 2)

	c.reset()

	if c.size() != 0 || c.bytes != 0 {
		t.Fatalf("size = %d, bytes = %d", c.size(), c.bytes)
	}
}
