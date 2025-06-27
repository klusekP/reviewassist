.PHONY: help build run test clean deps install

# Zmienne
BINARY_NAME=codereviewAI
BUILD_DIR=build
MAIN_FILE=main.go

# Domyślny target
help: ## Pokaż pomoc
	@echo "Dostępne komendy:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Instalacja zależności
deps: ## Zainstaluj zależności
	@echo "Instalowanie zależności..."
	go mod tidy
	go mod download

# Budowanie aplikacji
build: deps ## Zbuduj aplikację
	@echo "Budowanie aplikacji..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	@echo "Aplikacja zbudowana: $(BUILD_DIR)/$(BINARY_NAME)"

# Uruchomienie aplikacji
run: ## Uruchom aplikację w trybie deweloperskim
	@echo "Uruchamianie aplikacji..."
	go run $(MAIN_FILE)

# Uruchomienie zbudowanej aplikacji
run-build: build ## Uruchom zbudowaną aplikację
	@echo "Uruchamianie zbudowanej aplikacji..."
	./$(BUILD_DIR)/$(BINARY_NAME)

# Testy
test: ## Uruchom testy
	@echo "Uruchamianie testów..."
	go test -v ./...

# Testy z pokryciem
test-coverage: ## Uruchom testy z pokryciem kodu
	@echo "Uruchamianie testów z pokryciem..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Raport pokrycia: coverage.html"

# Czyszczenie
clean: ## Wyczyść pliki build
	@echo "Czyszczenie..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	go clean

# Instalacja
install: build ## Zainstaluj aplikację systemowo
	@echo "Instalowanie aplikacji..."
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "Aplikacja zainstalowana w /usr/local/bin/$(BINARY_NAME)"

# Docker
docker-build: ## Zbuduj obraz Docker
	@echo "Budowanie obrazu Docker..."
	docker build -t $(BINARY_NAME) .

docker-run: ## Uruchom w kontenerze Docker
	@echo "Uruchamianie w Docker..."
	docker run -p 8080:8080 --env-file .env $(BINARY_NAME)

# Linting
lint: ## Uruchom linter
	@echo "Sprawdzanie kodu..."
	golangci-lint run

# Formatowanie kodu
fmt: ## Sformatuj kod
	@echo "Formatowanie kodu..."
	go fmt ./...

# Analiza kodu
vet: ## Uruchom go vet
	@echo "Analiza kodu..."
	go vet ./...

# Sprawdzenie zależności
check-deps: ## Sprawdź zależności
	@echo "Sprawdzanie zależności..."
	go mod verify
	go list -m all

# Generowanie dokumentacji
docs: ## Wygeneruj dokumentację
	@echo "Generowanie dokumentacji..."
	godoc -http=:6060 &
	@echo "Dokumentacja dostępna pod: http://localhost:6060"

# Monitorowanie
monitor: ## Monitoruj aplikację
	@echo "Monitorowanie aplikacji..."
	@echo "Logi aplikacji:"
	tail -f logs/app.log

# Setup development environment
setup: ## Przygotuj środowisko deweloperskie
	@echo "Przygotowywanie środowiska deweloperskiego..."
	@if [ ! -f .env ]; then \
		echo "Kopiowanie pliku konfiguracyjnego..."; \
		cp env.example .env; \
		echo "Edytuj plik .env i dodaj swoje klucze API"; \
	fi
	@echo "Instalowanie zależności..."
	go mod tidy
	@echo "Środowisko gotowe! Edytuj .env i uruchom 'make run'"

# Backup
backup: ## Utwórz backup projektu
	@echo "Tworzenie backup..."
	tar -czf backup-$(shell date +%Y%m%d-%H%M%S).tar.gz \
		--exclude='.git' \
		--exclude='build' \
		--exclude='*.log' \
		--exclude='coverage.*' \
		.

# Release
release: clean build ## Przygotuj release
	@echo "Przygotowywanie release..."
	@mkdir -p release
	cp $(BUILD_DIR)/$(BINARY_NAME) release/
	cp README.md release/
	cp env.example release/
	@echo "Release gotowy w katalogu release/"

# Development
dev: ## Uruchom w trybie deweloperskim z hot reload
	@echo "Uruchamianie w trybie deweloperskim..."
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "Instalowanie air..."; \
		go install github.com/cosmtrek/air@latest; \
		air; \
	fi 