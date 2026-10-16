package middleware

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// WSClaims contains the identity and expiry needed by a WebSocket session.
type WSClaims struct {
	UserID    uint
	ExpiresAt time.Time
}

// WSAuthValidator validates access tokens before a WebSocket upgrade.
type WSAuthValidator struct {
	secret []byte
}

func NewWSAuthValidator(secret string) *WSAuthValidator {
	return &WSAuthValidator{secret: []byte(secret)}
}

// Validate accepts either a raw JWT or a Bearer authorization value.
func (v *WSAuthValidator) Validate(raw string) (*WSClaims, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
	if raw == "" {
		return nil, fmt.Errorf("token is required")
	}

	token, err := jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("invalid signing method")
		}
		return v.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid or expired token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}
	if tokenType, _ := claims["type"].(string); tokenType != "access" {
		return nil, fmt.Errorf("access token required")
	}
	userID, ok := claims["user_id"].(float64)
	if !ok || userID < 1 {
		return nil, fmt.Errorf("invalid user id")
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil, fmt.Errorf("token expiry is required")
	}

	return &WSClaims{UserID: uint(userID), ExpiresAt: time.Unix(int64(exp), 0)}, nil
}
