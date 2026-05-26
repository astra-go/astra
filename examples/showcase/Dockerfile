# syntax=docker/dockerfile:1

# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /workspace

# Copy the entire monorepo so local replace directives resolve.
COPY . .

WORKDIR /workspace/examples/showcase

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/api     ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/grpc    ./cmd/grpc
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/worker  ./cmd/worker

# ── Runtime stages ────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12 AS api
COPY --from=builder /out/api /api
COPY --from=builder /workspace/examples/showcase/config /config
ENTRYPOINT ["/api"]

FROM gcr.io/distroless/static-debian12 AS grpc
COPY --from=builder /out/grpc /grpc
COPY --from=builder /workspace/examples/showcase/config /config
ENTRYPOINT ["/grpc"]

FROM gcr.io/distroless/static-debian12 AS worker
COPY --from=builder /out/worker /worker
ENTRYPOINT ["/worker"]
