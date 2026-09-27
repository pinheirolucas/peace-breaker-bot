package i18n

import (
	"testing"

	"golang.org/x/text/language"
)

func TestMatchResolvesPortugueseVariantsToBrazilianPortuguese(t *testing.T) {
	for _, locale := range []string{"pt-BR", "pt", "pt-PT", "pt-AO"} {
		if got := Match(locale); got != language.BrazilianPortuguese {
			t.Errorf("Match(%q) = %v, want %v", locale, got, language.BrazilianPortuguese)
		}
	}
}

func TestMatchFallsBackToEnglish(t *testing.T) {
	for _, locale := range []string{"", "not-a-locale", "ja", "de-DE", "en-GB"} {
		if got := Match(locale); got != language.AmericanEnglish {
			t.Errorf("Match(%q) = %v, want %v", locale, got, language.AmericanEnglish)
		}
	}
}

func TestMatchAcceptLanguageHonoursWeights(t *testing.T) {
	if got := MatchAcceptLanguage("fr;q=0.5, pt-BR;q=0.9"); got != language.BrazilianPortuguese {
		t.Errorf("MatchAcceptLanguage = %v, want %v", got, language.BrazilianPortuguese)
	}
	if got := MatchAcceptLanguage(""); got != language.AmericanEnglish {
		t.Errorf("MatchAcceptLanguage(empty) = %v, want %v", got, language.AmericanEnglish)
	}
}

func TestTextResolvesPerLocale(t *testing.T) {
	if got := Text(language.BrazilianPortuguese, "invalid_body"); got != "O bot não entendeu o pedido" {
		t.Errorf("Text(pt-BR, invalid_body) = %q", got)
	}
	if got := Text(language.AmericanEnglish, "invalid_body"); got != "The bot couldn't understand the request" {
		t.Errorf("Text(en-US, invalid_body) = %q", got)
	}
}

func TestTextFallsBackToEnglishThenToTheKeyItself(t *testing.T) {
	// A locale this catalog doesn't have an entry for at all falls to English.
	if got := Text(language.Japanese, "invalid_body"); got != "The bot couldn't understand the request" {
		t.Errorf("Text(ja, invalid_body) = %q, want the English fallback", got)
	}

	// A key that exists nowhere surfaces as itself, not a blank string.
	if got := Text(language.AmericanEnglish, "not_a_real_key"); got != "not_a_real_key" {
		t.Errorf("Text(en-US, not_a_real_key) = %q, want the key itself", got)
	}
}

func TestBothCatalogsCarryTheSameKeys(t *testing.T) {
	if len(enUS) != len(ptBR) {
		t.Fatalf("enUS has %d keys, ptBR has %d", len(enUS), len(ptBR))
	}
	for key := range enUS {
		if _, ok := ptBR[key]; !ok {
			t.Errorf("ptBR is missing key %q", key)
		}
	}
}
