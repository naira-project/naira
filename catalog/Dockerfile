# syntax=docker/dockerfile:1.28@sha256:bb22d9815c728170f72750f4e5b0d672e06176142e1d602c7e66c050100b7e5b
# --- Stage 1: Build ---
FROM golang:1.26.9-alpine3.24@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS builder

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
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

COPY --from=builder /out/catalog /catalog

EXPOSE 8090

USER nonroot:nonroot

ENTRYPOINT ["/catalog"]
