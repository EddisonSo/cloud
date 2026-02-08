package secrets

import (
	"fmt"
	"os"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

func init() {
	secret := os.Getenv("GFS_JWT_SECRET")
	if secret == "" {
		panic("GFS_JWT_SECRET environment variable is required")
	}
	jwtSecret = []byte(secret)
}

// GetSecret retrieves the shared secret for signing and verifying JWTs.
// When token is nil (signing path), returns the secret directly.
// When token is non-nil (verification path), validates the signing method.
func GetSecret(token *jwt.Token) (any, error) {
	if token != nil {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
	}
	return jwtSecret, nil
}
