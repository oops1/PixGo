package imaging

import (
	"container/list"
	"context"
	"fmt"
	"image"
	"io"
	"sort"
	"sync"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/par"
	"github.com/oops1/PixGo/internal/render"
)

const (
	TileSide          = 256
	DefaultTileBudget = 96 << 20
	MaxTileWorkers    = 3

	prefetchRing = 1
	tierVisible  = 0
	tierMargin   = 1
	tierPrefetch = 2

	maxWindowPixels    = 16 << 20
	windowReserveBytes = 64 << 20
	maxArrived         = 2048
	maxFailed          = 8192
	smallBuildCells    = 4
)

type TileOptions struct {
	Budget  int64
	Workers int
}

type TileStats struct {
	Tiles        int
	TileBytes    int64
	Inflight     int
	Pending      int
	Failed       int
	Decoded      uint64
	WindowBuilds uint64
	WindowPatch  uint64
	WindowHits   uint64
	Requests     uint64
	Placeholders uint64
	Closed       bool
}

type tileKey struct{ shift, tx, ty int }

type tileEntry struct {
	key    tileKey
	img    *image.NRGBA
	opaque bool
	elem   *list.Element
}

type wantItem struct {
	key  tileKey
	tier int
	dist int64
}

type windowState struct {
	img         *image.NRGBA
	buf         []byte
	valid       bool
	shift       int
	rect        image.Rectangle
	tx0, ty0    int
	cols, rows  int
	cellOpaque  []bool
	translucent int
}

type buildPlan struct {
	shift      int
	rect       image.Rectangle
	tx0, ty0   int
	cols, rows int
	cells      []*tileEntry
}

type TileManager struct {
	reader     codec.TileReader
	closer     io.Closer
	levels     []*image.NRGBA
	base       int
	width      int
	height     int
	minShift   int
	budget     int64
	workers    int
	baseOpaque bool

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	closed   bool
	tiles    map[tileKey]*tileEntry
	lru      *list.List
	used     int64
	failed   map[tileKey]struct{}
	inflight map[tileKey]context.CancelFunc
	pending  []wantItem
	active   int
	wg       sync.WaitGroup
	arrived  []tileKey
	overflow bool
	notified bool
	onChange func()

	haveView  bool
	viewShift int
	viewNeed  image.Rectangle
	viewRect  image.Rectangle
	stats     TileStats

	win windowState
}

func NewTileManager(reader codec.TileReader, levels []*image.NRGBA, baseShift, width, height int, opt TileOptions, closer io.Closer) *TileManager {
	if opt.Budget <= 0 {
		opt.Budget = DefaultTileBudget
	}
	opt.Workers = min(opt.Workers, MaxTileWorkers)
	if opt.Workers < 0 {
		opt.Workers = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &TileManager{
		reader:   reader,
		closer:   closer,
		levels:   levels,
		base:     baseShift,
		width:    width,
		height:   height,
		minShift: max(reader.MinShift(), 0),
		budget:   opt.Budget,
		workers:  opt.Workers,
		ctx:      ctx,
		cancel:   cancel,
		tiles:    map[tileKey]*tileEntry{},
		lru:      list.New(),
		failed:   map[tileKey]struct{}{},
		inflight: map[tileKey]context.CancelFunc{},
	}
	if len(levels) > 0 {
		m.baseOpaque = render.IsOpaque(levels[0])
	}
	return m
}

func (m *TileManager) MaxBytes() int64 { return m.budget + windowReserveBytes }

func (m *TileManager) SetOnChange(fn func()) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

func (m *TileManager) Stats() TileStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.stats
	s.Tiles = len(m.tiles)
	s.TileBytes = m.used
	s.Inflight = len(m.inflight)
	s.Pending = len(m.pending)
	s.Failed = len(m.failed)
	s.Closed = m.closed
	return s
}

