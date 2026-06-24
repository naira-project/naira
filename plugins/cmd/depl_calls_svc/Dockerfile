# --- Stage 1: Build ---
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY catalog/pluginapi/ ./catalog/pluginapi/
COPY plugins/internal/ ./plugins/internal/
COPY plugins/cmd/depl_calls_svc/ ./plugins/depl_calls_svc/

RUN CGO_ENABLED=0 go build -o /out/depl_calls_svc ./plugins/depl_calls_svc

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /out/depl_calls_svc /depl_calls_svc

EXPOSE 50053

USER nonroot:nonroot

ENTRYPOINT ["/depl_calls_svc"]