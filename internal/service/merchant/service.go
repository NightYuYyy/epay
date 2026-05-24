// Package merchant provides the MerchantService for merchant CRUD operations,
// pid/pkey credential management, and authentication verification.
package merchant

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"epay/ent"
	"epay/ent/merchant"

	"entgo.io/ent/dialect/sql"
	"golang.org/x/crypto/bcrypt"

	"github.com/google/uuid"
)

// Service handles merchant business logic.
type Service struct {
	client *ent.Client
}

// NewService creates a new MerchantService.
func NewService(client *ent.Client) *Service {
	return &Service{client: client}
}

// MerchantInfo is the public-facing representation of a merchant.
// Pkey is masked in list responses for security.
type MerchantInfo struct {
	ID        uuid.UUID `json:"id"`
	Pid       int       `json:"pid"`
	Pkey      string    `json:"pkey,omitempty"`
	Name      string    `json:"name"`
	FeeRate   float64   `json:"fee_rate"`
	Status    string    `json:"status"`
	NotifyURL string    `json:"notify_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListResult holds paginated merchant list results.
type ListResult struct {
	Items      []MerchantInfo `json:"items"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	TotalPages int            `json:"total_pages"`
}

// CreateMerchant creates a new merchant with a generated pid and pkey,
// and also creates the associated settlement record.
func (s *Service) CreateMerchant(ctx context.Context, name string, feeRate float64, notifyURL string) (*MerchantInfo, error) {
	pid, err := s.generateUniquePid(ctx)
	if err != nil {
		return nil, fmt.Errorf("generate pid: %w", err)
	}
	pkey := generatePkey()

	m, err := s.client.Merchant.Create().
		SetPid(pid).
		SetPkey(pkey).
		SetName(name).
		SetFeeRate(feeRate).
		SetNotifyURL(notifyURL).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create merchant: %w", err)
	}

	// Create settlement record for the new merchant
	_, err = s.client.Settlement.Create().
		SetMerchantID(m.ID).
		SetBalance(0).
		SetFrozen(0).
		SetTotalIncome(0).
		SetTotalWithdrawn(0).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create settlement: %w", err)
	}

	return &MerchantInfo{
		ID:        m.ID,
		Pid:       m.Pid,
		Pkey:      m.Pkey,
		Name:      m.Name,
		FeeRate:   m.FeeRate,
		Status:    string(m.Status),
		NotifyURL: m.NotifyURL,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}, nil
}

// CreateSelfRegisteredMerchant creates a merchant account that logs in with a password.
// The generated pkey remains the EasyPay API signing key and is returned once.
func (s *Service) CreateSelfRegisteredMerchant(ctx context.Context, name, password string, feeRate float64, notifyURL string) (*MerchantInfo, error) {
	passwordHash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}
	pid, err := s.generateUniquePid(ctx)
	if err != nil {
		return nil, fmt.Errorf("generate pid: %w", err)
	}
	pkey := generatePkey()

	m, err := s.client.Merchant.Create().
		SetPid(pid).
		SetPkey(pkey).
		SetPasswordHash(passwordHash).
		SetName(name).
		SetFeeRate(feeRate).
		SetNotifyURL(notifyURL).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create merchant: %w", err)
	}

	_, err = s.client.Settlement.Create().
		SetMerchantID(m.ID).
		SetBalance(0).
		SetFrozen(0).
		SetTotalIncome(0).
		SetTotalWithdrawn(0).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create settlement: %w", err)
	}

	return toMerchantInfo(m, false), nil
}

// GetMerchant returns a merchant by UUID id or integer pid.
// When maskPkey is true, the pkey field is hidden (for admin list responses).
func (s *Service) GetMerchant(ctx context.Context, id uuid.UUID, maskPkey bool) (*MerchantInfo, error) {
	m, err := s.client.Merchant.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return toMerchantInfo(m, maskPkey), nil
}

// GetByPid returns a merchant by pid. Used by EasyPay handler for sign verification.
func (s *Service) GetByPid(ctx context.Context, pid int) (*MerchantInfo, error) {
	m, err := s.client.Merchant.Query().
		Where(merchant.Pid(pid)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrMerchantNotFound
		}
		return nil, err
	}
	return toMerchantInfo(m, false), nil
}

