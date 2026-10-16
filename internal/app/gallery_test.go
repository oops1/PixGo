package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/commands"
	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/settings"
	uigallery "github.com/oops1/PixGo/internal/ui/gallery"
	uiwindow "github.com/oops1/PixGo/internal/ui/window"
)

type galleryRig struct {
	app  *App
	root string
	a    string
	b    string
	c    string
}

func newGalleryRig(t *testing.T, mutate func(*settings.Settings)) *galleryRig {
	t.Helper()
	a, _ := goldenApp(t, 770, 504, mutate)
	t.Cleanup(a.Shutdown)
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	r := &galleryRig{app: a, root: root, a: filepath.Join(root, "a.jpg"), b: filepath.Join(root, "b.jpg"), c: filepath.Join(sub, "c.jpg")}
	writeBigJPEG(t, r.a)
	writeBigJPEG(t, r.b)
	writeBigJPEG(t, r.c)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *galleryRig) settle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.app.Eng.Flush()
		if r.app.gal != nil && !r.app.gal.loading {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("the folder was not listed in time")
}

func (r *galleryRig) names() []string {
	var out []string
	for _, it := range r.app.gal.Items() {
		out = append(out, it.Name)
	}
	return out
}

func equalNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func (r *galleryRig) open(t *testing.T) {
	t.Helper()
	r.app.OpenGalleryAt(model.Folder(r.root), "")
	r.settle(t)
}

func TestGalleryOpensOnTheFolderOfTheCurrentImage(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.app.OpenFiles([]string{r.b})
	waitName(t, r.app, "b.jpg")

	r.app.OpenGallery()
	r.settle(t)

	if !r.app.GalleryOpen() || r.app.Page() != uiwindow.PageGallery {
		t.Fatal("the gallery page is not shown")
	}
	if got := r.names(); !equalNames(got, []string{"sub", "a.jpg", "b.jpg"}) {
		t.Fatalf("items = %v", got)
	}
	if r.app.gal.Selected() != 2 {
		t.Fatalf("selected = %d, want the open image", r.app.gal.Selected())
	}
	if r.app.Content() != r.app.gal.page {
		t.Fatal("the gallery page must be the window content")
	}
	if r.app.Panel() != nil {
		t.Fatal("the info panel must not show on the gallery page")
	}
}

func TestGalleryClosesBackToTheViewer(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)

	r.app.CloseGallery()

	if r.app.GalleryOpen() || r.app.Content() != r.app.Canvas {
		t.Fatal("the viewer must be back")
	}
}

func TestOpeningAFileFromTheGalleryShowsItInTheViewer(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)

	r.app.gal.actions().Open(2)

	if r.app.GalleryOpen() {
		t.Fatal("the gallery is still open")
	}
	waitName(t, r.app, "b.jpg")
}

func TestOpeningAFolderEntersItAndUpComesBackSelectingIt(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	act := r.app.gal.actions()

	act.Open(0)
	r.settle(t)

	if got := r.names(); !equalNames(got, []string{"c.jpg"}) {
		t.Fatalf("inside sub: %v", got)
	}
	if !r.app.gal.CanBack() || !r.app.gal.CanUp() || r.app.gal.CanForward() {
		t.Fatal("history buttons are wrong inside sub")
	}
	act.Up()
	r.settle(t)
	if got := r.names(); !equalNames(got, []string{"sub", "a.jpg", "b.jpg"}) {
		t.Fatalf("after up: %v", got)
	}
	if r.app.gal.Selected() != 0 {
		t.Fatalf("up must select the folder we came from, selected = %d", r.app.gal.Selected())
	}
}

func TestBackAndForwardWalkThroughVisitedPlaces(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	act := r.app.gal.actions()
	act.Open(0)
	r.settle(t)

	act.Back()
	r.settle(t)
	if r.app.gal.Location().Path != r.root || !r.app.gal.CanForward() {
		t.Fatalf("back landed on %v", r.app.gal.Location())
	}
	act.Forward()
	r.settle(t)

	if got := r.names(); !equalNames(got, []string{"c.jpg"}) {
		t.Fatalf("after forward: %v", got)
	}
}

