# Многостадийная сборка для минимального образа
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Копируем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем статический бинарник
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o app main.go

# Финальный образ
FROM alpine:3.18

# Устанавливаем только необходимые пакеты
RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -g 1001 -S appuser && \
    adduser -u 1001 -S appuser -G appuser

WORKDIR /app

# Копируем бинарник
COPY --from=builder --chown=appuser:appuser /app/app /app/app

# Копируем .env файл если есть
COPY .env /app/.env

USER appuser

EXPOSE 9000

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9000/metrics || exit 1

ENTRYPOINT ["/app/app"]
CMD ["-port", "9000"]