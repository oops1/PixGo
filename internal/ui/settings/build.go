package settings

import (
	"image"
	"math"
	"runtime"
	"strconv"
	"strings"

	"github.com/oops1/PixGo/internal/i18n"
	data "github.com/oops1/PixGo/internal/settings"
)

var goos = runtime.GOOS

type cardEntry struct {
	id     string
	text   string
	pic    picture
	on     bool
	change func(*data.Settings)
}

type builder struct {
	p      *Page
	x0, x1 int
	xt     int
	y      float64
	items  []item
	group  int
}

func (b *builder) width() int { return b.x1 - b.x0 }

func (b *builder) add(it item) {
	if it.box.Empty() {
		it.box = it.r
	}
	b.items = append(b.items, it)
}

func (b *builder) skip(n float64) { b.y += n }

func (b *builder) top() int { return int(math.Round(b.y)) }

func (b *builder) nextGroup() int {
	b.group++
	return b.group
}

func (b *builder) title(text string) {
	y := b.top()
	b.add(item{kind: kindTitle, text: fitText(titleFont, text, b.width()), r: image.Rect(b.x0, y, b.x1, y+titleHeight)})
	b.y += titleHeight + titleGap
}

func (b *builder) heading(text string) {
	y := b.top()
	b.add(item{kind: kindHeading, text: fitText(headingFont, text, b.width()), r: image.Rect(b.x0, y, b.x1, y+headingHeight)})
	b.y += headingHeight
}

func (b *builder) note(text string, indent int) {
	lines := wrapLines(noteFont, text, b.xt-b.x0-indent, 5)
	y := b.top()
	r := image.Rect(b.x0, y, b.x1, y+len(lines)*lineHeight)
	b.add(item{kind: kindNote, lines: lines, indent: indent, r: r})
	b.y += float64(r.Dy())
}

func (b *builder) value(text string) {
	lines := wrapLines(bodyFont, text, b.xt-b.x0, 4)
	y := b.top()
	r := image.Rect(b.x0, y, b.x1, y+valueHeight+(len(lines)-1)*lineHeight)
	b.add(item{kind: kindValue, lines: lines, r: r})
	b.y += float64(r.Dy())
}

func (b *builder) labelRow(kind kind, id, text string, on bool, group int, change func(*data.Settings)) {
	lines := wrapLines(bodyFont, text, b.xt-b.x0-checkLabelX, 3)
	widest := 0
	for _, l := range lines {
		widest = max(widest, measure(bodyFont, l))
	}
	h := rowPitch + float64(len(lines)-1)*lineHeight
	top := b.y
	y0, y1 := int(math.Round(top)), int(math.Round(top+h))
	r := image.Rect(b.x0, y0, b.x1, y1)
	hitR := image.Rect(b.x0, y0, min(b.x0+checkLabelX+widest+6, b.xt), y1)
	b.add(item{kind: kind, id: id, text: text, lines: lines, on: on, group: group, change: change, r: r, box: hitR, fy: top})
	b.y += h
}

func (b *builder) check(id, text string, on bool, set func(*data.Settings, bool)) {
	b.labelRow(kindCheck, id, text, on, 0, func(s *data.Settings) { set(s, !on) })
}

func (b *builder) radios(idPrefix string, entries []choice) {
	g := b.nextGroup()
	for _, e := range entries {
		b.labelRow(kindRadio, idPrefix+":"+e.id, e.text, e.on, g, e.change)
	}
}

func (b *builder) dropdown(id, name, text string, choices []choice) {
	y := b.top()
	r := image.Rect(b.x0, y, b.x1, y+dropdownHeight)
	b.add(item{kind: kindDropdown, id: id, name: name, text: fitText(bodyFont, text, b.width()-dropdownTextX-chevronFromR-6), choices: choices, r: r})
	b.y += dropdownHeight
}

func (b *builder) slider(id, name string, value, lo, hi, step int, valueText string, slide func(int) func(*data.Settings)) {
	y := b.top()
	r := image.Rect(b.x0, y, b.x1, y+rowHeight)
	b.add(item{kind: kindSlider, id: id, name: name, value: value, lo: lo, hi: hi, step: step, valueText: valueText, slide: slide, r: r})
	b.y += rowHeight
}

