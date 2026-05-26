package main

import (
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	db, err := gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	// 查 tenant id
	var tenantID uint
	db.Raw("SELECT id FROM tenants WHERE name='demo' LIMIT 1").Scan(&tenantID)
	fmt.Printf("tenant id = %d\n", tenantID)

	// 补 seller
	db.Exec(`INSERT INTO users (tenant_id, email, name, role, created_at, updated_at)
		SELECT ?, 'seller@demo.local', 'Demo Seller', 'seller', NOW(), NOW()
		WHERE NOT EXISTS (SELECT 1 FROM users WHERE email='seller@demo.local')`, tenantID)

	// 补 buyer
	db.Exec(`INSERT INTO users (tenant_id, email, name, role, created_at, updated_at)
		SELECT ?, 'buyer@demo.local', 'Demo Buyer', 'buyer', NOW(), NOW()
		WHERE NOT EXISTS (SELECT 1 FROM users WHERE email='buyer@demo.local')`, tenantID)

	// 补缺失的 products（Super Gizmo, Nano Widget, Rare Part）
	for _, p := range []struct{ name, cat string; price float64; stock int }{
		{"Super Gizmo", "gadgets", 149.99, 30},
		{"Nano Widget", "widgets", 4.99, 500},
		{"Rare Part", "misc", 299.99, 3},
	} {
		db.Exec(`INSERT INTO products (tenant_id, name, price, stock, category, created_at, updated_at)
			SELECT ?, ?, ?, ?, ?, NOW(), NOW()
			WHERE NOT EXISTS (SELECT 1 FROM products WHERE tenant_id=? AND name=?)`,
			tenantID, p.name, p.price, p.stock, p.cat, tenantID, p.name)
	}

	fmt.Println("=== users after seed ===")
	rows, _ := db.Raw("SELECT id, email, role FROM users ORDER BY id").Rows()
	defer rows.Close()
	for rows.Next() {
		var id uint
		var email, role string
		rows.Scan(&id, &email, &role)
		fmt.Printf("  id=%-3d role=%-8s email=%s\n", id, role, email)
	}

	fmt.Println("=== products after seed ===")
	rows2, _ := db.Raw("SELECT id, name, stock FROM products WHERE deleted_at IS NULL ORDER BY id").Rows()
	defer rows2.Close()
	for rows2.Next() {
		var id, stock uint
		var name string
		rows2.Scan(&id, &name, &stock)
		fmt.Printf("  id=%-3d stock=%-5d name=%s\n", id, stock, name)
	}
}
