//go:build sonic

package astra

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── SonicStd Tests ───────────────────────────────────────────────────────────

func TestSonicStd_MarshalUnmarshal(t *testing.T) {
	type User struct {
		Name  string `json:"name"`
		Age   int    `json:"age"`
		Email string `json:"email"`
	}

	original := User{
		Name:  "张三", // Chinese characters
		Age:   30,
		Email: "zhangsan@example.com",
	}

	// Marshal
	data, err := SonicStd.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	var decoded User
	if err := SonicStd.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify
	if decoded.Name != original.Name {
		t.Errorf("Name mismatch: got %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Age != original.Age {
		t.Errorf("Age mismatch: got %d, want %d", decoded.Age, original.Age)
	}
	if decoded.Email != original.Email {
		t.Errorf("Email mismatch: got %q, want %q", decoded.Email, original.Email)
	}
}

func TestSonicFast_MarshalUnmarshal(t *testing.T) {
	type Product struct {
		ID    int     `json:"id"`
		Name  string  `json:"name"`
		Price float64 `json:"price"`
	}

	original := Product{
		ID:    12345,
		Name:  "商品名称", // Chinese characters
		Price: 99.99,
	}

	data, err := SonicFast.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Product
	if err := SonicFast.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %d, want %d", decoded.ID, original.ID)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name mismatch: got %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Price != original.Price {
		t.Errorf("Price mismatch: got %f, want %f", decoded.Price, original.Price)
	}
}

// ─── EncodeInto Tests (Zero-Copy Path) ───────────────────────────────────────

func TestSonicStd_EncodeInto(t *testing.T) {
	type Response struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	resp := Response{
		Status:  "success",
		Message: "操作成功", // Chinese characters
	}

	var buf bytes.Buffer
	if err := SonicStd.EncodeInto(&buf, resp); err != nil {
		t.Fatalf("EncodeInto failed: %v", err)
	}

	// Verify no trailing newline (should match Marshal output)
	expected, err := SonicStd.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("EncodeInto output mismatch:\ngot:  %s\nwant: %s", buf.Bytes(), expected)
	}

	// Verify it's valid JSON
	var decoded Response
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Errorf("Output is not valid JSON: %v", err)
	}
}

func TestSonicFast_EncodeInto(t *testing.T) {
	data := map[string]interface{}{
		"key1": "value1",
		"key2": 12345,
		"key3": []string{"a", "b", "c"},
		"中文":   "测试", // Chinese key and value
	}

	var buf bytes.Buffer
	if err := SonicFast.EncodeInto(&buf, data); err != nil {
		t.Fatalf("EncodeInto failed: %v", err)
	}

	// Verify it's valid JSON
	var decoded map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Errorf("Output is not valid JSON: %v", err)
	}

	// Verify content
	if decoded["key1"] != "value1" {
		t.Errorf("key1 mismatch: got %v, want %v", decoded["key1"], "value1")
	}
	if decoded["中文"] != "测试" {
		t.Errorf("Chinese key mismatch: got %v, want %v", decoded["中文"], "测试")
	}
}

// ─── EncodeStream Tests ───────────────────────────────────────────────────────

func TestSonicStd_EncodeStream(t *testing.T) {
	type Item struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	item := Item{
		ID:   1,
		Name: "测试项目", // Chinese characters
	}

	var buf bytes.Buffer
	if err := SonicStd.EncodeStream(&buf, item); err != nil {
		t.Fatalf("EncodeStream failed: %v", err)
	}

	// Note: EncodeStream leaves trailing '\n', which is valid JSON whitespace
	output := buf.Bytes()

	// Verify it's valid JSON (with or without trailing newline)
	var decoded Item
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Errorf("Output is not valid JSON: %v", err)
	}

	if decoded.ID != item.ID {
		t.Errorf("ID mismatch: got %d, want %d", decoded.ID, item.ID)
	}
	if decoded.Name != item.Name {
		t.Errorf("Name mismatch: got %q, want %q", decoded.Name, item.Name)
	}
}

func TestSonicFast_EncodeStream(t *testing.T) {
	items := []map[string]interface{}{
		{"id": 1, "name": "第一项"},
		{"id": 2, "name": "第二项"},
		{"id": 3, "name": "第三项"},
	}

	for i, item := range items {
		var buf bytes.Buffer
		if err := SonicFast.EncodeStream(&buf, item); err != nil {
			t.Fatalf("EncodeStream failed for item %d: %v", i, err)
		}

		var decoded map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Errorf("Output for item %d is not valid JSON: %v", i, err)
		}
	}
}

// ─── WithSerializer Integration Tests ────────────────────────────────────────

