package jpegdec

import (
	"context"
	"image"
	"sync"
	"sync/atomic"
)

type coefStore struct {
	shift   int
	ks      [maxComponents]int
	kk      [maxComponents]int
	coef    [maxComponents][]int16
	nz      [maxComponents][]uint64
	cstride [maxComponents]int
	quant   [maxComponents][blockSize]int32
}

func newCoefStore(f *frame, shift int) *coefStore {
	g := f.geometry(shift, shift)
	st := &coefStore{shift: shift, ks: g.ks, kk: g.kk}
	mxx, myy := f.mcuCounts()
	for c := 0; c < f.nComp; c++ {
		comp := &f.comp[c]
		blocks := mxx * myy * comp.h * comp.v
		st.coef[c] = make([]int16, blocks*st.kk[c])
		if st.ks[c] < 8 && f.progressive {
			st.nz[c] = make([]uint64, blocks)
		}
		st.cstride[c] = mxx * comp.h
	}
	return st
}

func (st *coefStore) bytes() int {
	total := 0
	for c := range st.coef {
		total += len(st.coef[c])*2 + len(st.nz[c])*8
	}
	return total
}

func (d *Decoder) CoefficientBytes() int {
	d.storeMu.RLock()
	defer d.storeMu.RUnlock()
	if d.store == nil {
		return 0
	}
	return d.store.bytes()
}

type blockRef struct {
	blk []int16
	nz  *uint64
	zz  *[blockSize]int8
}

func (b *blockRef) nonzero(zig int32) bool {
	if i := b.zz[zig]; i >= 0 {
		return b.blk[i] != 0
	}
	return b.nz != nil && *b.nz>>uint(zig)&1 != 0
}

func (b *blockRef) store(zig int32, v int16) {
	if i := b.zz[zig]; i >= 0 {
		b.blk[i] = v
	} else if b.nz != nil && v != 0 {
		*b.nz |= 1 << uint(zig)
	}
}

func (b *blockRef) bump(zig int32, delta int32) {
	if i := b.zz[zig]; i >= 0 {
		if b.blk[i] >= 0 {
			b.blk[i] += int16(delta)
		} else {
			b.blk[i] -= int16(delta)
		}
	}
}

type coefScanner struct {
	p   *parser
	f   *frame
	st  *coefStore
	ent entropy
	zz  [maxComponents][blockSize]int8
}

func (cs *coefScanner) refineNonZeroes(b *blockRef, zig, se, nz, delta int32) int32 {
	for ; zig <= se; zig++ {
		if !b.nonzero(zig) {
			if nz == 0 {
				break
			}
			nz--
			continue
		}
		if !cs.ent.bit() {
			continue
		}
		b.bump(zig, delta)
	}
	return zig
}

func (cs *coefScanner) refine(b *blockRef, h *huffTable, ss, se, delta int32) error {
	e := &cs.ent
	if ss == 0 {
		if e.bit() {
			b.blk[0] |= int16(delta)
		}
		return nil
	}
	zig := ss
	if e.eobRun == 0 {
	loop:
		for ; zig <= se; zig++ {
			z := int32(0)
			value, err := e.decodeSymbol(h)
			if err != nil {
				return err
			}
			val0 := value >> 4
			val1 := value & 0x0f
			switch val1 {
			case 0:
				if val0 != 0x0f {
					e.eobRun = uint16(1) << val0
					if val0 != 0 {
						e.eobRun |= uint16(e.takeBits(int(val0)))
					}
					break loop
				}
			case 1:
				z = delta
				if !e.bit() {
					z = -z
				}
			default:
				return FormatError("unexpected Huffman code")
			}
			zig = cs.refineNonZeroes(b, zig, se, int32(val0), delta)
			if zig > se {
				return FormatError("too many coefficients")
			}
			if z != 0 {
				if i := b.zz[zig]; i >= 0 {
					b.blk[i] = int16(z)
				} else if b.nz != nil {
					*b.nz |= 1 << uint(zig)
				}
			}
		}
	}
	if e.eobRun > 0 {
		e.eobRun--
		cs.refineNonZeroes(b, zig, se, -1, delta)
	}
	return nil
}

