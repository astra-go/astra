#!/bin/bash
# 性能对比测试脚本 - 交替测试减少环境干扰

echo "=== Astra 性能优化对比测试 ==="
echo "测试时间: $(date)"
echo ""

# 编译优化版本
go build ./...

# 交替测试 5 轮
for i in {1..5}; do
    echo "=== 第 $i 轮测试 ==="

    echo ">>> 原始版本测试..."
    git stash
    go test -bench="BenchmarkRouter_Static-10|BenchmarkRouter_Param-10|BenchmarkMiddlewareChain_0-10|BenchmarkContext_JSON_Small-10" -benchmem -count=1 > /tmp/bench_orig_$i.txt
    git stash pop

    echo ">>> 优化版本测试..."
    go test -bench="BenchmarkRouter_Static-10|BenchmarkRouter_Param-10|BenchmarkMiddlewareChain_0-10|BenchmarkContext_JSON_Small-10" -benchmem -count=1 > /tmp/bench_opt_$i.txt

    echo ""
done

# 合并结果
echo "=== 合并测试结果 ==="
cat /tmp/bench_orig_*.txt > /tmp/bench_orig_all.txt
cat /tmp/bench_opt_*.txt > /tmp/bench_opt_all.txt

echo "=== 使用 benchstat 分析 ==="
~/go/bin/benchstat /tmp/bench_orig_all.txt /tmp/bench_opt_all.txt