func (m *TileManager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.cancel()
	m.pending = nil
	m.mu.Unlock()
	m.wg.Wait()
	err := m.reader.Close()
	if m.closer != nil {
		if cerr := m.closer.Close(); err == nil {
			err = cerr
		}
	}
	m.mu.Lock()
	m.tiles = map[tileKey]*tileEntry{}
	m.lru.Init()
	m.used = 0
	m.arrived = nil
	m.failed = map[tileKey]struct{}{}
	m.mu.Unlock()
	return err
}

func tileRange(r image.Rectangle) (tx0, ty0, tx1, ty1 int) {
	return r.Min.X / TileSide, r.Min.Y / TileSide, (r.Max.X + TileSide - 1) / TileSide, (r.Max.Y + TileSide - 1) / TileSide
}

func tileRect(tx, ty, lw, lh int) image.Rectangle {
	return image.Rect(tx*TileSide, ty*TileSide, min((tx+1)*TileSide, lw), min((ty+1)*TileSide, lh))
}

func alignOut(r image.Rectangle, lw, lh int) image.Rectangle {
	tx0, ty0, tx1, ty1 := tileRange(r)
	return image.Rect(tx0*TileSide, ty0*TileSide, tx1*TileSide, ty1*TileSide).Intersect(image.Rect(0, 0, lw, lh))
}

func pixelsOf(r image.Rectangle) int { return r.Dx() * r.Dy() }

func (m *TileManager) chooseRect(need image.Rectangle, lw, lh int) (image.Rectangle, bool) {
	level := image.Rect(0, 0, lw, lh)
	wide := image.Rect(need.Min.X-TileSide, need.Min.Y-TileSide, need.Max.X+TileSide, need.Max.Y+TileSide).Intersect(level)
	for _, r := range []image.Rectangle{alignOut(wide, lw, lh), alignOut(need, lw, lh), need} {
		if pixelsOf(r) <= maxWindowPixels {
			return r, true
		}
	}
	return image.Rectangle{}, false
}

func (ws *windowState) window() render.Window {
	return render.Window{Img: ws.img, Shift: ws.shift, X: ws.rect.Min.X, Y: ws.rect.Min.Y}
}

func (m *TileManager) Window(shift int, need image.Rectangle) render.Window {
	m.mu.Lock()
	if m.closed {
		m.win = windowState{}
		m.mu.Unlock()
		return render.Window{}
	}
	if shift < m.minShift || shift >= m.base || shift < 0 {
		m.mu.Unlock()
		return render.Window{}
	}
	lw, lh := codec.LevelSize(m.width, m.height, shift)
	need = need.Intersect(image.Rect(0, 0, lw, lh))
	if need.Empty() {
		m.mu.Unlock()
		return render.Window{}
	}
	m.notified = false
	m.stats.Requests++
	if m.missingLocked(shift, need) {
		m.stats.Placeholders++
	}
	ws := &m.win
	if ws.valid && ws.shift == shift && need.In(ws.rect) && !m.overflow {
		m.noteViewLocked(shift, need, ws.rect)
		m.touchLocked(shift, need)
		m.patchLocked()
		m.stats.WindowHits++
		w := ws.window()
		m.mu.Unlock()
		return w
	}
	rect, ok := m.chooseRect(need, lw, lh)
	if !ok {
		m.mu.Unlock()
		return render.Window{}
	}
	m.noteViewLocked(shift, need, rect)
	plan := m.planLocked(shift, rect, lw, lh)
	m.stats.WindowBuilds++
	m.mu.Unlock()
	m.build(plan, lw, lh)
	return ws.window()
}

func (m *TileManager) missingLocked(shift int, need image.Rectangle) bool {
	tx0, ty0, tx1, ty1 := tileRange(need)
	for ty := ty0; ty < ty1; ty++ {
		for tx := tx0; tx < tx1; tx++ {
			if _, ok := m.tiles[tileKey{shift, tx, ty}]; !ok {
				return true
			}
		}
	}
	return false
}

