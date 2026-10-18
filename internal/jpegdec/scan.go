package jpegdec

import (
	"context"
	"image"
	"sync"
	"sync/atomic"
)

const (
	bandTargetBlocks = 8192
	maxBandRows      = 8
)

type bandBuf struct {
	coef  [maxComponents][]int16
	my0   int
	count int
}

func newBandBuf(f *frame, g *geometry, rows, nCols int) *bandBuf {
	b := &bandBuf{}
	for c := 0; c < f.nComp; c++ {
		comp := &f.comp[c]
		b.coef[c] = make([]int16, rows*comp.v*nCols*comp.h*g.kk[c])
	}
	return b
}

func (b *bandBuf) clear(f *frame) {
	for c := 0; c < f.nComp; c++ {
		clear(b.coef[c])
	}
}

func (b *bandBuf) strides(f *frame, nCols int) [maxComponents]int {
	var strides [maxComponents]int
	for c := 0; c < f.nComp; c++ {
		strides[c] = nCols * f.comp[c].h
	}
	return strides
}

func (b *bandBuf) view(f *frame, g *geometry, nCols, row int) [maxComponents][]int16 {
	var views [maxComponents][]int16
	for c := 0; c < f.nComp; c++ {
		comp := &f.comp[c]
		views[c] = b.coef[c][row*comp.v*nCols*comp.h*g.kk[c]:]
	}
	return views
}

func zigMap(side int) [blockSize]int8 {
	var zz [blockSize]int8
	for z := 0; z < blockSize; z++ {
		n := unzig[z]
		u, v := n%8, n/8
		if u < side && v < side {
			zz[z] = int8(v*side + u)
		} else {
			zz[z] = -1
		}
	}
	return zz
}

type streamScan struct {
	f     *frame
	g     *geometry
	sh    *scanHeader
	s     *stream
	ent   entropy
	pred  [maxComponents]int16
	zz    [maxComponents][blockSize]int8
	last  [maxComponents]int
	dc    [maxComponents]*huffTable
	ac    [maxComponents]*huffTable
	junk  []int16
	total int
	ri    int
}

func newStreamScan(d *Decoder, g *geometry, s *stream) (*streamScan, error) {
	ss := &streamScan{f: d.f, g: g, sh: &d.first, s: s}
	ss.ent.s = s
	for c := 0; c < d.f.nComp; c++ {
		ss.zz[c] = zigMap(g.ks[c])
		for z := blockSize - 1; z >= 0; z-- {
			if ss.zz[c][z] >= 0 {
				ss.last[c] = z
				break
			}
		}
	}
	for i := 0; i < d.first.n; i++ {
		sc := d.first.comp[i]
		ss.dc[i] = &d.f.huff[dcTable][sc.td]
		ss.ac[i] = &d.f.huff[acTable][sc.ta]
		if ss.dc[i].nCodes == 0 || ss.ac[i].nCodes == 0 {
			return nil, FormatError("uninitialized Huffman table")
		}
	}
	ss.junk = make([]int16, blockSize)
	ss.total = g.mxx * g.myy
	ss.ri = d.f.ri
	return ss, nil
}

const (
	dcTable = 0
	acTable = 1
)

