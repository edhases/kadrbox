# Stage 1: Build з підтримкою CGO для tls-client
FROM golang:1.23.6-alpine3.20 AS builder

WORKDIR /app

RUN apk add --no-cache git gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Компіляція з увімкненим CGO для підтримки нативних крипто/TLS бібліотек.
# Будуємо пакет ./cmd/api, а не окремий main.go: тести та артефакт мають однакову
# межу одиниці компіляції, щоб не-експортовані символи з інших файлів пакету
# не ламали збірку.
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o /oxide-server ./cmd/api

# Stage 2: Runtime
FROM alpine:3.20.3

# gcompat необхідний для безпомилкового виконання CGO-бінарників в Alpine musl;
# curl — для HEALTHCHECK.
RUN apk --no-cache add ca-certificates tzdata gcompat curl

# Непривілейований користувач: процес не потребує root, а компромисований контейнер
# не отримаєCapabilities, якими володіє root.
RUN addgroup -S oxide && adduser -S -G oxide -h /app -s /sbin/nologin oxide

WORKDIR /app

COPY --from=builder --chown=oxide:oxide /oxide-server .

USER oxide

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=20s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8080/readyz || exit 1

ENTRYPOINT ["/app/oxide-server"]