func (cs *coefScanner) decodeBlock(b *blockRef, sh *scanHeader, i int, pred *int32) error {
	e := &cs.ent
	f := cs.f
	if sh.ah != 0 {
		return cs.refine(b, &f.huff[acTable][sh.comp[i].ta], sh.ss, sh.se, 1<<sh.al)
	}
	zig := sh.ss
	if zig == 0 {
		zig++
		t, err := e.decodeSymbol(&f.huff[dcTable][sh.comp[i].td])
		if err != nil {
			return err
		}
		if t > 16 {
			return UnsupportedError("excessive DC component")
		}
		*pred += e.receiveExtend(int(t))
		b.blk[0] = int16(*pred << sh.al)
	}
	if zig <= sh.se && e.eobRun > 0 {
		e.eobRun--
		return nil
	}
	h := &f.huff[acTable][sh.comp[i].ta]
	for ; zig <= sh.se; zig++ {
		value, err := e.decodeSymbol(h)
		if err != nil {
			return err
		}
		val0 := int32(value >> 4)
		val1 := int(value & 0x0f)
		if val1 != 0 {
			zig += val0
			if zig > sh.se {
				break
			}
			ac := e.receiveExtend(val1)
			b.store(zig, int16(ac<<sh.al))
			continue
		}
		if val0 != 0x0f {
			e.eobRun = uint16(1) << uint(val0)
			if val0 != 0 {
				e.eobRun |= uint16(e.takeBits(int(val0)))
			}
			e.eobRun--
			break
		}
		zig += 0x0f
	}
	return nil
}

func (cs *coefScanner) checkTables(sh *scanHeader) error {
	f := cs.f
	for i := 0; i < sh.n; i++ {
		needDC := sh.ah == 0 && sh.ss == 0
		needAC := !f.progressive || sh.ss != 0
		if needDC && f.huff[dcTable][sh.comp[i].td].nCodes == 0 {
			return FormatError("uninitialized Huffman table")
		}
		if needAC && f.huff[acTable][sh.comp[i].ta].nCodes == 0 {
			return FormatError("uninitialized Huffman table")
		}
	}
	return nil
}

func (cs *coefScanner) scan(ctx context.Context, sh *scanHeader) error {
	f := cs.f
	if err := cs.checkTables(sh); err != nil {
		return err
	}
	mxx, myy := f.mcuCounts()
	cs.ent.reset()
	var pred [maxComponents]int32
	mcu := 0
	blockCount := 0
	for my := 0; my < myy; my++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for mx := 0; mx < mxx; mx++ {
			for i := 0; i < sh.n; i++ {
				c := int(sh.comp[i].index)
				hi := f.comp[c].h
				vi := f.comp[c].v
				for j := 0; j < hi*vi; j++ {
					var bx, by int
					if sh.n != 1 {
						bx = hi*mx + j%hi
						by = vi*my + j/hi
					} else {
						q := mxx * hi
						bx = blockCount % q
						by = blockCount / q
						blockCount++
						if bx*8 >= f.width || by*8 >= f.height {
							continue
						}
					}
					index := by*mxx*hi + bx
					ref := blockRef{
						blk: cs.st.coef[c][index*cs.st.kk[c] : (index+1)*cs.st.kk[c]],
						zz:  &cs.zz[c],
					}
					if cs.st.nz[c] != nil {
						ref.nz = &cs.st.nz[c][index]
					}
					if err := cs.decodeBlock(&ref, sh, i, &pred[c]); err != nil {
						return err
					}
				}
			}
			if cs.ent.hit {
				if err := cs.ent.check(); err != nil {
					return err
				}
			}
			mcu++
			if f.ri > 0 && mcu%f.ri == 0 && mcu < mxx*myy {
				expected := byte(rst0Marker + ((mcu/f.ri)-1)%8)
				cs.ent.reset()
				var tmp [2]byte
				if err := cs.p.s.readFull(tmp[:]); err != nil {
					return err
				}
				if tmp[0] != 0xff || tmp[1] != expected {
					if err := findRST(cs.p.s, &tmp, expected); err != nil {
						return err
					}
				}
				pred = [maxComponents]int32{}
				cs.ent.eobRun = 0
			}
		}
	}
	return nil
}

func naturalQuant(f *frame, c int) [blockSize]int32 {
	var q [blockSize]int32
	table := &f.quant[f.comp[c].tq]
	for z := 0; z < blockSize; z++ {
		q[unzig[z]] = table[z]
	}
	return q
}

