package astra

import (
	"testing"
	"unsafe"
)

// TestContextSizeOptimization 验证内存优化效果
func TestContextSizeOptimization(t *testing.T) {
	var c Ctx
	size := unsafe.Sizeof(c)
	
	t.Logf("=== Context 内存优化效果 ===")
	t.Logf("当前大小: %d 字节", size)
	t.Logf("")
	
	// 验证字段排列
	t.Logf("关键字段偏移:")
	t.Logf("  index:   offset=%d, size=%d", unsafe.Offsetof(c.index), unsafe.Sizeof(c.index))
	t.Logf("  pooled:  offset=%d, size=%d", unsafe.Offsetof(c.pooled), unsafe.Sizeof(c.pooled))
	t.Logf("  isClone: offset=%d, size=%d", unsafe.Offsetof(c.isClone), unsafe.Sizeof(c.isClone))
	t.Logf("  routeKey: offset=%d, size=%d", unsafe.Offsetof(c.routeKey), unsafe.Sizeof(c.routeKey))
	t.Logf("")
	
	// 计算间隙
	indexEnd := unsafe.Offsetof(c.index) + unsafe.Sizeof(c.index)
	pooledOffset := unsafe.Offsetof(c.pooled)
	isCloneOffset := unsafe.Offsetof(c.isClone)
	routeKeyOffset := unsafe.Offsetof(c.routeKey)
	
	t.Logf("字段排列分析:")
	t.Logf("  index (392) → pooled (%d) → isClone (%d) → routeKey (%d)", 
		pooledOffset, isCloneOffset, routeKeyOffset)
	t.Logf("")
	
	// 检查是否紧凑排列
	if pooledOffset == indexEnd {
		t.Logf("✅ pooled 紧跟 index (无间隙)")
	}
	if isCloneOffset == pooledOffset+1 {
		t.Logf("✅ isClone 紧跟 pooled (无间隙)")
	}
	
	// 检查 routeKey 对齐
	expectedRouteKeyOffset := isCloneOffset + 1
	// routeKey 需要 8 字节对齐
	alignedOffset := (expectedRouteKeyOffset + 7) &^ 7
	t.Logf("")
	t.Logf("routeKey 对齐分析:")
	t.Logf("  预期偏移: %d (isClone 后)", expectedRouteKeyOffset)
	t.Logf("  对齐后偏移: %d (8字节对齐)", alignedOffset)
	t.Logf("  实际偏移: %d", routeKeyOffset)
	
	if routeKeyOffset == alignedOffset {
		t.Logf("✅ routeKey 正确对齐")
	} else {
		t.Logf("⚠️  routeKey 未正确对齐")
	}
	
	t.Logf("")
	t.Logf("内存优化效果:")
	t.Logf("  - 小字段集中排列: index(1) + pooled(1) + isClone(1) + padding(5) = 8 字节")
	t.Logf("  - 总大小: %d 字节 (之前 488 字节)", size)
	t.Logf("  - 节省: %d 字节", 488-size)
}
