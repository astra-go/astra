# Kubernetes Deployment Guide for Astra Showcase

Production-ready Kubernetes deployment for the Astra Showcase reference application.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [Deployment Steps](#deployment-steps)
- [Configuration](#configuration)
- [Scaling](#scaling)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)
- [Production Checklist](#production-checklist)

## Prerequisites

### Required

- Kubernetes 1.24+ cluster
- kubectl configured to access your cluster
- PostgreSQL database (external or in-cluster)
- Redis cache (external or in-cluster)

### Recommended

- Ingress controller (nginx-ingress or traefik)
- cert-manager for TLS certificates
- Prometheus Operator for monitoring
- Horizontal Pod Autoscaler metrics server

### Installation

```bash
# Install nginx-ingress controller
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.1/deploy/static/provider/cloud/deploy.yaml

# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml

# Install Prometheus Operator (if using Helm)
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus-community/kube-prometheus-stack -n monitoring --create-namespace

# Verify installations
kubectl get pods -n ingress-nginx
kubectl get pods -n cert-manager
kubectl get pods -n monitoring
```

## Quick Start

```bash
# 1. Create namespace and secrets
kubectl apply -f 00-namespace.yaml

# 2. Update secrets with actual values
kubectl edit secret showcase-secrets -n showcase

# 3. Deploy applications
kubectl apply -f 01-api-deployment.yaml
kubectl apply -f 02-grpc-deployment.yaml
kubectl apply -f 03-worker-deployment.yaml

# 4. Deploy ingress
kubectl apply -f 04-ingress.yaml

# 5. Deploy monitoring (optional)
kubectl apply -f 05-monitoring.yaml

# 6. Verify deployment
kubectl get pods -n showcase
kubectl get svc -n showcase
kubectl get ingress -n showcase
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                             │
│                                                                 │
│  ┌─────────────────┐                                           │
│  │  Ingress Nginx  │                                           │
│  │  (TLS/HTTPS)    │                                           │
│  └────────┬────────┘                                           │
│           │                                                     │
│     ┌─────┴──────┬──────────────┬─────────────┐              │
│     │            │              │             │              │
│  ┌──▼──────┐  ┌─▼────────┐  ┌─▼───────┐  ┌─▼────────┐      │
│  │ API Pod │  │ API Pod  │  │ gRPC    │  │ gRPC     │      │
│  │ (3x)    │  │          │  │ Pod     │  │ Pod      │      │
│  └──┬──────┘  └─┬────────┘  └─┬───────┘  └─┬────────┘      │
│     │           │              │             │              │
│     └───────────┴──────┬───────┴─────────────┘              │
│                        │                                      │
│                 ┌──────▼──────┐                              │
│                 │  PostgreSQL │                              │
│                 │  (External) │                              │
│                 └─────────────┘                              │
│                                                              │
│  ┌────────────┐                                             │
│  │ Worker Pod │                                             │
│  │ (2x)       │──► Task Queue (Redis/Kafka)                │
│  └────────────┘                                             │
│                                                              │
│  ┌────────────────────────────────────────┐                │
│  │  Prometheus + Grafana (Monitoring)     │                │
│  └────────────────────────────────────────┘                │
└─────────────────────────────────────────────────────────────┘
```

## Deployment Steps

### Step 1: Prepare Database

**Option A: External Managed Database** (Recommended for production)

```bash
# AWS RDS / Google Cloud SQL / Azure Database
# Get connection string and update secret

kubectl create secret generic showcase-secrets \
  --from-literal=database-url='postgres://user:pass@rds-endpoint:5432/showcase?sslmode=require' \
  --namespace=showcase
```

**Option B: In-Cluster PostgreSQL** (Development only)

```yaml
# Save as postgres-statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
  namespace: showcase
spec:
  serviceName: postgres
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgres:16-alpine
        env:
        - name: POSTGRES_DB
          value: showcase
        - name: POSTGRES_USER
          value: showcase
        - name: POSTGRES_PASSWORD
          value: showcase
        ports:
        - containerPort: 5432
        volumeMounts:
        - name: data
          mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: postgres-service
  namespace: showcase
spec:
  ports:
  - port: 5432
  selector:
    app: postgres
```

### Step 2: Update Secrets

```bash
# Generate JWT secret
JWT_SECRET=$(openssl rand -base64 32)

# Update secret
kubectl create secret generic showcase-secrets \
  --from-literal=database-url='postgres://...' \
  --from-literal=jwt-secret="$JWT_SECRET" \
  --from-literal=google-client-id='YOUR_GOOGLE_CLIENT_ID' \
  --from-literal=google-client-secret='YOUR_GOOGLE_SECRET' \
  --from-literal=github-client-id='YOUR_GITHUB_CLIENT_ID' \
  --from-literal=github-client-secret='YOUR_GITHUB_SECRET' \
  --namespace=showcase \
  --dry-run=client -o yaml | kubectl apply -f -
```

### Step 3: Build and Push Docker Images

```bash
cd examples/showcase

# Build images
docker build -t your-registry/showcase-api:v1.0.0 -f Dockerfile.api .
docker build -t your-registry/showcase-grpc:v1.0.0 -f Dockerfile.grpc .
docker build -t your-registry/showcase-worker:v1.0.0 -f Dockerfile.worker .

# Push to registry
docker push your-registry/showcase-api:v1.0.0
docker push your-registry/showcase-grpc:v1.0.0
docker push your-registry/showcase-worker:v1.0.0

# Update image references in YAML files
sed -i '' 's|astra/showcase-api:latest|your-registry/showcase-api:v1.0.0|g' deploy/kubernetes/*.yaml
sed -i '' 's|astra/showcase-grpc:latest|your-registry/showcase-grpc:v1.0.0|g' deploy/kubernetes/*.yaml
sed -i '' 's|astra/showcase-worker:latest|your-registry/showcase-worker:v1.0.0|g' deploy/kubernetes/*.yaml
```

### Step 4: Deploy Applications

```bash
# Deploy in order (dependencies first)
kubectl apply -f deploy/kubernetes/00-namespace.yaml
kubectl apply -f deploy/kubernetes/01-api-deployment.yaml
kubectl apply -f deploy/kubernetes/02-grpc-deployment.yaml
kubectl apply -f deploy/kubernetes/03-worker-deployment.yaml
kubectl apply -f deploy/kubernetes/04-ingress.yaml
kubectl apply -f deploy/kubernetes/05-monitoring.yaml

# Wait for pods to be ready
kubectl wait --for=condition=ready pod -l app=showcase -n showcase --timeout=300s
```

### Step 5: Verify Deployment

```bash
# Check pods
kubectl get pods -n showcase

# Expected output:
# NAME                             READY   STATUS    RESTARTS   AGE
# showcase-api-xxxxxxxxx-xxxxx     1/1     Running   0          2m
# showcase-api-xxxxxxxxx-xxxxx     1/1     Running   0          2m
# showcase-api-xxxxxxxxx-xxxxx     1/1     Running   0          2m
# showcase-grpc-xxxxxxxxx-xxxxx    1/1     Running   0          2m
# showcase-grpc-xxxxxxxxx-xxxxx    1/1     Running   0          2m
# showcase-worker-xxxxxxxxx-xxxxx  1/1     Running   0          2m

# Check services
kubectl get svc -n showcase

# Check ingress
kubectl get ingress -n showcase

# Test health endpoint
kubectl port-forward -n showcase svc/showcase-api 8080:80
curl http://localhost:8080/health
```

## Configuration

### Environment Variables

All configuration is managed via ConfigMap and Secrets:

**ConfigMap** (`showcase-config`):
- `REDIS_ADDR` - Redis connection string
- `OTEL_EXPORTER_OTLP_ENDPOINT` - OpenTelemetry collector endpoint
- `HTTP_ADDR` - HTTP listen address
- `GRPC_ADDR` - gRPC listen address
- `LOG_LEVEL` - Logging level (debug/info/warn/error)
- `LOG_FORMAT` - Log format (json/text)

**Secret** (`showcase-secrets`):
- `database-url` - PostgreSQL connection string
- `jwt-secret` - JWT signing key
- `google-client-id` - Google OAuth2 client ID
- `google-client-secret` - Google OAuth2 secret
- `github-client-id` - GitHub OAuth2 client ID
- `github-client-secret` - GitHub OAuth2 secret

### Update Configuration

```bash
# Edit ConfigMap
kubectl edit configmap showcase-config -n showcase

# Edit Secret
kubectl edit secret showcase-secrets -n showcase

# Restart pods to pick up changes
kubectl rollout restart deployment/showcase-api -n showcase
kubectl rollout restart deployment/showcase-grpc -n showcase
kubectl rollout restart deployment/showcase-worker -n showcase
```

## Scaling

### Manual Scaling

```bash
# Scale API pods
kubectl scale deployment showcase-api --replicas=5 -n showcase

# Scale gRPC pods
kubectl scale deployment showcase-grpc --replicas=4 -n showcase

# Scale workers
kubectl scale deployment showcase-worker --replicas=3 -n showcase
```

### Horizontal Pod Autoscaler (HPA)

HPA is configured in the deployment files and automatically scales based on:

**API Pods**:
- Min: 2, Max: 10
- Target CPU: 70%
- Target Memory: 80%

**gRPC Pods**:
- Min: 2, Max: 6
- Target CPU: 70%

**Worker Pods**:
- Min: 1, Max: 5
- Target CPU: 75%
- Target Memory: 85%

```bash
# Check HPA status
kubectl get hpa -n showcase

# View HPA details
kubectl describe hpa showcase-api-hpa -n showcase
```

### Vertical Pod Autoscaler (VPA)

```yaml
# Save as vpa.yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: showcase-api-vpa
  namespace: showcase
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: showcase-api
  updatePolicy:
    updateMode: "Auto"
```

## Monitoring

### Prometheus Metrics

Metrics are exposed on `/metrics` endpoint:

- **API**: `http://showcase-api:8080/metrics`
- **gRPC**: `http://showcase-grpc:8081/metrics`

Key metrics:
- `http_requests_total` - Total HTTP requests
- `http_request_duration_seconds` - Request latency histogram
- `grpc_server_handled_total` - gRPC requests
- `go_goroutines` - Active goroutines
- `go_memstats_alloc_bytes` - Memory usage

### Grafana Dashboards

1. Access Grafana:
```bash
kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80
open http://localhost:3000
```

2. Import dashboards:
   - Kubernetes cluster monitoring: Dashboard ID 7249
   - Go metrics: Dashboard ID 10826
   - Nginx Ingress: Dashboard ID 9614

### Logging

```bash
# View logs
kubectl logs -f -l app=showcase,component=api -n showcase
kubectl logs -f -l app=showcase,component=grpc -n showcase
kubectl logs -f -l app=showcase,component=worker -n showcase

# Filter by error level (if using JSON logging)
kubectl logs -l app=showcase -n showcase | jq 'select(.level=="error")'

# Follow logs from all API pods
kubectl logs -f deploy/showcase-api -n showcase
```

## Troubleshooting

### Pods Not Starting

```bash
# Check pod status
kubectl get pods -n showcase

# Describe pod for events
kubectl describe pod showcase-api-xxxxx -n showcase

# Check logs
kubectl logs showcase-api-xxxxx -n showcase
```

**Common issues**:
- Image pull errors → Check image name and registry credentials
- CrashLoopBackOff → Check logs for application errors
- Pending → Check resource requests and node capacity

### Database Connection Issues

```bash
# Test database connectivity from a pod
kubectl run -it --rm debug --image=postgres:16-alpine --restart=Never -n showcase -- psql "postgres://user:pass@host:5432/db"

# Check secret
kubectl get secret showcase-secrets -n showcase -o yaml

# Verify DATABASE_URL in pod
kubectl exec -it showcase-api-xxxxx -n showcase -- env | grep DATABASE_URL
```

### Ingress Not Working

```bash
# Check ingress status
kubectl get ingress -n showcase
kubectl describe ingress showcase-ingress -n showcase

# Check ingress controller logs
kubectl logs -n ingress-nginx -l app.kubernetes.io/component=controller

# Test without ingress
kubectl port-forward -n showcase svc/showcase-api 8080:80
curl http://localhost:8080/health
```

### High Memory/CPU Usage

```bash
# Check resource usage
kubectl top pods -n showcase
kubectl top nodes

# View HPA status
kubectl get hpa -n showcase

# Check for memory leaks with pprof
kubectl port-forward -n showcase svc/showcase-api 8080:80
go tool pprof http://localhost:8080/debug/pprof/heap
```

## Production Checklist

### Before Deployment

- [ ] Update all secrets with production values
- [ ] Use managed database (RDS/CloudSQL/Azure Database)
- [ ] Enable SSL/TLS with cert-manager
- [ ] Configure OAuth2 redirect URLs
- [ ] Set up external Redis cluster
- [ ] Review resource requests/limits
- [ ] Enable Pod Security Policies/Standards
- [ ] Configure Network Policies
- [ ] Set up backup for PostgreSQL
- [ ] Configure log aggregation (ELK/Loki)
- [ ] Set up alerting rules in Prometheus

### Security

```yaml
# Pod Security Context
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  fsGroup: 1000
  capabilities:
    drop:
    - ALL
  readOnlyRootFilesystem: true
```

### High Availability

- [ ] Run at least 2 replicas of each component
- [ ] Use Pod Disruption Budgets
- [ ] Enable pod anti-affinity for spreading across nodes
- [ ] Configure liveness and readiness probes
- [ ] Set up multi-zone node pools
- [ ] Use external load balancer with health checks

### Performance

- [ ] Enable HTTP/2 and gRPC keep-alive
- [ ] Configure connection pooling for database
- [ ] Set appropriate resource requests/limits
- [ ] Enable HPA with appropriate thresholds
- [ ] Use Redis for session storage
- [ ] Configure CDN for static assets

## Clean Up

```bash
# Delete all resources
kubectl delete namespace showcase

# Or delete individual components
kubectl delete -f deploy/kubernetes/
```

## References

- [Kubernetes Documentation](https://kubernetes.io/docs/)
- [nginx-ingress Controller](https://kubernetes.github.io/ingress-nginx/)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator)
- [Horizontal Pod Autoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)
