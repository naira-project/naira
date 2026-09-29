# syntax=docker/dockerfile:1
# --- Stage 1: Build ---
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# ARG cannot be declared earlier because BuildKit includes it in the cache key
# and we want to reuse cache across different plugins.
ARG PLUGIN=openmetadata

COPY plugins/pkg/ ./plugins/pkg/
COPY plugins/internal/ ./plugins/internal/
COPY plugins/cmd/${PLUGIN}/ ./plugins/${PLUGIN}/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o /out/plugin ./plugins/${PLUGIN}

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /out/plugin /plugin

EXPOSE 50051

USER nonroot:nonroot

ENTRYPOINT ["/plugin"]
