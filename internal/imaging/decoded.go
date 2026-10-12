package imaging

import (
	"image"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/render"
)

type Key struct {
	Path    string
	Size    int64
	ModTime time.Time
}

type Decoded struct {
	Key     Key
	Info    codec.Info
	Levels  []*image.NRGBA
	Frames  []codec.Frame
	Loop    int
	Preview bool
}

func (d *Decoded) Source() render.Source {
	if d == nil || len(d.Levels) == 0 {
		return render.Source{}
	}
	b := d.Levels[0].Bounds()
	return render.Source{Levels: d.Levels, Width: b.Dx(), Height: b.Dy()}
}

func (d *Decoded) FrameSource(i int) render.Source {
	if d == nil || i <= 0 || i >= len(d.Frames) {
		return d.Source()
	}
	f := d.Frames[i].Pix
	b := f.Bounds()
	return render.Source{Levels: []*image.NRGBA{f}, Width: b.Dx(), Height: b.Dy()}
}

func (d *Decoded) Bytes() int64 {
	if d == nil {
		return 0
	}
	var n int64
	for _, l := range d.Levels {
		n += int64(len(l.Pix))
	}
	for i, f := range d.Frames {
		if i == 0 && len(d.Levels) > 0 && f.Pix == d.Levels[0] {
			continue
		}
		n += int64(len(f.Pix.Pix))
	}
	return n
}

type State uint8

const (
	StateQueued State = iota
	StateProbed
	StateReady
	StateFailed
)

type Priority uint8

const (
	PrioCurrent Priority = iota
	PrioNeighbor
	PrioPrefetch
)
