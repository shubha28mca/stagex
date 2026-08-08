// Package identity handles admin authentication: login (email + password) and
// the seeding of mock admin accounts for local testing. It is shared by both
// role areas; the roles diverge only after login, at the route layer.
package identity

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/auth"
	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Admin is a console user (never serializes its password hash).
type Admin struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	PasswordHash string `json:"-"`
}

// Service implements login and bootstrap.
type Service struct {
	pool   *pgxpool.Pool
	tokens *auth.Manager
}

// NewService builds an identity Service.
func NewService(pool *pgxpool.Pool, tokens *auth.Manager) *Service {
	return &Service{pool: pool, tokens: tokens}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
	Admin Admin  `json:"admin"`
}

// Login verifies credentials and returns a role-bearing token.
func (s *Service) Login(ctx context.Context, email, password string) (authResponse, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, email, role, password_hash FROM admin_users WHERE email=$1 AND is_active`, email)
	var a Admin
	if err := row.Scan(&a.ID, &a.Name, &a.Email, &a.Role, &a.PasswordHash); err != nil {
		if err == pgx.ErrNoRows {
			return authResponse{}, httpx.ErrUnauthorized("invalid email or password")
		}
		return authResponse{}, err
	}
	if !auth.CheckPassword(a.PasswordHash, password) {
		return authResponse{}, httpx.ErrUnauthorized("invalid email or password")
	}
	token, err := s.tokens.Issue(a.ID, a.Email, a.Name, a.Role)
	if err != nil {
		return authResponse{}, httpx.ErrInternal("could not issue token")
	}
	a.PasswordHash = ""
	return authResponse{Token: token, Admin: a}, nil
}

// Bootstrap ensures the mock admin accounts exist so the console is testable
// immediately. Passwords are hashed here (bcrypt) rather than in SQL.
func (s *Service) Bootstrap(ctx context.Context) error {
	seed := []struct{ name, email, pass, role string }{
		{"Rohit Sharma (Ops)", "ops@stagex.test", "Ops@12345", auth.RoleOps},
		{"Priya Kapoor (Event)", "event@stagex.test", "Event@12345", auth.RoleEvent},
	}
	for _, m := range seed {
		hash, err := auth.HashPassword(m.pass)
		if err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO admin_users (name, email, password_hash, role)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT (email) DO NOTHING`, m.name, m.email, hash, m.role); err != nil {
			return err
		}
	}
	return nil
}

// Controller adapts HTTP to the identity Service.
type Controller struct {
	svc    *Service
	tokens *auth.Manager
}

// NewController builds an identity Controller.
func NewController(svc *Service, tokens *auth.Manager) *Controller {
	return &Controller{svc: svc, tokens: tokens}
}

// RegisterRoutes wires login (public) and me (protected, any role).
func (c *Controller) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/auth/login", c.login)
	mux.Handle("GET /admin/auth/me", c.tokens.Middleware(http.HandlerFunc(c.me)))
}

func (c *Controller) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	resp, err := c.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

func (c *Controller) me(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	httpx.JSON(w, http.StatusOK, map[string]string{
		"id": id.AdminID, "name": id.Name, "email": id.Email, "role": id.Role,
	})
}
