#!/usr/bin/env bash
# tidy-all.sh — 按拓扑顺序对所有 workspace 模块执行 go mod tidy
#
# 执行顺序：被依赖模块先 tidy，保证依赖方能拿到正确的 checksum。
# 用法：bash scripts/tidy-all.sh
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)

# 顺序说明：
#   第 1 批：根模块（无 workspace 内依赖）
#   第 2 批：无 workspace 内依赖的独立子模块
#   第 3 批：仅依赖根模块的子模块
#   第 4 批：依赖根模块 + 其他子模块的子模块
MODULES=(
    # ── 第 1 批：根模块 ──────────────────────────────────────
    .
    # ── 第 2 批：无 workspace 内依赖 ─────────────────────────
    otel
    mq
    taskqueue
    storage
    discovery
    config
    cache
    lock
    search
    notify
    mongodb
    lua
    # ── 第 3 批：依赖根模块 ──────────────────────────────────
    orm
    grpc
    session
    auth
    # ── 第 4 批：依赖根模块 + 其他子模块 ────────────────────
    runner      # 依赖根模块 + taskqueue
    client      # 依赖根模块 + discovery
    testutil    # 依赖根模块 + cache
)

for mod in "${MODULES[@]}"; do
    dir="$ROOT/$mod"
    [ "$mod" = "." ] && dir="$ROOT"
    echo "▶  go mod tidy — $mod"
    (cd "$dir" && go mod tidy)
done

echo "✓ 全部 ${#MODULES[@]} 个模块 tidy 完成"
