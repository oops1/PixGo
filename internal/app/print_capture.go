package app

import (
	"image"
	"os"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/colorm"
	pixprint "github.com/oops1/PixGo/internal/print"
)

func (a *App) capturePrint() (*printCapture, string) {
	if a.editing {
		return a.captureEditPrint()
	}
	shown := a.Ctl.Shown()
	switch {
	case shown == nil || len(shown.Levels) == 0:
		return nil, "edit.refused.none"
	case a.Ctl.Status() != StatusReady:
		return nil, "edit.refused.loading"
	}
	info := a.Ctl.Info()
	path := a.Ctl.Path()
	orient := a.Canvas.Orientation()
	reg, lim := a.Ctl.Registry(), a.Ctl.Limits()
	c := &printCapture{path: path, name: a.Ctl.Name()}
	fileDPI := func(ctx contextT) float64 { return readFileDPI(ctx, path, info.Format) }
	level := shown.Levels[0].Bounds().Size()
	stored := image.Pt(info.Width, info.Height)
	if stored.X < 1 || stored.Y < 1 {
		stored = level
	}
	switch {
	case info.Vector || info.Format == codec.SVG:
		c.dpi = func(contextT) float64 { return pixprint.VectorDPI }
		c.build = func(contextT) (pixprint.Source, error) {
			return pixprint.VectorSource{Dim: stored, Orient: orient, Decode: vectorDecoder(reg, lim, path)}, nil
		}
	case len(shown.Frames) > 1:
		idx := min(max(a.Ctl.anim.frame, 0), len(shown.Frames)-1)
		img := shown.Frames[idx].Pix
		if img == nil {
			img = shown.Levels[0]
		}
		c.dpi = fileDPI
		c.build = func(contextT) (pixprint.Source, error) { return pixprint.ImageSource{Img: img, Orient: orient}, nil }
	case a.Ctl.Decoded() != nil:
		img := shown.Levels[0]
		c.dpi = fileDPI
		c.build = func(contextT) (pixprint.Source, error) { return pixprint.ImageSource{Img: img, Orient: orient}, nil }
	case shown.Detail != nil:
		color := a.Ctl.Color()
		c.dpi = fileDPI
		c.build = func(contextT) (pixprint.Source, error) {
			return pixprint.TiledSource{
				Open:      tileOpener(reg, lim, path, color),
				Dim:       stored,
				Orient:    orient,
				MaxPixels: lim.WithDefaults().MaxPixels,
			}, nil
		}
	default:
		img := shown.Levels[0]
		ratio := float64(level.X) / float64(stored.X)
		c.dpi = func(ctx contextT) float64 {
			if d := fileDPI(ctx); d > 0 {
				return d * ratio
			}
			return 0
		}
		c.build = func(contextT) (pixprint.Source, error) { return pixprint.ImageSource{Img: img, Orient: orient}, nil }
	}
	return c, ""
}

func (a *App) captureEditPrint() (*printCapture, string) {
	if a.Editor.Busy() {
		return nil, "edit.busy"
	}
	job := a.captureEditJob()
	if job == nil || job.produce == nil {
		return nil, "edit.refused.none"
	}
	info := a.Ctl.Info()
	path := a.Ctl.Path()
	crop, out := a.Editor.CropSize(), a.Editor.OutputSize()
	c := &printCapture{path: path, name: a.Ctl.Name()}
	c.dpi = func(ctx contextT) float64 {
		d := readFileDPI(ctx, path, info.Format)
		if d <= 0 || crop.X < 1 {
			return d
		}
		return d * float64(out.X) / float64(crop.X)
	}
	c.build = func(ctx contextT) (pixprint.Source, error) {
		img, err := job.produce(ctx)
		if err != nil {
			return nil, err
		}
		return pixprint.ImageSource{Img: img, Orient: codec.OrientNormal}, nil
	}
	return c, ""
}

func readFileDPI(ctx contextT, path string, id codec.ID) float64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0
	}
	d, ok := pixprint.ReadDPI(ctx, f, st.Size(), id)
	if !ok {
		return 0
	}
	return d
}

func tileOpener(reg *codec.Registry, lim codec.Limits, path string, color *colorm.Manager) func(ctx contextT) (codec.TileReader, error) {
	return func(ctx contextT) (codec.TileReader, error) {
		src, err := codec.OpenFile(path)
		if err != nil {
			return nil, err
		}
		dec, ok := reg.Detect(src.Head, path)
		if !ok {
			src.Close()
			return nil, codec.ErrUnknownFormat
		}
		td, ok := dec.(codec.TiledDecoder)
		if !ok {
			src.Close()
			return nil, codec.ErrTooLarge
		}
		tr, err := td.OpenTiled(ctx, src.R, src.Size, lim)
		if err != nil {
			src.Close()
			return nil, err
		}
		if color != nil {
			tr, _ = color.WrapTiles(tr)
		}
		return tileCloser{TileReader: tr, src: src}, nil
	}
}

func vectorDecoder(reg *codec.Registry, lim codec.Limits, path string) func(ctx contextT, w, h int) (*image.NRGBA, error) {
	return func(ctx contextT, w, h int) (*image.NRGBA, error) {
		src, err := codec.OpenFile(path)
		if err != nil {
			return nil, err
		}
		defer src.Close()
		dec, ok := reg.Detect(src.Head, path)
		if !ok {
			return nil, codec.ErrUnknownFormat
		}
		im, err := dec.Decode(ctx, src.R, src.Size, codec.Options{Limits: lim, VectorW: w, VectorH: h})
		if err != nil {
			return nil, err
		}
		img := im.First()
		if img == nil {
			return nil, codec.ErrCorrupt
		}
		return img, nil
	}
}
