package step

import (
	"bytes"
	"context"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/oops1/PixGo/internal/par"
)

type vkind uint8

const (
	vNull vkind = iota
	vStar
	vNum
	vRef
	vStr
	vEnum
	vList
	vTyped
)

const maxValueDepth = 64

type value struct {
	kind vkind
	num  float64
	ref  int
	text []byte
	list []value
}

type entity struct {
	id    int
	typ   int32
	start int32
	end   int32
}

type part struct {
	name string
	args []value
}

type header struct {
	name   string
	schema string
	system string
}

type stepFile struct {
	data   []byte
	ents   []entity
	dense  []int32
	sparse map[int]int32
	names  []string
	byName map[string]int32
	head   header
	bad    int
}

var (
	magic       = []byte("ISO-10303-21")
	starSlash   = []byte("*/")
	newlineHash = []byte("\n#")
)

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isKeyStart(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '!'
}

func isKeyChar(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

func skipWS(d []byte, i int) int {
	for i < len(d) {
		c := d[i]
		if isSpace(c) {
			i++
			continue
		}
		if c == '/' && i+1 < len(d) && d[i+1] == '*' {
			j := bytes.Index(d[i+2:], starSlash)
			if j < 0 {
				return len(d)
			}
			i += j + 4
			continue
		}
		return i
	}
	return i
}

func skipString(d []byte, i int) int {
	j := i + 1
	for j < len(d) {
		k := bytes.IndexByte(d[j:], '\'')
		if k < 0 {
			return -1
		}
		j += k
		if j+1 < len(d) && d[j+1] == '\'' {
			j += 2
			continue
		}
		return j + 1
	}
	return -1
}

func matchParen(d []byte, i int) int {
	depth := 0
	n := len(d)
	for i < n {
		switch d[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		case '\'':
			j := skipString(d, i)
			if j < 0 {
				return -1
			}
			i = j
			continue
		case '"':
			j := bytes.IndexByte(d[i+1:], '"')
			if j < 0 {
				return -1
			}
			i += j + 2
			continue
		case '/':
			if i+1 < n && d[i+1] == '*' {
				j := bytes.Index(d[i+2:], starSlash)
				if j < 0 {
					return -1
				}
				i += j + 4
				continue
			}
		case ';':
			return -1
		}
		i++
	}
	return -1
}

func resync(d []byte, i int) int {
	i++
	for i < len(d) {
		switch d[i] {
		case ';':
			return i + 1
		case '\'':
			j := skipString(d, i)
			if j < 0 {
				return len(d)
			}
			i = j
			continue
		case '/':
			if i+1 < len(d) && d[i+1] == '*' {
				j := bytes.Index(d[i+2:], starSlash)
				if j < 0 {
					return len(d)
				}
				i += j + 4
				continue
			}
		}
		i++
	}
	return len(d)
}

func readKeyword(d []byte, i int) int {
	if i >= len(d) || !isKeyStart(d[i]) {
		return i
	}
	j := i + 1
	for j < len(d) && isKeyChar(d[j]) {
		j++
	}
	return j
}

func readID(d []byte, i int) (int, int, bool) {
	j := i
	id := 0
	for j < len(d) && isDigit(d[j]) {
		if j-i >= 18 {
			return 0, j, false
		}
		id = id*10 + int(d[j]-'0')
		j++
	}
	return id, j, j > i
}

func Sniff(head []byte) bool {
	i := 0
	if len(head) >= 3 && head[0] == 0xEF && head[1] == 0xBB && head[2] == 0xBF {
		i = 3
	}
	i = skipWS(head, i)
	if !bytes.HasPrefix(head[i:], magic) {
		return false
	}
	i = skipWS(head, i+len(magic))
	return i < len(head) && head[i] == ';'
}

func parseFile(ctx context.Context, data []byte, maxEntities int) (*stepFile, error) {
	f := &stepFile{data: data, byName: make(map[string]int32, 256)}
	i := 0
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		i = 3
	}
	i = skipWS(data, i)
	if !bytes.HasPrefix(data[i:], magic) {
		return nil, corrupt("missing ISO-10303-21 signature")
	}
	i = skipWS(data, i+len(magic))
	if i >= len(data) || data[i] != ';' {
		return nil, corrupt("missing ISO-10303-21 signature")
	}
	i++
	sawData := false
	for i < len(data) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		i = skipWS(data, i)
		if i >= len(data) {
			break
		}
		k := readKeyword(data, i)
		if k == i {
			i = resync(data, i)
			continue
		}
		kw := string(data[i:k])
		switch kw {
		case "HEADER":
			i = f.parseHeader(skipSemicolon(data, k))
		case "DATA":
			j := skipWS(data, k)
			if j < len(data) && data[j] == '(' {
				if e := matchParen(data, j); e > 0 {
					j = e
				}
			}
			var err error
			i, err = f.parseData(ctx, skipSemicolon(data, j), maxEntities)
			if err != nil {
				return nil, err
			}
			sawData = true
		case "END-ISO-10303-21":
			i = len(data)
		default:
			i = skipSection(data, k)
		}
	}
	if !sawData {
		return nil, corrupt("no DATA section")
	}
	f.buildIndex()
	return f, nil
}

