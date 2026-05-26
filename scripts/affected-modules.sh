#!/usr/bin/env bash
# affected-modules.sh — 输出受当前 branch 改动影响的最小模块集合（含传递下游）。
#
# 输出：换行分隔的模块目录路径（相对于仓库根）。
# 用途：驱动 CI 矩阵，只对变更涉及的模块执行 tidy / build / test。
#
# 用法：
#   bash scripts/affected-modules.sh              # 对比 origin/main
#   bash scripts/affected-modules.sh origin/HEAD  # 对比指定 ref
#   bash scripts/affected-modules.sh --all        # 输出全部模块（全量 CI）
set -euo pipefail

BASE=${1:-origin/main}

# ─── 1. 全部模块列表（拓扑顺序，被依赖的在前）──────────────────────────────────
# 与 tidy-all.sh 顺序一致，确保传递依赖计算正确。
ALL_MODULES=(
    .
    otel mq taskqueue storage discovery config cache lock search notify mongodb lua
    orm grpc session auth
    runner client testutil
)

# ── --all 或不在 git 仓库时，输出全部模块 ──────────────────────────────────────
if [ "${BASE}" = "--all" ]; then
    printf '%s\n' "${ALL_MODULES[@]}" | sort -u
    exit 0
fi

ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || {
    # 不在 git 仓库（本地未初始化）→ 全量输出，交给 CI 决策
    printf '%s\n' "${ALL_MODULES[@]}" | sort -u
    exit 0
}

# ── fallback: 无法访问 BASE ref 时，输出全部模块 ────────────────────────────────
if ! git rev-parse "$BASE" &>/dev/null 2>&1; then
    printf '%s\n' "${ALL_MODULES[@]}" | sort -u
    exit 0
fi

# ─── 2. 获取变更文件 ───────────────────────────────────────────────────────────
# 优先用三点号（merge-base diff），fallback 两点号（shallow clone 场景）。
changed=$(git diff --name-only "$BASE"...HEAD 2>/dev/null \
          || git diff --name-only "$BASE" HEAD 2>/dev/null \
          || echo "")

if [ -z "$changed" ]; then
    # 无法计算 diff（新分支、空仓库等），全量输出
    printf '%s\n' "${ALL_MODULES[@]}" | sort -u
    exit 0
fi

# ─── 3. 将变更文件映射到所属模块目录 ─────────────────────────────────────────
# 策略：最长前缀匹配，根模块作为兜底。
declare -A dirty

_owner() {
    local f="$1"
    local best="."
    for mod in "${ALL_MODULES[@]}"; do
        [ "$mod" = "." ] && continue
        if [[ "$f" == "$mod/"* ]] || [[ "$f" == "$mod" ]]; then
            # 取路径更深（更具体）的那个
            if [ ${#mod} -gt ${#best} ]; then
                best="$mod"
            fi
        fi
    done
    echo "$best"
}

while IFS= read -r f; do
    [ -z "$f" ] && continue
    # go.work / go.work.sum 改动 → 所有模块都受影响
    if [[ "$f" == "go.work" || "$f" == "go.work.sum" ]]; then
        for mod in "${ALL_MODULES[@]}"; do dirty["$mod"]=1; done
        break
    fi
    dirty["$(_owner "$f")"]=1
done <<< "$changed"

# ─── 4. 依赖图：child 依赖 parent（parent 变 → child 也需重测）─────────────────
# 格式：DEPS[child]="parent1 parent2 ..."
declare -A DEPS
DEPS["orm"]="."
DEPS["grpc"]="."
DEPS["session"]="."
DEPS["auth"]="."
DEPS["runner"]=". taskqueue"
DEPS["client"]=". discovery"
DEPS["testutil"]=". cache"

# 根模块变动影响所有子模块
_expand_root() {
    for mod in "${ALL_MODULES[@]}"; do dirty["$mod"]=1; done
}

# BFS 传递展开：如果 parent 在 dirty 集合中，把 child 也加进去
_expand() {
    local changed=1
    while [ $changed -eq 1 ]; do
        changed=0
        for child in "${!DEPS[@]}"; do
            [ "${dirty[$child]+_}" ] && continue  # 已标记，跳过
            for parent in ${DEPS[$child]}; do
                if [ "${dirty[$parent]+_}" ]; then
                    dirty["$child"]=1
                    changed=1
                    break
                fi
            done
        done
    done
}

# 根模块特殊处理
[ "${dirty[.]+_}" ] && _expand_root || true
_expand

# ─── 5. 输出（按拓扑顺序，方便 CI 串行依赖时使用）──────────────────────────────
for mod in "${ALL_MODULES[@]}"; do
    [ "${dirty[$mod]+_}" ] && echo "$mod" || true
done
