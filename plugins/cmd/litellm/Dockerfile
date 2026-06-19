# --- Stage 1: Build ---
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY catalog/pluginapi/ ./catalog/pluginapi/
COPY plugins/internal/ ./plugins/internal/
COPY plugins/cmd/litellm/ ./plugins/litellm/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /out/litellm ./plugins/litellm

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /out/litellm /litellm

EXPOSE 50051

USER nonroot:nonroot

ENTRYPOINT ["/litellm"]