func skipSemicolon(d []byte, i int) int {
	j := skipWS(d, i)
	if j < len(d) && d[j] == ';' {
		return j + 1
	}
	return i
}

func skipSection(d []byte, i int) int {
	for i < len(d) {
		i = skipWS(d, i)
		if i >= len(d) {
			return i
		}
		k := readKeyword(d, i)
		if k > i && string(d[i:k]) == "ENDSEC" {
			return skipSemicolon(d, k)
		}
		i = resync(d, i)
	}
	return i
}

func (f *stepFile) parseHeader(i int) int {
	d := f.data
	var origin, preproc string
	for i < len(d) {
		i = skipWS(d, i)
		k := readKeyword(d, i)
		if k == i {
			if i >= len(d) {
				return i
			}
			i = resync(d, i)
			continue
		}
		kw := string(d[i:k])
		if kw == "ENDSEC" {
			i = skipSemicolon(d, k)
			break
		}
		j := skipWS(d, k)
		if j >= len(d) || d[j] != '(' {
			i = resync(d, j)
			continue
		}
		e := matchParen(d, j)
		if e < 0 {
			i = resync(d, j)
			continue
		}
		p := vparser{d: d, i: j, end: e}
		args := p.list()
		if !p.bad {
			switch kw {
			case "FILE_NAME":
				if len(args) > 0 && args[0].kind == vStr {
					f.head.name = decodeString(args[0].text)
				}
				if len(args) > 4 && args[4].kind == vStr {
					preproc = decodeString(args[4].text)
				}
				if len(args) > 5 && args[5].kind == vStr {
					origin = decodeString(args[5].text)
				}
			case "FILE_SCHEMA":
				if len(args) > 0 && args[0].kind == vList && len(args[0].list) > 0 && args[0].list[0].kind == vStr {
					s := decodeString(args[0].list[0].text)
					if c := strings.IndexAny(s, " {"); c >= 0 {
						s = s[:c]
					}
					f.head.schema = strings.TrimSpace(s)
				}
			}
		}
		i = skipSemicolon(d, e)
	}
	origin = strings.TrimSpace(origin)
	preproc = strings.TrimSpace(preproc)
	switch {
	case origin != "" && !strings.EqualFold(origin, "unknown"):
		f.head.system = origin
	case preproc != "" && !strings.EqualFold(preproc, "unknown"):
		f.head.system = preproc
	}
	return i
}

func (f *stepFile) intern(name []byte) int32 {
	if t, ok := f.byName[string(name)]; ok {
		return t
	}
	s := strings.ToUpper(string(name))
	if t, ok := f.byName[s]; ok {
		f.byName[string(name)] = t
		return t
	}
	t := int32(len(f.names))
	f.names = append(f.names, s)
	f.byName[s] = t
	f.byName[string(name)] = t
	return t
}

type chunkLexer struct {
	d      []byte
	ents   []entity
	names  []string
	byName map[string]int32
	bad    int
	endsec bool
}

func (c *chunkLexer) intern(name []byte) int32 {
	if t, ok := c.byName[string(name)]; ok {
		return t
	}
	t := int32(len(c.names))
	c.names = append(c.names, strings.ToUpper(string(name)))
	c.byName[string(name)] = t
	return t
}