func (m *TileManager) noteViewLocked(shift int, need, rect image.Rectangle) {
	if m.haveView && m.viewShift == shift && m.viewNeed == need && m.viewRect == rect {
		return
	}
	m.haveView, m.viewShift, m.viewNeed, m.viewRect = true, shift, need, rect
	lw, lh := codec.LevelSize(m.width, m.height, shift)
	tx0, ty0, tx1, ty1 := m.prefetchRange(rect, lw, lh)
	for k, cancel := range m.inflight {
		if k.shift != shift || k.tx < tx0 || k.tx >= tx1 || k.ty < ty0 || k.ty >= ty1 {
			cancel()
		}
	}
	m.rebuildPendingLocked()
}

func (m *TileManager) prefetchRange(rect image.Rectangle, lw, lh int) (tx0, ty0, tx1, ty1 int) {
	tx0, ty0, tx1, ty1 = tileRange(rect)
	maxX, maxY := (lw+TileSide-1)/TileSide, (lh+TileSide-1)/TileSide
	return max(tx0-prefetchRing, 0), max(ty0-prefetchRing, 0), min(tx1+prefetchRing, maxX), min(ty1+prefetchRing, maxY)
}

func (m *TileManager) rebuildPendingLocked() {
	shift, need, rect := m.viewShift, m.viewNeed, m.viewRect
	lw, lh := codec.LevelSize(m.width, m.height, shift)
	tx0, ty0, tx1, ty1 := m.prefetchRange(rect, lw, lh)
	wx0, wy0, wx1, wy1 := tileRange(rect)
	cx, cy := (need.Min.X+need.Max.X)/2, (need.Min.Y+need.Max.Y)/2
	m.pending = m.pending[:0]
	for ty := ty0; ty < ty1; ty++ {
		for tx := tx0; tx < tx1; tx++ {
			key := tileKey{shift, tx, ty}
			if _, have := m.tiles[key]; have {
				continue
			}
			if _, bad := m.failed[key]; bad {
				continue
			}
			if _, busy := m.inflight[key]; busy {
				continue
			}
			tr := tileRect(tx, ty, lw, lh)
			tier := tierPrefetch
			switch {
			case tr.Overlaps(need):
				tier = tierVisible
			case tx >= wx0 && tx < wx1 && ty >= wy0 && ty < wy1:
				tier = tierMargin
			}
			dx, dy := int64((tr.Min.X+tr.Max.X)/2-cx), int64((tr.Min.Y+tr.Max.Y)/2-cy)
			m.pending = append(m.pending, wantItem{key: key, tier: tier, dist: dx*dx + dy*dy})
		}
	}
	sort.Slice(m.pending, func(i, j int) bool {
		a, b := m.pending[i], m.pending[j]
		if a.tier != b.tier {
			return a.tier < b.tier
		}
		return a.dist < b.dist
	})
	m.startWorkersLocked()
}

func (m *TileManager) touchLocked(shift int, need image.Rectangle) {
	tx0, ty0, tx1, ty1 := tileRange(need)
	for ty := ty0; ty < ty1; ty++ {
		for tx := tx0; tx < tx1; tx++ {
			if t, ok := m.tiles[tileKey{shift, tx, ty}]; ok {
				m.lru.MoveToFront(t.elem)
			}
		}
	}
}

func (m *TileManager) workerLimit() int {
	if m.workers > 0 {
		return m.workers
	}
	return min(par.Share(2), MaxTileWorkers)
}

func (m *TileManager) startWorkersLocked() {
	limit := m.workerLimit()
	for m.active < limit && m.active < len(m.pending) {
		m.active++
		m.wg.Add(1)
		go m.work()
	}
}

