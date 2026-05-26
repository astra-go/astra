# Astra Deployment — Helm Chart & Kustomize

Cloud-native deployment manifests for Astra Go Web Framework applications.

## Directory Structure

```
deploy/
├── README.md                          # This file
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