func (c *chunkLexer) run(ctx context.Context, i, limit, maxEntities int) (int, error) {
	d := c.d
	for i < len(d) {
		i = skipWS(d, i)
		if i >= limit || i >= len(d) {
			return i, nil
		}
		if d[i] != '#' {
			k := readKeyword(d, i)
			if k > i && string(d[i:k]) == "ENDSEC" {
				c.endsec = true
				return skipSemicolon(d, k), nil
			}
			c.bad++
			i = resync(d, i)
			continue
		}
		id, j, ok := readID(d, i+1)
		if !ok {
			c.bad++
			i = resync(d, i)
			continue
		}
		j = skipWS(d, j)
		if j >= len(d) || d[j] != '=' {
			c.bad++
			i = resync(d, i)
			continue
		}
		j = skipWS(d, j+1)
		if j >= len(d) {
			return j, nil
		}
		var typ int32 = -1
		if d[j] != '(' {
			k := readKeyword(d, j)
			if k == j {
				c.bad++
				i = resync(d, j)
				continue
			}
			typ = c.intern(d[j:k])
			j = skipWS(d, k)
			if j >= len(d) || d[j] != '(' {
				c.bad++
				i = resync(d, i)
				continue
			}
		}
		e := matchParen(d, j)
		if e < 0 {
			c.bad++
			i = resync(d, j)
			continue
		}
		c.ents = append(c.ents, entity{id: id, typ: typ, start: int32(j), end: int32(e)})
		if len(c.ents) > maxEntities {
			return 0, tooLarge("more than %d entities", maxEntities)
		}
		if len(c.ents)&0xffff == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		i = skipSemicolon(d, e)
	}
	return i, nil
}

func (f *stepFile) absorb(c *chunkLexer) {
	remap := make([]int32, len(c.names))
	for k, n := range c.names {
		remap[k] = f.intern([]byte(n))
	}
	for _, e := range c.ents {
		if e.typ >= 0 {
			e.typ = remap[e.typ]
		}
		f.ents = append(f.ents, e)
	}
	f.bad += c.bad
}

func instanceStart(d []byte, i int) bool {
	j := i + 1
	k := j
	for k < len(d) && isDigit(d[k]) {
		k++
	}
	if k == j {
		return false
	}
	for k < len(d) && (d[k] == ' ' || d[k] == '\t') {
		k++
	}
	return k < len(d) && d[k] == '='
}

func (f *stepFile) parseData(ctx context.Context, i int, maxEntities int) (int, error) {
	d := f.data
	workers := par.Count()
	if rest := len(d) - i; rest < 1<<20 || workers < 2 {
		return f.parseDataSeq(ctx, i, len(d), maxEntities)
	}
	workers = min(workers, 32)
	bounds := []int{i}
	for k := 1; k < workers; k++ {
		pos := i + (len(d)-i)*k/workers
		if pos <= bounds[len(bounds)-1] {
			continue
		}
		found := -1
		for p := pos; p < len(d)-1; {
			q := bytes.Index(d[p:], newlineHash)
			if q < 0 {
				break
			}
			cand := p + q + 1
			if instanceStart(d, cand) {
				found = cand
				break
			}
			p = cand
		}
		if found < 0 {
			break
		}
		if found > bounds[len(bounds)-1] {
			bounds = append(bounds, found)
		}
	}
	bounds = append(bounds, len(d))
	n := len(bounds) - 1
	lex := make([]*chunkLexer, n)
	ends := make([]int, n)
	errs := make([]error, n)
	par.Parallel(n, workers, func(k int) {
		c := &chunkLexer{d: d, byName: make(map[string]int32, 128)}
		c.ents = make([]entity, 0, (bounds[k+1]-bounds[k])/48+16)
		lex[k] = c
		ends[k], errs[k] = c.run(ctx, bounds[k], bounds[k+1], maxEntities)
	})
	total := 0
	for k := 0; k < n; k++ {
		if errs[k] != nil {
			return 0, errs[k]
		}
		total += len(lex[k].ents)
		if total > maxEntities {
			return 0, tooLarge("more than %d entities", maxEntities)
		}
		if lex[k].endsec {
			for j := 0; j <= k; j++ {
				f.absorb(lex[j])
			}
			return ends[k], nil
		}
		if k+1 < n && ends[k] != bounds[k+1] {
			return f.parseDataSeq(ctx, i, len(d), maxEntities)
		}
	}
	f.ents = make([]entity, 0, total)
	for _, c := range lex {
		f.absorb(c)
	}
	return ends[n-1], nil
}

