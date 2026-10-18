package colorm

import (
	"crypto/sha256"
	"fmt"
	"image"
	"sync"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

type Config struct {
	Enabled       bool
	SystemProfile bool
	HDR           HDRConfig
}

type DisplayProvider func() (icc []byte, name string)

const displayRefresh = time.Second

type Manager struct {
	mu       sync.Mutex
	cfg      Config
	provider DisplayProvider
	display  *Profile
	name     string
	checked  time.Time
	stamp    uint64
	parsed   map[[32]byte]*Profile
	order    [][32]byte
	now      func() time.Time
}

func NewManager(provider DisplayProvider) *Manager {
	return &Manager{provider: provider, parsed: map[[32]byte]*Profile{}, now: time.Now}
}

func (m *Manager) Configure(cfg Config) {
	m.mu.Lock()
	if m.cfg != cfg {
		m.cfg = cfg
		m.stamp++
		m.checked = time.Time{}
	}
	m.mu.Unlock()
}

func (m *Manager) Config() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

func (m *Manager) Enabled() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.Enabled
}

func (m *Manager) refreshLocked(force bool) {
	if !m.cfg.Enabled || !m.cfg.SystemProfile || m.provider == nil {
		if m.display != nil {
			m.display, m.name = nil, ""
			m.stamp++
		}
		return
	}
	now := m.now()
	if !force && !m.checked.IsZero() && now.Sub(m.checked) < displayRefresh {
		return
	}
	m.checked = now
	data, name := m.provider()
	var p *Profile
	if len(data) > 0 {
		p = m.parseLocked(data)
		if p != nil && (p.Space != SpaceRGB || p.hdr != hdrNone) {
			p = nil
		}
	}
	if p == nil {
		if m.display != nil {
			m.display, m.name = nil, ""
			m.stamp++
		}
		return
	}
	if m.display == nil || m.display.Hash != p.Hash {
		m.display, m.name = p, name
		m.stamp++
	}
}

func (m *Manager) parseLocked(data []byte) *Profile {
	h := sha256.Sum256(data)
	if p, ok := m.parsed[h]; ok {
		return p
	}
	p, err := ParseICC(data)
	if err != nil {
		p = nil
	}
	if len(m.order) >= 16 {
		delete(m.parsed, m.order[0])
		m.order = m.order[1:]
	}
	m.parsed[h] = p
	m.order = append(m.order, h)
	return p
}

func (m *Manager) Refresh() {
	m.mu.Lock()
	m.refreshLocked(true)
	m.mu.Unlock()
}

func (m *Manager) Stamp() uint64 {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshLocked(false)
	return m.stamp
}

func (m *Manager) Display() (*Profile, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshLocked(false)
	if m.display == nil {
		return SRGB(), ""
	}
	return m.display, m.name
}

type Report struct {
	Managed    bool
	Profile    string
	Origin     Origin
	HDR        string
	ToneMapped bool
	Deep       bool
	Converted  bool
	Display    string
	Gray       bool
	CMYK       bool
}

type Conv struct {
	T      *Transform
	Report Report
}

func (m *Manager) resolveSource(ci codec.ColorInfo) (*Profile, Origin) {
	if len(ci.ICC) > 0 {
		m.mu.Lock()
		p := m.parseLocked(ci.ICC)
		m.mu.Unlock()
		if p != nil {
			return p, OriginEmbedded
		}
	}
	if ci.CICP != nil {
		if p, err := FromCICP(*ci.CICP); err == nil {
			return p, OriginCICP
		}
	}
	return SRGB(), OriginNone
}

func (m *Manager) Resolve(ci codec.ColorInfo) *Conv {
	if m == nil {
		return &Conv{Report: Report{}}
	}
	cfg := m.Config()
	if !cfg.Enabled {
		return &Conv{}
	}
	src, origin := m.resolveSource(ci)
	dst, dname := m.Display()
	rep := Report{Managed: true, Origin: origin, HDR: src.HDR(), Gray: src.Space == SpaceGray, CMYK: src.Space == SpaceCMYK}
	if origin != OriginNone {
		rep.Profile = src.Description
	}
	if src.Space == SpaceCMYK {
		src = SRGB()
	}
	if dname != "" {
		rep.Display = dname
	} else if dst != SRGB() {
		rep.Display = dst.Description
	}
	opt := Options{HDR: cfg.HDR}
	if src.IsHDR() {
		opt.SoftClip = true
	}
	t, err := NewTransform(src, dst, opt)
	if err != nil {
		if src.IsHDR() || src.Space == SpaceCMYK {
			if t2, err2 := NewTransform(src, SRGB(), opt); err2 == nil {
				t, err = t2, nil
			}
		}
	}
	if err != nil {
		t, err = NewTransform(SRGB(), dst, Options{})
		if err != nil {
			t, _ = NewTransform(SRGB(), SRGB(), Options{})
		}
		rep.Profile, rep.Origin, rep.HDR = "", OriginNone, ""
	}
	rep.ToneMapped = t.HDR()
	rep.Converted = !t.Identity()
	return &Conv{T: t, Report: rep}
}

func (c *Conv) Needed() bool { return c != nil && c.T != nil && !c.T.Identity() }

func (c *Conv) Tile(img *image.NRGBA) {
	if c.Needed() {
		c.T.Apply(img)
	}
}

func (c *Conv) Frame(f *codec.Frame) error {
	if f.Deep != nil {
		t := c.T
		if t == nil {
			var err error
			t, err = NewTransform(SRGB(), SRGB(), Options{})
			if err != nil {
				return err
			}
		}
		b := f.Deep.Rect
		out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		if err := t.Convert(out, f.Deep); err != nil {
			return err
		}
		f.Pix, f.Deep = out, nil
		return nil
	}
	if f.Pix != nil {
		c.Tile(f.Pix)
	}
	return nil
}

func (c *Conv) Image(img *codec.Image) (Report, error) {
	rep := c.Report
	for i := range img.Frames {
		if img.Frames[i].Deep != nil {
			rep.Deep = true
		}
		if err := c.Frame(&img.Frames[i]); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

func (m *Manager) ConvertImage(img *codec.Image) (Report, error) {
	return m.Resolve(img.Color).Image(img)
}

func (m *Manager) ResolveCMYK(ci codec.ColorInfo) (*Transform, bool) {
	if !m.Enabled() || len(ci.ICC) == 0 {
		return nil, false
	}
	src, origin := m.resolveSource(ci)
	if origin != OriginEmbedded || src.Space != SpaceCMYK {
		return nil, false
	}
	dst, _ := m.Display()
	t, err := NewTransform(src, dst, Options{})
	if err != nil {
		t, err = NewTransform(src, SRGB(), Options{})
	}
	return t, err == nil
}

func (m *Manager) Tag() string {
	if !m.Enabled() {
		return ""
	}
	p, _ := m.Display()
	if p == SRGB() {
		return "cm1"
	}
	return fmt.Sprintf("cm1-%x", p.Hash[:6])
}

func (m *Manager) DisplayICC() []byte {
	if !m.Enabled() {
		return nil
	}
	p, _ := m.Display()
	if p == SRGB() {
		return nil
	}
	return p.Bytes()
}
