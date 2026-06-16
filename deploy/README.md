# Deploy — Deployment Configuration

Kubernetes, Docker Compose, and Helm deployment configuration templates.

## Directory Structure

```
deploy/
├── config/
│   ├── configmap.yaml       # Environment config ConfigMap template
│   └── secret.yaml          # Secret template
├── docker-compose.dev.yml   # Local development environment
├── docker-compose.prod.yml  # Production environment (recommended)
├── Dockerfile               # Multi-stage build Dockerfile
├── helm/
│   └── astra/               # Helm Chart
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
├── kustomize/
│   ├── base/                # Kustomize base config
│   └── overlays/            # Environment overlays
│       ├── dev/
│       ├── staging/
│       └── prod/
└── init/
    ├── init-db.sql          # Database initialization script
    └── init-redis.sh        # Redis initialization
```

## Quick Start

### Docker Compose (Development)

```bash
docker-compose -f deploy/docker-compose.dev.yml up -d
```

### Helm (Production K8s Deployment)

```bash
# Add Helm repo
helm repo add astra https://charts.example.com

# Install
helm install my-app astra/astra \
    --set image.tag=v1.0.0 \
    --set replicaCount=3 \
    --set resources.limits.cpu=500m \
    --set resources.limits.memory=512Mi

# Upgrade
helm upgrade my-app astra/astra -f values.yaml
```

### Kustomize (Multi-Environment)

```bash
# Development environment
kubectl apply -k deploy/kustomize/overlays/dev

# Production environment
kubectl apply -k deploy/kustomize/overlays/prod
```

## Helm values.yaml Common Config

```yaml
# values.yaml
replicaCount: 3

image:
  repository: myregistry/myapp
  tag: "1.0.0"
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8080

resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80

ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: api.example.com
      paths: ["/"]
  tls:
    - secretName: myapp-tls
      hosts: ["api.example.com"]

podSecurityContext:
  runAsNonRoot: true
  runAsUser: 1000

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true

config:
  # Non-sensitive config (written to ConfigMap)
  LOG_LEVEL: "info"
  REDIS_ADDR: "redis:6379"
```

## Kubernetes Probe Config

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 5
```

## Notes

- Always configure `podSecurityContext` and `securityContext` in production; don't run as root
- TLS certificates recommended managed automatically via cert-manager
- Sensitive info like passwords in `values.yaml` should be injected via `--set-file` or external secrets, never written in plaintext
- Kustomize `overlays/prod` should override image tag to fixed version; never use `latest`
