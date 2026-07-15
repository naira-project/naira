# --- Stage 1: Build ---
FROM golang:1.26-alpine AS builder

ARG PLUGIN=fluxcd

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY plugins/pkg/ ./plugins/pkg/
COPY plugins/internal/ ./plugins/internal/
COPY plugins/cmd/${PLUGIN}/ ./plugins/${PLUGIN}/

RUN CGO_ENABLED=0 go build -trimpath -o /out/plugin ./plugins/${PLUGIN}

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /out/plugin /plugin

EXPOSE 50051

USER nonroot:nonroot

ENTRYPOINT ["/plugin"]
