FROM golang:1.23-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-w -s" -o /out/group-fund-bot ./cmd/bot

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -G app app \
    && mkdir -p /app/data \
    && chown -R app:app /app

ENV TZ=Europe/Kyiv
WORKDIR /app

COPY --from=builder /out/group-fund-bot /app/group-fund-bot
COPY --from=builder /src/migrations /app/migrations

RUN chown app:app /app/group-fund-bot /app/migrations
USER app

ENTRYPOINT ["/app/group-fund-bot"]
