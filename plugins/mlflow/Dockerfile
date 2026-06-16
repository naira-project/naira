# --- Stage 1: Build ---
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY catalog/pluginapi/ ./catalog/pluginapi/
COPY plugins/mlflow/ ./plugins/mlflow/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /out/mlflow ./plugins/mlflow

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /out/mlflow /mlflow

EXPOSE 50052

USER nonroot:nonroot

ENTRYPOINT ["/mlflow"]