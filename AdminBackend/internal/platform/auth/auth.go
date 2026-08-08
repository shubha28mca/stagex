// Package auth is the admin platform authentication toolkit: password hashing,
// role-aware JWTs, a protect middleware and a RequireRole gate. Roles are
// "ops" (Operational Admin) and "event" (Event Admin) — Super Admin is out of
// scope. The two roles never share routes: each route tree is guarded by
// RequireRole so an Event Admin token cannot reach Ops endpoints and vice versa.
package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Roles.
const (
	RoleOps   = "ops"
	RoleEvent = "event"
)

// Manager issues and validates admin tokens.
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager builds a token Manager.
func NewManager(secret string, ttl time.Duration) *Manager {
	return &Manager{secret: []byte(secret), ttl: ttl}
}

// Claims is the admin JWT payload.
type Claims struct {
	Role  string `json:"role"`
	Email string `json:"email"`
	Name  string `json:"name"`
	jwt.RegisteredClaims
}

// HashPassword returns a bcrypt hash.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether plain matches the stored hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// Issue signs a token for an admin.
func (m *Manager) Issue(adminID, email, name, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		Role:  role,
		Email: email,
		Name:  name,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   adminID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
			Issuer:    "stagex-admin",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Parse validates a token string.
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

type ctxKey struct{}

// Identity is the authenticated admin placed on the request context.
type Identity struct {
	AdminID string
	Email   string
	Name    string
	Role    string
}

// Middleware requires a valid Bearer token and injects the Identity.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			httpx.Error(w, httpx.ErrUnauthorized("missing bearer token"))
			return
		}
		claims, err := m.Parse(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			httpx.Error(w, httpx.ErrUnauthorized("invalid or expired token"))
			return
		}
		id := Identity{AdminID: claims.Subject, Email: claims.Email, Name: claims.Name, Role: claims.Role}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

// RequireRole wraps a protected handler and rejects tokens whose role does not
// match — this is what keeps the Ops and Event Admin route trees disjoint.
func (m *Manager) RequireRole(role string, next http.Handler) http.Handler {
	return m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := FromContext(r.Context())
		if id.Role != role {
			httpx.Error(w, httpx.ErrForbidden("your role cannot access this area"))
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// FromContext returns the authenticated admin Identity, if present.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}
