# syntax=docker/dockerfile:1.28@sha256:bb22d9815c728170f72750f4e5b0d672e06176142e1d602c7e66c050100b7e5b
# --- Stage 1: Build the plugin binary ---
FROM golang:1.26.9-alpine3.24@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# ARG cannot be declared earlier because BuildKit includes it in the cache key
# and we want to reuse cache across different plugins.
ARG PLUGIN=github

COPY plugins/pkg/ ./plugins/pkg/
COPY plugins/internal/ ./plugins/internal/
COPY plugins/cmd/${PLUGIN}/ ./plugins/${PLUGIN}/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/plugin ./plugins/${PLUGIN}

# --- Stage 2: Fetch the gh CLI ---
#
# The github plugin shells out to `gh attestation verify`. GitHub ships gh as a fully
# static Go binary (verified against the real v2.63.2 linux_amd64 release.
# `file` report "statically linked"), so it drops straight into distroless image
FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6 AS gh-cli

ARG GH_CLI_VERSION=2.63.2
ARG GH_CLI_SHA256_AMD64=912fdb1ca29cb005fb746fc5d2b787a289078923a29d0f9ec19a0b00272ded00
ARG GH_CLI_SHA256_ARM64=0f31e2a8549c64b5c1679f0b99ce5e0dac7c91da9e86f6246adb8805b0f0b4bb
ARG TARGETARCH=amd64

WORKDIR /gh
# Download GH CLI and verify its integrity against reviewed checksums.
RUN apk add --no-cache curl \
 && curl -sSLO https://github.com/cli/cli/releases/download/v${GH_CLI_VERSION}/gh_${GH_CLI_VERSION}_linux_${TARGETARCH}.tar.gz \
 && case "${TARGETARCH}" in \
      amd64) echo "${GH_CLI_SHA256_AMD64}  gh_${GH_CLI_VERSION}_linux_${TARGETARCH}.tar.gz" | sha256sum -c - ;; \
      arm64) echo "${GH_CLI_SHA256_ARM64}  gh_${GH_CLI_VERSION}_linux_${TARGETARCH}.tar.gz" | sha256sum -c - ;; \
      *) echo "Unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
 && tar xzf gh_${GH_CLI_VERSION}_linux_${TARGETARCH}.tar.gz \
 && cp gh_${GH_CLI_VERSION}_linux_${TARGETARCH}/bin/gh /gh/gh

# --- Stage 3: Final image for the github plugin + gh ---
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

COPY --from=builder /out/plugin /plugin
COPY --from=gh-cli /gh/gh /usr/local/bin/gh

EXPOSE 50051

USER nonroot:nonroot

ENTRYPOINT ["/plugin"]
