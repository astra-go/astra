package testapp

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

func validateJWT(tokenStr, secret string) error {
	parser := jwt.NewParser(jwt.WithExpirationRequired())
	token, err := parser.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return err
	}
	if !token.Valid {
		return errors.New("invalid token")
	}
	return nil
}