func (d *Decoder) readScans(ctx context.Context, shift int) (*coefStore, error) {
	s := newStream(ctx, d.reader, d.size)
	defer s.release()
	p := newParser(s, false)
	if err := p.readSOI(); err != nil {
		return nil, err
	}
	cs := &coefScanner{p: p, f: &p.f}
	cs.ent.s = s
	storeGeometry := d.f.geometry(shift, shift)
	for c := 0; c < d.f.nComp; c++ {
		cs.zz[c] = zigMap(storeGeometry.ks[c])
	}
	var st *coefStore
	for {
		marker, n, err := p.next()
		if err != nil {
			return nil, err
		}
		if marker == eoiMarker {
			break
		}
		if marker != sosMarker {
			if err = p.segment(marker, n); err != nil {
				return nil, err
			}
			continue
		}
		sh, err := p.processSOS(n)
		if err != nil {
			return nil, err
		}
		if st == nil {
			if p.f.width != d.f.width || p.f.height != d.f.height || p.f.nComp != d.f.nComp {
				return nil, FormatError("file changed while decoding")
			}
			st = newCoefStore(&p.f, shift)
			cs.st = st
		}
		if err = cs.scan(ctx, &sh); err != nil {
			return nil, err
		}
		if !p.f.progressive {
			for i := 0; i < sh.n; i++ {
				c := int(sh.comp[i].index)
				st.quant[c] = naturalQuant(&p.f, c)
			}
		}
	}
	if st == nil {
		return nil, FormatError("missing SOS marker")
	}
	if p.f.progressive {
		for c := 0; c < p.f.nComp; c++ {
			st.quant[c] = naturalQuant(&p.f, c)
		}
	}
	return st, nil
}

func (d *Decoder) Prepare(ctx context.Context, shift int) error {
	if err := d.checkShift(shift); err != nil {
		return err
	}
	if d.streaming {
		return nil
	}
	d.storeMu.RLock()
	ready := d.store != nil && d.store.shift <= shift
	d.storeMu.RUnlock()
	if ready {
		return nil
	}
	d.storeMu.Lock()
	defer d.storeMu.Unlock()
	if d.store != nil && d.store.shift <= shift {
		return nil
	}
	st, err := d.readScans(ctx, shift)
	if err != nil {
		return err
	}
	d.store = st
	return nil
}

func (d *Decoder) decodeCoefficients(ctx context.Context, shift int, mode colorMode) (*image.NRGBA, error) {
	g := d.f.geometry(shift, shift)
	rect := image.Rect(0, 0, g.outW, g.outH)
	for {
		if err := d.Prepare(ctx, shift); err != nil {
			return nil, err
		}
		d.storeMu.RLock()
		if st := d.store; st != nil && st.shift <= shift {
			defer d.storeMu.RUnlock()
			return d.reconstruct(ctx, st, shift, rect, mode)
		}
		d.storeMu.RUnlock()
	}
}

func (d *Decoder) regionCoefficients(ctx context.Context, shift int, rect image.Rectangle, mode colorMode) (*image.NRGBA, error) {
	d.storeMu.RLock()
	defer d.storeMu.RUnlock()
	st := d.store
	if st == nil || st.shift > shift {
		return nil, UnsupportedError("region access finer than the decoded level")
	}
	return d.reconstruct(ctx, st, shift, rect, mode)
}

func (d *Decoder) reconstruct(ctx context.Context, st *coefStore, shift int, rect image.Rectangle, mode colorMode) (*image.NRGBA, error) {
	g := d.f.geometry(shift, st.shift)
	out := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	r := newRecon(d.f, mode, g, &st.quant)
	mx0, mx1 := rect.Min.X/g.mcuW, (rect.Max.X-1)/g.mcuW
	my0, my1 := rect.Min.Y/g.mcuH, (rect.Max.Y-1)/g.mcuH
	cols := mx1 - mx0 + 1
	rows := my1 - my0 + 1
	row := func(sc *scratch, my int) {
		var views [maxComponents][]int16
		for c := 0; c < d.f.nComp; c++ {
			comp := &d.f.comp[c]
			views[c] = st.coef[c][(my*comp.v*st.cstride[c]+mx0*comp.h)*g.kk[c]:]
		}
		r.row(sc, views, st.cstride, cols, mx0, my, out, rect)
	}
	workers := min(d.workerCount(), rows)
	if rect.Dx()*rect.Dy() < parallelPixels && d.opt.Workers <= 0 {
		workers = 1
	}
	if workers <= 1 {
		sc := scratchPool.Get().(*scratch)
		defer scratchPool.Put(sc)
		for my := my0; my <= my1; my++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			row(sc, my)
		}
		return out, nil
	}
	var (
		next   atomic.Int64
		wg     sync.WaitGroup
		failed atomic.Bool
		box    panicBox
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer box.capture()
			sc := scratchPool.Get().(*scratch)
			defer scratchPool.Put(sc)
			for !failed.Load() {
				my := my0 + int(next.Add(1)) - 1
				if my > my1 {
					return
				}
				if ctx.Err() != nil {
					failed.Store(true)
					return
				}
				row(sc, my)
			}
		}()
	}
	wg.Wait()
	if box.value != nil {
		panic(box.value)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
