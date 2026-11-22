# Multi-stage image for earnings-sync HTTP API.
# Build: docker build -t earnings-sync:local .
# Run:   docker run --rm -p 8090:8090 -e STORAGE_MODE=sheets-only earnings-sync:local

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/server /app/server
COPY configs /app/configs
COPY migrations /app/migrations
COPY trpc_go.yaml /app/trpc_go.yaml
ENV MIGRATIONS_DIR=/app/migrations
ENV STORAGE_MODE=sheets-only
EXPOSE 8090 8091
USER nobody
ENTRYPOINT ["/app/server"]
CMD ["-conf", "/app/trpc_go.yaml"]