func (b *builder) button(id, text string, run func()) {
	if run == nil {
		return
	}
	w := min(max(measure(bodyFont, text)+2*buttonPadding, buttonMin), b.width())
	y := b.top()
	r := image.Rect(b.x0, y, b.x0+w, y+buttonHeight)
	b.add(item{kind: kindButton, id: id, text: fitText(bodyFont, text, w-2*buttonPadding), run: run, r: r})
	b.y += buttonHeight
}

func (b *builder) link(id, text, detail string, run func()) {
	if run == nil {
		return
	}
	w := b.width()
	text = fitText(bodyFont, text, w)
	tw := measure(bodyFont, text)
	if detail != "" {
		detail = fitText(bodyFont, " — "+detail, max(w-tw, 0))
	}
	y := b.top()
	r := image.Rect(b.x0, y, b.x1, y+linkHeight)
	hitR := image.Rect(b.x0, y, min(b.x0+tw+4, b.x1), y+linkHeight)
	b.add(item{kind: kindLink, id: id, text: text, detail: detail, run: run, r: r, box: hitR})
	b.y += linkHeight
}

func (b *builder) cards(entries []cardEntry, w, h int, pitch float64, offsets []int, rowGap int) {
	group := b.nextGroup()
	perRow := max(int(math.Floor(float64(b.width()-w+cardSlack)/pitch))+1, 1)
	top := b.top()
	for i, e := range entries {
		col, row := i%perRow, i/perRow
		x := b.x0 + int(math.Round(float64(col)*pitch))
		if col < len(offsets) {
			x = b.x0 + offsets[col]
		}
		y := top + row*(h+rowGap)
		b.add(item{kind: kindCard, id: e.id, text: e.text, pic: e.pic, on: e.on, group: group, change: e.change, r: image.Rect(x, y, x+w, y+h)})
	}
	rows := (len(entries) + perRow - 1) / perRow
	b.y += float64(rows*h + max(rows-1, 0)*rowGap)
}

func (b *builder) formatRow(f Format) {
	y := b.top()
	r := image.Rect(b.x0, y, b.x1, y+formatRowHeight)
	it := item{kind: kindFormat, id: "format:" + f.ID, name: f.Name, on: f.Enabled, r: r, disabled: f.Optional && !f.Available}
	exts := strings.Join(f.Extensions, " ")
	it.detail = exts
	if it.disabled {
		unavailable := i18n.T("settings.format.unavailable")
		if exts != "" {
			it.detail = exts + " · " + unavailable
		} else {
			it.detail = unavailable
		}
	}
	if f.Optional && f.Available {
		id := f.ID
		on := f.Enabled
		it.change = func(s *data.Settings) {
			formats := make(map[string]data.Format, len(s.Formats)+1)
			for k, v := range s.Formats {
				formats[k] = v
			}
			formats[id] = data.Format{Enabled: !on}
			s.Formats = formats
		}
		label := i18n.T("settings.format.enable")
		lw := checkLabelX + measure(bodyFont, label)
		it.box = image.Rect(max(b.x1-lw-6, b.x0), y, b.x1, y+rowHeight)
		it.text = label
		it.fy = float64(y)
	}
	b.add(it)
	b.y += formatRowHeight + formatGap
}

func (b *builder) section(s Section) {
	switch s {
	case SectionView:
		b.viewSection()
	case SectionFormats:
		b.formatsSection()
	case SectionSlideshow:
		b.slideshowSection()
	case SectionPerformance:
		b.performanceSection()
	case SectionLanguage:
		b.languageSection()
	case SectionAbout:
		b.aboutSection()
	default:
		b.generalSection()
	}
}

