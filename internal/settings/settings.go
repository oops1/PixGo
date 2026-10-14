package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/oops1/PixGo/internal/fsutil"
)

const Current = 2

const (
	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"

	LanguageSystem = "system"

	BackgroundSystem  = "system"
	BackgroundWhite   = "white"
	BackgroundBlack   = "black"
	BackgroundChecker = "checker"

	ZoomFit    = "fit"
	ZoomActual = "actual"
	ZoomWidth  = "width"

	WheelZoom     = "zoom"
	WheelNavigate = "navigate"

	SortName = "name"
	SortDate = "date"
	SortSize = "size"

	TransitionNone = "none"
	TransitionFade = "fade"

	GalleryGrid = "grid"
	GalleryList = "list"
)

const (
	MinSaveQuality         = 1
	MaxSaveQuality         = 100
	DefaultSaveJPEGQuality = 90
	DefaultSaveWebPQuality = 80
)

var OptionalFormats = []string{"apng", "avif", "jxl", "heic", "raw"}

type Format struct {
	Enabled bool `json:"enabled"`
}

type Slideshow struct {
	Interval   int    `json:"interval"`
	Loop       bool   `json:"loop"`
	Shuffle    bool   `json:"shuffle"`
	Transition string `json:"transition"`
}

type Updates struct {
	Check     bool      `json:"check"`
	LastCheck time.Time `json:"lastCheck"`
}

type Settings struct {
	Version int `json:"version"`

	Theme                 string `json:"theme"`
	Language              string `json:"language"`
	RememberWindow        bool   `json:"rememberWindow"`
	RestoreLastImage      bool   `json:"restoreLastImage"`
	ConfirmDelete         bool   `json:"confirmDelete"`
	AutoHidePanels        bool   `json:"autoHidePanels"`
	TransparentBackground string `json:"transparentBackground"`
	SingleWindow          bool   `json:"singleWindow"`

	InitialZoom          string `json:"initialZoom"`
	Wheel                string `json:"wheel"`
	CtrlWheelZoom        bool   `json:"ctrlWheelZoom"`
	SmoothZoom           bool   `json:"smoothZoom"`
	Preload              int    `json:"preload"`
	ApplyExifOrientation bool   `json:"applyExifOrientation"`
	KeepZoomOnNavigate   bool   `json:"keepZoomOnNavigate"`
	LoopFolder           bool   `json:"loopFolder"`
	Sort                 string `json:"sort"`
	SortDesc             bool   `json:"sortDesc"`
	GalleryView          string `json:"galleryView"`
	GalleryThumbSize     int    `json:"galleryThumbSize"`

	SaveJPEGQuality  int  `json:"saveJpegQuality"`
	SaveWebPQuality  int  `json:"saveWebpQuality"`
	SaveWebPLossless bool `json:"saveWebpLossless"`

	Formats   map[string]Format `json:"formats"`
	Slideshow Slideshow         `json:"slideshow"`
	Updates   Updates           `json:"updates"`

	CacheMB       int `json:"cacheMB"`
	MaxPixels     int `json:"maxPixels"`
	DecodeThreads int `json:"decodeThreads"`
	AnimationFPS  int `json:"animationFPS"`
	ThumbCacheMB  int `json:"thumbCacheMB"`

	extra map[string]json.RawMessage
}

func Defaults() Settings {
	formats := make(map[string]Format, len(OptionalFormats))
	for _, id := range OptionalFormats {
		formats[id] = Format{Enabled: true}
	}
	return Settings{
		Version: Current,

		Theme: ThemeSystem, Language: LanguageSystem,
		RememberWindow: true, RestoreLastImage: true, ConfirmDelete: true, AutoHidePanels: true,
		TransparentBackground: BackgroundChecker, SingleWindow: true,

		InitialZoom: ZoomFit, Wheel: WheelNavigate, CtrlWheelZoom: true, SmoothZoom: false,
		Preload: 1, ApplyExifOrientation: true, KeepZoomOnNavigate: false, LoopFolder: false,
		Sort: SortName, SortDesc: false,
		GalleryView: GalleryGrid, GalleryThumbSize: 128,

		SaveJPEGQuality: DefaultSaveJPEGQuality, SaveWebPQuality: DefaultSaveWebPQuality, SaveWebPLossless: false,

		Formats:   formats,
		Slideshow: Slideshow{Interval: 5, Loop: true, Shuffle: false, Transition: TransitionNone},
		Updates:   Updates{Check: true},

		CacheMB: DefaultCacheMB(), MaxPixels: 100, DecodeThreads: 0,
		AnimationFPS: DefaultAnimationFPS(), ThumbCacheMB: 256,
	}
}

func (s Settings) FormatEnabled(id string) bool {
	f, ok := s.Formats[id]
	return !ok || f.Enabled
}

var languageTag = regexp.MustCompile(`^[a-z]{2,3}(-[a-zA-Z0-9]{2,8})*$`)

