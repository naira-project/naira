# --- Stage 1: Build the plugin binary ---
FROM golang:1.26-alpine AS builder

ARG PLUGIN=github

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY plugins/pkg/ ./plugins/pkg/
COPY plugins/internal/ ./plugins/internal/
COPY plugins/cmd/${PLUGIN}/ ./plugins/${PLUGIN}/

RUN CGO_ENABLED=0 go build -trimpath -o /out/plugin ./plugins/${PLUGIN}

# --- Stage 2: Fetch the gh CLI ---
#
# The github plugin shells out to `gh attestation verify`. GitHub ships gh as a fully
# static Go binary (verified against the real v2.63.2 linux_amd64 release.
# `file` report "statically linked"), so it drops straight into distroless image
FROM alpine:3.20 AS gh-cli

ARG GH_CLI_VERSION=2.63.2
ARG TARGETARCH=amd64

WORKDIR /gh
# Download GH CLI and verify file integrity against the downloaded checksums
RUN apk add --no-cache curl \
 && curl -sSLO https://github.com/cli/cli/releases/download/v${GH_CLI_VERSION}/gh_${GH_CLI_VERSION}_linux_${TARGETARCH}.tar.gz \
 && curl -sSLO https://github.com/cli/cli/releases/download/v${GH_CLI_VERSION}/gh_${GH_CLI_VERSION}_checksums.txt \
 && grep " gh_${GH_CLI_VERSION}_linux_${TARGETARCH}.tar.gz\$" gh_${GH_CLI_VERSION}_checksums.txt | sha256sum -c - \
 && tar xzf gh_${GH_CLI_VERSION}_linux_${TARGETARCH}.tar.gz \
 && cp gh_${GH_CLI_VERSION}_linux_${TARGETARCH}/bin/gh /gh/gh

# --- Stage 3: Final image for the github plugin + gh ---
FROM gcr.io/distroless/static-debian13:latest

COPY --from=builder /out/plugin /plugin
COPY --from=gh-cli /gh/gh /usr/local/bin/gh

EXPOSE 50051

USER nonroot:nonroot

ENTRYPOINT ["/plugin"]