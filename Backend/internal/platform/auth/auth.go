// Package auth is the platform-level authentication toolkit: password hashing,
// JWT issuing/verification, and the HTTP middleware that protects routes.
//
// It is intentionally domain-agnostic — it knows about "a subject id" (the
// family account id) and nothing about people, events or payments. Domain
// packages depend on this; this depends on nothing domain-specific.
package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Manager issues and validates tokens using a shared secret.
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager builds a token Manager.
func NewManager(secret string, ttl time.Duration) *Manager {
	return &Manager{secret: []byte(secret), ttl: ttl}
}

// Claims is the JWT payload. Subject is the family account id.
type Claims struct {
	Phone string `json:"phone"`
	jwt.RegisteredClaims
}

// HashPassword returns a bcrypt hash suitable for storage.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether plain matches the stored bcrypt hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// Issue creates a signed JWT for the given account id and phone.
func (m *Manager) Issue(accountID, phone string) (string, error) {
	now := time.Now()
	claims := Claims{
		Phone: phone,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   accountID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
			Issuer:    "stagex",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(m.secret)
}

// Parse validates a token string and returns its claims.
func (m *Manager) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// ctxKey avoids context collisions.
type ctxKey struct{}

// Identity is the authenticated caller placed on the request context.
type Identity struct {
	AccountID string
	Phone     string
}

// Middleware protects a handler: it requires a valid Bearer token and injects
// the resolved Identity into the request context.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			httpx.Error(w, httpx.ErrUnauthorized("missing bearer token"))
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		claims, err := m.Parse(token)
		if err != nil {
			httpx.Error(w, httpx.ErrUnauthorized("invalid or expired token"))
			return
		}
		id := Identity{AccountID: claims.Subject, Phone: claims.Phone}
		ctx := context.WithValue(r.Context(), ctxKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromContext returns the authenticated Identity, if present.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}
