# --- Stage 1: Build ---
FROM golang:1.26.9-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS builder

ARG TOOL=deploy/dev/stacks/core/tools/mcp-mock

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY ${TOOL}/ ./tool/

RUN CGO_ENABLED=0 go build -trimpath -o /out/mcp-mock ./tool

# --- Stage 2: Final ---
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

COPY --from=builder /out/mcp-mock /mcp-mock

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/mcp-mock"]
