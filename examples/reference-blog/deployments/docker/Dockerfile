# deployments/docker/Dockerfile
# Multi-stage build: builds all three services from one Dockerfile.
# Usage:
#   docker build -t reference-blog/api-server:latest -f deployments/docker/Dockerfile --target api-server .
#   docker build -t reference-blog/comment-service:latest -f deployments/docker/Dockerfile --target comment-service .
#   docker build -t reference-blog/worker:latest -f deployments/docker/Dockerfile --target worker .

# ── Stage 1: Builder ────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG SERVICE

WORKDIR /src

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files and download
COPY go.mod go.sum* ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" \
    -o /bin/${SERVICE} \
    ./cmd/${SERVICE}

# ── Stage 2: api-server ──────────────────────────────────────────────────────
FROM --platform=$TARGETARCH gcr.io/distroless/static-debian12 AS api-server
COPY --from=builder /bin/api-server /bin/api-server
EXPOSE 8080 9091
ENTRYPOINT ["/bin/api-server"]

# ── Stage 3: comment-service ────────────────────────────────────────────────
FROM --platform=$TARGETARCH gcr.io/distroless/static-debian12 AS comment-service
COPY --from=builder /bin/comment-service /bin/comment-service
EXPOSE 9090 9092
ENTRYPOINT ["/bin/comment-service"]

# ── Stage 4: worker ─────────────────────────────────────────────────────────
FROM --platform=$TARGETARCH gcr.io/distroless/static-debian12 AS worker
COPY --from=builder /bin/worker /bin/worker
ENTRYPOINT ["/bin/worker"]
