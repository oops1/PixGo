package i18n

const (
	PluralOne   = "one"
	PluralFew   = "few"
	PluralMany  = "many"
	PluralOther = "other"
)

func PluralCategory(l string, n int) string {
	if n < 0 {
		n = -n
	}
	switch l {
	case "ru", "uk":
		return eastSlavicCategory(n)
	case "pl":
		if n == 1 {
			return PluralOne
		}
		return slavicFewMany(n)
	case "cs":
		switch {
		case n == 1:
			return PluralOne
		case n >= 2 && n <= 4:
			return PluralFew
		}
		return PluralOther
	case "fr", "pt-BR":
		if n <= 1 {
			return PluralOne
		}
		return PluralOther
	case "tr":
		return PluralOther
	}
	if n == 1 {
		return PluralOne
	}
	return PluralOther
}

func eastSlavicCategory(n int) string {
	if n%10 == 1 && n%100 != 11 {
		return PluralOne
	}
	return slavicFewMany(n)
}

func slavicFewMany(n int) string {
	if m := n % 10; m >= 2 && m <= 4 {
		if h := n % 100; h < 12 || h > 14 {
			return PluralFew
		}
	}
	return PluralMany
}

func Plural(n int, one, few, many string) string {
	switch PluralCategory(Lang(), n) {
	case PluralOne:
		return one
	case PluralFew:
		return few
	}
	return many
}

func PluralKey(prefix string, n int) string {
	switch PluralCategory(Lang(), n) {
	case PluralOne:
		return T(prefix + ".one")
	case PluralFew:
		return T(prefix + ".few")
	}
	return T(prefix + ".many")
}
