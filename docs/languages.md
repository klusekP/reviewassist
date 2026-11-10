# Languages

The application has two independent language settings.

## Interface language – `UI_LANGUAGE`

The language of the web panel, toast notifications, settings labels and API error messages.

- Supported: `pl` (Polish, default) and `en` (English).
- `UI_LANGUAGE` sets the default for everyone (**Settings → Server → Interface language**).
- Each browser can switch it with the **`PL` / `EN` button** in the sidebar status bar. The choice is
  remembered in that browser. Saving a new `UI_LANGUAGE` in Settings resets the browser choice to
  the new default.
- The panel sends the chosen language to the server in the `Accept-Language` header. API clients can
  do the same; without the header the server answers in `UI_LANGUAGE`.

## Review language – `REVIEW_LANGUAGE`

The language of everything the review writes: findings, summaries, fixes, the GitLab summary note,
replies on re-review threads and the summaries stored in the panel.

| Value | Model writes in | Headings and fixed texts |
|---|---|---|
| `pl` (default) | Polish | Polish |
| `en` | English | English |
| `de` | German | German |
| anything else (e.g. `fr`) | that language, as named | English |

Changing `REVIEW_LANGUAGE` affects new reviews only; stored reviews keep their language.

## Adding a translation

Translations live in two files:

- **Backend** – `services/i18n/messages.go`. Maps `reviewXX` (review output) and `uiXX` (API and
  settings) per language. Messages use Go `fmt` verbs (`%s`, `%d`).
- **Frontend** – `static/js/i18n.js`, object `MESSAGES`. Placeholders use `{name}`; plural messages
  are objects keyed by [`Intl.PluralRules`](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Intl/PluralRules)
  categories (`one`, `few`, `many`, `other`).

To add a language, e.g. Czech (`cs`):

1. Add `CS = "cs"` in `services/i18n/i18n.go` and, for the interface, append it to `UILanguages`.
2. Add `reviewCS` and/or `uiCS` maps in `messages.go` and register them in `catalog`.
3. For the interface, add a `cs` block to `MESSAGES`, add `'cs'` to `UI_LANGUAGES` and a locale to
   `DATE_LOCALES` in `static/js/i18n.js`.
4. Optionally add `"cs"` to the `REVIEW_LANGUAGE` options in `services/settings.go` and a name in
   `LanguageName()` in `services/config.go` (used in the model prompt).
5. Run `go test ./...`. The tests fail when a key is missing in a language or when the format verbs
   differ from English.

Missing keys fall back to English, then to the key name, so a partial translation never breaks the
application.
