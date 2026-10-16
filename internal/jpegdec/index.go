package jpegdec

import "math"

const (
	maxCheckpoints = 100_000
	maxIndexOffset = math.MaxUint32
	invalidSlot    = math.MaxUint32
)

type checkpointIndex struct {
	every int
	nComp int
	off   []uint32
	bit   []uint8
	dc    []int16
}

func (ix *checkpointIndex) Bytes() int {
	return len(ix.off)*4 + len(ix.bit) + len(ix.dc)*2
}

func (ix *checkpointIndex) slotFor(m int) int {
	slot := min(m/ix.every, len(ix.off)-1)
	for slot > 0 && ix.off[slot] == invalidSlot {
		slot--
	}
	return slot
}

type indexRecorder struct {
	checkpointIndex
}

func newIndexRecorder(d *Decoder, g *geometry) *indexRecorder {
	total := g.mxx * g.myy
	every := d.opt.CheckpointEvery
	if every <= 0 {
		every = DefaultCheckpoints
	}
	every = max(every, (total+maxCheckpoints-1)/maxCheckpoints)
	slots := (total + every - 1) / every
	rec := &indexRecorder{checkpointIndex{
		every: every,
		nComp: d.f.nComp,
		off:   make([]uint32, slots),
		bit:   make([]uint8, slots),
		dc:    make([]int16, slots*d.f.nComp),
	}}
	for i := range rec.off {
		rec.off[i] = invalidSlot
	}
	return rec
}

func (r *indexRecorder) capture(m int, ss *streamScan) {
	slot := m / r.every
	offset, bit, ok := ss.ent.position()
	if !ok || ss.ent.eobRun != 0 || offset >= maxIndexOffset {
		return
	}
	r.off[slot] = uint32(offset)
	r.bit[slot] = bit
	copy(r.dc[slot*r.nComp:(slot+1)*r.nComp], ss.pred[:r.nComp])
}

func (r *indexRecorder) finish() *checkpointIndex {
	ix := r.checkpointIndex
	return &ix
}

func (d *Decoder) IndexBytes() int {
	if ix := d.index.Load(); ix != nil {
		return ix.Bytes()
	}
	return 0
}
