package colorm

import (
	"context"
	"image"

	"github.com/oops1/PixGo/internal/codec"
)

type tileReader struct {
	codec.TileReader
	deep codec.DeepTileReader
	conv *Conv
}

func (t *tileReader) Overview(ctx context.Context, shift int) (*image.NRGBA, error) {
	if t.deep != nil {
		d, err := t.deep.OverviewDeep(ctx, shift)
		if err != nil {
			return nil, err
		}
		return t.fromDeep(d)
	}
	img, err := t.TileReader.Overview(ctx, shift)
	if err == nil && img != nil {
		t.conv.Tile(img)
	}
	return img, err
}

func (t *tileReader) Region(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA, error) {
	if t.deep != nil {
		d, err := t.deep.RegionDeep(ctx, shift, r)
		if err != nil {
			return nil, err
		}
		return t.fromDeep(d)
	}
	img, err := t.TileReader.Region(ctx, shift, r)
	if err == nil && img != nil {
		t.conv.Tile(img)
	}
	return img, err
}

func (t *tileReader) fromDeep(d *image.NRGBA64) (*image.NRGBA, error) {
	if d == nil {
		return nil, codec.ErrCorrupt
	}
	f := codec.Frame{Deep: d}
	if err := t.conv.Frame(&f); err != nil {
		return nil, err
	}
	return f.Pix, nil
}

func (m *Manager) WrapTiles(tr codec.TileReader) (codec.TileReader, Report) {
	if !m.Enabled() {
		return tr, Report{}
	}
	var ci codec.ColorInfo
	if c, ok := tr.(codec.ColorTileReader); ok {
		ci = c.Color()
	}
	conv := m.Resolve(ci)
	rep := conv.Report
	var deep codec.DeepTileReader
	if d, ok := tr.(codec.DeepTileReader); ok && d.Deep() {
		deep = d
		rep.Deep = true
	}
	if deep == nil && !conv.Needed() {
		return tr, rep
	}
	return &tileReader{TileReader: tr, deep: deep, conv: conv}, rep
}