func TestSelectingAnItemKeepsTheSelectionAndProbesItsSize(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)

	r.app.gal.actions().Select(1)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && r.app.gal.dims[r.a] == [2]int{} {
		r.app.Eng.Flush()
		time.Sleep(2 * time.Millisecond)
	}

	if r.app.gal.Selected() != 1 {
		t.Fatalf("selected = %d", r.app.gal.Selected())
	}
	if d := r.app.gal.dims[r.a]; d != [2]int{600, 400} {
		t.Fatalf("probed size = %v", d)
	}
	parts := r.app.gal.Status()
	if len(parts) != 3 {
		t.Fatalf("status = %q", parts)
	}
}

func TestStatusCountsItemsWhenNothingIsSelected(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)

	parts := r.app.gal.Status()

	if len(parts) != 1 {
		t.Fatalf("status = %q", parts)
	}
}

func TestFormatFilterKeepsFoldersAndMatchingFiles(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	if got := r.app.gal.Formats(); len(got) != 1 || got[0] != "JPEG" {
		t.Fatalf("formats = %v", got)
	}

	r.app.gal.actions().SetFormat("PNG")

	if got := r.names(); !equalNames(got, []string{"sub"}) {
		t.Fatalf("filtered = %v", got)
	}
	r.app.gal.actions().SetFormat("")
	if len(r.names()) != 3 {
		t.Fatalf("unfiltered = %v", r.names())
	}
}

func TestAddressOfAFileOpensItAndOfAFolderNavigates(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	act := r.app.gal.actions()

	if err := act.Address(filepath.Join(r.root, "sub")); err != nil {
		t.Fatal(err)
	}
	r.settle(t)
	if got := r.names(); !equalNames(got, []string{"c.jpg"}) {
		t.Fatalf("after address: %v", got)
	}
	if err := act.Address(filepath.Join(r.root, "missing")); err == nil {
		t.Fatal("a missing folder must be reported")
	}
	if err := act.Address(r.b); err != nil {
		t.Fatal(err)
	}
	waitName(t, r.app, "b.jpg")
	if r.app.GalleryOpen() {
		t.Fatal("opening a file by its address must show the viewer")
	}
}

func TestFavoritingFromTheGalleryUpdatesTheSharedList(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	act := r.app.gal.actions()

	act.ToggleFavorite(1)

	if !r.app.gal.Favorite(r.a) || !r.app.Ctl.Favorites.Contains(r.a) {
		t.Fatal("the file is not a favorite")
	}
	act.ToggleFavorite(1)
	if r.app.gal.Favorite(r.a) {
		t.Fatal("the favorite was not removed")
	}
	act.ToggleFavorite(0)
	if r.app.Ctl.Favorites.Len() != 0 {
		t.Fatal("folders cannot be favorites")
	}
}

func TestFavoritesPlaceListsOnlyFavorites(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.app.Ctl.Favorites.Add(r.b)
	r.app.OpenGalleryAt(model.Location{Kind: model.KindFavorites}, "")
	r.settle(t)

	if got := r.names(); !equalNames(got, []string{"b.jpg"}) {
		t.Fatalf("favorites = %v", got)
	}

	r.app.gal.actions().ToggleFavorite(0)
	r.settle(t)
	if len(r.names()) != 0 {
		t.Fatalf("after removing the last favorite: %v", r.names())
	}
}

func TestRemovedFileDisappearsFromTheGallery(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	r.app.gal.actions().Select(1)

	r.app.fileRemoved(r.a)

	if got := r.names(); !equalNames(got, []string{"sub", "b.jpg"}) {
		t.Fatalf("after removal: %v", got)
	}
	if r.app.gal.Selected() < 0 {
		t.Fatal("the selection must move to a neighbour")
	}
}

