# Deployment Guide

## Docker Deployment

### Base Image

```dockerfile
# Multi-stage build
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/server

# Runtime image
FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/server .
COPY ./config.yaml .
COPY ./public ./public

EXPOSE 8080
ENTRYPOINT ["./server"]
```

### docker-compose.yml

```yaml
version: "3.9"

services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - APP_ENV=production
      - APP__DB__HOST=postgres
      - APP__REDIS__ADDR=redis:6379
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_started
    volumes:
      - ./config.yaml:/app/config.yaml
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/ping"]
      interval: 15s
      timeout: 3s
      retries: 3

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: myapp
      POSTGRES_USER: myapp
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U myapp"]
      interval: 5s
      timeout: 3s

  redis:
    image: redis:7-alpine
    volumes:
      - redis-data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]

volumes:
  pgdata:
  redis-data:
```

## Kubernetes Deployment

### deployment.yaml

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  replicas: 3
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
      - name: myapp
        image: myregistry/myapp:latest
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: grpc
        env:
        - name: APP_ENV
          value: "production"
        - name: APP__DB__HOST
          valueFrom:
            configMapKeyRef:
              name: app-config
              key: db_host
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: app-secrets
              key: db_password
        resources:
          requests:
            cpu: 250m
            memory: 256Mi
          limits:
            cpu: 1000m
            memory: 512Mi
        livenessProbe:
          httpGet:
            path: /ping
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 15
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
```

### service.yaml

```yaml
apiVersion: v1
kind: Service
metadata:
  name: myapp
spec:
  selector:
    app: myapp
  ports:
  - port: 80
    targetPort: 8080
    name: http
  - port: 9090
    targetPort: 9090
    name: grpc
  type: ClusterIP
```

### ingress.yaml

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: myapp
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - api.myapp.com
    secretName: myapp-tls
  rules:
  - host: api.myapp.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: myapp
            port:
              number: 80
```

## Helm Chart

Astra provides a pre-built Helm Chart:

```bash
# Use built-in Chart
helm install myapp ./deploy/helm/myapp \
  --set image.tag=v1.0.0 \
  --set replicaCount=5 \
  --set ingress.enabled=true

# Custom values
helm install myapp ./deploy/helm/myapp -f values.prod.yaml
```

## Kustomize

```bash
# Use Kustomize
kubectl apply -k deploy/kustomize/overlays/production
```

## Environment Variable Config

```bash
# Config file
export APP_ENV=production
export APP__SERVER__PORT=8080
export APP__DB__HOST=postgres.internal
export APP__DB__PORT=5432
export APP__DB__USER=myapp
export APP__DB__PASSWORD=****
export APP__REDIS__ADDR=redis.internal:6379
export APP__LOG_LEVEL=info
```

## Graceful Shutdown

Astra has built-in graceful shutdown support:

```go
func main() {
    app := astra.New(
        astra.WithShutdownTimeout(30), // 30 second wait
    )

    // Register resource cleanup hooks
    app.OnStop(func(ctx context.Context) error {
        return db.Close()
    })
    app.OnStop(func(ctx context.Context) error {
        return cache.Close()
    })

    // Auto-triggered on SIGINT/SIGTERM
    app.Run(":8080")
}
```

## CI/CD Example (GitHub Actions)

```yaml
name: Deploy

on:
  push:
    branches: [main]

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4

    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version: "1.25"

    - name: Build
      run: go build -o server ./cmd/server

    - name: Build Docker
      run: docker build -t myapp:${{ github.sha }} .

    - name: Push to Registry
      run: docker push myapp:${{ github.sha }}

    - name: Deploy to K8s
      run: |
        kubectl set image deployment/myapp \
          myapp=myapp:${{ github.sha }}
```