// UpdateMerchant updates a merchant's mutable fields.
func (s *Service) UpdateMerchant(ctx context.Context, id uuid.UUID, name *string, feeRate *float64, status *string, notifyURL *string) (*MerchantInfo, error) {
	upd := s.client.Merchant.UpdateOneID(id)
	if name != nil {
		upd.SetName(*name)
	}
	if feeRate != nil {
		upd.SetFeeRate(*feeRate)
	}
	if status != nil {
		upd.SetStatus(merchant.Status(*status))
	}
	if notifyURL != nil {
		upd.SetNotifyURL(*notifyURL)
	}
	m, err := upd.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrMerchantNotFound
		}
		return nil, fmt.Errorf("update merchant: %w", err)
	}
	return toMerchantInfo(m, true), nil
}

// ListMerchants returns a paginated list of merchants, optionally filtered by status.
func (s *Service) ListMerchants(ctx context.Context, page, limit int, statusFilter string) (*ListResult, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	q := s.client.Merchant.Query()
	if statusFilter != "" {
		q.Where(merchant.StatusEQ(merchant.Status(statusFilter)))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count merchants: %w", err)
	}

	offset := (page - 1) * limit
	items, err := q.
		Order(merchant.ByCreatedAt(sql.OrderDesc())).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list merchants: %w", err)
	}

	result := make([]MerchantInfo, 0, len(items))
	for _, m := range items {
		result = append(result, *toMerchantInfo(m, true))
	}

	totalPages := (total + limit - 1) / limit
	return &ListResult{
		Items:      result,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

// RegeneratePkey generates a new random pkey for the given merchant.
func (s *Service) RegeneratePkey(ctx context.Context, id uuid.UUID) (string, error) {
	newPkey := generatePkey()
	err := s.client.Merchant.UpdateOneID(id).
		SetPkey(newPkey).
		Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", ErrMerchantNotFound
		}
		return "", fmt.Errorf("regenerate pkey: %w", err)
	}
	return newPkey, nil
}

func hashPassword(password string) (string, error) {
	if password == "" {
		return "", ErrInvalidCredential
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

// VerifyCredential verifies that a merchant with the given pid exists and the
// provided key matches the stored pkey. Used for merchant API authentication.
func (s *Service) VerifyCredential(ctx context.Context, pid int, key string) (*MerchantInfo, error) {
	m, err := s.client.Merchant.Query().
		Where(merchant.Pid(pid)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrInvalidCredential
		}
		return nil, err
	}

	if m.Status != merchant.StatusActive {
		return nil, ErrMerchantDisabled
	}

	if m.PasswordHash != "" {
		if !verifyPassword(m.PasswordHash, key) {
			return nil, ErrInvalidCredential
		}
	} else if m.Pkey != key {
		return nil, ErrInvalidCredential
	}

	return toMerchantInfo(m, false), nil
}

// generateUniquePid generates a random 4-digit pid (1000-9999) and ensures
// it is unique by checking against the database.
func (s *Service) generateUniquePid(ctx context.Context) (int, error) {
	const maxRetries = 100
	for range maxRetries {
		n, err := rand.Int(rand.Reader, big.NewInt(9000))
		if err != nil {
			return 0, fmt.Errorf("crypto rand: %w", err)
		}
		pid := int(n.Int64()) + 1000 // range 1000–9999

		exists, err := s.client.Merchant.Query().
			Where(merchant.Pid(pid)).
			Exist(ctx)
		if err != nil {
			return 0, fmt.Errorf("check pid existence: %w", err)
		}
		if !exists {
			return pid, nil
		}
	}
	return 0, errors.New("failed to generate unique pid after max retries")
}

// generatePkey generates a 32-character random hex string.
func generatePkey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto rand: %v", err))
	}
	return hex.EncodeToString(b)
}

// toMerchantInfo converts an ent.Merchant to MerchantInfo.
// When maskPkey is true, pkey is hidden.
func toMerchantInfo(m *ent.Merchant, maskPkey bool) *MerchantInfo {
	pkey := m.Pkey
	if maskPkey {
		pkey = ""
	}
	return &MerchantInfo{
		ID:        m.ID,
		Pid:       m.Pid,
		Pkey:      pkey,
		Name:      m.Name,
		FeeRate:   m.FeeRate,
		Status:    string(m.Status),
		NotifyURL: m.NotifyURL,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

// Sentinel errors.
var (
	ErrMerchantNotFound  = errors.New("merchant not found")
	ErrInvalidCredential = errors.New("invalid pid or key")
	ErrMerchantDisabled  = errors.New("merchant is disabled")
)