func (b *builder) generalSection() {
	cfg := b.p.cfg
	b.title(i18n.T(sectionKeys[SectionGeneral]))
	b.heading(i18n.T("settings.theme"))
	b.skip(headingToTheme)
	b.cards([]cardEntry{
		{"theme:system", i18n.T("settings.theme.system"), picThemeSystem, cfg.Theme == data.ThemeSystem, func(s *data.Settings) { s.Theme = data.ThemeSystem }},
		{"theme:light", i18n.T("settings.theme.light"), picThemeLight, cfg.Theme == data.ThemeLight, func(s *data.Settings) { s.Theme = data.ThemeLight }},
		{"theme:dark", i18n.T("settings.theme.dark"), picThemeDark, cfg.Theme == data.ThemeDark, func(s *data.Settings) { s.Theme = data.ThemeDark }},
	}, themeCardW, themeItemH, themeCardPitch, nil, cardRowGap)
	b.skip(afterCards)
	b.heading(i18n.T("settings.language"))
	b.skip(headingToField)
	b.dropdown("language", i18n.T("settings.language"), b.p.languageText(), b.p.languageChoices())
	b.skip(dropdownAfter)
	b.check("rememberWindow", i18n.T("settings.rememberWindow"), cfg.RememberWindow, func(s *data.Settings, v bool) { s.RememberWindow = v })
	b.check("restoreLastImage", i18n.T("settings.restoreLastImage"), cfg.RestoreLastImage, func(s *data.Settings, v bool) { s.RestoreLastImage = v })
	b.check("confirmDelete", i18n.T("settings.confirmDelete"), cfg.ConfirmDelete, func(s *data.Settings, v bool) { s.ConfirmDelete = v })
	b.check("autoHidePanels", i18n.T("settings.autoHidePanels"), cfg.AutoHidePanels, func(s *data.Settings, v bool) { s.AutoHidePanels = v })
	b.skip(groupGap)
	b.heading(i18n.T("settings.transparentBackground"))
	b.skip(headingToBg)
	b.cards([]cardEntry{
		{"background:system", i18n.T("settings.background.system"), picBgSystem, cfg.TransparentBackground == data.BackgroundSystem, func(s *data.Settings) { s.TransparentBackground = data.BackgroundSystem }},
		{"background:white", i18n.T("settings.background.white"), picBgWhite, cfg.TransparentBackground == data.BackgroundWhite, func(s *data.Settings) { s.TransparentBackground = data.BackgroundWhite }},
		{"background:black", i18n.T("settings.background.black"), picBgBlack, cfg.TransparentBackground == data.BackgroundBlack, func(s *data.Settings) { s.TransparentBackground = data.BackgroundBlack }},
		{"background:checker", i18n.T("settings.background.checker"), picBgChecker, cfg.TransparentBackground == data.BackgroundChecker, func(s *data.Settings) { s.TransparentBackground = data.BackgroundChecker }},
	}, bgItemW, bgItemH, bgPitch, bgOffsets, 6)
	b.skip(afterBgCards)
	b.check("singleWindow", i18n.T("settings.singleWindow"), cfg.SingleWindow, func(s *data.Settings, v bool) { s.SingleWindow = v })
	b.note(i18n.T("settings.singleWindow.desc"), checkLabelX)
	b.check("updatesCheck", i18n.T("settings.updatesCheck"), cfg.Updates.Check, func(s *data.Settings, v bool) { s.Updates.Check = v })
}

func (p *Page) currentLanguageName() string {
	id := i18n.Lang()
	for _, l := range p.langs {
		if l.ID == id {
			return l.Name
		}
	}
	return id
}

func (p *Page) languageList() []Language {
	list := make([]Language, 0, len(p.langs)+1)
	hasSystem := false
	for _, l := range p.langs {
		hasSystem = hasSystem || l.ID == data.LanguageSystem
	}
	if !hasSystem {
		list = append(list, Language{ID: data.LanguageSystem, Name: i18n.T("settings.language.system", p.currentLanguageName())})
	}
	return append(list, p.langs...)
}

func (p *Page) languageText() string {
	for _, l := range p.languageList() {
		if l.ID == p.cfg.Language {
			return l.Name
		}
	}
	return p.cfg.Language
}

func (p *Page) languageChoices() []choice {
	list := p.languageList()
	out := make([]choice, 0, len(list))
	for _, l := range list {
		id := l.ID
		out = append(out, choice{id: id, text: l.Name, on: id == p.cfg.Language, change: func(s *data.Settings) { s.Language = id }})
	}
	return out
}

