package raw

import (
	"context"
	"io"
	"sort"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

const (
	maxCandidates     = 128
	smallPreviewPixel = 1_000_000
)

type candidate struct {
	offset int64
	length int64
	head   jpegHead
	carved bool
}

func (c candidate) pixels() int64 {
	return int64(c.head.width) * int64(c.head.height)
}

func (c candidate) span(size int64) int64 {
	if c.length > 0 {
		return c.length
	}
	return size - c.offset
}

type finder struct {
	ctx         context.Context
	r           io.ReaderAt
	size        int64
	list        []candidate
	known       map[int64]bool
	truncated   bool
	unsupported bool
}

func newFinder(ctx context.Context, r io.ReaderAt, size int64) *finder {
	return &finder{ctx: ctx, r: r, size: size, known: map[int64]bool{}}
}

func (f *finder) add(offset, length int64, carved bool) bool {
	if len(f.list) >= maxCandidates || f.known[offset] {
		return len(f.list) < maxCandidates
	}
	head, status := probeJPEG(f.r, f.size, offset, length)
	switch status {
	case jpegOutOfRange:
		if !carved {
			f.truncated = true
		}
		return true
	case jpegUnsupported:
		if !carved {
			f.unsupported = true
		}
		f.known[offset] = true
		return true
	case jpegNotJPEG:
		return true
	}
	f.known[offset] = true
	f.list = append(f.list, candidate{offset: offset, length: length, head: head, carved: carved})
	return true
}

func (f *finder) addWithin(offset, length int64) error {
	if offset < 0 || length <= 0 || offset > f.size {
		f.truncated = true
		return nil
	}
	end := offset + length
	if end > f.size {
		end = f.size
		f.truncated = true
	}
	return carve(f.ctx, f.r, f.size, offset, end, func(at int64) bool { return f.add(at, 0, true) })
}

func (f *finder) largest() int64 {
	var best int64
	for _, c := range f.list {
		if p := c.pixels(); p > best {
			best = p
		}
	}
	return best
}

func (f *finder) carveWhenSmall() error {
	if f.largest() >= smallPreviewPixel {
		return nil
	}
	return carve(f.ctx, f.r, f.size, 0, f.size, func(at int64) bool { return f.add(at, 0, true) })
}

func (f *finder) sorted() []candidate {
	out := append([]candidate(nil), f.list...)
	sort.SliceStable(out, func(i, j int) bool {
		if pi, pj := out[i].pixels(), out[j].pixels(); pi != pj {
			return pi > pj
		}
		if out[i].carved != out[j].carved {
			return !out[i].carved
		}
		return out[i].offset < out[j].offset
	})
	return out
}

func (f *finder) choose(lim codec.Limits) (candidate, error) {
	ordered := f.sorted()
	for _, c := range ordered {
		if lim.Check(c.head.width, c.head.height) == nil {
			return c, nil
		}
	}
	switch {
	case len(ordered) > 0:
		return candidate{}, guard.TooLarge("raw: the embedded preview of %dx%d exceeds the pixel limit", ordered[0].head.width, ordered[0].head.height)
	case f.truncated:
		return candidate{}, guard.Corrupt("raw: the embedded preview lies outside the file")
	case f.unsupported:
		return candidate{}, guard.Unsupported("raw: the embedded preview uses an unsupported JPEG variant")
	}
	return candidate{}, guard.Unsupported("raw: no embedded JPEG preview")
}
