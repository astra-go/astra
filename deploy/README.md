# Astra Deployment

Deployment configurations for Astra Go Web Framework applications, including local development environment and cloud-native deployment manifests.

## Directory Structure

```
deploy/
├── README.md                          # This file
├── docker-compose.dev.yml             # Local development environment
│
├── config/
│   ├── prometheus.yml                 # Prometheus configuration
│   └── grafana/
│       └── provisioning/              # Grafana datasources and dashboards
│
├── init/
│   ├── postgres/                      # PostgreSQL init scripts
│   ├── mysql/                         # MySQL init scripts
│   └── mongo/                         # MongoDB init scripts
│
├── helm/
│   └── astra/                        # Helm chart
│       ├── Chart.yaml
│       ├── values.yaml                # Full customization reference
│       └── templates/
│           ├── _helpers.tpl           # Named templates
│           ├── deployment.yaml
│           ├── service.yaml
│           ├── configmap.yaml
│           ├── secret.yaml
│           ├── ingress.yaml
│           ├── hpa.yaml               # HorizontalPodAutoscaler
│           ├── pdb.yaml               # PodDisruptionBudget
│           ├── serviceaccount.yaml
│           └── NOTES.txt              # Post-install instructions
│
└── kustomize/
    ├── base/
    │   ├── kustomization.yaml
    │   ├── deployment.yaml
    │   └── service.yaml
    └── overlays/
        ├── staging/
        │   └── kustomization.yaml
        └── production/
            ├── kustomization.yaml
            └── ...                    # Per-environment overrides
```

---

## 🚀 Local Development Environment

### Quick Start

The fastest way to get started with Astra development:

```bash
# Start minimal environment (PostgreSQL + Redis)
./scripts/dev.sh start

# Start with observability stack
./scripts/dev.sh start observability

# Start all services
./scripts/dev.sh start full

# Check service status
./scripts/dev.sh status

# View logs
./scripts/dev.sh logs postgres

# Stop environment
./scripts/dev.sh stop

# Reset environment (removes all data)
./scripts/dev.sh reset
```

### Available Profiles

#### Minimal (Default)
Core services for most development needs:
- **PostgreSQL** - Primary database (port 5432)
- **Redis** - Cache and session store (port 6379)

```bash
./scripts/dev.sh start minimal
# or simply
./scripts/dev.sh start
```

#### Observability
Minimal + monitoring and tracing:
- **PostgreSQL** + **Redis** (from minimal)
- **Prometheus** - Metrics collection (port 9090)
- **Grafana** - Metrics visualization (port 3000, admin/admin)
- **Jaeger** - Distributed tracing (port 16686)

```bash
./scripts/dev.sh start observability
```

#### Full
All services including message queues, search, and service discovery:
- All services from minimal and observability profiles
- **MySQL** - Alternative RDBMS (port 3306)
- **MongoDB** - Document database (port 27017)
- **Kafka** + **Zookeeper** - Event streaming (port 9092)
- **RabbitMQ** - Message broker (port 5672, UI: 15672)
- **NATS** - Lightweight messaging (port 4222)
- **Elasticsearch** - Full-text search (port 9200)
- **Consul** - Service discovery (port 8500)
- **etcd** - Distributed configuration (port 2379)

```bash
./scripts/dev.sh start full
```

### Service Credentials

All services use consistent credentials for easy development:

| Service | Connection String |
|---------|-------------------|
| PostgreSQL | `postgresql://astra_dev:dev123@localhost:5432/astra_dev` |
| MySQL | `mysql://astra_dev:dev123@localhost:3306/astra_dev` |
| MongoDB | `mongodb://astra_dev:dev123@localhost:27017/astra_dev` |
| Redis | `redis://:dev123@localhost:6379` |
| RabbitMQ | `amqp://astra_dev:dev123@localhost:5672` |
| Kafka | `localhost:9092` |
| NATS | `nats://localhost:4222` |
| Elasticsearch | `http://localhost:9200` |
| Consul | `http://localhost:8500` |
| etcd | `http://localhost:2379` |

### Web UIs

| Service | URL | Credentials |
|---------|-----|-------------|
| Grafana | http://localhost:3000 | admin / admin |
| Jaeger UI | http://localhost:16686 | - |
| Prometheus | http://localhost:9090 | - |
| RabbitMQ Management | http://localhost:15672 | astra_dev / dev123 |
| Consul UI | http://localhost:8500 | - |

### Management Commands

```bash
# Start environment
./scripts/dev.sh start [minimal|observability|full]

# Stop services (preserves data)
./scripts/dev.sh stop

# Restart all services
./scripts/dev.sh restart

# Restart specific service
./scripts/dev.sh restart postgres

# Check service status
./scripts/dev.sh status

# Check health status
./scripts/dev.sh health

# View logs
./scripts/dev.sh logs [service] [-f]

# Shutdown and remove containers
./scripts/dev.sh down

# Shutdown and remove volumes (deletes all data)
./scripts/dev.sh down --volumes

# Clean up Docker resources
./scripts/dev.sh clean

# Complete reset (removes everything)
./scripts/dev.sh reset

# Show help
./scripts/dev.sh help
```

