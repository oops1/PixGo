package codec

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	decoders []Decoder
	encoders []Encoder
	disabled map[ID]bool
}

func NewRegistry() *Registry {
	return &Registry{disabled: map[ID]bool{}}
}

var Default = NewRegistry()

func (r *Registry) RegisterDecoder(d Decoder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.decoders = append(r.decoders, d)
}

func (r *Registry) RegisterEncoder(e Encoder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.encoders = append(r.encoders, e)
}

func (r *Registry) SetEnabled(id ID, on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.disabled[id] = !on
}

func (r *Registry) IsEnabled(id ID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return !r.disabled[id]
}

func (r *Registry) Registered() []Decoder {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Decoder(nil), r.decoders...)
}

func (r *Registry) Decoders() []Decoder {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Decoder, 0, len(r.decoders))
	for _, d := range r.decoders {
		if !r.disabled[d.ID()] {
			out = append(out, d)
		}
	}
	return out
}

func (r *Registry) Encoders() []Encoder {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Encoder(nil), r.encoders...)
}

func (r *Registry) Encoder(id ID) (Encoder, bool) {
	for _, e := range r.Encoders() {
		if e.ID() == id {
			return e, true
		}
	}
	return nil, false
}

func (r *Registry) Extensions() []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range r.Decoders() {
		for _, e := range d.Extensions() {
			e = strings.ToLower(e)
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (r *Registry) Supports(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	for _, d := range r.Decoders() {
		for _, e := range d.Extensions() {
			if strings.EqualFold(e, ext) {
				return true
			}
		}
	}
	return false
}

func (r *Registry) Detect(head []byte, path string) (Decoder, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	var byExt Decoder
	var sniffed []Decoder
	for _, d := range r.Decoders() {
		if d.Sniff(head) {
			sniffed = append(sniffed, d)
		}
		for _, e := range d.Extensions() {
			if byExt == nil && strings.EqualFold(e, ext) {
				byExt = d
			}
		}
	}
	if len(sniffed) == 1 {
		return sniffed[0], true
	}
	for _, d := range sniffed {
		if byExt != nil && d.ID() == byExt.ID() {
			return d, true
		}
	}
	if len(sniffed) > 0 {
		return sniffed[0], true
	}
	return nil, false
}

type Source struct {
	Path string
	R    io.ReaderAt
	Size int64
	Head []byte
	f    *os.File
}

func OpenFile(path string) (*Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	head := make([]byte, SniffLen)
	n, _ := f.ReadAt(head, 0)
	return &Source{Path: path, R: f, Size: st.Size(), Head: head[:n], f: f}, nil
}

func (s *Source) Close() error {
	if s.f != nil {
		return s.f.Close()
	}
	return nil
}

func (r *Registry) Probe(ctx context.Context, src *Source, lim Limits) (d Decoder, info Info, err error) {
	lim = lim.WithDefaults()
	if src.Size > lim.MaxFileBytes {
		return nil, Info{}, ErrTooLarge
	}
	d, ok := r.Detect(src.Head, src.Path)
	if !ok {
		return nil, Info{}, ErrUnknownFormat
	}
	defer recoverInto(d, &err)
	info, err = d.Probe(ctx, src.R, src.Size, lim)
	return d, info, err
}

func (r *Registry) Decode(ctx context.Context, d Decoder, src *Source, o Options) (img *Image, err error) {
	defer recoverInto(d, &err)
	img, err = d.Decode(ctx, src.R, src.Size, o)
	if err == nil && (img == nil || len(img.Frames) == 0 || img.Frames[0].Pix == nil && img.Frames[0].Deep == nil && img.Frames[0].CMYK == nil) {
		err = ErrCorrupt
	}
	return img, err
}

type PanicError struct {
	Codec ID
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("codec %s: panic: %v", e.Codec, e.Value)
}

func (e *PanicError) Unwrap() error { return ErrCorrupt }

func recoverInto(d Decoder, err *error) {
	if v := recover(); v != nil {
		*err = &PanicError{Codec: d.ID(), Value: v, Stack: debug.Stack()}
	}
}
