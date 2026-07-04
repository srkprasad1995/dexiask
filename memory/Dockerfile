# syntax=docker/dockerfile:1

# ---- build stage ---------------------------------------------------------
# mcp-go requires a recent Go toolchain (see go.mod).
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Build the statically-linked server binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/dexiask-memory ./cmd/server

# ---- runtime stage -------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 dexiask
COPY --from=build /out/dexiask-memory /usr/local/bin/dexiask-memory

# The memory volume is mounted at /memory; the runtime user must own it.
RUN mkdir -p /memory && chown dexiask /memory
USER dexiask
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/dexiask-memory"]