func (f *stepFile) parseDataSeq(ctx context.Context, i, limit, maxEntities int) (int, error) {
	c := &chunkLexer{d: f.data, byName: make(map[string]int32, 128)}
	end, err := c.run(ctx, i, limit, maxEntities-len(f.ents))
	if err != nil {
		return 0, err
	}
	f.absorb(c)
	return end, nil
}

func (f *stepFile) buildIndex() {
	maxID := 0
	for _, e := range f.ents {
		if e.id > maxID {
			maxID = e.id
		}
	}
	if maxID < 1<<28 && maxID <= 4*len(f.ents)+1024 {
		f.dense = make([]int32, maxID+1)
		for i := range f.dense {
			f.dense[i] = -1
		}
		for i, e := range f.ents {
			if f.dense[e.id] < 0 {
				f.dense[e.id] = int32(i)
			}
		}
		return
	}
	f.sparse = make(map[int]int32, len(f.ents))
	for i, e := range f.ents {
		if _, ok := f.sparse[e.id]; !ok {
			f.sparse[e.id] = int32(i)
		}
	}
}

func (f *stepFile) lookup(id int) int {
	if f.dense != nil {
		if id < 0 || id >= len(f.dense) {
			return -1
		}
		return int(f.dense[id])
	}
	if i, ok := f.sparse[id]; ok {
		return int(i)
	}
	return -1
}

func (f *stepFile) name(ei int) string {
	if ei < 0 || ei >= len(f.ents) || f.ents[ei].typ < 0 {
		return ""
	}
	return f.names[f.ents[ei].typ]
}

func (f *stepFile) complex(ei int) bool {
	return ei >= 0 && ei < len(f.ents) && f.ents[ei].typ < 0
}

func (f *stepFile) args(ei int) []value {
	if ei < 0 || ei >= len(f.ents) || f.ents[ei].typ < 0 {
		return nil
	}
	e := f.ents[ei]
	p := vparser{d: f.data, i: int(e.start), end: int(e.end)}
	a := p.list()
	if p.bad {
		return nil
	}
	return a
}

func (f *stepFile) parts(ei int) []part {
	if ei < 0 || ei >= len(f.ents) {
		return nil
	}
	e := f.ents[ei]
	if e.typ >= 0 {
		a := f.args(ei)
		if a == nil {
			return nil
		}
		return []part{{name: f.names[e.typ], args: a}}
	}
	d := f.data
	i := int(e.start) + 1
	end := int(e.end) - 1
	var out []part
	for i < end {
		i = skipWS(d[:end], i)
		if i >= end {
			break
		}
		k := readKeyword(d[:end], i)
		if k == i {
			return nil
		}
		nm := strings.ToUpper(string(d[i:k]))
		j := skipWS(d[:end], k)
		if j >= end || d[j] != '(' {
			return nil
		}
		p := vparser{d: d, i: j, end: end}
		a := p.list()
		if p.bad {
			return nil
		}
		out = append(out, part{name: nm, args: a})
		i = p.i
		if len(out) > 64 {
			return nil
		}
	}
	return out
}

func (f *stepFile) partNames(ei int, buf [][]byte) [][]byte {
	buf = buf[:0]
	if ei < 0 || ei >= len(f.ents) || f.ents[ei].typ >= 0 {
		return buf
	}
	e := f.ents[ei]
	d := f.data[:int(e.end)-1]
	i := int(e.start) + 1
	for i < len(d) && len(buf) < 64 {
		i = skipWS(d, i)
		k := readKeyword(d, i)
		if k == i {
			break
		}
		buf = append(buf, d[i:k])
		j := skipWS(d, k)
		if j >= len(d) || d[j] != '(' {
			break
		}
		end := matchParen(d, j)
		if end < 0 {
			break
		}
		i = end
	}
	return buf
}

func (f *stepFile) argsOf(ei int, nm string) []value {
	if !f.complex(ei) {
		if f.name(ei) == nm {
			return f.args(ei)
		}
		return nil
	}
	for _, p := range f.parts(ei) {
		if p.name == nm {
			return p.args
		}
	}
	return nil
}

