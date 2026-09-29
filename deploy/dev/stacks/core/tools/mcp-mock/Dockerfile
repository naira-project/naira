# --- Stage 1: Build ---
FROM golang:1.26-alpine AS builder

ARG TOOL=deploy/dev/stacks/core/tools/mcp-mock

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY ${TOOL}/ ./tool/

RUN CGO_ENABLED=0 go build -trimpath -o /out/mcp-mock ./tool

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian12:latest

COPY --from=builder /out/mcp-mock /mcp-mock

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/mcp-mock"]
