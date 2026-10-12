package thumbs

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	stdjpeg "image/jpeg"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/fsutil"
)

const (
	diskQuality      = 85
	diskExt          = ".jpg"
	diskNameLen      = 16
	diskMaxFileBytes = 8 << 20
	diskTouchAfter   = time.Hour
	diskStaleTemp    = time.Hour
	diskPruneTarget  = 90
	comMarker        = 0xfe
	comPrefix        = "pixgo-thumb v1 "
	maxComLen        = 65533
)

func (k Key) File() string {
	h := fnv.New64a()
	var buf [64]byte
	b := append(buf[:0], k.Path...)
	b = append(b, '|')
	b = strconv.AppendInt(b, k.Size, 10)
	b = append(b, '|')
	b = strconv.AppendInt(b, k.ModTime.UnixNano(), 10)
	b = append(b, '|')
	b = strconv.AppendInt(b, int64(k.Side), 10)
	h.Write(b)
	return fmt.Sprintf("%016x%s", h.Sum64(), diskExt)
}

func isCacheName(name string) bool {
	if len(name) != diskNameLen+len(diskExt) || !strings.HasSuffix(name, diskExt) {
		return false
	}
	for _, c := range name[:diskNameLen] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func isStaleTemp(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, ".tmp-")
}

type diskCache struct {
	dir string
}

func (d *diskCache) path(k Key) string {
	return filepath.Join(d.dir, k.File())
}

func comSegment(t Thumb) []byte {
	text := fmt.Sprintf("%s%d %d %d", comPrefix, t.SrcW, t.SrcH, validOrientation(t.Orientation))
	seg := make([]byte, 0, 4+len(text))
	seg = append(seg, 0xff, comMarker)
	seg = binary.BigEndian.AppendUint16(seg, uint16(2+len(text)))
	return append(seg, text...)
}

func parseCom(data []byte) (w, h int, o codec.Orientation, ok bool) {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return 0, 0, 0, false
	}
	pos := 2
	for steps := 0; steps < 64 && pos+4 <= len(data); steps++ {
		if data[pos] != 0xff {
			return 0, 0, 0, false
		}
		marker := data[pos+1]
		if marker == 0xda || marker == 0xd9 {
			return 0, 0, 0, false
		}
		n := int(binary.BigEndian.Uint16(data[pos+2:]))
		if n < 2 || pos+2+n > len(data) {
			return 0, 0, 0, false
		}
		if marker == comMarker {
			text := string(data[pos+4 : pos+2+n])
			if strings.HasPrefix(text, comPrefix) {
				var ov int
				cnt, _ := fmt.Sscanf(text[len(comPrefix):], "%d %d %d", &w, &h, &ov)
				if cnt < 2 || w <= 0 || h <= 0 {
					return 0, 0, 0, false
				}
				o = codec.OrientNormal
				if cnt == 3 && codec.Orientation(ov).Valid() {
					o = codec.Orientation(ov)
				}
				return w, h, o, true
			}
		}
		pos += 2 + n
	}
	return 0, 0, 0, false
}

func (d *diskCache) load(k Key) (Thumb, bool) {
	path := d.path(k)
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return Thumb{}, false
	}
	if st.Size() <= 0 || st.Size() > diskMaxFileBytes {
		os.Remove(path)
		return Thumb{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Thumb{}, false
	}
	t, ok := decodeCached(data, k.Side)
	if !ok {
		os.Remove(path)
		return Thumb{}, false
	}
	if time.Since(st.ModTime()) > diskTouchAfter {
		now := time.Now()
		os.Chtimes(path, now, now)
	}
	return t, true
}

func decodeCached(data []byte, side int) (Thumb, bool) {
	srcW, srcH, o, ok := parseCom(data)
	if !ok {
		return Thumb{}, false
	}
	cfg, err := stdjpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || max(cfg.Width, cfg.Height) > side {
		return Thumb{}, false
	}
	im, err := stdjpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return Thumb{}, false
	}
	return Thumb{Img: toRGBA(im), SrcW: srcW, SrcH: srcH, Orientation: o}, true
}

func (d *diskCache) store(k Key, t Thumb) error {
	if t.Img == nil || t.Err != nil || t.Img.Bounds().Empty() || !isOpaque(t.Img) {
		return nil
	}
	var buf bytes.Buffer
	buf.Grow(16 << 10)
	if err := stdjpeg.Encode(&buf, t.Img, &stdjpeg.Options{Quality: diskQuality}); err != nil {
		return err
	}
	enc := buf.Bytes()
	com := comSegment(t)
	if len(com)-4 > maxComLen || len(enc) < 2 {
		return nil
	}
	out := make([]byte, 0, len(enc)+len(com))
	out = append(out, enc[:2]...)
	out = append(out, com...)
	out = append(out, enc[2:]...)
	return fsutil.WriteAtomic(d.path(k), out, 0o600)
}

type cacheFile struct {
	path  string
	size  int64
	mtime time.Time
}

func (d *diskCache) scan(now time.Time, sweepTemp bool) ([]cacheFile, int64) {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return nil, 0
	}
	files := make([]cacheFile, 0, len(entries))
	var total int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		cache := isCacheName(name)
		temp := sweepTemp && isStaleTemp(name)
		if !cache && !temp {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		full := filepath.Join(d.dir, name)
		if temp {
			if now.Sub(info.ModTime()) > diskStaleTemp {
				os.Remove(full)
			}
			continue
		}
		files = append(files, cacheFile{path: full, size: info.Size(), mtime: info.ModTime()})
		total += info.Size()
	}
	return files, total
}

func (d *diskCache) usage() int64 {
	_, total := d.scan(time.Time{}, false)
	return total
}

func (d *diskCache) prune(limit int64, now time.Time) {
	files, total := d.scan(now, true)
	if total <= limit {
		return
	}
	target := limit / 100 * diskPruneTarget
	sort.Slice(files, func(i, j int) bool {
		if !files[i].mtime.Equal(files[j].mtime) {
			return files[i].mtime.Before(files[j].mtime)
		}
		return files[i].path < files[j].path
	})
	for _, f := range files {
		if total <= target {
			return
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}

func (d *diskCache) clear() error {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var first error
	for _, e := range entries {
		if e.IsDir() || !(isCacheName(e.Name()) || isStaleTemp(e.Name())) {
			continue
		}
		if err := os.Remove(filepath.Join(d.dir, e.Name())); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
	}
	return first
}