func (m *TileManager) planLocked(shift int, rect image.Rectangle, lw, lh int) buildPlan {
	tx0, ty0, tx1, ty1 := tileRange(rect)
	p := buildPlan{shift: shift, rect: rect, tx0: tx0, ty0: ty0, cols: tx1 - tx0, rows: ty1 - ty0}
	p.cells = make([]*tileEntry, p.cols*p.rows)
	for ty := ty0; ty < ty1; ty++ {
		for tx := tx0; tx < tx1; tx++ {
			if t, ok := m.tiles[tileKey{shift, tx, ty}]; ok {
				m.lru.MoveToFront(t.elem)
				p.cells[(ty-ty0)*p.cols+(tx-tx0)] = t
			}
		}
	}
	m.arrived = m.arrived[:0]
	m.overflow = false
	return p
}

func (m *TileManager) build(p buildPlan, lw, lh int) {
	ws := &m.win
	w, h := p.rect.Dx(), p.rect.Dy()
	size := w * h * 4
	if cap(ws.buf) < size || cap(ws.buf) > 4*size+windowReserveBytes {
		ws.buf = make([]byte, size)
	}
	ws.buf = ws.buf[:size]
	if ws.img == nil {
		ws.img = new(image.NRGBA)
	}
	ws.img.Pix, ws.img.Stride, ws.img.Rect = ws.buf, w*4, image.Rect(0, 0, w, h)
	ws.shift, ws.rect, ws.tx0, ws.ty0, ws.cols, ws.rows = p.shift, p.rect, p.tx0, p.ty0, p.cols, p.rows
	if cap(ws.cellOpaque) < len(p.cells) {
		ws.cellOpaque = make([]bool, len(p.cells))
	}
	ws.cellOpaque = ws.cellOpaque[:len(p.cells)]
	factor := uint(m.base - p.shift)
	coarse := m.levels[0]
	org := p.rect.Min
	workers := par.Count()
	if len(p.cells) <= smallBuildCells {
		workers = 1
	}
	par.Parallel(len(p.cells), workers, func(i int) {
		tx, ty := p.tx0+i%p.cols, p.ty0+i/p.cols
		region := tileRect(tx, ty, lw, lh).Intersect(p.rect)
		if t := p.cells[i]; t != nil {
			copyTile(ws.img, org, region, t, tileRect(tx, ty, lw, lh))
			ws.cellOpaque[i] = t.opaque
			return
		}
		fillFromCoarse(ws.img, org, region, coarse, factor, m.baseOpaque)
		ws.cellOpaque[i] = m.baseOpaque
	})
	ws.valid = true
	m.markWindowOpacity()
}

func (m *TileManager) markWindowOpacity() {
	ws := &m.win
	ws.translucent = 0
	for _, o := range ws.cellOpaque {
		if !o {
			ws.translucent++
		}
	}
	if ws.translucent == 0 {
		render.MarkOpaque(ws.img)
	} else {
		render.ForgetOpacity(ws.img)
	}
}

func copyTile(dst *image.NRGBA, org image.Point, region image.Rectangle, t *tileEntry, full image.Rectangle) {
	if region.Empty() {
		return
	}
	n := region.Dx() * 4
	for y := region.Min.Y; y < region.Max.Y; y++ {
		so := t.img.PixOffset(t.img.Rect.Min.X+region.Min.X-full.Min.X, t.img.Rect.Min.Y+y-full.Min.Y)
		do := dst.PixOffset(region.Min.X-org.X, y-org.Y)
		copy(dst.Pix[do:do+n], t.img.Pix[so:so+n])
	}
}

func (m *TileManager) patchLocked() {
	ws := &m.win
	if len(m.arrived) == 0 {
		return
	}
	lw, lh := codec.LevelSize(m.width, m.height, ws.shift)
	touched := false
	for _, key := range m.arrived {
		if key.shift != ws.shift || key.tx < ws.tx0 || key.tx >= ws.tx0+ws.cols || key.ty < ws.ty0 || key.ty >= ws.ty0+ws.rows {
			continue
		}
		t, ok := m.tiles[key]
		if !ok {
			continue
		}
		full := tileRect(key.tx, key.ty, lw, lh)
		copyTile(ws.img, ws.rect.Min, full.Intersect(ws.rect), t, full)
		ws.cellOpaque[(key.ty-ws.ty0)*ws.cols+(key.tx-ws.tx0)] = t.opaque
		touched = true
	}
	m.arrived = m.arrived[:0]
	if touched {
		m.stats.WindowPatch++
		m.markWindowOpacity()
	}
}

