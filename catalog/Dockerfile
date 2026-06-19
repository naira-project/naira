FROM --platform=$BUILDPLATFORM golang:1.26 AS build

WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY pluginapi ./pluginapi

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build \
    -trimpath \
    -o /out/catalog ./cmd/catalog

FROM gcr.io/distroless/static-debian12

ARG VERSION=dev
ARG COMMIT=unknown
ARG SOURCE=https://github.com/naira-project/naira

LABEL org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.source="${SOURCE}"

COPY --from=build /out/catalog /catalog

EXPOSE 8090

ENTRYPOINT ["/catalog"]