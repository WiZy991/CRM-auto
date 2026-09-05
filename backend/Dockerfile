# Multi-stage build: API, worker и migrate в одном образе.
# Профиль docker compose full использует этот Dockerfile для api и worker.

FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 GOOS=linux

RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
 && go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 app

WORKDIR /app

COPY --from=builder /out/api /app/api
COPY --from=builder /out/worker /app/worker
COPY --from=builder /out/migrate /app/migrate
COPY --from=builder /out/seed /app/seed

RUN mkdir -p /app/storage/uploads \
 && chown -R app:app /app

USER app

EXPOSE 8080

CMD ["/app/api"]
