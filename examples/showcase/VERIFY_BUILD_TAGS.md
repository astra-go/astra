# 验证构建标签配置

## 问题说明

`NewRedisBroker` 函数位于 `taskqueue/redis_broker.go`，该文件使用了 `//go:build redis` 构建标签。

## 为什么会显示"不存在"？

这是 IDE 语言服务器（gopls）的缓存问题，不是代码问题。

## 证明代码可以正常工作

```bash
# 1. 编译成功
cd examples/showcase
make build

# 2. 程序可以运行
./bin/worker
# 输出: INFO worker started redis=localhost:6379 concurrency=10

# 3. gopls 手动验证
GOFLAGS="-tags=redis" gopls check ./cmd/worker/main.go
# 无错误输出
```

## 解决方案

1. **重启 VS Code 语言服务器**
   - `Cmd+Shift+P` → "Go: Restart Language Server"

2. **或重启 VS Code**

## 配置文件已更新

- ✅ `.vscode/settings.json` (项目根目录)
- ✅ `examples/showcase/.vscode/settings.json` (showcase 子目录)

两个配置文件都已添加 `build.env.GOFLAGS = "-tags=redis"`
