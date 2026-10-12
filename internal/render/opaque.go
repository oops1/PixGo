package render

import (
	"encoding/binary"
	"image"
	"sync"
	"sync/atomic"
	"weak"
)

const opacityRingSize = 192

type opacityEntry struct {
	ptr    weak.Pointer[image.NRGBA]
	rect   image.Rectangle
	opaque bool
}

var opacityCache struct {
	mu   sync.Mutex
	ring [opacityRingSize]opacityEntry
	next int
}

func lookupOpacity(img *image.NRGBA) (opaque, ok bool) {
	key := weak.Make(img)
	opacityCache.mu.Lock()
	defer opacityCache.mu.Unlock()
	for i := range opacityCache.ring {
		e := &opacityCache.ring[i]
		if e.ptr == key && e.rect == img.Rect {
			return e.opaque, true
		}
	}
	return false, false
}

func storeOpacity(img *image.NRGBA, opaque bool) {
	key := weak.Make(img)
	opacityCache.mu.Lock()
	defer opacityCache.mu.Unlock()
	for i := range opacityCache.ring {
		e := &opacityCache.ring[i]
		if e.ptr == key {
			e.rect = img.Rect
			e.opaque = opaque
			return
		}
	}
	opacityCache.ring[opacityCache.next] = opacityEntry{ptr: key, rect: img.Rect, opaque: opaque}
	opacityCache.next = (opacityCache.next + 1) % opacityRingSize
}

func MarkOpaque(img *image.NRGBA) {
	if img != nil {
		storeOpacity(img, true)
	}
}

func ForgetOpacity(img *image.NRGBA) {
	if img == nil {
		return
	}
	key := weak.Make(img)
	opacityCache.mu.Lock()
	defer opacityCache.mu.Unlock()
	for i := range opacityCache.ring {
		if opacityCache.ring[i].ptr == key {
			opacityCache.ring[i] = opacityEntry{}
		}
	}
}

func IsOpaque(img *image.NRGBA) bool {
	if img == nil || img.Rect.Empty() {
		return true
	}
	if v, ok := lookupOpacity(img); ok {
		return v
	}
	v := scanOpaque(img)
	storeOpacity(img, v)
	return v
}

func rowOpaque(row []byte) bool {
	acc := ^uint64(0)
	i := 0
	for ; i+8 <= len(row); i += 8 {
		acc &= binary.LittleEndian.Uint64(row[i:])
	}
	if acc&0xFF000000 != 0xFF000000 || acc>>56 != 0xFF {
		return false
	}
	if i < len(row) {
		return row[i+3] == 0xFF
	}
	return true
}

func scanOpaque(img *image.NRGBA) bool {
	b := img.Rect
	w4, h := b.Dx()*4, b.Dy()
	base := img.PixOffset(b.Min.X, b.Min.Y)
	workers := workerCount()
	if w4*h < 1<<18 {
		workers = 1
	}
	chunk := max(1, (h+workers*4-1)/(workers*4))
	tasks := (h + chunk - 1) / chunk
	var translucent atomic.Bool
	parallel(tasks, workers, func(t int) {
		for y := t * chunk; y < min(h, (t+1)*chunk); y++ {
			if translucent.Load() {
				return
			}
			o := base + y*img.Stride
			if !rowOpaque(img.Pix[o : o+w4]) {
				translucent.Store(true)
				return
			}
		}
	})
	return !translucent.Load()
}
