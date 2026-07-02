# --- Stage 1: Build ---
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY catalog/internal/ ./catalog/internal/
COPY plugins/pkg/pluginapi/ ./plugins/pkg/pluginapi/
COPY catalog/cmd/catalog/ ./catalog/cmd/catalog/

RUN CGO_ENABLED=0 go build -o /out/catalog ./catalog/cmd/catalog

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /out/catalog /catalog

EXPOSE 8090

USER nonroot:nonroot

ENTRYPOINT ["/catalog"]
