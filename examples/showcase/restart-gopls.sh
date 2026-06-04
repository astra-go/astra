#!/bin/bash
# 重启 gopls 并清除缓存的脚本

echo "🔧 清除 gopls 和 Go 缓存..."

# 1. 杀死所有 gopls 进程
killall gopls 2>/dev/null && echo "✅ 已停止 gopls 进程" || echo "ℹ️  没有运行中的 gopls 进程"

# 2. 清除 gopls 缓存
rm -rf ~/Library/Caches/gopls 2>/dev/null && echo "✅ gopls 缓存已清除"

# 3. 清除 Go 构建缓存
go clean -cache && echo "✅ Go 构建缓存已清除"

# 4. 重新构建以验证
echo ""
echo "🔨 验证构建..."
cd "$(dirname "$0")"
go build -tags redis -o /dev/null ./cmd/worker && echo "✅ worker 编译成功" || echo "❌ worker 编译失败"
go build -tags redis -o /dev/null ./cmd/api && echo "✅ api 编译成功" || echo "❌ api 编译失败"

echo ""
echo "✨ 完成！现在请在 VS Code 中执行以下操作："
echo "   1. 按 Cmd+Shift+P"
echo "   2. 输入 'Go: Restart Language Server'"
echo "   3. 或者直接重新加载窗口（Cmd+Shift+P → 'Developer: Reload Window'）"
