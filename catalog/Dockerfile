FROM golang:1.26 AS build

WORKDIR /src

COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY pluginapi ./pluginapi

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/catalog ./cmd/catalog

FROM gcr.io/distroless/static-debian12

ARG VERSION=dev
ARG REVISION=unknown
ARG CREATED=unknown
ARG SOURCE=https://github.com/naira-project/naira

LABEL org.opencontainers.image.source="${SOURCE}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${CREATED}"

COPY --from=build /out/catalog /catalog

EXPOSE 8090

ENTRYPOINT ["/catalog"]
