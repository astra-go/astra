package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env so JWT_SECRET matches the running server
	_ = godotenv.Load()

	role := "admin"
	userID := uint64(1)
	if len(os.Args) >= 2 {
		role = os.Args[1]
	}
	if len(os.Args) >= 3 {
		if n, err := strconv.ParseUint(os.Args[2], 10, 64); err == nil {
			userID = n
		}
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "change-me-in-production"
	}

	claims := jwt.MapClaims{
		"user_id":   userID,
		"tenant_id": uint64(1),
		"role":      role,
		"exp":       time.Now().Add(720 * time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(signed)
}
