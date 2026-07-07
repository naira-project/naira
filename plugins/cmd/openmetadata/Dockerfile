# --- Stage 1: Build ---
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY plugins/pkg/ ./plugins/pkg/
COPY plugins/internal/ ./plugins/internal/
COPY plugins/cmd/openmetadata/ ./plugins/openmetadata/

RUN CGO_ENABLED=0 go build -o /out/openmetadata ./plugins/openmetadata

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /out/openmetadata /openmetadata

EXPOSE 50055

USER nonroot:nonroot

ENTRYPOINT ["/openmetadata"]
