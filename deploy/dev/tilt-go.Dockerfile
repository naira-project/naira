FROM gcr.io/distroless/static-debian12:latest

ARG BINARY_NAME

COPY ${BINARY_NAME} /app

USER nonroot:nonroot

ENTRYPOINT ["/app"]