func (b *builder) viewSection() {
	cfg := b.p.cfg
	b.title(i18n.T(sectionKeys[SectionView]))
	b.heading(i18n.T("settings.initialZoom"))
	b.skip(headingToField)
	zoom := func(id, key string) choice {
		return choice{id: id, text: i18n.T(key), on: cfg.InitialZoom == id, change: func(s *data.Settings) { s.InitialZoom = id }}
	}
	choices := []choice{zoom(data.ZoomFit, "settings.initialZoom.fit"), zoom(data.ZoomActual, "settings.initialZoom.actual"), zoom(data.ZoomWidth, "settings.initialZoom.width")}
	b.dropdown("initialZoom", i18n.T("settings.initialZoom"), selectedText(choices), choices)
	b.skip(groupGap)
	b.heading(i18n.T("settings.wheel"))
	b.skip(headingToRows)
	wheel := func(id, key string) choice {
		return choice{id: id, text: i18n.T(key), on: cfg.Wheel == id, change: func(s *data.Settings) { s.Wheel = id }}
	}
	b.radios("wheel", []choice{wheel(data.WheelNavigate, "settings.wheel.navigate"), wheel(data.WheelZoom, "settings.wheel.zoom")})
	b.check("ctrlWheelZoom", i18n.T("settings.ctrlWheelZoom"), cfg.CtrlWheelZoom, func(s *data.Settings, v bool) { s.CtrlWheelZoom = v })
	b.check("smoothZoom", i18n.T("settings.smoothZoom"), cfg.SmoothZoom, func(s *data.Settings, v bool) { s.SmoothZoom = v })
	b.note(i18n.T("settings.smoothZoom.desc"), checkLabelX)
	b.skip(groupGap)
	b.heading(i18n.T("settings.preload"))
	b.skip(headingToRows)
	preload := func(n int) choice {
		text := i18n.T("settings.preload.off")
		if n > 0 {
			text = i18n.T("settings.preload.n", n)
		}
		return choice{id: strconv.Itoa(n), text: text, on: cfg.Preload == n, change: func(s *data.Settings) { s.Preload = n }}
	}
	b.radios("preload", []choice{preload(0), preload(1), preload(2)})
	b.skip(groupGap)
	b.check("applyExifOrientation", i18n.T("settings.applyExifOrientation"), cfg.ApplyExifOrientation, func(s *data.Settings, v bool) { s.ApplyExifOrientation = v })
	b.check("keepZoomOnNavigate", i18n.T("settings.keepZoomOnNavigate"), cfg.KeepZoomOnNavigate, func(s *data.Settings, v bool) { s.KeepZoomOnNavigate = v })
	b.check("loopFolder", i18n.T("settings.loopFolder"), cfg.LoopFolder, func(s *data.Settings, v bool) { s.LoopFolder = v })
	b.skip(groupGap)
	b.heading(i18n.T("settings.sort"))
	b.skip(headingToField)
	sortBy := func(id, key string) choice {
		return choice{id: id, text: i18n.T(key), on: cfg.Sort == id, change: func(s *data.Settings) { s.Sort = id }}
	}
	sorts := []choice{sortBy(data.SortName, "sort.name"), sortBy(data.SortDate, "sort.date"), sortBy(data.SortSize, "sort.size")}
	b.dropdown("sort", i18n.T("settings.sort"), selectedText(sorts), sorts)
	b.skip(dropdownAfter)
	b.check("sortDesc", i18n.T("settings.sortDesc"), cfg.SortDesc, func(s *data.Settings, v bool) { s.SortDesc = v })
}

func selectedText(choices []choice) string {
	for _, c := range choices {
		if c.on {
			return c.text
		}
	}
	if len(choices) > 0 {
		return choices[0].text
	}
	return ""
}

