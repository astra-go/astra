// Package config - 使用示例
//
// 本文件演示 Config 模块 v2.0 统一接口的使用方法。
// 包含：基础用法、类型安全构造器、工厂方法、配置监听、迁移示例。
//
// 注意：这些示例只编译验证，不实际执行（因为没有真实的配置中心服务）。
// 要运行完整测试，需要启动 Nacos/Etcd/Apollo 服务。
package config

import (
	"fmt"
	"log/slog"
	"time"
)

// ════════════════════════════════════════════════════════════════
// 示例 1: 基础用法 - Nacos 客户端
// ════════════════════════════════════════════════════════════════

func ExampleNewNacosClient() {
	// 创建 Nacos 客户端
	client, err := NewNacosClient(NacosOptions{
		ServerAddr: "localhost:8848",
		Namespace:  "public",
		Group:      "DEFAULT_GROUP",
		DataID:     "app-config.yaml",
		Format:     YAMLFormat,
		Options: Options{
			Timeout:        5 * time.Second,
			EnableCache:     true,
			LogLevel:        "warn",
			AutoRefresh:     true,
			RefreshInterval: 30 * time.Second,
		},
	})
	if err != nil {
		slog.Error("创建 Nacos 客户端失败", "error", err)
		return
	}
	defer client.Close()

	// 读取配置
	value, err := client.Get("db.host")
	if err != nil {
		slog.Error("读取配置失败", "key", "db.host", "error", err)
		return
	}
	fmt.Printf("db.host = %s\n", value)
}

// ════════════════════════════════════════════════════════════════
// 示例 2: 基础用法 - Etcd 客户端
// ════════════════════════════════════════════════════════════════

func ExampleNewEtcdClient() {
	// 创建 Etcd 客户端
	client, err := NewEtcdClient(EtcdOptions{
		Endpoints:    []string{"localhost:2379"},
		DialTimeout:  5 * time.Second,
		KeyPrefix:    "/myapp/config",
		WatchEnabled: true,
		Options: Options{
			Timeout:     5 * time.Second,
			EnableCache:  true,
			LogLevel:     "warn",
		},
	})
	if err != nil {
		slog.Error("创建 Etcd 客户端失败", "error", err)
		return
	}
	defer client.Close()

	// 读取配置
	value, err := client.Get("db.host")
	if err != nil {
		slog.Error("读取配置失败", "key", "db.host", "error", err)
		return
	}
	fmt.Printf("db.host = %s\n", value)
}

// ════════════════════════════════════════════════════════════════
// 示例 3: 使用 ConfigClient 接口（多态）
// ════════════════════════════════════════════════════════════════

func ExampleConfigClient_interface() {
	// 使用接口类型，支持多态
	var client ConfigClient

	// 可以根据配置动态选择后端
	backend := "nacos" // 从配置文件读取

	switch backend {
	case "nacos":
		client, _ = NewNacosClient(NacosOptions{
			ServerAddr: "localhost:8848",
			DataID:     "app-config.yaml",
		})
	case "etcd":
		client, _ = NewEtcdClient(EtcdOptions{
			Endpoints: []string{"localhost:2379"},
			KeyPrefix: "/myapp/config",
		})
	}

	// 统一使用 ConfigClient 接口
	value, err := client.Get("db.host")
	if err != nil {
		slog.Error("读取配置失败", "error", err)
		return
	}
	fmt.Printf("db.host = %s\n", value)

	client.Close()
}

// ════════════════════════════════════════════════════════════════
// 示例 4: 错误处理
// ════════════════════════════════════════════════════════════════

func ExampleConfigClient_errorHandling() {
	// 注意：这个示例需要真实的配置中心服务才能运行
	// 这里只是演示 API 用法
	
	fmt.Println("参考 errorHandling 示例：")
	fmt.Println("  value, err := client.Get('nonexistent.key')")
	fmt.Println("  if err == ErrKeyNotFound {")
	fmt.Println("      fmt.Println('配置键不存在')")
	fmt.Println("  }")
}

// ════════════════════════════════════════════════════════════════
// 示例 5: 迁移指南 - v1.x → v2.0
// ════════════════════════════════════════════════════════════════

func Example_v1_to_v2_migration() {
	fmt.Println("=== 迁移指南：v1.x → v2.0 ===")
	fmt.Println()
	fmt.Println("【v1.x 用法】")
	fmt.Println("  import \"github.com/astra-go/astra/config/nacos\"")
	fmt.Println("  client := nacos.NewClient(nacos.Options{...})")
	fmt.Println("  value, _ := client.Get(\"db.host\")")
	fmt.Println()
	fmt.Println("【v2.0 用法】")
	fmt.Println("  import \"github.com/astra-go/astra/config\"")
	fmt.Println("  client, _ := config.NewNacosClient(config.NacosOptions{...})")
	fmt.Println("  value, _ := client.Get(\"db.host\")")
	fmt.Println()
	fmt.Println("【主要变化】")
	fmt.Println("  1. 包路径：config/nacos → config")
	fmt.Println("  2. 构造器：nacos.NewClient() → config.NewNacosClient()")
	fmt.Println("  3. 选项类型：nacos.Options → config.NacosOptions")
	fmt.Println("  4. 统一接口：所有客户端实现 config.ConfigClient")
}

// ════════════════════════════════════════════════════════════════
// 辅助函数
// ════════════════════════════════════════════════════════════════

// demoConfigClient 演示 ConfigClient 接口的用法
func demoConfigClient(client ConfigClient) {
	// 读取字符串
	value, err := client.Get("db.host")
	if err != nil {
		slog.Error("读取配置失败", "error", err)
		return
	}
	fmt.Printf("db.host = %s\n", value)
}
