# Stage 1: Build з підтримкою CGO для tls-client
FROM golang:1.23-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Компіляція з увімкненим CGO для підтримки нативних крипто/TLS бібліотек
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o /oxide-server ./cmd/api/main.go

# Stage 2: Runtime
FROM alpine:3.20

# gcompat необхідний для безпомилкового виконання CGO-бінарників в Alpine musl
RUN apk --no-cache add ca-certificates tzdata gcompat

WORKDIR /root/

COPY --from=builder /oxide-server .

EXPOSE 8080

ENTRYPOINT ["./oxide-server"]
