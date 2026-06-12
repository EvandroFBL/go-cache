# Stage 1: Build
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Cache dependencies (go.mod/go.sum)
COPY go.mod ./
# No external deps, but this layer is ready if you add some later

# Copy source
COPY *.go ./

# Build a static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o go-cache .

# Stage 2: Minimal runtime
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/go-cache .

EXPOSE 8080

ENV PORT=8080
ENV CLEANUP_INTERVAL=10s
ENV MAX_KEYS=0

ENTRYPOINT ["./go-cache"]
