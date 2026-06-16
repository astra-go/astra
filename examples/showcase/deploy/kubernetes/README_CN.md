# Astra Showcase Kubernetes 部署指南

Astra Showcase 参考应用的、生产级 Kubernetes 部署方案。

## 目录

- [前置要求](#前置要求)
- [快速开始](#快速开始)
- [架构](#架构)
- [部署步骤](#部署步骤)
- [配置](#配置)
- [扩缩容](#扩缩容)
- [监控](#监控)
- [故障排除](#故障排除)
- [生产检查清单](#生产检查清单)

## 前置要求

### 必需

- Kubernetes 1.24+ 集群
- kubectl 配置好集群访问
- PostgreSQL 数据库（集群外或集群内）
- Redis 缓存（集群外或集群内）

### 推荐

- Ingress 控制器（nginx-ingress 或 traefik）
- cert-manager 用于 TLS 证书
- Prometheus Operator 用于监控
- Horizontal Pod Autoscaler 指标服务器

### 安装

```bash
# 安装 nginx-ingress 控制器
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.1/deploy/static/provider/cloud/deploy.yaml

# 安装 cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml

# 安装 Prometheus Operator（使用 Helm）
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus-community/kube-prometheus-stack -n monitoring --create-namespace

# 验证安装
kubectl get pods -n ingress-nginx
kubectl get pods -n cert-manager
kubectl get pods -n monitoring
```

## 快速开始

```bash
# 1. 创建命名空间和 Secret
kubectl apply -f 00-namespace.yaml

# 2. 更新 Secret（填入实际值）
kubectl edit secret showcase-secrets -n showcase

# 3. 部署应用
kubectl apply -f 01-api-deployment.yaml
kubectl apply -f 02-grpc-deployment.yaml
kubectl apply -f 03-worker-deployment.yaml

# 4. 部署 Ingress
kubectl apply -f 04-ingress.yaml

# 5. 部署监控（可选）
kubectl apply -f 05-monitoring.yaml

# 6. 验证部署
kubectl get pods -n showcase
kubectl get svc -n showcase
kubectl get ingress -n showcase
```

## 架构

```
┌─────────────────────────────────────────────────────────────────┐
│  Kubernetes 集群                                             │
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

## 部署步骤

### 步骤 1：准备数据库

**方式 A：外部托管数据库**（生产推荐）

```bash
# AWS RDS / Google Cloud SQL / Azure Database
# 获取连接字符串并更新 Secret

kubectl create secret generic showcase-secrets \
  --from-literal=database-url='postgres://user:pass@rds-endpoint:5432/showcase?sslmode=require' \
  --namespace=showcase
```

**方式 B：集群内 PostgreSQL**（仅开发）

```yaml
# 保存为 postgres-statefulset.yaml
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

### 步骤 2：更新 Secret

```bash
# 生成 JWT 密钥
JWT_SECRET=$(openssl rand -base64 32)

# 更新 Secret
kubectl create secret generic showcase-secrets \
  --from-literal=database-url='postgres://...' \
  --from-literal=jwt-secret="$JWT_SECRET" \
  --from-literal=google-client-id='YOUR_GOOGLE_CLIENT_ID' \
  --from-literal=google-client-secret='YOUR_GOOGLE_SECRET' \
  --namespace=showcase \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 步骤 3：构建并推送 Docker 镜像

```bash
cd examples/showcase

# 构建镜像
docker build -t your-registry/showcase-api:v1.0.0 -f Dockerfile.api .
docker build -t your-registry/showcase-grpc:v1.0.0 -f Dockerfile.grpc .
docker build -t your-registry/showcase-worker:v1.0.0 -f Dockerfile.worker .

# 推送到仓库
docker push your-registry/showcase-api:v1.0.0
docker push your-registry/showcase-grpc:v1.0.0
docker push your-registry/showcase-worker:v1.0.0
```

### 步骤 4：部署应用

```bash
# 按顺序部署（依赖优先）
kubectl apply -f deploy/kubernetes/00-namespace.yaml
kubectl apply -f deploy/kubernetes/01-api-deployment.yaml
kubectl apply -f deploy/kubernetes/02-grpc-deployment.yaml
kubectl apply -f deploy/kubernetes/03-worker-deployment.yaml
kubectl apply -f deploy/kubernetes/04-ingress.yaml
kubectl apply -f deploy/kubernetes/05-monitoring.yaml

# 等待 Pod 就绪
kubectl wait --for=condition=ready pod -l app=showcase -n showcase --timeout=300s
```

### 步骤 5：验证部署

```bash
# 检查 Pod
kubectl get pods -n showcase

# 检查服务
kubectl get svc -n showcase

# 检查 Ingress
kubectl get ingress -n showcase

# 测试健康端点
kubectl port-forward -n showcase svc/showcase-api 8080:80
curl http://localhost:8080/health
```

## 配置

### 环境变量

所有配置通过 ConfigMap 和 Secret 管理：

**ConfigMap** (`showcase-config`):
- `REDIS_ADDR` - Redis 连接字符串
- `OTEL_EXPORTER_OTLP_ENDPOINT` - OpenTelemetry Collector 端点
- `HTTP_ADDR` - HTTP 监听地址
- `GRPC_ADDR` - gRPC 监听地址
- `LOG_LEVEL` - 日志级别 (debug/info/warn/error)
- `LOG_FORMAT` - 日志格式 (json/text)

**Secret** (`showcase-secrets`):
- `database-url` - PostgreSQL 连接字符串
- `jwt-secret` - JWT 签名密钥
- `google-client-id` - Google OAuth2 客户端 ID
- `google-client-secret` - Google OAuth2 密钥
- `github-client-id` - GitHub OAuth2 客户端 ID
- `github-client-secret` - GitHub OAuth2 密钥

### 更新配置

```bash
# 编辑 ConfigMap
kubectl edit configmap showcase-config -n showcase

# 编辑 Secret
kubectl edit secret showcase-secrets -n showcase

# 重启 Pod 以应用变更
kubectl rollout restart deployment/showcase-api -n showcase
kubectl rollout restart deployment/showcase-grpc -n showcase
kubectl rollout restart deployment/showcase-worker -n showcase
```

## 扩缩容

### 手动扩缩容

```bash
# 扩缩 API Pod
kubectl scale deployment showcase-api --replicas=5 -n showcase

# 扩缩 gRPC Pod
kubectl scale deployment showcase-grpc --replicas=4 -n showcase

# 扩缩 Worker
kubectl scale deployment showcase-worker --replicas=3 -n showcase
```

### Horizontal Pod Autoscaler (HPA)

**API Pod**:
- 最小: 2，最大: 10
- 目标 CPU: 70%
- 目标内存: 80%

**gRPC Pod**:
- 最小: 2，最大: 6
- 目标 CPU: 70%

**Worker Pod**:
- 最小: 1，最大: 5
- 目标 CPU: 75%
- 目标内存: 85%

## 监控

### Prometheus 指标

指标暴露在 `/metrics` 端点：

- **API**: `http://showcase-api:8080/metrics`
- **gRPC**: `http://showcase-grpc:8081/metrics`

关键指标：
- `http_requests_total` - HTTP 请求总数
- `http_request_duration_seconds` - 请求延迟直方图
- `grpc_server_handled_total` - gRPC 请求
- `go_goroutines` - 活跃 goroutine 数
- `go_memstats_alloc_bytes` - 内存使用

### Grafana 仪表盘

1. 访问 Grafana：
```bash
kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80
open http://localhost:3000
```

2. 导入仪表盘：
   - Kubernetes 集群监控: Dashboard ID 7249
   - Go 指标: Dashboard ID 10826
   - Nginx Ingress: Dashboard ID 9614

## 故障排除

### Pod 不启动

```bash
# 检查 Pod 状态
kubectl get pods -n showcase

# 描述 Pod 获取事件
kubectl describe pod showcase-api-xxxxx -n showcase

# 检查日志
kubectl logs showcase-api-xxxxx -n showcase
```

**常见问题**：
- 镜像拉取错误 → 检查镜像名和仓库凭证
- CrashLoopBackOff → 检查日志中的应用错误
- Pending → 检查资源请求和节点容量

### 数据库连接问题

```bash
# 从 Pod 测试数据库连接
kubectl run -it --rm debug --image=postgres:16-alpine --restart=Never -n showcase -- psql "postgres://user:pass@host:5432/db"

# 检查 Secret
kubectl get secret showcase-secrets -n showcase -o yaml

# 验证 Pod 中的 DATABASE_URL
kubectl exec -it showcase-api-xxxxx -n showcase -- env | grep DATABASE_URL
```

### Ingress 不工作

```bash
# 检查 Ingress 状态
kubectl get ingress -n showcase
kubectl describe ingress showcase-ingress -n showcase

# 检查 Ingress 控制器日志
kubectl logs -n ingress-nginx -l app.kubernetes.io/component=controller

# 不用 Ingress 测试
kubectl port-forward -n showcase svc/showcase-api 8080:80
curl http://localhost:8080/health
```

### 内存/CPU 使用高

```bash
# 检查资源使用
kubectl top pods -n showcase
kubectl top nodes

# 查看 HPA 状态
kubectl get hpa -n showcase

# 用 pprof 检查内存泄漏
kubectl port-forward -n showcase svc/showcase-api 8080:80
go tool pprof http://localhost:8080/debug/pprof/heap
```

## 生产检查清单

### 部署前

- [ ] 用生产值更新所有 Secret
- [ ] 使用托管数据库（RDS/CloudSQL/Azure Database）
- [ ] 启用 cert-manager SSL/TLS
- [ ] 配置 OAuth2 重定向 URL
- [ ] 设置外部 Redis 集群
- [ ] 审查资源请求/限制
- [ ] 启用 Pod 安全策略/标准
- [ ] 配置网络策略
- [ ] 为 PostgreSQL 设置备份
- [ ] 配置日志聚合（ELK/Loki）
- [ ] 在 Prometheus 中设置告警规则

### 安全

```yaml
# Pod 安全上下文
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  fsGroup: 1000
  capabilities:
    drop:
    - ALL
  readOnlyRootFilesystem: true
```

### 高可用

- [ ] 每个组件至少运行 2 个副本
- [ ] 使用 Pod Disruption Budgets
- [ ] 启用 Pod 反亲和性跨节点分布
- [ ] 配置存活和就绪探针
- [ ] 设置多可用区节点池
- [ ] 使用带健康检查的外部负载均衡器

### 性能

- [ ] 启用 HTTP/2 和 gRPC keep-alive
- [ ] 配置数据库连接池
- [ ] 设置适当的资源请求/限制
- [ ] 启用阈值适当的 HPA
- [ ] 使用 Redis 存储 Session
- [ ] 配置 CDN 提供静态资源

## 清理

```bash
# 删除所有资源
kubectl delete namespace showcase

# 或删除各个组件
kubectl delete -f deploy/kubernetes/
```

## 参考资料

- [Kubernetes 文档](https://kubernetes.io/docs/)
- [nginx-ingress 控制器](https://kubernetes.github.io/ingress-nginx/)
- [cert-manager 文档](https://cert-manager.io/docs/)
- [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator)
- [Horizontal Pod Autoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)