func TestWithSerializer_SonicStd(t *testing.T) {
	app := New(WithSerializer(SonicStd))

	// Verify the serializer works end-to-end by making a request
	app.GET("/ser", func(c *Ctx) error {
		return c.JSON(200, Map{"msg": "你好世界"})
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ser")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "你好世界" {
		t.Errorf("expected 你好世界, got %v", m["msg"])
	}
}

func TestWithSerializer_SonicFast(t *testing.T) {
	app := New(WithSerializer(SonicFast))

	app.GET("/ser", func(c *Ctx) error {
		return c.JSON(200, Map{"key": "value"})
	})

	srv := httptest.NewServer(app)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ser")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ─── Chinese and Special Character Tests ─────────────────────────────────────

func TestSonicStd_ChineseCharacters(t *testing.T) {
	testCases := []struct {
		name  string
		input interface{}
	}{
		{
			name: "simple string",
			input: map[string]string{
				"message": "你好，世界！",
			},
		},
		{
			name: "nested structure",
			input: map[string]interface{}{
				"用户": map[string]string{
					"姓名": "张三",
					"地址": "北京市朝阳区",
				},
			},
		},
		{
			name: "array of Chinese",
			input: map[string]interface{}{
				"城市": []string{"北京", "上海", "广州", "深圳"},
			},
		},
		{
			name: "mixed content",
			input: map[string]interface{}{
				"title":       "中文标题",
				"description": "这是一段中文描述，包含标点符号：，。！？",
				"tags":        []string{"标签1", "标签2", "标签3"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := SonicStd.Marshal(tc.input)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			// Verify it's valid JSON
			var decoded map[string]interface{}
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Errorf("Output is not valid JSON: %v\nOutput: %s", err, data)
			}
		})
	}
}

func TestSonicStd_SpecialCharacters(t *testing.T) {
	testCases := []struct {
		name  string
		input interface{}
	}{
		{
			name: "emoji",
			input: map[string]string{
				"emoji": "😀🎉🚀",
			},
		},
		{
			name: "escape sequences",
			input: map[string]string{
				"text": "line1\nline2\ttabbed",
			},
		},
		{
			name: "unicode",
			input: map[string]string{
				"greek":    "αβγδ",
				"japanese": "日本語",
				"korean":   "한국어",
				"russian":  "русский",
			},
		},
		{
			name: "html-like",
			input: map[string]string{
				"html": "<script>alert('xss')</script>",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := SonicStd.Marshal(tc.input)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var decoded map[string]interface{}
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Errorf("Output is not valid JSON: %v\nOutput: %s", err, data)
			}
		})
	}
}

// ─── Large Data Tests (>64KB to trigger JSONStream path) ─────────────────────

func TestSonicStd_LargeData(t *testing.T) {
	// Create a large dataset (>64KB)
	type Record struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Address string `json:"address"`
	}

	// Generate enough records to exceed 64KB
	records := make([]Record, 1000)
	for i := 0; i < 1000; i++ {
		records[i] = Record{
			ID:      i,
			Name:    "用户名称测试数据",
			Email:   "user@example.com",
			Address: "北京市朝阳区测试地址123号",
		}
	}

	// Test Marshal
	data, err := SonicStd.Marshal(records)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if len(data) <= 64*1024 {
		t.Logf("Warning: data size %d bytes is <= 64KB, test may not exercise JSONStream path", len(data))
	}

	// Verify it's valid JSON
	var decoded []Record
	if err := SonicStd.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded) != len(records) {
		t.Errorf("Record count mismatch: got %d, want %d", len(decoded), len(records))
	}
}

func TestSonicStd_EncodeStream_LargeData(t *testing.T) {
	// Create large nested structure
	data := map[string]interface{}{
		"items": make([]map[string]interface{}, 500),
	}

	for i := 0; i < 500; i++ {
		data["items"].([]map[string]interface{})[i] = map[string]interface{}{
			"id":          i,
			"name":        "测试项目名称",
			"description": "这是一个详细描述，包含中文内容",
			"tags":        []string{"标签1", "标签2", "标签3"},
		}
	}

	var buf bytes.Buffer
	if err := SonicStd.EncodeStream(&buf, data); err != nil {
		t.Fatalf("EncodeStream failed: %v", err)
	}

	if buf.Len() <= 64*1024 {
		t.Logf("Warning: buffer size %d bytes is <= 64KB", buf.Len())
	}

	// Verify it's valid JSON
	var decoded map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Errorf("Output is not valid JSON: %v", err)
	}
}

func TestSonicFast_LargeData(t *testing.T) {
	// Create large map with many keys
	data := make(map[string]string, 1000)
	for i := 0; i < 1000; i++ {
		data[("key_" + string(rune('A'+i%26)) + string(rune('0'+i%10)))] = "值_" + string(rune('A'+i%26))
	}

	encoded, err := SonicFast.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded map[string]string
	if err := SonicFast.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded) != len(data) {
		t.Errorf("Key count mismatch: got %d, want %d", len(decoded), len(data))
	}
}

// ─── Interface Compliance Tests ──────────────────────────────────────────────

func TestSonicSerializer_ImplementsSerializer(t *testing.T) {
	// Compile-time interface compliance check
	var _ Serializer = SonicStd
	var _ Serializer = SonicFast
}

func TestSonicSerializer_ImplementsBufEncoder(t *testing.T) {
	// Compile-time interface compliance check
	var _ bufEncoder = SonicStd
	var _ bufEncoder = SonicFast
}

func TestSonicSerializer_ImplementsStreamEncoder(t *testing.T) {
	// Compile-time interface compliance check
	var _ streamEncoder = SonicStd
	var _ streamEncoder = SonicFast
}

// ─── Benchmark Comparisons ───────────────────────────────────────────────────

func BenchmarkSonicStd_Marshal(b *testing.B) {
	data := map[string]interface{}{
		"name":  "测试用户",
		"age":   30,
		"email": "test@example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = SonicStd.Marshal(data)
	}
}

func BenchmarkSonicFast_Marshal(b *testing.B) {
	data := map[string]interface{}{
		"name":  "测试用户",
		"age":   30,
		"email": "test@example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = SonicFast.Marshal(data)
	}
}

func BenchmarkSonicStd_EncodeInto(b *testing.B) {
	data := map[string]interface{}{
		"name":  "测试用户",
		"age":   30,
		"email": "test@example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = SonicStd.EncodeInto(&buf, data)
	}
}

func BenchmarkSonicFast_EncodeInto(b *testing.B) {
	data := map[string]interface{}{
		"name":  "测试用户",
		"age":   30,
		"email": "test@example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = SonicFast.EncodeInto(&buf, data)
	}
}