func TestUnavailableFolderIsReportedAsFailed(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.app.OpenGalleryAt(model.Folder(filepath.Join(r.root, "gone")), "")
	r.settle(t)

	if !r.app.gal.Failed() || len(r.app.gal.Items()) != 0 {
		t.Fatalf("failed = %v, items = %v", r.app.gal.Failed(), r.names())
	}
}

func TestViewModeAndThumbnailSizeAreStoredInSettings(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	act := r.app.gal.actions()

	act.SetViewMode(uigallery.ViewList)
	act.SetThumbSize(250)

	st := r.app.Settings.Get()
	if st.GalleryView != settings.GalleryList || st.GalleryThumbSize != 256 {
		t.Fatalf("settings: %q %d", st.GalleryView, st.GalleryThumbSize)
	}
	if r.app.gal.ViewMode() != uigallery.ViewList || r.app.gal.ThumbSize() != 256 {
		t.Fatalf("source: %v %d", r.app.gal.ViewMode(), r.app.gal.ThumbSize())
	}
	act.SetViewMode(uigallery.ViewGrid)
	if r.app.gal.ViewMode() != uigallery.ViewGrid {
		t.Fatal("the grid view was not restored")
	}
}

func TestThumbnailArrivesAfterTheRequest(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	if _, ok := r.app.gal.Thumb(r.a, 128, true); ok {
		t.Fatal("a thumbnail cannot be ready before it was requested")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.app.Eng.Flush()
		if img, ok := r.app.gal.Thumb(r.a, 128, true); ok {
			if img.Bounds().Dx() != 128 {
				t.Fatalf("thumbnail width = %d", img.Bounds().Dx())
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the thumbnail never arrived")
}

func TestVisibleRangePrioritisesOnlyFiles(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)

	r.app.gal.actions().Visible(0, 3)
	r.app.gal.actions().Visible(5, 2)
	r.app.gal.actions().Visible(-1, 10)
}

func TestHandoffFromOutsideLeavesTheGallery(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)

	r.app.Handoff([]string{r.b})

	if r.app.GalleryOpen() {
		t.Fatal("a file handed over from the file manager must be shown")
	}
	waitName(t, r.app, "b.jpg")
}

func TestGalleryIsBusyWhileListing(t *testing.T) {
	r := newGalleryRig(t, nil)
	if r.app.GalleryBusy() {
		t.Fatal("no gallery yet")
	}
	r.app.OpenGalleryAt(model.Folder(r.root), "")
	if !r.app.GalleryBusy() {
		t.Fatal("the listing is running")
	}
	r.settle(t)
}

func TestGalleryOpensAgainAfterBeingClosed(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	r.app.CloseGallery()
	r.app.OpenGallery()
	r.settle(t)

	if !r.app.GalleryOpen() {
		t.Fatal("the gallery did not open a second time")
	}
}

func TestBackButtonAndOpenCommandsOpenTheGallery(t *testing.T) {
	r := newGalleryRig(t, nil)

	for _, id := range []string{"app.back", "file.open", "view.gallery"} {
		r.app.CloseGallery()
		r.app.Commands.Run(commands.ID(id))
		if !r.app.GalleryOpen() {
			t.Fatalf("%s did not open the gallery", id)
		}
	}
	r.settle(t)
}

func TestOpeningAPictureWhileTheAddressBarIsEditedDoesNotFreeze(t *testing.T) {
	a, _ := goldenApp(t, 1100, 700, nil)
	first, _ := twoImages(t)
	a.OpenGalleryAt(model.Folder(filepath.Dir(first)), "")
	a.Eng.Flush()
	a.gal.page.FocusAddress()
	done := make(chan struct{})
	go func() {
		a.Eng.Post(func() { a.gal.openFile(first) })
		a.Eng.Flush()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("opening a picture from the gallery while the address bar was edited froze the interface")
	}
	waitName(t, a, filepath.Base(first))
}
