# Deploy — 部署配置

Kubernetes、Docker Compose 和 Helm 部署配置模板。

## 目录结构

```
deploy/
├── config/
│   ├── configmap.yaml       # 环境配置 ConfigMap 模板
│   └── secret.yaml          # Secret 模板
├── docker-compose.dev.yml   # 本地开发环境
├── docker-compose.prod.yml  # 生产环境（推荐）
├── Dockerfile               # 多阶段构建 Dockerfile
├── helm/
│   └── astra/               # Helm Chart
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
├── kustomize/
│   ├── base/                # Kustomize 基础配置
│   └── overlays/            # 环境 overlays
│       ├── dev/
│       ├── staging/
│       └── prod/
└── init/
    ├── init-db.sql          # 数据库初始化脚本
    └── init-redis.sh        # Redis 初始化
```

## 快速开始

### Docker Compose（开发）

```bash
docker-compose -f deploy/docker-compose.dev.yml up -d
```

### Helm（生产 K8s 部署）

```bash
# 添加 Helm 仓库
helm repo add astra https://charts.example.com

# 安装
helm install my-app astra/astra \
    --set image.tag=v1.0.0 \
    --set replicaCount=3 \
    --set resources.limits.cpu=500m \
    --set resources.limits.memory=512Mi

# 升级
helm upgrade my-app astra/astra -f values.yaml
```

### Kustomize（多环境）

```bash
# 开发环境
kubectl apply -k deploy/kustomize/overlays/dev

# 生产环境
kubectl apply -k deploy/kustomize/overlays/prod
```

## Helm values.yaml 常用配置

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
  # 非敏感配置（写入 ConfigMap）
  LOG_LEVEL: "info"
  REDIS_ADDR: "redis:6379"
```

## Kubernetes 探针配置

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

## 注意事项

- 生产环境务必配置 `podSecurityContext` 和 `securityContext`；不要以 root 运行
- 推荐通过 cert-manager 自动管理 TLS 证书
- `values.yaml` 中的敏感信息（如密码）应通过 `--set-file` 或外部 Secret 注入，切勿明文写入
- Kustomize `overlays/prod` 应覆盖镜像 tag 为固定版本；切勿使用 `latest`