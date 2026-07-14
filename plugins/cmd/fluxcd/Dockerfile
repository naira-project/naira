# --- Stage 1: Build ---
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY plugins/pkg/ ./plugins/pkg/
COPY plugins/internal/ ./plugins/internal/
COPY plugins/cmd/fluxcd/ ./plugins/fluxcd/

RUN CGO_ENABLED=0 go build -trimpath -o /out/fluxcd ./plugins/fluxcd

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /out/fluxcd /fluxcd

EXPOSE 50053

USER nonroot:nonroot

ENTRYPOINT ["/fluxcd"]