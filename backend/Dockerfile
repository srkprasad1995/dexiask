# syntax=docker/dockerfile:1

# ---- build stage ---------------------------------------------------------
FROM golang:1.23-alpine AS build
WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Build the statically-linked server binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/dexiask-server ./cmd/server

# ---- runtime stage -------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 dexiask
COPY --from=build /out/dexiask-server /usr/local/bin/dexiask-server

USER dexiask
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/dexiask-server"]
