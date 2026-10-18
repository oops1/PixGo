package tiff

import (
	"context"
	"image"
	"sync"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/codec/internal/reduce"
	"github.com/oops1/PixGo/internal/codec/internal/shield"
)

const (
	pipelineDepth = 1
	lookahead     = 2
)

type piece struct {
	x, y int
	img  *image.NRGBA
	deep *image.NRGBA64
	done func()
}

func (p piece) release() {
	if p.done != nil {
		p.done()
	}
}

type pieceResult struct {
	piece piece
	err   error
	last  bool
}

type producer func(ctx context.Context, chunk int, emit func(piece) error) error

func walk(ctx context.Context, chunks []int, workers int, produce producer, consume func(piece) error) error {
	workers = min(workers, len(chunks))
	if workers <= 1 {
		for _, chunk := range chunks {
			if err := ctx.Err(); err != nil {
				return err
			}
			err := shield.Run("tiff", func() error {
				return produce(ctx, chunk, func(p piece) error {
					err := consume(p)
					p.release()
					return err
				})
			})
			if err != nil {
				return err
			}
		}
		return nil
	}
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	inFlight := workers * lookahead
	slots := make(chan struct{}, inFlight)
	ring := make([]chan pieceResult, inFlight)
	for i := range ring {
		ring[i] = make(chan pieceResult, pipelineDepth+2)
	}
	var (
		next atomic.Int64
		wg   sync.WaitGroup
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case slots <- struct{}{}:
				case <-ctx.Done():
					return
				}
				i := int(next.Add(1)) - 1
				if i >= len(chunks) {
					return
				}
				out := ring[i%inFlight]
				send := func(r pieceResult) error {
					select {
					case out <- r:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				err := shield.Run("tiff", func() error {
					return produce(ctx, chunks[i], func(p piece) error { return send(pieceResult{piece: p}) })
				})
				if err != nil {
					send(pieceResult{err: err})
				}
				send(pieceResult{last: true})
			}
		}()
	}
	failure := collect(ctx, ring, slots, len(chunks), consume)
	cancel()
	wg.Wait()
	if failure != nil {
		return failure
	}
	return parent.Err()
}

type blitter struct {
	source  image.Rectangle
	shift   int
	out     *image.NRGBA
	scratch reduce.Scratch
}

func (b *blitter) add(p piece) error {
	at := image.Rect(p.x, p.y, p.x+p.img.Rect.Dx(), p.y+p.img.Rect.Dy()).Intersect(b.source)
	if at.Empty() {
		return nil
	}
	view := p.img.SubImage(at.Sub(image.Pt(p.x, p.y))).(*image.NRGBA)
	reduced := b.scratch.Levels(view, b.shift)
	destX := (at.Min.X - b.source.Min.X) >> uint(b.shift)
	destY := (at.Min.Y - b.source.Min.Y) >> uint(b.shift)
	width, height := reduced.Rect.Dx(), reduced.Rect.Dy()
	if destX+width > b.out.Rect.Dx() || destY+height > b.out.Rect.Dy() {
		return guard.Corrupt("tiff: reduced piece overflows the output")
	}
	for row := 0; row < height; row++ {
		copy(b.out.Pix[(destY+row)*b.out.Stride+destX*4:], reduced.Pix[row*reduced.Stride:row*reduced.Stride+width*4])
	}
	return nil
}

type rowAccumulator struct {
	source  image.Rectangle
	shift   int
	out     *image.NRGBA
	scratch reduce.Scratch
	pending *image.NRGBA
	filled  int
	nextRow int
	outRow  int
}

func newRowAccumulator(source image.Rectangle, shift int, out *image.NRGBA) *rowAccumulator {
	return &rowAccumulator{
		source:  source,
		shift:   shift,
		out:     out,
		pending: image.NewNRGBA(image.Rect(0, 0, source.Dx(), 1<<uint(shift))),
		nextRow: source.Min.Y,
	}
}

func (a *rowAccumulator) add(p piece) error {
	top := max(p.y, a.source.Min.Y)
	bottom := min(p.y+p.img.Rect.Dy(), a.source.Max.Y)
	if top >= bottom {
		return nil
	}
	if top != a.nextRow || p.x > a.source.Min.X || p.x+p.img.Rect.Dx() < a.source.Max.X {
		return guard.Corrupt("tiff: rows arrive out of order")
	}
	from := (a.source.Min.X - p.x) * 4
	width := a.source.Dx() * 4
	for y := top; y < bottom; y++ {
		src := p.img.Pix[(y-p.y)*p.img.Stride+from:]
		copy(a.pending.Pix[a.filled*a.pending.Stride:], src[:width])
		a.filled++
		a.nextRow++
		if a.filled == 1<<uint(a.shift) {
			a.flush()
		}
	}
	return nil
}

func (a *rowAccumulator) flush() {
	if a.filled == 0 {
		return
	}
	view := a.pending.SubImage(image.Rect(0, 0, a.pending.Rect.Dx(), a.filled)).(*image.NRGBA)
	reduced := a.scratch.Levels(view, a.shift)
	copy(a.out.Pix[a.outRow*a.out.Stride:a.outRow*a.out.Stride+a.out.Rect.Dx()*4], reduced.Pix[:reduced.Rect.Dx()*4])
	a.outRow++
	a.filled = 0
}

func (a *rowAccumulator) finish() error {
	a.flush()
	if a.outRow != a.out.Rect.Dy() {
		return guard.Corrupt("tiff: %d of %d output rows produced", a.outRow, a.out.Rect.Dy())
	}
	return nil
}

func collect(ctx context.Context, ring []chan pieceResult, slots chan struct{}, total int, consume func(piece) error) error {
	for i := 0; i < total; i++ {
		channel := ring[i%len(ring)]
		for finished := false; !finished; {
			select {
			case r := <-channel:
				switch {
				case r.last:
					<-slots
					finished = true
				case r.err != nil:
					return r.err
				default:
					err := consume(r.piece)
					r.piece.release()
					if err != nil {
						return err
					}
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return nil
}
