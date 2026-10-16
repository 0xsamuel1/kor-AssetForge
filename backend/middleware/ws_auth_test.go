package middleware

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestWSAuthValidatorRejectsExpiredAndWrongTokenTypes(t *testing.T) {
	validator := NewWSAuthValidator("test-secret")
	for name, claims := range map[string]jwt.MapClaims{
		"expired": {
			"user_id": 1,
			"type":    "access",
			"exp":     time.Now().Add(-time.Minute).Unix(),
		},
		"refresh": {
			"user_id": 1,
			"type":    "refresh",
			"exp":     time.Now().Add(time.Hour).Unix(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			raw, err := token.SignedString([]byte("test-secret"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := validator.Validate(raw); err == nil {
				t.Fatal("expected token validation to fail")
			}
		})
	}
}

func TestWSAuthValidatorReturnsExpiry(t *testing.T) {
	validator := NewWSAuthValidator("test-secret")
	expiry := time.Now().Add(time.Hour).Truncate(time.Second)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": 7,
		"type":    "access",
		"exp":     expiry.Unix(),
	})
	raw, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}

	claims, err := validator.Validate("Bearer " + raw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 7 || !claims.ExpiresAt.Equal(expiry) {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}
