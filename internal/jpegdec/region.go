package jpegdec

import (
	"context"
	"image"
	"sync"
	"sync/atomic"
)

type regionWorker struct {
	d    *Decoder
	g    *geometry
	ix   *checkpointIndex
	r    *recon
	s    *stream
	ss   *streamScan
	band *bandBuf
	sc   *scratch
	cols int
}

func (d *Decoder) newRegionWorker(ctx context.Context, g *geometry, ix *checkpointIndex, r *recon, cols int) (*regionWorker, error) {
	s := newStream(ctx, d.reader, d.size)
	ss, err := newStreamScan(d, g, s)
	if err != nil {
		s.release()
		return nil, err
	}
	return &regionWorker{
		d:    d,
		g:    g,
		ix:   ix,
		r:    r,
		s:    s,
		ss:   ss,
		band: newBandBuf(d.f, g, 1, cols),
		sc:   scratchPool.Get().(*scratch),
		cols: cols,
	}, nil
}

func (w *regionWorker) release() {
	w.s.release()
	scratchPool.Put(w.sc)
}

func (w *regionWorker) row(my, mx0 int, out *image.NRGBA, rect image.Rectangle) error {
	g, ss, f := w.g, w.ss, w.d.f
	mStart := my*g.mxx + mx0
	mEnd := mStart + w.cols - 1
	slot := w.ix.slotFor(mStart)
	ss.ent.resume(int64(w.ix.off[slot]), w.ix.bit[slot])
	ss.pred = [maxComponents]int16{}
	copy(ss.pred[:f.nComp], w.ix.dc[slot*w.ix.nComp:(slot+1)*w.ix.nComp])
	ss.ent.eobRun = 0
	w.band.clear(f)
	views := w.band.view(f, g, w.cols, 0)
	strides := w.band.strides(f, w.cols)
	for m := slot * w.ix.every; m <= mEnd; m++ {
		var err error
		if m >= mStart {
			err = ss.decodeMCU(views, strides, m-mStart, 0, false)
		} else {
			err = ss.decodeMCU(views, strides, 0, 0, true)
		}
		if err == nil {
			err = ss.afterMCU(m)
		}
		if err != nil {
			return err
		}
	}
	w.r.row(w.sc, views, strides, w.cols, mx0, my, out, rect)
	return nil
}

func (d *Decoder) regionStreaming(ctx context.Context, g *geometry, rect image.Rectangle, mode colorMode) (*image.NRGBA, error) {
	ix, err := d.ensureIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	mx0, mx1 := rect.Min.X/g.mcuW, (rect.Max.X-1)/g.mcuW
	my0, my1 := rect.Min.Y/g.mcuH, (rect.Max.Y-1)/g.mcuH
	cols := mx1 - mx0 + 1
	quant := dequantTables(d.f)
	r := newRecon(d.f, mode, g, &quant)
	rows := my1 - my0 + 1
	workers := min(d.workerCount(), rows)
	if rect.Dx()*rect.Dy() < parallelPixels && d.opt.Workers <= 0 {
		workers = 1
	}
	if workers <= 1 {
		w, err := d.newRegionWorker(ctx, g, ix, r, cols)
		if err != nil {
			return nil, err
		}
		defer w.release()
		for my := my0; my <= my1; my++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := w.row(my, mx0, out, rect); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	var (
		next    atomic.Int64
		failed  atomic.Bool
		wg      sync.WaitGroup
		errOnce sync.Once
		firstEr error
		box     panicBox
	)
	fail := func(err error) {
		failed.Store(true)
		errOnce.Do(func() { firstEr = err })
	}
	for i := 0; i < workers; i++ {
		w, err := d.newRegionWorker(ctx, g, ix, r, cols)
		if err != nil {
			fail(err)
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer w.release()
			defer box.capture()
			for !failed.Load() {
				my := my0 + int(next.Add(1)) - 1
				if my > my1 {
					return
				}
				if err := ctx.Err(); err != nil {
					fail(err)
					return
				}
				if err := w.row(my, mx0, out, rect); err != nil {
					fail(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if box.value != nil {
		panic(box.value)
	}
	if firstEr != nil {
		return nil, firstEr
	}
	return out, nil
}
