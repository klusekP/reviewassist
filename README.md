# Code Review AI

Aplikacja AI do automatycznego code review Merge Requests w GitLab. Aplikacja analizuje zmiany w kodzie za pomocą OpenAI GPT-4 i wyświetla wyniki recenzji w interfejsie webowym.

## Funkcjonalności

- 🔍 **Automatyczna analiza kodu** - AI analizuje zmiany w Merge Requestach
- 🤖 **Inteligentne komentarze** - Wyświetla szczegółowe komentarze do konkretnych linii kodu
- 🛡️ **Bezpieczeństwo** - Sprawdza potencjalne problemy bezpieczeństwa
- ⚡ **Wydajność** - Analizuje optymalizację kodu
- 📝 **Best Practices** - Sprawdza zgodność z dobrymi praktykami
- 📊 **Ocena jakości** - Generuje ocenę jakości kodu (1-10)
- 💾 **Lokalne przechowywanie** - Wyniki recenzji są zapisywane lokalnie w przeglądarce
- 🔒 **Prywatność** - Komentarze nie są automatycznie wysyłane do GitLab

## Wymagania

- Go 1.21+
- GitLab Personal Access Token
- OpenAI API Key

## Instalacja

1. **Sklonuj repozytorium:**
```bash
git clone <repository-url>
cd codereviewAI
```

2. **Zainstaluj zależności:**
```bash
go mod tidy
```

3. **Skonfiguruj zmienne środowiskowe:**
```bash
cp env.example .env
```

Edytuj plik `.env` i dodaj swoje klucze API:
```env
GITLAB_TOKEN=your_gitlab_personal_access_token
GITLAB_URL=https://gitlab.com
GITLAB_PROJECT_ID=123456
OPENAI_API_KEY=your_openai_api_key
PORT=8080
```

4. **Uruchom aplikację:**
```bash
go run main.go
```

## Konfiguracja GitLab

1. **Utwórz Personal Access Token:**
   - Przejdź do GitLab → Settings → Access Tokens
   - Utwórz token z uprawnieniami: `api`, `read_user`, `read_repository`
   - Skopiuj token do zmiennej `GITLAB_TOKEN`

2. **Znajdź Project ID:**
   - Przejdź do swojego projektu w GitLab
   - Project ID znajduje się na stronie głównej projektu
   - Dodaj do zmiennej `GITLAB_PROJECT_ID`

## Konfiguracja OpenAI

1. **Utwórz konto OpenAI:**
   - Przejdź do [OpenAI](https://platform.openai.com/)
   - Utwórz konto i dodaj środki
   - Wygeneruj API Key
   - Dodaj do zmiennej `OPENAI_API_KEY`

## API Endpoints

### Health Check
```http
GET /api/v1/health
```

### Lista Merge Requests
```http
GET /api/v1/merge-requests?state=opened
```

### Szczegóły Merge Request
```http
GET /api/v1/merge-requests/{id}
```

### Automatyczna recenzja (lokalna)
```http
POST /api/v1/merge-requests/{id}/review
```

### Lista projektów
```http
GET /api/v1/projects
```

## Przykład użycia

1. **Pobierz listę otwartych Merge Requests:**
```bash
curl http://localhost:8080/api/v1/merge-requests
```

2. **Przeprowadź automatyczną recenzję:**
```bash
curl -X POST http://localhost:8080/api/v1/merge-requests/123/review
```

## Struktura projektu

```
codereviewAI/
├── main.go              # Główny plik aplikacji
├── services/
│   ├── gitlab.go        # Serwis GitLab API
│   ├── ai.go           # Serwis OpenAI AI
│   └── review.go       # Serwis recenzji
├── static/
│   └── index.html      # Interfejs webowy
├── go.mod              # Zależności Go
├── env.example         # Przykład konfiguracji
└── README.md           # Dokumentacja
```

## Jak to działa

1. **Pobieranie danych:** Aplikacja łączy się z GitLab API i pobiera Merge Request wraz z diff
2. **Analiza AI:** Kod jest analizowany przez OpenAI GPT-4 z kontekstem zmian
3. **Generowanie komentarzy:** AI generuje szczegółowe komentarze do konkretnych linii
4. **Wyświetlanie wyników:** Komentarze są wyświetlane w interfejsie webowym i zapisywane lokalnie

## Typy problemów wykrywane przez AI

- 🐛 **Błędy** - Problemy logiczne i składniowe
- 🛡️ **Bezpieczeństwo** - SQL injection, XSS, etc.
- ⚡ **Wydajność** - Problemy z optymalizacją
- 📝 **Styl** - Zgodność z konwencjami kodowania
- ✅ **Best Practices** - Zgodność z dobrymi praktykami

## Poziomy ważności

- **Low** - Drobne sugestie i ulepszenia
- **Medium** - Problemy, które warto naprawić
- **High** - Ważne problemy wymagające uwagi
- **Critical** - Krytyczne problemy bezpieczeństwa lub funkcjonalności

## Funkcje interfejsu webowego

- **Przeglądanie MR:** Lista wszystkich otwartych Merge Requests
- **Automatyczna recenzja:** Jednoklikowa analiza AI
- **Lokalne przechowywanie:** Wyniki recenzji są zapisywane w przeglądarce
- **Statystyki:** Liczba przejrzanych MR, średnia ocena, liczba problemów
- **Responsywny design:** Działa na urządzeniach mobilnych

## Rozwój

### Dodanie nowych funkcji

1. **Nowe typy analizy** - Dodaj nowe kategorie w `services/ai.go`
2. **Dodatkowe endpointy** - Rozszerz `services/review.go`
3. **Integracje** - Dodaj wsparcie dla innych platform (GitHub, Bitbucket)
4. **Baza danych** - Dodaj trwałe przechowywanie wyników recenzji

### Testowanie

```bash
# Uruchom testy
go test ./...

# Test z przykładowymi danymi
curl http://localhost:8080/api/v1/health
```

## Licencja

MIT License

## Wsparcie

W przypadku problemów:
1. Sprawdź logi aplikacji
2. Zweryfikuj konfigurację zmiennych środowiskowych
3. Sprawdź uprawnienia GitLab token
4. Zweryfikuj saldo OpenAI API

## Roadmap

- [ ] Wsparcie dla GitHub
- [ ] Integracja z Slack/Discord
- [ ] Baza danych dla wyników recenzji
- [ ] Konfiguracja reguł recenzji
- [ ] Historia recenzji
- [ ] Statystyki jakości kodu
- [ ] Eksport wyników do PDF
- [ ] Integracja z CI/CD 