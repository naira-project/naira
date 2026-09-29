# syntax=docker/dockerfile:1
# --- Stage 1: Build ---
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
# Cache mounts: BuildKit persists these across this run's sequential image
# builds (catalog + each plugin), so shared packages/deps aren't recompiled
# from scratch for every image.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# ARG declared here, not before go mod download: BuildKit's cache key for a
# RUN includes the build-arg environment in scope at that point, so an ARG
# declared earlier — even unused by go mod download — busts that layer's
# cache across different plugin builds despite identical go.mod/go.sum,
# forcing a redundant full module download per plugin image.
ARG PLUGIN=mcp_servers

COPY plugins/pkg/ ./plugins/pkg/
COPY plugins/internal/ ./plugins/internal/
COPY plugins/cmd/${PLUGIN}/ ./plugins/${PLUGIN}/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/plugin ./plugins/${PLUGIN}

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /out/plugin /plugin

EXPOSE 50051

USER nonroot:nonroot

ENTRYPOINT ["/plugin"]
