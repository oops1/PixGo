package imaging

import (
	"errors"
	"fmt"
	"math/bits"
	"os"

	"github.com/oops1/PixGo/internal/codec"
)

const (
	previewMinPixels  = 4_000_000
	maxPreviewScale   = 8
	overviewMaxPixels = 16_000_000
	maxOverviewShift  = 12
)

func (l *Loader) SetViewportHint(w, h int) {
	if w > 0 && h > 0 {
		l.viewW.Store(int32(min(w, 1<<20)))
		l.viewH.Store(int32(min(h, 1<<20)))
	}
}

func (l *Loader) SetTileBudget(bytes int64) {
	if bytes > 0 {
		l.tileBudget.Store(bytes)
	}
}

func (l *Loader) jobPrio(j *job) Priority { return j.currentPrio() }

func (l *Loader) releaseHeavyUnless(reqs []Request) {
	hp := l.heavy.Load()
	if hp == nil {
		return
	}
	for _, r := range reqs {
		if r.Prio == PrioCurrent && r.Path == *hp {
			return
		}
	}
	l.heavy.Store(nil)
}

func (l *Loader) noteHeavy(j *job) {
	if j.currentPrio() == PrioCurrent {
		path := j.path
		l.heavy.Store(&path)
	}
}

func (l *Loader) heavyElsewhere(j *job) bool {
	hp := l.heavy.Load()
	return hp != nil && *hp != j.path
}

func (l *Loader) reserve(j *job, bytes int64) (*Reservation, bool) {
	if l.jobPrio(j) == PrioCurrent {
		r, err := l.cache.Reserve(j.ctx, bytes, j.currentPrio)
		return r, err == nil
	}
	if !l.heavyElsewhere(j) {
		if r, ok := l.cache.TryReserve(bytes); ok {
			return r, true
		}
	}
	if j.ctx.Err() == nil {
		l.post(func() { l.apply(j, stateSkipped, codec.Info{}, nil, nil) })
	}
	return nil, false
}

func (l *Loader) run(j *job) {
	defer func() {
		if v := recover(); v != nil {
			l.publish(j, StateFailed, codec.Info{}, nil, fmt.Errorf("imaging: panic while loading %s: %v", j.path, v))
		}
	}()
	if j.ctx.Err() != nil {
		return
	}
	st, err := os.Stat(j.path)
	if err == nil && !st.Mode().IsRegular() {
		err = errNotRegularFile
	}
	if err != nil {
		l.publish(j, StateFailed, codec.Info{}, nil, err)
		return
	}
	key := Key{Path: j.path, Size: st.Size(), ModTime: st.ModTime()}
	stamp := l.colorStamp()
	if d, ok := l.cache.Get(key); ok && !d.Released() && d.ColorStamp == stamp {
		if d.Detail != nil {
			l.noteHeavy(j)
		}
		l.deliver(j, d, false)
		return
	} else if ok && d.ColorStamp != stamp {
		l.cache.Remove(j.path)
	}
	src, file, err := openSource(j.path)
	if err != nil {
		l.publish(j, StateFailed, codec.Info{}, nil, err)
		return
	}
	handedOver := false
	defer func() {
		if !handedOver {
			file.Close()
		}
	}()
	dec, info, err := l.reg.Probe(j.ctx, src, j.lim)
	oversize := false
	if err != nil {
		if dec == nil || !errors.Is(err, codec.ErrTooLarge) || info.Width <= 0 || info.Height <= 0 {
			l.publish(j, StateFailed, codec.Info{}, nil, err)
			return
		}
		oversize = true
	}
	l.publish(j, StateProbed, info, nil, nil)
	lim := j.lim.WithDefaults()
	if td, ok := dec.(codec.TiledDecoder); ok && (oversize || int64(info.Width)*int64(info.Height) > lim.TiledAbove) {
		done, adopted := l.runTiled(j, td, src, file, info, key, lim, oversize)
		handedOver = adopted
		if done {
			return
		}
	}
	if oversize {
		l.publish(j, StateFailed, info, nil, err)
		return
	}
	res, ok := l.reserve(j, directEstimate(info, j.vw, j.vh)+l.deepReserve(info))
	if !ok {
		return
	}
	defer res.Release()
	l.decodeDirect(j, dec, src, info, key, stamp)
}

func (l *Loader) runTiled(j *job, td codec.TiledDecoder, src *codec.Source, file *sourceFile, info codec.Info, key Key, lim codec.Limits, oversize bool) (done, adopted bool) {
	res, ok := l.reserve(j, l.tiledEstimate(info))
	if !ok {
		return true, false
	}
	defer res.Release()
	d, err := l.openTiled(j, td, src, file, info, key, lim, l.colorStamp())
	switch {
	case err == nil:
		if d.Detail != nil {
			l.noteHeavy(j)
		}
		l.deliver(j, d, true)
		return true, d.Detail != nil
	case j.ctx.Err() != nil:
		return true, false
	case oversize:
		l.publish(j, StateFailed, info, nil, err)
		return true, false
	}
	return false, false
}

