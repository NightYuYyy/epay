// Package user provides UserService for user CRUD, authentication, and
// settlement provisioning. A user owns one or more Products which carry the
// EasyPay API credentials.
package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"epay/ent"
	"epay/ent/user"

	"golang.org/x/crypto/bcrypt"

	"github.com/google/uuid"
)

// Service handles user business logic.
type Service struct {
	client *ent.Client
}

// NewService constructs a Service.
func NewService(client *ent.Client) *Service {
	return &Service{client: client}
}

// Info is the public-facing representation of a user.
type Info struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	FeeRate   float64   `json:"fee_rate"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListResult holds paginated list output.
type ListResult struct {
	Items      []Info `json:"items"`
	Total      int    `json:"total"`
	Page       int    `json:"page"`
	Limit      int    `json:"limit"`
	TotalPages int    `json:"total_pages"`
}

func validateFeeRate(rate float64) error {
	if rate < 0 || rate > 1 {
		return ErrInvalidFeeRate
	}
	return nil
}

// Register creates a self-service user account. Pairs with a per-user
// Settlement record so balance lookups always succeed.
func (s *Service) Register(ctx context.Context, email, password, name string, feeRate *float64) (*Info, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" || name == "" {
		return nil, ErrInvalidInput
	}
	if feeRate != nil {
		if err := validateFeeRate(*feeRate); err != nil {
			return nil, err
		}
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}

	exists, err := s.client.User.Query().Where(user.Email(email)).Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("check email exists: %w", err)
	}
	if exists {
		return nil, ErrEmailTaken
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start registration transaction: %w", err)
	}
	defer tx.Rollback()

	create := tx.User.Create().
		SetEmail(email).
		SetPasswordHash(hash).
		SetName(name)
	if feeRate != nil {
		create.SetFeeRate(*feeRate)
	}
	u, err := create.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if _, err := tx.Settlement.Create().
		SetUserID(u.ID).
		SetBalance(0).
		SetFrozen(0).
		SetTotalIncome(0).
		SetTotalWithdrawn(0).
		Save(ctx); err != nil {
		return nil, fmt.Errorf("create settlement: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit registration: %w", err)
	}

	return toInfo(u), nil
}

// VerifyCredential authenticates a user by email + password.
func (s *Service) VerifyCredential(ctx context.Context, email, password string) (*Info, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := s.client.User.Query().Where(user.Email(email)).First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrInvalidCredential
		}
		return nil, fmt.Errorf("load user: %w", err)
	}
	if u.Status != user.StatusActive {
		return nil, ErrUserDisabled
	}
	if !verifyPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredential
	}
	return toInfo(u), nil
}

// Get returns a user by id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Info, error) {
	u, err := s.client.User.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return toInfo(u), nil
}

// Update mutates allowed fields. Pass nil to leave a field unchanged.
func (s *Service) Update(ctx context.Context, id uuid.UUID, name *string, feeRate *float64, status *string) (*Info, error) {
	upd := s.client.User.UpdateOneID(id)
	if name != nil {
		upd.SetName(*name)
	}
	if feeRate != nil {
		if err := validateFeeRate(*feeRate); err != nil {
			return nil, err
		}
		upd.SetFeeRate(*feeRate)
	}
	if status != nil {
		switch *status {
		case "active":
			upd.SetStatus(user.StatusActive)
		case "disabled":
			upd.SetStatus(user.StatusDisabled)
		default:
			return nil, fmt.Errorf("invalid status: %s", *status)
		}
	}
	u, err := upd.Save(ctx)
	if err != nil {
		return nil, err
	}
	return toInfo(u), nil
}

// ChangePassword updates the password after verifying the current one.
func (s *Service) ChangePassword(ctx context.Context, id uuid.UUID, current, next string) error {
	if next == "" {
		return ErrInvalidInput
	}
	u, err := s.client.User.Get(ctx, id)
	if err != nil {
		return err
	}
	if !verifyPassword(u.PasswordHash, current) {
		return ErrInvalidCredential
	}
	hash, err := hashPassword(next)
	if err != nil {
		return err
	}
	return s.client.User.UpdateOneID(id).SetPasswordHash(hash).Exec(ctx)
}

// List returns a paginated list of users, optionally filtered by status.
func (s *Service) List(ctx context.Context, page, limit int, statusFilter string) (*ListResult, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	q := s.client.User.Query()
	if statusFilter != "" {
		q.Where(user.StatusEQ(user.Status(statusFilter)))
	}
	total, err := q.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}
	rows, err := q.Limit(limit).Offset((page - 1) * limit).Order(ent.Desc("created_at")).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	items := make([]Info, 0, len(rows))
	for _, u := range rows {
		items = append(items, *toInfo(u))
	}
	totalPages := (total + limit - 1) / limit
	return &ListResult{Items: items, Total: total, Page: page, Limit: limit, TotalPages: totalPages}, nil
}

func toInfo(u *ent.User) *Info {
	return &Info{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		FeeRate:   u.FeeRate,
		Status:    string(u.Status),
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

func hashPassword(password string) (string, error) {
	if password == "" {
		return "", ErrInvalidInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func verifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// Sentinel errors.
var (
	ErrInvalidInput      = errors.New("email, password, and name are required")
	ErrEmailTaken        = errors.New("email is already registered")
	ErrInvalidCredential = errors.New("invalid email or password")
	ErrUserDisabled      = errors.New("account disabled")
	ErrInvalidFeeRate    = errors.New("fee rate must be between 0 and 1")
)
