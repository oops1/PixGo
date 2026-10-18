package imaging

import (
	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/colorm"
)

var strayColor = colorm.NewManager(nil)

func (l *Loader) SetColor(m *colorm.Manager) { l.color.Store(m) }

func (l *Loader) colorManager() *colorm.Manager { return l.color.Load() }

func (l *Loader) colorStamp() uint64 {
	if m := l.colorManager(); m != nil {
		return m.Stamp()
	}
	return 0
}

func (l *Loader) wantDeep() bool { return l.colorManager().Enabled() }

func (l *Loader) applyColor(img *codec.Image) (colorm.Report, error) {
	m := l.colorManager()
	if m == nil {
		m = strayColor
	}
	return m.ConvertImage(img)
}

func (l *Loader) wrapTiles(tr codec.TileReader) (codec.TileReader, colorm.Report) {
	m := l.colorManager()
	if m == nil {
		return tr, colorm.Report{}
	}
	return m.WrapTiles(tr)
}

const inkBitsPerPixel = 32

func deepExtraBytes(info codec.Info) int64 {
	switch info.Format {
	case codec.JPEG:
		if info.BitDepth == inkBitsPerPixel {
			return int64(info.Width) * int64(info.Height) * 4
		}
		return 0
	case codec.PNG, codec.APNG, codec.TIFF:
	default:
		return 0
	}
	switch info.BitDepth {
	case 0, 1, 2, 4, 8, 24, 32:
		return 0
	}
	return int64(info.Width) * int64(info.Height) * 8
}

func (l *Loader) deepReserve(info codec.Info) int64 {
	if !l.wantDeep() {
		return 0
	}
	return deepExtraBytes(info)
}

func (l *Loader) ColorChanged() {
	if l.closed.Load() {
		return
	}
	stamp := l.colorStamp()
	for path, s := range l.slots {
		if s.entry.State != StateReady || s.entry.Img == nil || s.entry.Img.ColorStamp == stamp {
			continue
		}
		l.cache.Remove(path)
		l.requeue(s)
		p := path
		l.post(func() {
			if !l.closed.Load() {
				l.notify(p)
			}
		})
	}
}
