# --- Этап сборки ---
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Сначала копируем файлы модуля, чтобы слой с зависимостями кешировался
# отдельно. Маска go.* подхватит и go.sum, если он есть.
COPY go.* ./

# Копируем исходники и приводим зависимости в порядок. `go mod tidy`
# гарантирует корректный go.sum, даже если он не был закоммичен.
COPY . .
RUN go mod tidy

# Собираем статический бинарник без CGO ради маленького финального образа.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bot ./cmd/bot

# --- Этап запуска ---
FROM alpine:3.20

# CA-сертификаты нужны для HTTPS-запросов к Telegram и OpenRouter.
RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 appuser

USER appuser
COPY --from=builder /bot /bot

ENTRYPOINT ["/bot"]