func (s *Settings) Normalize() {
	d := Defaults()
	oneOf := func(v *string, def string, ok ...string) {
		for _, o := range ok {
			if *v == o {
				return
			}
		}
		*v = def
	}
	clamp := func(v *int, lo, hi, def int) {
		if *v < lo || *v > hi {
			*v = def
		}
	}
	oneOf(&s.Theme, d.Theme, ThemeSystem, ThemeLight, ThemeDark)
	if s.Language != LanguageSystem && !languageTag.MatchString(s.Language) {
		s.Language = d.Language
	}
	oneOf(&s.TransparentBackground, d.TransparentBackground, BackgroundSystem, BackgroundWhite, BackgroundBlack, BackgroundChecker)
	oneOf(&s.InitialZoom, d.InitialZoom, ZoomFit, ZoomActual, ZoomWidth)
	oneOf(&s.Wheel, d.Wheel, WheelZoom, WheelNavigate)
	oneOf(&s.Sort, d.Sort, SortName, SortDate, SortSize)
	oneOf(&s.GalleryView, d.GalleryView, GalleryGrid, GalleryList)
	oneOf(&s.Slideshow.Transition, d.Slideshow.Transition, TransitionNone, TransitionFade)
	clamp(&s.Preload, 0, 2, d.Preload)
	clamp(&s.GalleryThumbSize, 96, 384, d.GalleryThumbSize)
	clamp(&s.SaveJPEGQuality, MinSaveQuality, MaxSaveQuality, d.SaveJPEGQuality)
	clamp(&s.SaveWebPQuality, MinSaveQuality, MaxSaveQuality, d.SaveWebPQuality)
	clamp(&s.Slideshow.Interval, 1, 60, d.Slideshow.Interval)
	clamp(&s.CacheMB, 256, 8192, d.CacheMB)
	clamp(&s.MaxPixels, 4, 1000, d.MaxPixels)
	clamp(&s.DecodeThreads, 0, 64, d.DecodeThreads)
	clamp(&s.AnimationFPS, 1, 60, d.AnimationFPS)
	clamp(&s.ThumbCacheMB, 16, 8192, d.ThumbCacheMB)
	formats := make(map[string]Format, len(s.Formats)+len(OptionalFormats))
	for id, f := range s.Formats {
		formats[id] = f
	}
	for _, id := range OptionalFormats {
		if _, ok := formats[id]; !ok {
			formats[id] = Format{Enabled: true}
		}
	}
	s.Formats = formats
	s.Version = Current
}

func (s Settings) clone() Settings {
	c := s
	c.Formats = make(map[string]Format, len(s.Formats))
	for k, v := range s.Formats {
		c.Formats[k] = v
	}
	return c
}

func Decode(data []byte) (Settings, error) {
	s := Defaults()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return Defaults(), err
	}
	migrate(&s, raw)
	known := map[string]bool{}
	b, _ := json.Marshal(Defaults())
	var km map[string]json.RawMessage
	json.Unmarshal(b, &km)
	for k := range km {
		known[k] = true
	}
	for k, v := range raw {
		if !known[k] {
			if s.extra == nil {
				s.extra = map[string]json.RawMessage{}
			}
			s.extra[k] = v
		}
	}
	s.Normalize()
	return s, nil
}

var migrations = map[int]func(s *Settings, raw map[string]json.RawMessage){
	1: func(s *Settings, raw map[string]json.RawMessage) {
		if s.Wheel == WheelZoom {
			s.Wheel = WheelNavigate
		}
	},
}

func migrate(s *Settings, raw map[string]json.RawMessage) {
	if s.Version > Current {
		return
	}
	for v := s.Version; v < Current; v++ {
		if fn := migrations[v]; fn != nil {
			fn(s, raw)
		}
	}
	s.Version = Current
}

func (s Settings) Encode() ([]byte, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if len(s.extra) > 0 {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		for k, v := range s.extra {
			if _, ok := m[k]; !ok {
				m[k] = v
			}
		}
		if b, err = json.Marshal(m); err != nil {
			return nil, err
		}
	}
	return indent(b)
}

func indent(b []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(v, "", "  ")
	return append(out, '\n'), err
}

func Path() string { return filepath.Join(fsutil.ConfigDir(), "settings.json") }

type Store struct {
	mu      sync.Mutex
	path    string
	cur     Settings
	subs    map[int]func(old, new Settings)
	nextSub int
	timer   *time.Timer
	delay   time.Duration
	dirty   bool
	saveErr error
}

func Load(path string) (*Store, error) {
	st := &Store{path: path, cur: Defaults(), subs: map[int]func(old, new Settings){}, delay: 500 * time.Millisecond}
	if path == "" {
		return st, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	s, err := Decode(data)
	if err != nil {
		os.Rename(path, path+".broken")
		return st, err
	}
	st.cur = s
	return st, nil
}

func NewMemory(s Settings) *Store {
	s.Normalize()
	return &Store{cur: s, subs: map[int]func(old, new Settings){}}
}

func (st *Store) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.cur.clone()
}

func (st *Store) Update(fn func(*Settings)) {
	st.mu.Lock()
	old := st.cur
	next := st.cur.clone()
	fn(&next)
	next.extra = old.extra
	next.Normalize()
	st.cur = next
	subs := make([]func(old, new Settings), 0, len(st.subs))
	for i := 0; i < st.nextSub; i++ {
		if f, ok := st.subs[i]; ok {
			subs = append(subs, f)
		}
	}
	st.dirty = true
	if st.path != "" {
		if st.timer == nil {
			st.timer = time.AfterFunc(st.delay, func() { st.Flush() })
		} else {
			st.timer.Reset(st.delay)
		}
	}
	st.mu.Unlock()
	for _, f := range subs {
		f(old.clone(), next.clone())
	}
}

func (st *Store) Subscribe(fn func(old, new Settings)) func() {
	st.mu.Lock()
	defer st.mu.Unlock()
	id := st.nextSub
	st.nextSub++
	st.subs[id] = fn
	return func() {
		st.mu.Lock()
		delete(st.subs, id)
		st.mu.Unlock()
	}
}

func (st *Store) Flush() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.timer != nil {
		st.timer.Stop()
	}
	if !st.dirty || st.path == "" {
		return nil
	}
	data, err := st.cur.Encode()
	if err == nil {
		err = fsutil.WriteAtomic(st.path, data, 0o600)
	}
	st.saveErr = err
	if err == nil {
		st.dirty = false
	}
	return err
}

func (st *Store) Err() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.saveErr
}

func (st *Store) Path() string { return st.path }
