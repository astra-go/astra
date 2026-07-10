#!/bin/bash
# 简化版性能对比测试

echo "=== Astra 性能优化对比测试 ==="
echo "测试时间: $(date)"
echo ""

# 保存当前修改
echo ">>> 保存当前修改..."
git stash

echo "=== 测试原始版本 ==="
go test -bench="BenchmarkRouter_Static$" -benchmem -count=3 2>&1 | tee /tmp/orig_static.txt
go test -bench="BenchmarkRouter_Param$" -benchmem -count=3 2>&1 | tee /tmp/orig_param.txt
go test -bench="BenchmarkMiddlewareChain_0$" -benchmem -count=3 2>&1 | tee /tmp/orig_mw.txt

echo ""
echo "=== 恢复优化版本 ==="
git stash pop

echo "=== 测试优化版本 ==="
go test -bench="BenchmarkRouter_Static$" -benchmem -count=3 2>&1 | tee /tmp/opt_static.txt
go test -bench="BenchmarkRouter_Param$" -benchmem -count=3 2>&1 | tee /tmp/opt_param.txt
go test -bench="BenchmarkMiddlewareChain_0$" -benchmem -count=3 2>&1 | tee /tmp/opt_mw.txt

echo ""
echo "=== 对比结果 ==="
echo ">>> 静态路由性能:"
echo "原始版本:"
grep "BenchmarkRouter_Static-10" /tmp/orig_static.txt | awk '{print $1, $2, $3}'
echo "优化版本:"
grep "BenchmarkRouter_Static-10" /tmp/opt_static.txt | awk '{print $1, $2, $3}'

echo ""
echo ">>> 参数路由性能:"
echo "原始版本:"
grep "BenchmarkRouter_Param-10" /tmp/orig_param.txt | awk '{print $1, $2, $3}'
echo "优化版本:"
grep "BenchmarkRouter_Param-10" /tmp/opt_param.txt | awk '{print $1, $2, $3}'

echo ""
echo ">>> 中间件链性能:"
echo "原始版本:"
grep "BenchmarkMiddlewareChain_0-10" /tmp/orig_mw.txt | awk '{print $1, $2, $3}'
echo "优化版本:"
grep "BenchmarkMiddlewareChain_0-10" /tmp/opt_mw.txt | awk '{print $1, $2, $3}'