func (f *stepFile) hasPart(ei int, nm string) bool {
	if !f.complex(ei) {
		return f.name(ei) == nm
	}
	e := f.ents[ei]
	d := f.data[e.start:e.end]
	idx := 0
	for {
		k := bytes.Index(d[idx:], []byte(nm))
		if k < 0 {
			return false
		}
		k += idx
		before := k == 0 || !isKeyChar(d[k-1])
		after := k+len(nm) < len(d) && !isKeyChar(d[k+len(nm)])
		if before && after {
			j := skipWS(d, k+len(nm))
			if j < len(d) && d[j] == '(' {
				return true
			}
		}
		idx = k + len(nm)
	}
}

type vparser struct {
	d     []byte
	i     int
	end   int
	depth int
	bad   bool
}

func (p *vparser) ws() {
	p.i = skipWS(p.d[:p.end], p.i)
}

func (p *vparser) list() []value {
	if p.i >= p.end || p.d[p.i] != '(' {
		p.bad = true
		return nil
	}
	p.i++
	p.depth++
	if p.depth > maxValueDepth {
		p.bad = true
		return nil
	}
	p.ws()
	if p.i < p.end && p.d[p.i] == ')' {
		p.i++
		p.depth--
		return []value{}
	}
	out := make([]value, 0, 4)
	for {
		v := p.value()
		if p.bad {
			return nil
		}
		out = append(out, v)
		p.ws()
		if p.i >= p.end {
			p.bad = true
			return nil
		}
		switch p.d[p.i] {
		case ',':
			p.i++
		case ')':
			p.i++
			p.depth--
			return out
		default:
			p.bad = true
			return nil
		}
	}
}

func (p *vparser) value() value {
	p.ws()
	if p.i >= p.end {
		p.bad = true
		return value{}
	}
	d := p.d
	c := d[p.i]
	switch {
	case c == '$':
		p.i++
		return value{kind: vNull}
	case c == '*':
		p.i++
		return value{kind: vStar}
	case c == '#':
		id, j, ok := readID(d[:p.end], p.i+1)
		if !ok {
			p.bad = true
			return value{}
		}
		p.i = j
		return value{kind: vRef, ref: id}
	case c == '\'':
		j := skipString(d[:p.end], p.i)
		if j < 0 {
			p.bad = true
			return value{}
		}
		v := value{kind: vStr, text: d[p.i+1 : j-1]}
		p.i = j
		return v
	case c == '"':
		k := bytes.IndexByte(d[p.i+1:p.end], '"')
		if k < 0 {
			p.bad = true
			return value{}
		}
		v := value{kind: vStr, text: d[p.i+1 : p.i+1+k]}
		p.i += k + 2
		return v
	case c == '.' && !(p.i+1 < p.end && isDigit(d[p.i+1])):
		k := bytes.IndexByte(d[p.i+1:p.end], '.')
		if k < 0 {
			p.bad = true
			return value{}
		}
		v := value{kind: vEnum, text: d[p.i+1 : p.i+1+k]}
		p.i += k + 2
		return v
	case c == '(':
		l := p.list()
		return value{kind: vList, list: l}
	case c == '+' || c == '-' || c == '.' || isDigit(c):
		j := p.i + 1
		for j < p.end {
			ch := d[j]
			if isDigit(ch) || ch == '.' || ch == 'E' || ch == 'e' || ch == '+' || ch == '-' {
				j++
				continue
			}
			break
		}
		x, ok := parseNum(d[p.i:j])
		if !ok {
			p.bad = true
			return value{}
		}
		p.i = j
		return value{kind: vNum, num: x}
	case isKeyStart(c):
		k := readKeyword(d[:p.end], p.i)
		name := d[p.i:k]
		p.i = skipWS(d[:p.end], k)
		if p.i >= p.end || d[p.i] != '(' {
			p.bad = true
			return value{}
		}
		l := p.list()
		return value{kind: vTyped, text: name, list: l}
	}
	p.bad = true
	return value{}
}

var pow10tab = [...]float64{1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19, 1e20, 1e21, 1e22}