func (l *Loader) decodeDirect(j *job, dec codec.Decoder, src *codec.Source, info codec.Info, key Key, stamp uint64) {
	opts := codec.Options{Limits: j.lim, Frames: codec.FirstFrame, Deep: l.wantDeep()}
	if info.Animated {
		opts.Frames = codec.AllFrames
	}
	if info.Vector {
		opts.VectorW, opts.VectorH = j.vw, j.vh
	}
	if scale := l.previewScale(j, info); scale > 1 {
		hinted := opts
		hinted.Scale = scale
		img, err := l.reg.Decode(j.ctx, dec, src, hinted)
		if j.ctx.Err() != nil {
			return
		}
		if err == nil {
			if img.Scale <= 1 {
				l.finish(j, key, info, img, stamp)
				return
			}
			l.publishPreview(j, key, info, img, stamp)
			if j.ctx.Err() != nil {
				return
			}
		}
	}
	img, err := l.reg.Decode(j.ctx, dec, src, opts)
	if err != nil {
		l.publish(j, StateFailed, info, nil, err)
		return
	}
	l.finish(j, key, info, img, stamp)
}

func (l *Loader) finish(j *job, key Key, info codec.Info, img *codec.Image, stamp uint64) {
	if j.ctx.Err() != nil {
		return
	}
	rep, err := l.applyColor(img)
	if err != nil {
		l.publish(j, StateFailed, info, nil, err)
		return
	}
	decoded := img.Info
	if decoded.Format == "" {
		decoded = info
	}
	d := &Decoded{
		Key:     key,
		Info:    decoded,
		Levels:  BuildPyramid(img.Frames[0].Pix, 0),
		Frames:  img.Frames,
		Loop:    img.Loop,
		Preview: info.Preview,

		Color:      rep,
		ColorStamp: stamp,
	}
	l.deliver(j, d, true)
}

func (l *Loader) previewScale(j *job, info codec.Info) int {
	if info.Animated || info.Vector || info.Frames > 1 {
		return 0
	}
	w, h := info.Width, info.Height
	if w <= 0 || h <= 0 || int64(w)*int64(h) < previewMinPixels || l.jobPrio(j) != PrioCurrent {
		return 0
	}
	vw, vh := float64(l.viewW.Load()), float64(l.viewH.Load())
	fw, fh := float64(w), float64(h)
	fit := max(min(vw/fw, vh/fh), min(vw/fh, vh/fw))
	for scale := maxPreviewScale; scale >= 2; scale /= 2 {
		if 1/float64(scale) >= fit {
			return scale
		}
	}
	return 0
}

func (l *Loader) publishPreview(j *job, key Key, info codec.Info, img *codec.Image, stamp uint64) {
	if img.Scale <= 1 || img.Scale&(img.Scale-1) != 0 {
		return
	}
	rep, err := l.applyColor(img)
	if err != nil {
		return
	}
	pix := img.First()
	if pix == nil {
		return
	}
	shift := bits.TrailingZeros(uint(img.Scale))
	if lw, lh := codec.LevelSize(info.Width, info.Height, shift); pix.Rect.Dx() != lw || pix.Rect.Dy() != lh {
		return
	}
	reduced := img.Info
	if reduced.Format == "" {
		reduced = info
	}
	reduced.Width, reduced.Height = info.Width, info.Height
	d := &Decoded{
		Key:     key,
		Info:    reduced,
		Levels:  BuildPyramid(pix, 0),
		Frames:  img.Frames,
		Preview: info.Preview,
		Shift:   shift,
		Reduced: true,

		Color:      rep,
		ColorStamp: stamp,
	}
	l.publish(j, StatePreview, d.Info, d, nil)
}

func overviewShift(w, h, minShift int) int {
	n := max(minShift, 0)
	for n < maxOverviewShift {
		lw, lh := codec.LevelSize(w, h, n)
		if int64(lw)*int64(lh) <= overviewMaxPixels {
			break
		}
		n++
	}
	return n
}

func (l *Loader) openTiled(j *job, td codec.TiledDecoder, src *codec.Source, file *sourceFile, info codec.Info, key Key, lim codec.Limits, stamp uint64) (*Decoded, error) {
	tr, err := td.OpenTiled(j.ctx, src.R, src.Size, lim)
	if err != nil {
		return nil, err
	}
	tr, rep := l.wrapTiles(tr)
	adopted := false
	defer func() {
		if !adopted {
			tr.Close()
		}
	}()
	w, h := info.Width, info.Height
	if ti := tr.Info(); ti.Width > 0 && ti.Height > 0 && (ti.Width != w || ti.Height != h) {
		return nil, codec.ErrCorrupt
	}
	shift := overviewShift(w, h, tr.MinShift())
	ov, err := tr.Overview(j.ctx, shift)
	if err != nil {
		return nil, err
	}
	if lw, lh := codec.LevelSize(w, h, shift); ov == nil || ov.Rect.Dx() != lw || ov.Rect.Dy() != lh {
		return nil, codec.ErrCorrupt
	}
	if err := j.ctx.Err(); err != nil {
		return nil, err
	}
	levels := BuildPyramid(ov, 0)
	d := &Decoded{
		Color:      rep,
		ColorStamp: stamp,
		Key:        key,
		Info:       info,
		Levels:     levels,
		Frames:     []codec.Frame{{Pix: levels[0]}},
		Preview:    info.Preview,
		Shift:      shift,
	}
	if shift == 0 {
		return d, nil
	}
	adopted = true
	opt := TileOptions{Budget: l.tileBudget.Load(), Workers: l.tileWorkerCount()}
	d.Detail = NewTileManager(tr, levels, shift, w, h, opt, file)
	return d, nil
}

func (l *Loader) deliver(j *job, d *Decoded, put bool) {
	l.cache.hold(d.Key)
	if put {
		l.cache.Put(d)
	}
	if j.ctx.Err() != nil {
		l.cache.letGo(d.Key)
		return
	}
	l.post(func() {
		defer l.cache.letGo(d.Key)
		l.apply(j, StateReady, d.Info, d, nil)
	})
}