### Database Initialization

Each database service comes with pre-configured initialization scripts that run on first startup:

- **PostgreSQL**: Creates extensions (uuid-ossp, pg_trgm), sample users table
- **MySQL**: Creates sample users table with proper charset
- **MongoDB**: Creates validated collections with indexes

Scripts are located in `deploy/init/{postgres,mysql,mongo}/` and can be customized for your needs.

### Data Persistence

All services use Docker volumes for data persistence:
- Data survives container restarts (`./scripts/dev.sh restart`)
- Data survives container recreation (`./scripts/dev.sh down` + `./scripts/dev.sh start`)
- Data is removed only with `./scripts/dev.sh down --volumes` or `./scripts/dev.sh reset`

### Troubleshooting

#### Port Conflicts

If you see port binding errors, check if services are already running:

```bash
# Check what's using port 5432
lsof -i :5432

# Or use the built-in status check
./scripts/dev.sh status
```

#### Service Not Healthy

Wait a few seconds for services to initialize, then check health:

```bash
./scripts/dev.sh health
```

View service logs to diagnose issues:

```bash
./scripts/dev.sh logs postgres
./scripts/dev.sh logs redis -f  # follow logs in real-time
```

#### Reset Environment

If services are in a bad state, perform a complete reset:

```bash
./scripts/dev.sh reset
./scripts/dev.sh start
```

#### Docker Resources

If Docker is running slow or out of resources:

```bash
# Clean up unused containers, images, and build cache
./scripts/dev.sh clean

# Check Docker disk usage
docker system df
```

### Direct Docker Compose Usage

You can also use Docker Compose directly for more control:

```bash
# Minimal (specific services)
docker-compose -f deploy/docker-compose.dev.yml up -d postgres redis

# With observability
docker-compose -f deploy/docker-compose.dev.yml --profile observability up -d

# Full stack
docker-compose -f deploy/docker-compose.dev.yml --profile full up -d

# View all services
docker-compose -f deploy/docker-compose.dev.yml ps

# Stop all
docker-compose -f deploy/docker-compose.dev.yml down

# Remove volumes
docker-compose -f deploy/docker-compose.dev.yml down -v
```

---

## ☸️ Cloud-Native Deployment

## Quick Start

### Helm

```bash
# Add the chart (if published to a chart repo)
helm repo add astra https://charts.astra-go.io
helm repo update

# Install
helm install astra astra/astra \
  --namespace astra \
  --create-namespace \
  --set image.repository=myorg/myastraapp \
  --set image.tag=v1.2.3 \
  --set replicaCount=3 \
  --set service.type=LoadBalancer

# Upgrade (after image tag changes)
helm upgrade astra astra/astra \
  --set image.tag=v1.2.4

# Render locally without installing
helm template astra astra/ \
  --set image.repository=myorg/myastraapp \
  --set image.tag=latest

# Dry-run (requires a kubecontext)
helm upgrade astra astra/astra \
  --install \
  --dry-run=server \
  --set image.repository=myorg/myastraapp
```

### Kustomize

```bash
# Install base
kubectl apply -k deploy/kustomize/base

# Install staging
kubectl apply -k deploy/kustomize/overlays/staging

# Install production
kubectl apply -k deploy/kustomize/overlays/production

# Diff (dry-run) before applying
kubectl diff -k deploy/kustomize/overlays/production

# Preview rendered YAML
kustomize build deploy/kustomize/overlays/staging

# Update image tag from CI (production)
cd deploy/kustomize/overlays/production
kustomize edit set image myorg/astra-app:v1.2.4
cd -
```

## Astra-Specific Deployment Considerations

### 1. Graceful Shutdown (Critical)

Astra supports graceful shutdown on `SIGTERM`. Configure `terminationGracePeriodSeconds: 30`
or higher to allow:
- Running requests to complete
- Reactor worker pool to drain
- Existing connections to close cleanly

```yaml
# Helm
pod:
  terminationGracePeriodSeconds: 30

# Kustomize base
spec:
  terminationGracePeriodSeconds: 30
```

The `preStop` hook in the chart adds a `sleep 5` to give kube-proxy time to
remove the pod from Service endpoints before SIGKILL.

### 2. Health Endpoints

Astra provides three built-in endpoints (configurable via `ASTRA_HEALTH_*` env vars):

| Endpoint | Purpose | Probe |
|----------|---------|-------|
| `/ready` | Readiness — is the app ready to serve traffic? | ReadinessProbe |
| `/live` | Liveness — is the app alive? | LivenessProbe |
| `/health` | Detailed health — component checks | — |

The chart uses `/ready` and `/live` as probe paths by default.

### 3. Reactor Mode vs net/http Mode

For Reactor mode (epoll/kqueue), the app binary must be compiled for Linux/macOS.
For Windows, Astra falls back to net/http automatically. Set `nodeSelector` accordingly:

```yaml
# Helm
pod:
  nodeSelector:
    kubernetes.io/os: linux

# Kustomize base
spec:
  template:
    spec:
      nodeSelector:
        kubernetes.io/os: linux
```

### 4. gRPC + HTTP Dual-Stack

If using Astra's gRPC server alongside HTTP, expose both ports:

```yaml
# Helm
service:
  extraPorts:
    - name: grpc
      port: 9090
      targetPort: 9090
      protocol: TCP
```

### 5. TLS Termination

Astra supports `RunReactorTLS` / `RunTLS` for TLS at the application layer.
For production, TLS is usually terminated at the Ingress (cert-manager + Let's Encrypt).
Only use Astra TLS mode when:
- Running outside Kubernetes (direct pod-to-pod mTLS)
- Using Istio sidecar with custom TLS rules

```yaml
# Mount TLS cert as a volume
app:
  extraVolumes:
    - name: tls-cert
      secret:
        secretName: astra-tls-cert
  extraVolumeMounts:
    - name: tls-cert
      mountPath: /etc/astra/tls
```

### 6. Reactive Mode (HTTP/2, HTTP/3)

| Protocol | TLS | Plain-text | Notes |
|----------|-----|------------|-------|
| HTTP/1.1 | via HTTPS | ✓ (default) | Standard |
| HTTP/2 | ALPN h2 | h2c (connection preface) | Astra Reactor engine |
| HTTP/3 | QUIC | N/A | Via quic-go module |

For h2c (HTTP/2 over plain TCP), no special chart configuration is needed beyond
using the Reactor engine. Set `ASTRA_MODE=RunReactor` or `RunReactorTLS` for TLS.

### 7. Environment Variables

```yaml
# Common Astra env vars
app:
  env:
    LOG_LEVEL: info           # debug | info | warn | error
    GIN_MODE: release         # debug | release (disables logger color, etc.)
    ASTRA_MODE: RunReactor    # Run | RunTLS | RunReactor | RunReactorTLS
    ASTRA_SHUTDOWN_TIMEOUT: 30  # seconds to wait for graceful shutdown
    ASTRA_READ_TIMEOUT: 30
    ASTRA_WRITE_TIMEOUT: 60
    # For Reactor engine:
    ASTRA_NUM_LOOPS: 0         # 0 = GOMAXPROCS
    ASTRA_WORKER_POOL_SIZE: 0   # 0 = 4 * GOMAXPROCS
    ASTRA_READ_BUFFER_SIZE: 16384
```

### 8. Observability Integration

Astra exposes Prometheus metrics at `/metrics`. The base deployment annotation enables scraping:

```yaml
annotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "8080"
  prometheus.io/path: "/metrics"
```

For OTel tracing, set env vars:
```yaml
app:
  env:
    OTEL_EXPORTER_OTLP_ENDPOINT: http://otel-collector:4317
    OTEL_SERVICE_NAME: astra-myapp
```

## Production Checklist

- [ ] Use an immutable image tag (e.g. `v1.2.4`), never `:latest`
- [ ] Set `replicaCount >= 3` with `maxUnavailable: 0` for zero-downtime rolling updates
- [ ] Enable HPA with CPU/memory thresholds appropriate for your workload
- [ ] Configure `PodDisruptionBudget` with `minAvailable: 2` or `maxUnavailable: 1`
- [ ] Set `resources.requests` (guaranteed QoS) and `resources.limits`
- [ ] Mount TLS certificates via secret volume, don't bake into image
- [ ] Set `pod.containerSecurityContext.readOnlyRootFilesystem: true`
- [ ] Configure `terminationGracePeriodSeconds: 30` minimum
- [ ] Set `GIN_MODE: release` and `LOG_LEVEL: warn` or `error`
- [ ] Point `/metrics` and `/health` to your observability stack
- [ ] Use a `PodDisruptionBudget` to prevent voluntary disruptions from causing downtime
- [ ] Set up cert-manager for automatic TLS certificate management

## CI/CD Integration

### GitHub Actions (Helm)

```yaml
- name: Deploy to Kubernetes
  uses: azure/k8s-deploy@v4
  with:
    namespace: production
    manifests: |
      deploy/manifests/production-*.yaml
    images: |
      myorg/astra-app:${{ github.sha }}
```

### GitHub Actions (Kustomize)

```yaml
- name: Set image tag
  run: |
    cd deploy/kustomize/overlays/production
    kustomize edit set image myorg/astra-app:${{ github.sha }}
    cat kustomization.yaml

- name: Deploy
  uses: azure/k8s-deploy@v4
  with:
    namespace: production
    kustomization: deploy/kustomize/overlays/production
```

### ArgoCD

For GitOps, point ArgoCD at the `deploy/` directory. Example `Application.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: astra-prod
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/astra-go/astra.git
    targetRevision: HEAD
    path: deploy/kustomize/overlays/production
  destination:
    server: https://kubernetes.default.svc
    namespace: production
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```
