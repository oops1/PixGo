package app

import (
	"bytes"
	"context"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	uiwindow "github.com/oops1/PixGo/internal/ui/window"
)

type previewFormat struct {
	stubFormat
	preview bool
}

func (p previewFormat) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	info, err := p.stubFormat.Probe(ctx, r, size, lim)
	info.Preview = p.preview
	return info, err
}

func (p previewFormat) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	img, err := p.stubFormat.Decode(ctx, r, size, o)
	if img != nil {
		img.Info.Preview = p.preview
	}
	return img, err
}

func (p previewFormat) Sniff(head []byte) bool {
	return bytes.HasPrefix(head, []byte("STUB-"+string(p.id)))
}

func previewApp(t *testing.T) (*App, string) {
	t.Helper()
	reg := formatsRegistry()
	reg.RegisterDecoder(previewFormat{stubFormat: stubFor(codec.RAW), preview: true})
	a := formatsApp(t, reg, nil)
	dir := t.TempDir()
	writeStub(t, filepath.Join(dir, "a.cr2"), codec.RAW)
	writeBigJPEG(t, filepath.Join(dir, "b.jpg"))
	if err := os.WriteFile(filepath.Join(dir, "c.cr2"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	return a, dir
}

func TestPreviewFlagReachesTheWindowForAnEmbeddedPreview(t *testing.T) {
	a, dir := previewApp(t)

	a.OpenFiles([]string{filepath.Join(dir, "a.cr2")})
	waitName(t, a, "a.cr2")

	if !a.Preview() || !a.Canvas.Preview() || !a.Ctl.Info().Preview {
		t.Fatalf("host %v canvas %v controller %v", a.Preview(), a.Canvas.Preview(), a.Ctl.Info().Preview)
	}
	if b := a.Root.PreviewBadge(); b.Empty() {
		t.Fatal("the window draws no badge for a preview")
	}
}

func TestPreviewFlagGoesAwayWithAnOrdinaryImage(t *testing.T) {
	a, dir := previewApp(t)
	a.OpenFiles([]string{filepath.Join(dir, "a.cr2")})
	waitName(t, a, "a.cr2")

	a.Ctl.Step(1)
	waitName(t, a, "b.jpg")

	if a.Preview() || !a.Root.PreviewBadge().Empty() {
		t.Fatal("an ordinary JPEG must not carry the badge")
	}
}

func TestPreviewFlagReturnsWhenSteppingBack(t *testing.T) {
	a, dir := previewApp(t)
	a.OpenFiles([]string{filepath.Join(dir, "b.jpg")})
	waitName(t, a, "b.jpg")
	a.Ctl.Step(-1)
	waitName(t, a, "a.cr2")

	if !a.Preview() {
		t.Fatal("the badge must come back with the preview")
	}
}

func TestBadgeDisappearsWhenTheFileFailsToOpen(t *testing.T) {
	a, dir := previewApp(t)
	a.OpenFiles([]string{filepath.Join(dir, "a.cr2")})
	waitName(t, a, "a.cr2")

	a.Ctl.Step(1)
	waitName(t, a, "b.jpg")
	a.Ctl.Step(1)
	eventually(t, a, "the broken file fails", func() bool { return a.Ctl.Name() == "c.cr2" && a.Ctl.Status() == StatusFailed })

	if a.Preview() || !a.Root.PreviewBadge().Empty() {
		t.Fatal("a failed file must not show a preview badge")
	}
}

func TestBadgeDisappearsWhenTheFolderBecomesEmpty(t *testing.T) {
	a, _ := previewApp(t)
	dir := t.TempDir()
	only := filepath.Join(dir, "only.cr2")
	writeStub(t, only, codec.RAW)
	a.OpenFiles([]string{only})
	waitName(t, a, "only.cr2")
	if !a.Preview() {
		t.Fatal("no badge for the only file")
	}

	a.Ctl.Removed(only)

	if a.Preview() || a.State() != uiwindow.StateEmpty {
		t.Fatalf("badge %v state %v after the folder was emptied", a.Preview(), a.State())
	}
}

func TestInfoPanelMarksThePreviewFormat(t *testing.T) {
	a, dir := previewApp(t)
	a.OpenFiles([]string{filepath.Join(dir, "a.cr2")})
	waitName(t, a, "a.cr2")

	a.toggleInfo()

	m := a.info.Model()
	if !strings.HasSuffix(m.Format, i18n.T("info.previewMark")) || m.FormatNote != i18n.T("info.previewMark") {
		t.Fatalf("format line %q note %q", m.Format, m.FormatNote)
	}
	a.Ctl.Step(1)
	waitName(t, a, "b.jpg")
	eventually(t, a, "the panel follows", func() bool { return a.info.Model().Name == "b.jpg" })
	if m := a.info.Model(); strings.Contains(m.Format, i18n.T("info.previewMark")) || m.FormatNote != "" {
		t.Fatalf("an ordinary file got the mark: %q / %q", m.Format, m.FormatNote)
	}
}

func TestBadgeIsDrawnOverTheImageInTheRealWindow(t *testing.T) {
	a, dir := previewApp(t)
	a.OpenFiles([]string{filepath.Join(dir, "a.cr2")})
	waitName(t, a, "a.cr2")
	a.Root.Refresh()
	a.Eng.Flush()
	with := a.Eng.RenderOnce()
	b := a.Root.PreviewBadge()
	snapshot := image.NewRGBA(with.Bounds())
	copy(snapshot.Pix, with.Pix)

	a.Ctl.Step(1)
	waitName(t, a, "b.jpg")
	a.Root.Refresh()
	a.Eng.Flush()
	without := a.Eng.RenderOnce()

	changed := false
	for y := b.Min.Y; y < b.Max.Y && !changed; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if snapshot.RGBAAt(x, y) != without.RGBAAt(x, y) {
				changed = true
				break
			}
		}
	}
	if b.Empty() || !changed {
		t.Fatalf("badge %v changed=%v", b, changed)
	}
}
