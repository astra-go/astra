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

	rows, _ := db.Raw("SELECT id, email, role FROM users ORDER BY id").Rows()
	defer rows.Close()
	for rows.Next() {
		var id uint
		var email, role string
		rows.Scan(&id, &email, &role)
		fmt.Printf("id=%-3d role=%-8s email=%s\n", id, role, email)
	}

	rows2, _ := db.Raw(
		"SELECT id, name, stock FROM products WHERE deleted_at IS NULL ORDER BY id",
	).Rows()
	defer rows2.Close()
	for rows2.Next() {
		var id, stock uint
		var name string
		rows2.Scan(&id, &name, &stock)
		fmt.Printf("id=%-3d stock=%-5d name=%s\n", id, stock, name)
	}
}