func (b *builder) formatsSection() {
	b.title(i18n.T(sectionKeys[SectionFormats]))
	b.heading(i18n.T("settings.formats.supported"))
	b.skip(headingToRows)
	for _, f := range b.p.formats {
		b.formatRow(f)
	}
	b.skip(groupGap - formatGap)
	b.heading(i18n.T("settings.defaultApps"))
	b.skip(headingToRows)
	hint := i18n.T("settings.defaultApps.linux")
	if goos == "windows" {
		hint = i18n.T("settings.defaultApps.hint")
	}
	b.note(hint, 0)
	b.skip(4)
	b.button("defaultApps", i18n.T("settings.defaultApps"), b.p.deps.Actions.OpenDefaultApps)
}

func (b *builder) slideshowSection() {
	cfg := b.p.cfg
	b.title(i18n.T(sectionKeys[SectionSlideshow]))
	b.heading(i18n.T("settings.slideshow.interval"))
	b.skip(headingToRows)
	b.slider("slideshow.interval", i18n.T("settings.slideshow.interval"), cfg.Slideshow.Interval, 1, 60, 1, i18n.T("unit.seconds", cfg.Slideshow.Interval),
		func(v int) func(*data.Settings) { return func(s *data.Settings) { s.Slideshow.Interval = v } })
	b.skip(groupGap)
	b.check("slideshow.loop", i18n.T("settings.slideshow.loop"), cfg.Slideshow.Loop, func(s *data.Settings, v bool) { s.Slideshow.Loop = v })
	b.check("slideshow.shuffle", i18n.T("settings.slideshow.shuffle"), cfg.Slideshow.Shuffle, func(s *data.Settings, v bool) { s.Slideshow.Shuffle = v })
}

func (b *builder) performanceSection() {
	cfg := b.p.cfg
	b.title(i18n.T(sectionKeys[SectionPerformance]))
	b.heading(i18n.T("settings.cacheMB"))
	b.skip(headingToRows)
	b.slider("cacheMB", i18n.T("settings.cacheMB"), cfg.CacheMB, 256, 8192, 64, i18n.T("unit.megabytes", cfg.CacheMB),
		func(v int) func(*data.Settings) { return func(s *data.Settings) { s.CacheMB = v } })
	b.note(i18n.T("settings.cacheMB.desc"), 0)
	b.skip(groupGap - 4)
	b.heading(i18n.T("settings.maxPixels"))
	b.skip(headingToRows)
	b.slider("maxPixels", i18n.T("settings.maxPixels"), cfg.MaxPixels, 4, 1000, 4, i18n.T("unit.megapixels", cfg.MaxPixels),
		func(v int) func(*data.Settings) { return func(s *data.Settings) { s.MaxPixels = v } })
	b.skip(groupGap)
	b.heading(i18n.T("settings.decodeThreads"))
	b.skip(headingToRows)
	threads := i18n.T("settings.decodeThreads.auto")
	if cfg.DecodeThreads > 0 {
		threads = strconv.Itoa(cfg.DecodeThreads)
	}
	b.slider("decodeThreads", i18n.T("settings.decodeThreads"), cfg.DecodeThreads, 0, 64, 1, threads,
		func(v int) func(*data.Settings) { return func(s *data.Settings) { s.DecodeThreads = v } })
	b.note(i18n.T("settings.restartNote"), 0)
	b.skip(groupGap - 4)
	b.heading(i18n.T("settings.animationFPS"))
	b.skip(headingToRows)
	b.slider("animationFPS", i18n.T("settings.animationFPS"), cfg.AnimationFPS, 1, 60, 1, i18n.T("unit.fps", cfg.AnimationFPS),
		func(v int) func(*data.Settings) { return func(s *data.Settings) { s.AnimationFPS = v } })
	b.skip(groupGap)
	b.heading(i18n.T("settings.thumbCacheMB"))
	b.skip(headingToRows)
	b.slider("thumbCacheMB", i18n.T("settings.thumbCacheMB"), cfg.ThumbCacheMB, 16, 8192, 16, i18n.T("unit.megabytes", cfg.ThumbCacheMB),
		func(v int) func(*data.Settings) { return func(s *data.Settings) { s.ThumbCacheMB = v } })
	b.note(i18n.T("settings.restartNote"), 0)
	b.skip(8)
	b.value(i18n.T("settings.thumbCache.used", i18n.FormatBytes(b.p.diskBytes)))
	b.button("clearThumbCache", i18n.T("settings.clearThumbCache"), b.p.clearThumbCache)
	if b.p.noted {
		b.skip(4)
		b.note(i18n.T("settings.clearThumbCache.done"), 0)
	}
}

