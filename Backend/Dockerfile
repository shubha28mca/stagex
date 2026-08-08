# --- Build stage -------------------------------------------------------------
# Compiles a static Linux binary. Using the official Go image keeps the build
# reproducible; the final image below carries only the binary.
FROM golang:1.23-alpine AS build
WORKDIR /src

# Cache dependencies first for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download

# Copy the source and build. CGO is disabled for a fully static binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/stagex-api ./cmd/server

# --- Runtime stage -----------------------------------------------------------
# Distroless-style minimal image: just the binary and CA certs. Runs as non-root.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 appuser
COPY --from=build /out/stagex-api /usr/local/bin/stagex-api
USER appuser
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/stagex-api"]