func (m *TileManager) work() {
	defer m.wg.Done()
	for {
		key, ctx, ok := m.next()
		if !ok {
			return
		}
		img, err := m.decode(ctx, key)
		m.deliver(key, ctx, img, err)
	}
}

func (m *TileManager) next() (tileKey, context.Context, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for {
		if m.closed || len(m.pending) == 0 || m.active > m.workerLimit() {
			m.active--
			return tileKey{}, nil, false
		}
		it := m.pending[0]
		m.pending = m.pending[1:]
		if _, have := m.tiles[it.key]; have {
			continue
		}
		if _, busy := m.inflight[it.key]; busy {
			continue
		}
		if _, bad := m.failed[it.key]; bad {
			continue
		}
		ctx, cancel := context.WithCancel(m.ctx)
		m.inflight[it.key] = cancel
		return it.key, ctx, true
	}
}

func (m *TileManager) decode(ctx context.Context, key tileKey) (img *image.NRGBA, err error) {
	defer func() {
		if v := recover(); v != nil {
			img, err = nil, fmt.Errorf("imaging: tile decoder panic: %v", v)
		}
	}()
	lw, lh := codec.LevelSize(m.width, m.height, key.shift)
	r := tileRect(key.tx, key.ty, lw, lh)
	if r.Empty() {
		return nil, codec.ErrCorrupt
	}
	img, err = m.reader.Region(ctx, key.shift, r)
	if err != nil {
		return nil, err
	}
	if img == nil || img.Rect.Dx() != r.Dx() || img.Rect.Dy() != r.Dy() {
		return nil, codec.ErrCorrupt
	}
	return img, nil
}

func fullyOpaque(img *image.NRGBA) bool {
	b := img.Rect
	w4 := b.Dx() * 4
	for y := b.Min.Y; y < b.Max.Y; y++ {
		o := img.PixOffset(b.Min.X, y)
		row := img.Pix[o : o+w4]
		for i := 3; i < len(row); i += 4 {
			if row[i] != 255 {
				return false
			}
		}
	}
	return true
}

func (m *TileManager) deliver(key tileKey, ctx context.Context, img *image.NRGBA, err error) {
	var entry *tileEntry
	if err == nil {
		entry = &tileEntry{key: key, img: img, opaque: fullyOpaque(img)}
	}
	m.mu.Lock()
	cancel := m.inflight[key]
	delete(m.inflight, key)
	var notify func()
	switch {
	case m.closed:
	case entry != nil:
		m.storeLocked(entry)
		m.stats.Decoded++
		if !m.notified && m.onChange != nil {
			m.notified = true
			notify = m.onChange
		}
	case ctx.Err() == nil:
		if len(m.failed) >= maxFailed {
			m.failed = map[tileKey]struct{}{}
		}
		m.failed[key] = struct{}{}
	case m.haveView:
		m.rebuildPendingLocked()
	}
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if notify != nil {
		notify()
	}
}

func (m *TileManager) storeLocked(t *tileEntry) {
	if old, ok := m.tiles[t.key]; ok {
		m.lru.Remove(old.elem)
		m.used -= int64(len(old.img.Pix))
	}
	t.elem = m.lru.PushFront(t)
	m.tiles[t.key] = t
	m.used += int64(len(t.img.Pix))
	for m.used > m.budget {
		back := m.lru.Back()
		if back == nil || back == t.elem {
			break
		}
		victim := back.Value.(*tileEntry)
		m.lru.Remove(back)
		delete(m.tiles, victim.key)
		m.used -= int64(len(victim.img.Pix))
	}
	if len(m.arrived) >= maxArrived {
		m.overflow = true
		return
	}
	m.arrived = append(m.arrived, t.key)
}