func (p *Page) clearThumbCache() {
	if f := p.deps.Actions.ClearThumbCache; f != nil {
		f()
	}
	p.noted = true
	if p.stopNot != nil {
		p.stopNot()
	}
	p.stopNot = p.after(noteDelay, func() {
		p.noted = false
		p.stopNot = nil
		p.rebuild()
		p.invalidate()
	})
	p.resync()
	p.invalidate()
}

func (b *builder) languageSection() {
	cfg := b.p.cfg
	b.title(i18n.T(sectionKeys[SectionLanguage]))
	b.heading(i18n.T("settings.language"))
	b.skip(headingToRows)
	var entries []choice
	for _, l := range b.p.languageList() {
		id := l.ID
		entries = append(entries, choice{id: id, text: l.Name, on: cfg.Language == id, change: func(s *data.Settings) { s.Language = id }})
	}
	b.radios("language", entries)
	b.skip(4)
	b.note(i18n.T("settings.language.desc"), 0)
	if repo := b.p.about.Repository; repo != "" {
		b.skip(groupGap - 4)
		b.link("translate", i18n.T("settings.language.translate"), "", b.p.openURL(repo))
	}
}

func (p *Page) openURL(url string) func() {
	f := p.deps.Actions.OpenURL
	if f == nil || url == "" {
		return nil
	}
	return func() { f(url) }
}

func (b *builder) aboutSection() {
	a := b.p.about
	b.title(i18n.T(sectionKeys[SectionAbout]))
	version := a.Version
	if a.Revision != "" {
		version += " (" + a.Revision + ")"
	}
	y := b.top()
	b.add(item{kind: kindHeading, text: fitText(aboutFont, i18n.T("app.name"), b.width()), name: "big", r: image.Rect(b.x0, y, b.x1, y+headingHeight+6)})
	b.y += headingHeight + 6 + 6
	b.value(i18n.T("settings.about.version", version))
	if a.Engine != "" {
		b.value(i18n.T("settings.about.engine", a.Engine))
	}
	platform := a.Platform
	if a.Go != "" {
		platform = strings.TrimSpace(platform + " · " + a.Go)
	}
	if platform != "" {
		b.value(i18n.T("settings.about.platform", platform))
	}
	if a.License != "" {
		b.value(a.License)
	} else {
		b.value(i18n.T("settings.about.license"))
	}
	if a.Repository != "" {
		b.skip(groupGap - rowHeight/2)
		b.heading(i18n.T("settings.about.repository"))
		b.skip(headingToRows)
		b.link("repository", a.Repository, "", b.p.openURL(a.Repository))
	}
	b.skip(groupGap)
	b.button("checkUpdates", i18n.T("settings.about.checkUpdates"), b.p.deps.Actions.CheckUpdates)
	b.skip(6)
	last := i18n.T("settings.about.neverChecked")
	if !b.p.lastCheck.IsZero() {
		last = i18n.FormatDateTime(b.p.lastCheck.Local())
	}
	b.note(i18n.T("settings.about.lastCheck", last), 0)
	if len(a.Components) > 0 {
		b.skip(groupGap - 4)
		b.heading(i18n.T("settings.about.codecs"))
		b.skip(headingToRows)
		for i, c := range a.Components {
			b.componentRow(i, c)
		}
	}
	b.skip(groupGap - 4)
	b.note(i18n.T("settings.about.fontLicense", "Open Sans"), 0)
}

func (b *builder) componentRow(i int, c Component) {
	if run := b.p.openURL(c.URL); run != nil {
		b.link("component:"+strconv.Itoa(i), c.Name, c.License, run)
		return
	}
	text := c.Name
	if c.License != "" {
		text += " — " + c.License
	}
	b.value(text)
}
