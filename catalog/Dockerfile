FROM golang:1.26 AS build

WORKDIR /src

COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY pluginapi ./pluginapi

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/catalog ./cmd/catalog

FROM gcr.io/distroless/static-debian12

COPY --from=build /out/catalog /catalog

EXPOSE 8090

ENTRYPOINT ["/catalog"]