# syntax=docker/dockerfile:1
# --- Stage 1: Build ---
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

WORKDIR /app

COPY go.mod go.sum ./
# Cache mounts: BuildKit persists these across this run's sequential image
# builds (catalog + each plugin), so shared packages/deps aren't recompiled
# from scratch for every image.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY catalog/internal/ ./catalog/internal/
COPY plugins/pkg/pluginapi/ ./plugins/pkg/pluginapi/
COPY catalog/cmd/catalog/ ./catalog/cmd/catalog/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/catalog ./catalog/cmd/catalog

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian13:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5

COPY --from=builder /out/catalog /catalog

EXPOSE 8090

USER nonroot:nonroot

ENTRYPOINT ["/catalog"]
