package imaging

import (
	"image"
	"io"
	"sync/atomic"
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
	Shift   int
	Detail  render.Detail
	Reduced bool

	released atomic.Bool
}

type reserver interface {
	MaxBytes() int64
}

func (d *Decoded) fullSize() (int, int) {
	b := d.Levels[0].Bounds()
	if d.Shift <= 0 {
		return b.Dx(), b.Dy()
	}
	if d.Info.Width > 0 && d.Info.Height > 0 {
		return d.Info.Width, d.Info.Height
	}
	return b.Dx() << uint(d.Shift), b.Dy() << uint(d.Shift)
}

func (d *Decoded) Source() render.Source {
	if d == nil || len(d.Levels) == 0 {
		return render.Source{}
	}
	w, h := d.fullSize()
	return render.Source{Levels: d.Levels, Width: w, Height: h, Shift: max(d.Shift, 0), Detail: d.Detail}
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
	if r, ok := d.Detail.(reserver); ok {
		n += r.MaxBytes()
	}
	return n
}

func (d *Decoded) FullResolution() bool {
	return d != nil && d.Shift <= 0 && !d.Reduced && len(d.Levels) > 0
}

func (d *Decoded) Release() {
	if d == nil || !d.released.CompareAndSwap(false, true) {
		return
	}
	if c, ok := d.Detail.(io.Closer); ok {
		c.Close()
	}
}

func (d *Decoded) Released() bool { return d != nil && d.released.Load() }

type State uint8

const (
	StateQueued State = iota
	StateProbed
	StateReady
	StateFailed
	StatePreview

	stateSkipped State = 0xff
)

type Priority uint8

const (
	PrioCurrent Priority = iota
	PrioNeighbor
	PrioPrefetch
)
