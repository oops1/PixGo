package commands

import (
	"sort"
	"strings"

	"github.com/oops1/PixGo/internal/i18n"
)

type ID string

type Mod uint8

const (
	Ctrl Mod = 1 << iota
	Shift
	Alt
)

type Key int

const (
	KeyBackspace Key = 0x08
	KeyTab       Key = 0x09
	KeyEnter     Key = 0x0D
	KeyEscape    Key = 0x1B
	KeySpace     Key = 0x20
	KeyPageUp    Key = 0x21
	KeyPageDown  Key = 0x22
	KeyEnd       Key = 0x23
	KeyHome      Key = 0x24
	KeyLeft      Key = 0x25
	KeyUp        Key = 0x26
	KeyRight     Key = 0x27
	KeyDown      Key = 0x28
	KeyInsert    Key = 0x2D
	KeyDelete    Key = 0x2E
	KeyNumAdd    Key = 0x6B
	KeyNumSub    Key = 0x6D
	KeyF1        Key = 0x70
	KeyF3        Key = 0x72
	KeyF5        Key = 0x74
	KeyF6        Key = 0x75
	KeyF10       Key = 0x79
	KeyF11       Key = 0x7A
	KeyPlus      Key = 0xBB
	KeyComma     Key = 0xBC
	KeyMinus     Key = 0xBD
)

type Chord struct {
	Key Key
	Mod Mod
}

func C(mod Mod, key Key) Chord { return Chord{key, mod} }

func (c Chord) String() string {
	var b strings.Builder
	if c.Mod&Ctrl != 0 {
		b.WriteString("Ctrl+")
	}
	if c.Mod&Alt != 0 {
		b.WriteString("Alt+")
	}
	if c.Mod&Shift != 0 {
		b.WriteString("Shift+")
	}
	b.WriteString(KeyName(c.Key))
	return b.String()
}

func KeyName(k Key) string {
	switch {
	case k >= 'A' && k <= 'Z', k >= '0' && k <= '9':
		return string(rune(k))
	case k >= KeyF1 && k <= KeyF1+23:
		return "F" + itoa(int(k-KeyF1)+1)
	}
	switch k {
	case KeyPlus, KeyNumAdd:
		return i18n.T("key.plus")
	case KeyMinus, KeyNumSub:
		return i18n.T("key.minus")
	case KeyComma:
		return ","
	case KeyDelete:
		return "Del"
	case KeyInsert:
		return "Ins"
	case KeyTab:
		return "Tab"
	case KeyEnter:
		return "Enter"
	case KeyEscape:
		return "Esc"
	case KeySpace:
		return i18n.T("key.space")
	case KeyLeft:
		return i18n.T("key.left")
	case KeyRight:
		return i18n.T("key.right")
	case KeyUp:
		return i18n.T("key.up")
	case KeyDown:
		return i18n.T("key.down")
	case KeyHome:
		return "Home"
	case KeyEnd:
		return "End"
	case KeyPageUp:
		return "PgUp"
	case KeyPageDown:
		return "PgDn"
	case KeyBackspace:
		return "Backspace"
	}
	return "?"
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

type Command struct {
	ID    ID
	Title string
	Icon  string
	Keys  []Chord

	Display string

	Enabled func() bool

	Checked func() bool
	Run     func()
}

func (c *Command) Label() string { return i18n.T(c.Title) }

func (c *Command) Shortcut() string {
	if c.Display != "" {
		return c.Display
	}
	if len(c.Keys) == 0 {
		return ""
	}
	return c.Keys[0].String()
}

func (c *Command) IsEnabled() bool { return c.Run != nil && (c.Enabled == nil || c.Enabled()) }

func (c *Command) IsChecked() bool { return c.Checked != nil && c.Checked() }

type Registry struct {
	cmds map[ID]*Command
	keys map[Chord]ID
}

func NewRegistry() *Registry {
	return &Registry{cmds: map[ID]*Command{}, keys: map[Chord]ID{}}
}

func (r *Registry) Add(c Command) *Command {
	cp := c
	r.cmds[c.ID] = &cp
	for _, k := range c.Keys {
		r.keys[k] = c.ID
	}
	return &cp
}

func (r *Registry) Get(id ID) *Command { return r.cmds[id] }

func (r *Registry) Run(id ID) bool {
	c := r.cmds[id]
	if c == nil || !c.IsEnabled() {
		return false
	}
	c.Run()
	return true
}

func (r *Registry) ForChord(ch Chord) *Command {
	if id, ok := r.keys[ch]; ok {
		return r.cmds[id]
	}
	return nil
}

func (r *Registry) IDs() []ID {
	out := make([]ID, 0, len(r.cmds))
	for id := range r.cmds {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

type Item struct {
	Cmd       ID
	Separator bool
	Title     string
	Icon      string
	Sub       []Item

	Dynamic func() []Item

	Label string
	Run   func()

	Strip []int
}

var Sep = Item{Separator: true}

func Cmd(id ID) Item { return Item{Cmd: id} }
