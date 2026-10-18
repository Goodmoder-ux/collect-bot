# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/group-fund-bot ./cmd/bot \
    && mkdir -p /out/data

# Static binary with embedded migrations and tzdata; only CA certificates are needed.
FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
LABEL org.opencontainers.image.title="collect-bot" \
      org.opencontainers.image.description="Telegram bot for group collections with Monobank jar payment matching" \
      org.opencontainers.image.source="https://github.com/Goodmoder-ux/collect-bot" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"

ENV DB_PATH=/data/app.db \
    TZ=Europe/Kyiv

COPY --from=builder --chown=nonroot:nonroot /out/group-fund-bot /group-fund-bot
# Pre-create /data so named volumes inherit nonroot ownership.
COPY --from=builder --chown=nonroot:nonroot /out/data /data
VOLUME ["/data"]
USER nonroot:nonroot

HEALTHCHECK --interval=1m --timeout=10s --start-period=30s --retries=3 \
    CMD ["/group-fund-bot", "healthcheck"]

ENTRYPOINT ["/group-fund-bot"]