func (ss *streamScan) decodeMCU(coef [maxComponents][]int16, cstride [maxComponents]int, mx, row int, discard bool) error {
	for i := 0; i < ss.sh.n; i++ {
		c := int(ss.sh.comp[i].index)
		comp := &ss.f.comp[c]
		kk := ss.g.kk[c]
		for j := 0; j < comp.v; j++ {
			by := comp.v*row + j
			for l := 0; l < comp.h; l++ {
				var blk []int16
				if discard {
					blk = ss.junk[:kk]
					clear(blk)
				} else {
					at := (by*cstride[c] + comp.h*mx + l) * kk
					blk = coef[c][at : at+kk : at+kk]
				}
				if err := ss.ent.decodeBlock(blk, &ss.zz[c], ss.last[c], ss.dc[i], ss.ac[i], &ss.pred[c]); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (ss *streamScan) restart(m int) error {
	ss.ent.reset()
	expected := byte(rst0Marker + ((m+1)/ss.ri-1)%8)
	var tmp [2]byte
	if err := ss.s.readFull(tmp[:]); err != nil {
		return err
	}
	if tmp[0] != 0xff || tmp[1] != expected {
		if err := findRST(ss.s, &tmp, expected); err != nil {
			return err
		}
	}
	ss.pred = [maxComponents]int16{}
	ss.ent.eobRun = 0
	return nil
}

func findRST(s *stream, tmp *[2]byte, expected byte) error {
	for {
		i := 0
		if tmp[0] == 0xff {
			if tmp[1] == expected {
				return nil
			} else if tmp[1] == 0xff {
				i = 1
			} else if tmp[1] != 0x00 {
				return FormatError("bad RST marker")
			}
		} else if tmp[1] == 0xff {
			tmp[0] = 0xff
			i = 1
		}
		if err := s.readFull(tmp[i:2]); err != nil {
			return err
		}
	}
}

func (ss *streamScan) afterMCU(m int) error {
	if ss.ent.hit {
		if err := ss.ent.check(); err != nil {
			return err
		}
	}
	if ss.ri > 0 && (m+1)%ss.ri == 0 && m+1 < ss.total {
		return ss.restart(m)
	}
	return nil
}

func (ss *streamScan) decodeRow(my int, b *bandBuf, row int, rec *indexRecorder) error {
	g := ss.g
	views := b.view(ss.f, g, g.mxx, row)
	strides := b.strides(ss.f, g.mxx)
	base := my * g.mxx
	for mx := 0; mx < g.mxx; mx++ {
		m := base + mx
		if rec != nil && m%rec.every == 0 {
			rec.capture(m, ss)
		}
		if err := ss.decodeMCU(views, strides, mx, 0, false); err != nil {
			return err
		}
		if err := ss.afterMCU(m); err != nil {
			return err
		}
	}
	return nil
}

func (ss *streamScan) skipRow(my int, rec *indexRecorder) error {
	g := ss.g
	base := my * g.mxx
	for mx := 0; mx < g.mxx; mx++ {
		m := base + mx
		if rec != nil && m%rec.every == 0 {
			rec.capture(m, ss)
		}
		if err := ss.decodeMCU([maxComponents][]int16{}, [maxComponents]int{}, mx, 0, true); err != nil {
			return err
		}
		if err := ss.afterMCU(m); err != nil {
			return err
		}
	}
	return nil
}

func (ss *streamScan) finish() error {
	ss.ent.reset()
	p := &parser{s: ss.s}
	for {
		marker, n, err := p.next()
		if err != nil {
			return err
		}
		switch {
		case marker == eoiMarker:
			return nil
		case marker == sosMarker:
			return UnsupportedError("several scans in a sequential file")
		case marker >= sof0Marker && marker <= sof2Marker:
			return FormatError("multiple SOF markers")
		case marker == dhtMarker, marker == dqtMarker, marker == driMarker,
			app0Marker <= marker && marker <= app15Marker, marker == comMarker:
			err = ss.s.skip(n)
		case marker < 0xc0:
			err = FormatError("unknown marker")
		default:
			err = UnsupportedError("unknown marker")
		}
		if err != nil {
			return err
		}
	}
}

func bandRowsFor(f *frame, g *geometry) int {
	perRow := 0
	for c := 0; c < f.nComp; c++ {
		perRow += g.mxx * f.comp[c].h * f.comp[c].v
	}
	return min(max(bandTargetBlocks/max(perRow, 1), 1), maxBandRows)
}

func (d *Decoder) decodeStreaming(ctx context.Context, shift int, mode colorMode) (*image.NRGBA, error) {
	g := d.f.geometry(shift, shift)
	dst := image.NewNRGBA(image.Rect(0, 0, g.outW, g.outH))
	var rec *indexRecorder
	if d.index.Load() == nil && d.CanRegion() {
		rec = newIndexRecorder(d, g)
	}
	if err := d.runStreaming(ctx, g, dst, rec, mode); err != nil {
		return nil, err
	}
	if rec != nil {
		d.index.CompareAndSwap(nil, rec.finish())
	}
	return dst, nil
}

func (d *Decoder) ensureIndex(ctx context.Context) (*checkpointIndex, error) {
	if ix := d.index.Load(); ix != nil {
		return ix, nil
	}
	d.indexMu.Lock()
	defer d.indexMu.Unlock()
	if ix := d.index.Load(); ix != nil {
		return ix, nil
	}
	g := d.f.geometry(MaxShift, MaxShift)
	rec := newIndexRecorder(d, g)
	if err := d.runStreaming(ctx, g, nil, rec, d.mode); err != nil {
		return nil, err
	}
	ix := rec.finish()
	d.index.Store(ix)
	return ix, nil
}

func (d *Decoder) BuildIndex(ctx context.Context) error {
	if d.closed.Load() {
		return ErrClosed
	}
	if !d.CanRegion() {
		return UnsupportedError("random access to this file")
	}
	_, err := d.ensureIndex(ctx)
	return err
}

func (d *Decoder) runStreaming(ctx context.Context, g *geometry, dst *image.NRGBA, rec *indexRecorder, mode colorMode) error {
	s := newStream(ctx, d.reader, d.size)
	defer s.release()
	s.seek(d.scanOffset)
	ss, err := newStreamScan(d, g, s)
	if err != nil {
		return err
	}
	if dst == nil {
		return d.runIndexOnly(ctx, ss, rec)
	}
	quant := dequantTables(d.f)
	r := newRecon(d.f, mode, g, &quant)
	workers := d.workerCount()
	parallel := workers > 1 && g.myy > 1 && (d.opt.Workers > 0 || g.outW*g.outH >= parallelPixels)
	if parallel {
		return ss.runParallel(ctx, r, dst, rec, workers)
	}
	return ss.runSerial(ctx, r, dst, rec)
}

func (d *Decoder) runIndexOnly(ctx context.Context, ss *streamScan, rec *indexRecorder) error {
	for my := 0; my < ss.g.myy; my++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := ss.skipRow(my, rec); err != nil {
			return err
		}
	}
	return ss.finish()
}

func (ss *streamScan) runSerial(ctx context.Context, r *recon, dst *image.NRGBA, rec *indexRecorder) error {
	g := ss.g
	b := newBandBuf(ss.f, g, 1, g.mxx)
	sc := new(scratch)
	rect := image.Rect(0, 0, g.outW, g.outH)
	strides := b.strides(ss.f, g.mxx)
	for my := 0; my < g.myy; my++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		b.clear(ss.f)
		if err := ss.decodeRow(my, b, 0, rec); err != nil {
			return err
		}
		r.row(sc, b.view(ss.f, g, g.mxx, 0), strides, g.mxx, 0, my, dst, rect)
	}
	return ss.finish()
}

type panicBox struct {
	once  sync.Once
	value any
}

func (p *panicBox) capture() {
	if v := recover(); v != nil {
		p.once.Do(func() { p.value = v })
	}
}

func (ss *streamScan) runParallel(ctx context.Context, r *recon, dst *image.NRGBA, rec *indexRecorder, workers int) error {
	g := ss.g
	bandRows := bandRowsFor(ss.f, g)
	bands := (g.myy + bandRows - 1) / bandRows
	workers = min(workers, bands)
	buffers := min(workers+2, bands)
	jobs := make(chan *bandBuf, buffers)
	free := make(chan *bandBuf, buffers)
	for i := 0; i < buffers; i++ {
		free <- newBandBuf(ss.f, g, bandRows, g.mxx)
	}
	rect := image.Rect(0, 0, g.outW, g.outH)
	strides := (&bandBuf{}).strides(ss.f, g.mxx)
	var (
		wg     sync.WaitGroup
		failed atomic.Bool
		box    panicBox
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := new(scratch)
			for b := range jobs {
				if !failed.Load() {
					func() {
						defer box.capture()
						for j := 0; j < b.count; j++ {
							r.row(sc, b.view(ss.f, g, g.mxx, j), strides, g.mxx, 0, b.my0+j, dst, rect)
						}
					}()
				}
				free <- b
			}
		}()
	}
	var err error
	my := 0
	for my < g.myy && err == nil {
		var b *bandBuf
		select {
		case b = <-free:
		case <-ctx.Done():
			err = ctx.Err()
		}
		if err != nil {
			break
		}
		b.clear(ss.f)
		b.my0 = my
		b.count = 0
		for j := 0; j < bandRows && my < g.myy; j++ {
			if err = ctx.Err(); err != nil {
				break
			}
			if err = ss.decodeRow(my, b, j, rec); err != nil {
				break
			}
			b.count++
			my++
		}
		if err != nil {
			break
		}
		jobs <- b
	}
	if err == nil {
		err = ss.finish()
	}
	if err != nil {
		failed.Store(true)
	}
	close(jobs)
	wg.Wait()
	if box.value != nil {
		panic(box.value)
	}
	return err
}
