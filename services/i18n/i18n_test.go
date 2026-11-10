package i18n

import (
	"errors"
	"regexp"
	"sort"
	"testing"
)

var verbRe = regexp.MustCompile(`%[-+# 0]*[0-9]*(?:\.[0-9]+)?[a-zA-Z%]`)

func verbs(s string) []string {
	v := verbRe.FindAllString(s, -1)
	sort.Strings(v)
	return v
}

// Every translation must exist in English and use the same format verbs.
func TestCatalogConsistency(t *testing.T) {
	for lang, msgs := range catalog {
		for key, msg := range msgs {
			en, ok := catalog[EN][key]
			if !ok {
				t.Errorf("%s: key %q missing in en", lang, key)
				continue
			}
			if a, b := verbs(msg), verbs(en); len(a) != len(b) || (len(a) > 0 && !equal(a, b)) {
				t.Errorf("%s %q: verbs %v differ from en %v", lang, key, a, b)
			}
		}
	}
	// UI languages must be complete
	for _, lang := range UILanguages {
		for key := range catalog[EN] {
			if _, ok := catalog[lang][key]; !ok {
				t.Errorf("%s: missing translation for %q", lang, key)
			}
		}
	}
	// review output keys must be complete in every review language
	for key := range reviewEN {
		if _, ok := reviewDE[key]; !ok {
			t.Errorf("de: missing translation for %q", key)
		}
	}
}

func equal(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNormalizeAndAcceptLanguage(t *testing.T) {
	for in, want := range map[string]string{"pl": PL, "Polski": PL, "en-US": EN, "de_DE": DE, "fr": "", "": ""} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := FromAcceptLanguage("fr-FR,en;q=0.8,pl;q=0.5", PL); got != EN {
		t.Errorf("FromAcceptLanguage = %q, want en", got)
	}
	if got := FromAcceptLanguage("de-DE", PL); got != PL {
		t.Errorf("de is not a UI language, want default, got %q", got)
	}
}

func TestTranslateAndErrors(t *testing.T) {
	if got := T(DE, "api.error", "x"); got != "Error: x" {
		t.Errorf("de UI message should fall back to en, got %q", got)
	}
	if got := T(PL, "no.such.key"); got != "no.such.key" {
		t.Errorf("missing key = %q", got)
	}
	err := Errorf("settings.err.int", Key("settings.REVIEW_MAX_FILES"))
	if got := Message(PL, err); got != "Maks. plików w review: wymagana liczba całkowita ≥ 0" {
		t.Errorf("pl error = %q", got)
	}
	if got := err.Error(); got != "Max. files per review: a whole number ≥ 0 is required" {
		t.Errorf("en error = %q", got)
	}
	if got := Message(PL, errors.New("plain")); got != "plain" {
		t.Errorf("plain error = %q", got)
	}
}
