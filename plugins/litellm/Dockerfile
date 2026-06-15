FROM golang:1.26 AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# (fixme: libera13): it should not be coupled with catalog code. Can be done after .proto files are with tag on github
COPY go.work go.work.sum ./
COPY catalog/go.mod catalog/go.sum ./catalog/
COPY plugins/go.mod plugins/go.sum ./plugins/

COPY catalog/ ./catalog/
COPY plugins/ ./plugins/

RUN CGO_ENABLED=0 go build -o /out/litellm ./plugins/litellm/*.go

# Use a lightweight base image with cp utility to copy the plugin to the shared volume
FROM alpine:3.18

WORKDIR /app

COPY --from=build /out/litellm /app/litellm

# todo: simplifiy the dockerfiles
# Ensure the binary has executable permissions
RUN chmod +x /app/litellm

ENTRYPOINT ["/app/litellm"]
