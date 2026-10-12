package i18n

import (
	"math"
	"math/bits"
	"strconv"
	"strings"
	"time"
)

type Locale struct {
	Name       string
	Decimal    string
	Group      string
	ByteUnits  [5]string
	DateLayout string
	TimeLayout string
}

var (
	Russian = Locale{
		Name: "ru", Decimal: ",", Group: "\xc2\xa0",
		ByteUnits:  [5]string{"Б", "КБ", "МБ", "ГБ", "ТБ"},
		DateLayout: "02.01.2006", TimeLayout: "15:04",
	}
	English = Locale{
		Name: "en", Decimal: ".", Group: ",",
		ByteUnits:  [5]string{"B", "KB", "MB", "GB", "TB"},
		DateLayout: "1/2/2006", TimeLayout: "3:04 PM",
	}
)

func LocaleFor(name string) Locale {
	if Match(name) == "ru" {
		return Russian
	}
	return English
}

func CurrentLocale() Locale { return LocaleFor(Lang()) }

func (l Locale) Number(s string) string {
	sign, rest := "", s
	if strings.HasPrefix(rest, "-") {
		sign, rest = "-", rest[1:]
	}
	mant, exp, hasExp := rest, "", false
	if i := strings.IndexAny(rest, "eE"); i >= 0 {
		mant, exp, hasExp = rest[:i], rest[i+1:], true
	}
	intPart, frac, hasPoint := mant, "", false
	if i := strings.IndexByte(mant, '.'); i >= 0 {
		intPart, frac, hasPoint = mant[:i], mant[i+1:], true
	}
	if intPart == "" || !allDigits(intPart) || !allDigits(frac) {
		return s
	}
	var b strings.Builder
	b.WriteString(sign)
	b.WriteString(Group(intPart, 3, l.Group))
	if hasPoint {
		b.WriteString(l.Decimal)
		b.WriteString(frac)
	}
	if hasExp {
		expSign := "+"
		if strings.HasPrefix(exp, "+") || strings.HasPrefix(exp, "-") {
			expSign, exp = exp[:1], exp[1:]
		}
		if exp == "" || !allDigits(exp) {
			return s
		}
		exp = strings.TrimLeft(exp, "0")
		if exp == "" {
			exp = "0"
		}
		b.WriteString("e" + expSign + exp)
	}
	return b.String()
}

func (l Locale) Text(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if !isDigit(s[i]) && s[i] != '.' {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := scanNumber(s, i)
		if num := s[i:j]; num[0] == '.' {
			b.WriteString(strings.TrimPrefix(l.Number("0"+num), "0"))
		} else {
			b.WriteString(l.Number(num))
		}
		i = j
	}
	return b.String()
}

func (l Locale) Int(n int64) string {
	return l.Number(strconv.FormatInt(n, 10))
}

func (l Locale) Float(f float64, prec int) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return l.Number(strconv.FormatFloat(f, 'f', prec, 64))
}

func (l Locale) Bytes(n int64) string {
	if n < 0 {
		n = 0
	}
	u := uint64(n)
	if u < 1024 {
		return strconv.FormatUint(u, 10) + " " + l.ByteUnits[0]
	}
	unit, div := 1, uint64(1024)
	for unit < len(l.ByteUnits)-1 && u/div >= 1024 {
		unit++
		div <<= 10
	}
	whole, rem := u/div, u%div
	var s string
	switch {
	case whole >= 100:
		s = strconv.FormatUint(whole, 10)
	case whole >= 10:
		s = strconv.FormatUint(whole, 10) + "." + strconv.FormatUint(fraction(rem, div, 10), 10)
	default:
		d := fraction(rem, div, 100)
		s = strconv.FormatUint(whole, 10) + "." + string([]byte{byte('0' + d/10), byte('0' + d%10)})
	}
	return strings.Replace(s, ".", l.Decimal, 1) + " " + l.ByteUnits[unit]
}

func fraction(rem, div, scale uint64) uint64 {
	hi, lo := bits.Mul64(rem, scale)
	q, _ := bits.Div64(hi, lo, div)
	return q
}

func (l Locale) Date(t time.Time) string { return t.Format(l.DateLayout) }

func (l Locale) DateTime(t time.Time) string {
	return t.Format(l.DateLayout + " " + l.TimeLayout)
}

func FormatInt(n int64) string { return CurrentLocale().Int(n) }

func FormatFloat(f float64, prec int) string { return CurrentLocale().Float(f, prec) }

func FormatBytes(n int64) string { return CurrentLocale().Bytes(n) }

func FormatDate(t time.Time) string { return CurrentLocale().Date(t) }

func FormatDateTime(t time.Time) string { return CurrentLocale().DateTime(t) }

func scanNumber(s string, i int) int {
	j := i
	for j < len(s) && isDigit(s[j]) {
		j++
	}
	if j < len(s) && s[j] == '.' {
		j++
		for j < len(s) && isDigit(s[j]) {
			j++
		}
	}
	if j < len(s) && (s[j] == 'e' || s[j] == 'E') {
		k := j + 1
		if k < len(s) && (s[k] == '+' || s[k] == '-') {
			k++
		}
		if k < len(s) && isDigit(s[k]) {
			for k < len(s) && isDigit(s[k]) {
				k++
			}
			j = k
		}
	}
	return j
}

func Group(digits string, n int, sep string) string {
	if n <= 0 || len(digits) <= n {
		return digits
	}
	var b strings.Builder
	head := len(digits) % n
	if head == 0 {
		head = n
	}
	b.WriteString(digits[:head])
	for i := head; i < len(digits); i += n {
		b.WriteString(sep)
		b.WriteString(digits[i : i+n])
	}
	return b.String()
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}
