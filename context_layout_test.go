package astra

import (
	"testing"
	"unsafe"
)

// TestContextLayout 分析 Context 结构体的内存布局
func TestContextLayout(t *testing.T) {
	var c Ctx
	
	// 整体大小
	totalSize := unsafe.Sizeof(c)
	t.Logf("=== Context 结构体内存布局分析 ===")
	t.Logf("总大小: %d 字节 (%.2f KB)", totalSize, float64(totalSize)/1024)
	t.Logf("")
	
	// 关键字段分析
	typeLayout := []struct {
		name   string
		offset uintptr
		size   uintptr
	}{
		// Request
		{"req", unsafe.Offsetof(c.req), unsafe.Sizeof(c.req)},
		
		// Debug fields (empty struct in production)
		{"debugFields", unsafe.Offsetof(c.debugFields), unsafe.Sizeof(c.debugFields)},
		
		// Response writer
		{"rw", unsafe.Offsetof(c.rw), unsafe.Sizeof(c.rw)},
		{"writer", unsafe.Offsetof(c.writer), unsafe.Sizeof(c.writer)},
		
		// Params
		{"paramsArr", unsafe.Offsetof(c.paramsArr), unsafe.Sizeof(c.paramsArr)},
		{"params", unsafe.Offsetof(c.params), unsafe.Sizeof(c.params)},
		{"overflowParams", unsafe.Offsetof(c.overflowParams), unsafe.Sizeof(c.overflowParams)},
		
		// Handler chain
		{"handlers", unsafe.Offsetof(c.handlers), unsafe.Sizeof(c.handlers)},
		{"index", unsafe.Offsetof(c.index), unsafe.Sizeof(c.index)},
		
		// Route key
		{"routeKey", unsafe.Offsetof(c.routeKey), unsafe.Sizeof(c.routeKey)},
		{"allowedMethods", unsafe.Offsetof(c.allowedMethods), unsafe.Sizeof(c.allowedMethods)},
		
		// KV store
		{"kvStore", unsafe.Offsetof(c.kvStore), unsafe.Sizeof(c.kvStore)},
		{"kvMap", unsafe.Offsetof(c.kvMap), unsafe.Sizeof(c.kvMap)},
		
		// Query cache
		{"queryCache", unsafe.Offsetof(c.queryCache), unsafe.Sizeof(c.queryCache)},
		
		// App reference
		{"app", unsafe.Offsetof(c.app), unsafe.Sizeof(c.app)},
		
		// Flags
		{"pooled", unsafe.Offsetof(c.pooled), unsafe.Sizeof(c.pooled)},
		{"isClone", unsafe.Offsetof(c.isClone), unsafe.Sizeof(c.isClone)},
	}
	
	t.Logf("字段详情:")
	t.Logf("%-20s %10s %10s %10s", "字段名", "偏移量", "大小", "填充")
	
	var lastEnd uintptr
	var totalPadding uintptr
	
	for _, field := range typeLayout {
		padding := field.offset - lastEnd
		if padding > 0 && lastEnd > 0 {
			totalPadding += padding
			t.Logf("%-20s %10d %10d %10d ← 填充 %d 字节!", field.name, field.offset, field.size, padding, padding)
		} else {
			t.Logf("%-20s %10d %10d %10d", field.name, field.offset, field.size, 0)
		}
		lastEnd = field.offset + field.size
	}
	
	t.Logf("")
	t.Logf("=== 内存使用分析 ===")
	t.Logf("有效数据: %d 字节", totalSize-totalPadding)
	t.Logf("填充浪费: %d 字节 (%.1f%%)", totalPadding, float64(totalPadding)/float64(totalSize)*100)
	t.Logf("")
	
	// 分析 index int8 优化效果
	t.Logf("=== index int8 优化效果 ===")
	t.Logf("index 字段偏移: %d", unsafe.Offsetof(c.index))
	t.Logf("index 字段大小: %d 字节", unsafe.Sizeof(c.index))
	t.Logf("index 字段类型: int8")
	t.Logf("")
	
	// 检查前一个字段
	handlersOffset := unsafe.Offsetof(c.handlers)
	handlersSize := unsafe.Sizeof(c.handlers)
	indexOffset := unsafe.Offsetof(c.index)
	
	gap := indexOffset - (handlersOffset + handlersSize)
	
	t.Logf("前一个字段: handlers")
	t.Logf("  - 偏移: %d", handlersOffset)
	t.Logf("  - 大小: %d", handlersSize)
	t.Logf("  - 结束: %d", handlersOffset+handlersSize)
	t.Logf("")
	t.Logf("index 字段:")
	t.Logf("  - 偏移: %d", indexOffset)
	t.Logf("  - 间隙: %d 字节", gap)
	t.Logf("")
	
	if gap > 0 {
		t.Logf("⚠️  发现 %d 字节填充！", gap)
		t.Logf("原因: handlers (切片) 需要对齐到 8 字节边界")
	} else {
		t.Logf("✅ 无填充，index 紧跟 handlers")
	}
	
	// 检查后一个字段
	routeKeyOffset := unsafe.Offsetof(c.routeKey)
	indexSize := unsafe.Sizeof(c.index)
	gap2 := routeKeyOffset - (indexOffset + indexSize)
	
	t.Logf("")
	t.Logf("后一个字段: routeKey")
	t.Logf("  - 偏移: %d", routeKeyOffset)
	t.Logf("  - 间隙: %d 字节", gap2)
	t.Logf("")
	
	if gap2 > 0 {
		t.Logf("⚠️  发现 %d 字节填充！", gap2)
		t.Logf("原因: routeKey (string) 需要对齐到 8 字节边界")
	} else {
		t.Logf("✅ 无填充，routeKey 紧跟 index")
	}
	
	// 建议
	t.Logf("")
	t.Logf("=== 优化建议 ===")
	t.Logf("1. 将 bool 字段 (pooled, isClone) 移到 index 旁边")
	t.Logf("   - 可以将 3 个小字段 (int8 + 2*bool) 填充到同一个 8 字节槽")
	t.Logf("   - 节省 2-4 字节填充")
	t.Logf("")
	t.Logf("2. 分析 paramsArr 大小")
	t.Logf("   - 当前大小: %d 字节", unsafe.Sizeof(c.paramsArr))
	t.Logf("   - 如果路由参数很少，可以减少 maxRouteParams")
	t.Logf("")
}

// TestParamSize 分析 Param 结构体大小
func TestParamSize(t *testing.T) {
	var p Param
	size := unsafe.Sizeof(p)
	
	t.Logf("=== Param 结构体分析 ===")
	t.Logf("大小: %d 字节", size)
	t.Logf("字段:")
	t.Logf("  - Key: string (16 字节)")
	t.Logf("  - Value: string (16 字节)")
	t.Logf("")
	t.Logf("paramsArr[%d] 总大小: %d 字节", maxRouteParams, size*maxRouteParams)
}

// TestResponseWriterSize 分析 responseWriter 结构体大小
func TestResponseWriterSize(t *testing.T) {
	var rw responseWriter
	size := unsafe.Sizeof(rw)
	
	t.Logf("=== responseWriter 结构体分析 ===")
	t.Logf("大小: %d 字节", size)
	t.Logf("")
	t.Logf("嵌入式结构，包含:")
	t.Logf("  - ResponseWriter 接口 (16 字节)")
	t.Logf("  - status int (8 字节)")
	t.Logf("  - size int (8 字节)")
	t.Logf("  - written bool (1 字节 + 填充)")
}
