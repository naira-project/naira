# syntax=docker/dockerfile:1
# --- Stage 1: Build ---
FROM golang:1.26.9-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# ARG cannot be declared earlier because BuildKit includes it in the cache key
# and we want to reuse cache across different plugins.
ARG PLUGIN=mcp_servers

COPY plugins/pkg/ ./plugins/pkg/
COPY plugins/internal/ ./plugins/internal/
COPY plugins/cmd/${PLUGIN}/ ./plugins/${PLUGIN}/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/plugin ./plugins/${PLUGIN}

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

COPY --from=builder /out/plugin /plugin

EXPOSE 50051

USER nonroot:nonroot

ENTRYPOINT ["/plugin"]