func parseNum(b []byte) (float64, bool) {
	n := len(b)
	i := 0
	neg := false
	if i < n && (b[i] == '+' || b[i] == '-') {
		neg = b[i] == '-'
		i++
	}
	var mant uint64
	sig := 0
	exp := 0
	digits := 0
	for i < n && isDigit(b[i]) {
		dg := uint64(b[i] - '0')
		digits++
		if mant != 0 || dg != 0 {
			if sig < 19 {
				mant = mant*10 + dg
				sig++
			} else {
				exp++
			}
		}
		i++
	}
	if i < n && b[i] == '.' {
		i++
		for i < n && isDigit(b[i]) {
			dg := uint64(b[i] - '0')
			digits++
			if mant != 0 || dg != 0 {
				if sig < 19 {
					mant = mant*10 + dg
					sig++
					exp--
				}
			} else {
				exp--
			}
			i++
		}
	}
	if digits == 0 {
		return 0, false
	}
	if i < n && (b[i] == 'E' || b[i] == 'e') {
		i++
		eneg := false
		if i < n && (b[i] == '+' || b[i] == '-') {
			eneg = b[i] == '-'
			i++
		}
		if i >= n {
			return 0, false
		}
		e := 0
		for i < n && isDigit(b[i]) {
			if e < 100000 {
				e = e*10 + int(b[i]-'0')
			}
			i++
		}
		if eneg {
			e = -e
		}
		exp += e
	}
	if i != n {
		return 0, false
	}
	if mant == 0 {
		if neg {
			return math.Copysign(0, -1), true
		}
		return 0, true
	}
	if sig <= 15 && exp >= -22 && exp <= 22 {
		x := float64(mant)
		if exp < 0 {
			x /= pow10tab[-exp]
		} else {
			x *= pow10tab[exp]
		}
		if neg {
			x = -x
		}
		return x, true
	}
	x, err := strconv.ParseFloat(string(b), 64)
	if err != nil || math.IsInf(x, 0) || math.IsNaN(x) {
		return 0, false
	}
	return x, true
}

func decodeString(raw []byte) string {
	if bytes.IndexByte(raw, '\'') < 0 && bytes.IndexByte(raw, '\\') < 0 {
		return string(raw)
	}
	var sb strings.Builder
	for i := 0; i < len(raw); {
		c := raw[i]
		if c == '\'' && i+1 < len(raw) && raw[i+1] == '\'' {
			sb.WriteByte('\'')
			i += 2
			continue
		}
		if c != '\\' {
			sb.WriteByte(c)
			i++
			continue
		}
		rest := raw[i:]
		switch {
		case bytes.HasPrefix(rest, []byte(`\\`)):
			sb.WriteByte('\\')
			i += 2
		case bytes.HasPrefix(rest, []byte(`\X2\`)):
			j := i + 4
			var units []uint16
			for j+4 <= len(raw) && raw[j] != '\\' {
				v, ok := hexVal(raw[j : j+4])
				if !ok {
					break
				}
				units = append(units, uint16(v))
				j += 4
			}
			sb.WriteString(string(utf16.Decode(units)))
			if bytes.HasPrefix(raw[j:], []byte(`\X0\`)) {
				j += 4
			}
			i = j
		case bytes.HasPrefix(rest, []byte(`\X4\`)):
			j := i + 4
			for j+8 <= len(raw) && raw[j] != '\\' {
				v, ok := hexVal(raw[j : j+8])
				if !ok {
					break
				}
				if r := rune(v); utf8.ValidRune(r) {
					sb.WriteRune(r)
				}
				j += 8
			}
			if bytes.HasPrefix(raw[j:], []byte(`\X0\`)) {
				j += 4
			}
			i = j
		case bytes.HasPrefix(rest, []byte(`\X\`)) && len(rest) >= 5:
			if v, ok := hexVal(rest[3:5]); ok {
				sb.WriteRune(rune(v))
			}
			i += 5
		case bytes.HasPrefix(rest, []byte(`\S\`)) && len(rest) >= 4:
			sb.WriteRune(rune(rest[3]) + 128)
			i += 4
		case len(rest) >= 4 && rest[1] == 'P' && rest[3] == '\\':
			i += 4
		default:
			sb.WriteByte(c)
			i++
		}
	}
	return sb.String()
}

func hexVal(b []byte) (uint32, bool) {
	var v uint32
	for _, c := range b {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= uint32(c - '0')
		case c >= 'A' && c <= 'F':
			v |= uint32(c-'A') + 10
		case c >= 'a' && c <= 'f':
			v |= uint32(c-'a') + 10
		default:
			return 0, false
		}
	}
	return v, true
}
