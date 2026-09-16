// Package i18n is a small locale catalog for the API's error messages and
// the Discord bot's command descriptions.
package i18n

import "golang.org/x/text/language"

// Supported lists this app's locales. The first entry is the default that
// Match and MatchAcceptLanguage fall back to.
var Supported = []language.Tag{
	language.AmericanEnglish,
	language.BrazilianPortuguese,
}

var matcher = language.NewMatcher(Supported)

// Match resolves an arbitrary locale string to one of Supported. Unparseable
// or unrecognized input falls back to Supported[0].
func Match(locale string) language.Tag {
	tag, err := language.Parse(locale)
	if err != nil {
		return Supported[0]
	}

	// Index into Supported, not matcher.Match's own tag: that tag can carry
	// a region extension (e.g. "pt-BR-u-rg-ptzzzz") that Text won't match.
	_, index, _ := matcher.Match(tag)
	return Supported[index]
}

// MatchAcceptLanguage is Match for an HTTP Accept-Language header.
func MatchAcceptLanguage(header string) language.Tag {
	tags, _, err := language.ParseAcceptLanguage(header)
	if err != nil || len(tags) == 0 {
		return Supported[0]
	}

	_, index, _ := matcher.Match(tags...)
	return Supported[index]
}

var catalogs = map[language.Tag]map[string]string{
	language.AmericanEnglish:     enUS,
	language.BrazilianPortuguese: ptBR,
}

// Text resolves key in tag's catalog, falling back to Supported[0]'s catalog
// and finally to key itself.
func Text(tag language.Tag, key string) string {
	if text, ok := catalogs[tag][key]; ok {
		return text
	}
	if text, ok := catalogs[Supported[0]][key]; ok {
		return text
	}
	return key
}
