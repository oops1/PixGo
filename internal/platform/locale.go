package platform

import "github.com/oops1/PixGo/internal/i18n"

type osLocale struct{}

func NewLocale() Locale { return osLocale{} }

func (osLocale) UILanguage() string { return i18n.SystemLanguage() }

func (osLocale) Region() string { return i18n.SystemRegion() }
