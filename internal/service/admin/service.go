// Package admin provides the AdminAuthService for admin authentication,
// JWT token management, and password operations.
package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"epay/ent"
	"epay/ent/admin"
	"epay/internal/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// AdminClaims holds the JWT claims for an admin user.
type AdminClaims struct {
	jwt.RegisteredClaims
	AdminID  uuid.UUID `json:"admin_id"`
	Username string    `json:"username"`
	Role     string    `json:"role"`
}

// Service handles admin authentication business logic.
type Service struct {
	client *ent.Client
	cfg    *config.Config
}

// NewService creates a new AdminAuthService.
func NewService(client *ent.Client, cfg *config.Config) *Service {
	return &Service{client: client, cfg: cfg}
}

// LoginResult is the response from a successful login.
type LoginResult struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Login authenticates an admin by username and password, returning a JWT token.
func (s *Service) Login(ctx context.Context, username, password string) (*LoginResult, error) {
	a, err := s.client.Admin.Query().
		Where(admin.UsernameEQ(username)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrInvalidCredential
		}
		return nil, fmt.Errorf("query admin: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredential
	}

	expireHours := s.cfg.JWT.ExpireHour
	if expireHours <= 0 {
		expireHours = 24
	}
	expiresAt := time.Now().Add(time.Duration(expireHours) * time.Hour)

	claims := AdminClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Subject:   a.ID.String(),
		},
		AdminID:  a.ID,
		Username: a.Username,
		Role:     "admin",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.cfg.JWT.Secret))
	if err != nil {
		return nil, fmt.Errorf("sign token: %w", err)
	}

	return &LoginResult{
		Token:     tokenString,
		ExpiresAt: expiresAt,
	}, nil
}

// VerifyToken parses and validates a JWT token, returning the admin claims.
func (s *Service) VerifyToken(tokenString string) (*AdminClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AdminClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(s.cfg.JWT.Secret), nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*AdminClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

// ChangePassword validates the old password and sets a new one.
func (s *Service) ChangePassword(ctx context.Context, adminID uuid.UUID, oldPassword, newPassword string) error {
	if newPassword == "" {
		return errors.New("new password must not be empty")
	}

	a, err := s.client.Admin.Get(ctx, adminID)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrAdminNotFound
		}
		return fmt.Errorf("get admin: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrInvalidCredential
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	return s.client.Admin.UpdateOneID(adminID).
		SetPasswordHash(string(newHash)).
		Exec(ctx)
}

// SeedDefaultAdmin creates the default admin user if it does not already exist.
// Returns true if the admin was newly created.
func (s *Service) SeedDefaultAdmin(ctx context.Context) (bool, error) {
	exists, err := s.client.Admin.Query().
		Where(admin.UsernameEQ("admin")).
		Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("check admin existence: %w", err)
	}
	if exists {
		return false, nil
	}

	password := s.cfg.Admin.DefaultPassword
	if password == "" {
		password = "admin123"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, fmt.Errorf("hash password: %w", err)
	}

	_, err = s.client.Admin.Create().
		SetUsername("admin").
		SetPasswordHash(string(hash)).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("create admin: %w", err)
	}

	return true, nil
}

// Sentinel errors.
var (
	ErrAdminNotFound    = errors.New("admin not found")
	ErrInvalidCredential = errors.New("invalid username or password")
)
