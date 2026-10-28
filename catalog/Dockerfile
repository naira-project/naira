# syntax=docker/dockerfile:1
# --- Stage 1: Build ---
FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS builder

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
FROM gcr.io/distroless/static-debian13:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99

COPY --from=builder /out/catalog /catalog

EXPOSE 8090

USER nonroot:nonroot

ENTRYPOINT ["/catalog"]
