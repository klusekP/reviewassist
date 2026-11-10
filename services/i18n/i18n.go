// Package i18n holds the translated messages of the application.
//
// Two languages are in play:
//   - the UI language (UI_LANGUAGE, or the Accept-Language header sent by the web UI) for
//     API errors, connection test results and settings labels;
//   - the review language (REVIEW_LANGUAGE) for everything written into reviews and GitLab.
//
// Messages are looked up by key; a missing translation falls back to English, then to the key.
package i18n

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

const (
	PL = "pl"
	EN = "en"
	DE = "de"

	Default = PL // language used when nothing else is configured
)

// UILanguages are the languages the web interface is translated to.
var UILanguages = []string{PL, EN}

// Normalize maps a language name or tag ("Polski", "en-US", "de_DE") to a supported code, or "".
func Normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "polish", "polski":
		return PL
	case "english":
		return EN
	case "german", "deutsch":
		return DE
	}
	if i := strings.IndexAny(s, "-_"); i > 0 {
		s = s[:i]
	}
	if _, ok := catalog[s]; ok {
		return s
	}
	return ""
}

// Or returns the normalized language or def when lang is not supported.
func Or(lang, def string) string {
	if l := Normalize(lang); l != "" {
		return l
	}
	return def
}

// UILanguage is the default UI language from the UI_LANGUAGE setting.
func UILanguage() string {
	return Or(os.Getenv("UI_LANGUAGE"), Default)
}

// FromAcceptLanguage returns the first UI language listed in an Accept-Language header, or def.
func FromAcceptLanguage(header, def string) string {
	for part := range strings.SplitSeq(header, ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if l := Normalize(tag); l != "" && isUILanguage(l) {
			return l
		}
	}
	return def
}

func isUILanguage(l string) bool { return slices.Contains(UILanguages, l) }

// Key is a message key passed as an argument to T; it is translated before formatting.
type Key string

// Has reports whether the key exists in the English catalog.
func Has(key string) bool {
	_, ok := catalog[EN][key]
	return ok
}

// T returns the message for key in lang, formatted with args (fmt verbs).
func T(lang, key string, args ...any) string {
	msg, ok := catalog[Or(lang, Default)][key]
	if !ok {
		if msg, ok = catalog[EN][key]; !ok {
			msg = key
		}
	}
	if len(args) == 0 {
		return msg
	}
	for i, a := range args {
		switch v := a.(type) {
		case Key:
			args[i] = T(lang, string(v))
		case error:
			args[i] = Message(lang, v)
		}
	}
	return fmt.Sprintf(msg, args...)
}

// Error is an error whose text is translated when shown to the user.
type Error struct {
	Key  string
	Args []any
}

func (e *Error) Error() string { return T(EN, e.Key, append([]any(nil), e.Args...)...) }

// Errorf creates a translatable error.
func Errorf(key string, args ...any) error { return &Error{Key: key, Args: args} }

// Message renders err in lang when it is translatable, otherwise returns its text.
func Message(lang string, err error) string {
	var e *Error
	if errors.As(err, &e) {
		return T(lang, e.Key, append([]any(nil), e.Args...)...)
	}
	return err.Error()
}
