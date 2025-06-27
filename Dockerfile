# Multi-stage build dla mniejszego obrazu
FROM golang:1.21-alpine AS builder

# Ustawienie katalogu roboczego
WORKDIR /app

# Kopiowanie plików go mod
COPY go.mod go.sum ./

# Pobieranie zależności
RUN go mod download

# Kopiowanie kodu źródłowego
COPY . .

# Budowanie aplikacji
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o codereviewAI .

# Ostateczny obraz
FROM alpine:latest

# Instalacja ca-certificates dla HTTPS
RUN apk --no-cache add ca-certificates

# Utworzenie użytkownika non-root
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Ustawienie katalogu roboczego
WORKDIR /root/

# Kopiowanie binarnego pliku z builder stage
COPY --from=builder /app/codereviewAI .

# Kopiowanie statycznych plików
COPY --from=builder /app/static ./static

# Zmiana właściciela plików
RUN chown -R appuser:appgroup /root/

# Przełączenie na użytkownika non-root
USER appuser

# Ekspozycja portu
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health || exit 1

# Uruchomienie aplikacji
CMD ["./codereviewAI"] 