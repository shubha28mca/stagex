# --- Build stage ---
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/stagex-admin-api ./cmd/server

# --- Runtime stage ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 appuser \
    && mkdir -p /data/media && chown -R appuser:appuser /data
COPY --from=build /out/stagex-admin-api /usr/local/bin/stagex-admin-api
USER appuser
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/stagex-admin-api"]
