// Package product provides ProductService for product CRUD and pid/pkey
// credential management. A product is owned by a user and carries the
// EasyPay API credentials used by third parties to call /mapi.php.
package product

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"epay/ent"
	"epay/ent/product"

	"github.com/google/uuid"
)

// Service handles product business logic.
type Service struct {
	client *ent.Client
}

// NewService constructs a Service.
func NewService(client *ent.Client) *Service {
	return &Service{client: client}
}

// Info is the public-facing representation of a product.
// Pkey is returned only on create / regenerate / GetWithSecret.
type Info struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Pid         int       `json:"pid"`
	Pkey        string    `json:"pkey,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	NotifyURL   string    `json:"notify_url"`
	ReturnURL   string    `json:"return_url"`
	FeeRate     *float64  `json:"fee_rate,omitempty"`
	Status      string    `json:"status"`
	Keytype     int       `json:"keytype"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListResult holds paginated product list results.
type ListResult struct {
	Items      []Info `json:"items"`
	Total      int    `json:"total"`
	Page       int    `json:"page"`
	Limit      int    `json:"limit"`
	TotalPages int    `json:"total_pages"`
}

// CreateParams describes the input for product creation.
type CreateParams struct {
	UserID      uuid.UUID
	Name        string
	Description string
	NotifyURL   string
	ReturnURL   string
	FeeRate     *float64
}

func validateFeeRate(rate float64) error {
	if rate < 0 || rate > 1 {
		return ErrInvalidFeeRate
	}
	return nil
}

// Create allocates a unique pid + pkey and inserts the product.
// The returned Info carries the plaintext pkey (call site is responsible for
// surfacing it to the user securely).
func (s *Service) Create(ctx context.Context, params CreateParams) (*Info, error) {
	if strings.TrimSpace(params.Name) == "" {
		return nil, ErrNameRequired
	}
	if params.FeeRate != nil {
		if err := validateFeeRate(*params.FeeRate); err != nil {
			return nil, err
		}
	}
	pkey := generatePkey()
	for attempt := 0; attempt < 32; attempt++ {
		pid, err := s.generateUniquePid(ctx)
		if err != nil {
			return nil, fmt.Errorf("generate pid: %w", err)
		}
		create := s.client.Product.Create().
			SetUserID(params.UserID).
			SetPid(pid).
			SetPkey(pkey).
			SetName(strings.TrimSpace(params.Name)).
			SetDescription(strings.TrimSpace(params.Description)).
			SetNotifyURL(strings.TrimSpace(params.NotifyURL)).
			SetReturnURL(strings.TrimSpace(params.ReturnURL))
		if params.FeeRate != nil {
			create.SetFeeRate(*params.FeeRate)
		}
		p, err := create.Save(ctx)
		if err == nil {
			return toInfo(p, true), nil
		}
		if ent.IsConstraintError(err) {
			continue
		}
		return nil, fmt.Errorf("create product: %w", err)
	}
	return nil, fmt.Errorf("create product: could not allocate a unique pid after 32 attempts")
}

// Get returns a product by id. Pkey is masked unless includeSecret is true.
func (s *Service) Get(ctx context.Context, id uuid.UUID, includeSecret bool) (*Info, error) {
	p, err := s.client.Product.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return toInfo(p, includeSecret), nil
}

// GetForUser returns a product by id scoped to one owner. Pkey is masked unless includeSecret is true.
func (s *Service) GetForUser(ctx context.Context, id, userID uuid.UUID, includeSecret bool) (*Info, error) {
	p, err := s.client.Product.Query().Where(product.IDEQ(id), product.UserIDEQ(userID)).Only(ctx)
	if err != nil {
		return nil, err
	}
	return toInfo(p, includeSecret), nil
}

// GetByPid returns a product by its public pid. Used by EasyPay handler.
// includeSecret returns the pkey for sign-verification flows.
func (s *Service) GetByPid(ctx context.Context, pid int, includeSecret bool) (*Info, error) {
	p, err := s.client.Product.Query().Where(product.Pid(pid)).First(ctx)
	if err != nil {
		return nil, err
	}
	return toInfo(p, includeSecret), nil
}

// ListByUser returns the user's products (most recent first).
func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID, page, limit int) (*ListResult, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	q := s.client.Product.Query().Where(product.UserID(userID))
	total, err := q.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count products: %w", err)
	}
	rows, err := q.Limit(limit).Offset((page - 1) * limit).Order(ent.Desc("created_at")).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	items := make([]Info, 0, len(rows))
	for _, p := range rows {
		items = append(items, *toInfo(p, false))
	}
	totalPages := (total + limit - 1) / limit
	return &ListResult{Items: items, Total: total, Page: page, Limit: limit, TotalPages: totalPages}, nil
}

// ListAll returns all products (admin scope).
func (s *Service) ListAll(ctx context.Context, page, limit int, statusFilter string) (*ListResult, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	q := s.client.Product.Query()
	if statusFilter != "" {
		q.Where(product.StatusEQ(product.Status(statusFilter)))
	}
	total, err := q.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count products: %w", err)
	}
	rows, err := q.Limit(limit).Offset((page - 1) * limit).Order(ent.Desc("created_at")).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	items := make([]Info, 0, len(rows))
	for _, p := range rows {
		items = append(items, *toInfo(p, false))
	}
	totalPages := (total + limit - 1) / limit
	return &ListResult{Items: items, Total: total, Page: page, Limit: limit, TotalPages: totalPages}, nil
}

// UpdateParams describes a partial update.
type UpdateParams struct {
	Name        *string
	Description *string
	NotifyURL   *string
	ReturnURL   *string
	FeeRate     *float64
	ClearFee    bool // true means: clear product.fee_rate so user.fee_rate is used
	Status      *string
}

// Update mutates allowed fields. ownerScope, when non-nil, requires the
// updated row to belong to that user (self-service safety).
func (s *Service) Update(ctx context.Context, id uuid.UUID, ownerScope *uuid.UUID, params UpdateParams) (*Info, error) {
	if params.ClearFee && params.FeeRate != nil {
		return nil, ErrConflictingFeeUpdate
	}
	if params.FeeRate != nil {
		if err := validateFeeRate(*params.FeeRate); err != nil {
			return nil, err
		}
	}
	upd := s.client.Product.Update().Where(product.IDEQ(id))
	if ownerScope != nil {
		upd.Where(product.UserIDEQ(*ownerScope))
	}
	if params.Name != nil {
		upd.SetName(strings.TrimSpace(*params.Name))
	}
	if params.Description != nil {
		upd.SetDescription(strings.TrimSpace(*params.Description))
	}
	if params.NotifyURL != nil {
		upd.SetNotifyURL(strings.TrimSpace(*params.NotifyURL))
	}
	if params.ReturnURL != nil {
		upd.SetReturnURL(strings.TrimSpace(*params.ReturnURL))
	}
	if params.ClearFee {
		upd.ClearFeeRate()
	} else if params.FeeRate != nil {
		upd.SetFeeRate(*params.FeeRate)
	}
	if params.Status != nil {
		switch *params.Status {
		case "active":
			upd.SetStatus(product.StatusActive)
		case "disabled":
			upd.SetStatus(product.StatusDisabled)
		default:
			return nil, fmt.Errorf("invalid status: %s", *params.Status)
		}
	}
	affected, err := upd.Save(ctx)
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		if ownerScope != nil {
			return nil, ErrNotOwned
		}
		return nil, fmt.Errorf("product not found")
	}
	return s.Get(ctx, id, false)
}

// RegeneratePkey rolls the API signing key. ownerScope, when non-nil,
// requires the product to belong to that user.
func (s *Service) RegeneratePkey(ctx context.Context, id uuid.UUID, ownerScope *uuid.UUID) (string, error) {
	newKey := generatePkey()
	upd := s.client.Product.Update().Where(product.IDEQ(id))
	if ownerScope != nil {
		upd.Where(product.UserIDEQ(*ownerScope))
	}
	affected, err := upd.SetPkey(newKey).Save(ctx)
	if err != nil {
		return "", err
	}
	if affected == 0 {
		if ownerScope != nil {
			return "", ErrNotOwned
		}
		return "", fmt.Errorf("product not found")
	}
	return newKey, nil
}

func (s *Service) generateUniquePid(ctx context.Context) (int, error) {
	for attempt := 0; attempt < 32; attempt++ {
		n, err := rand.Int(rand.Reader, big.NewInt(9000))
		if err != nil {
			return 0, err
		}
		pid := int(n.Int64()) + 1000
		exists, err := s.client.Product.Query().Where(product.Pid(pid)).Exist(ctx)
		if err != nil {
			return 0, err
		}
		if !exists {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("could not allocate a unique pid after 32 attempts")
}

func generatePkey() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func toInfo(p *ent.Product, includeSecret bool) *Info {
	out := &Info{
		ID:          p.ID,
		UserID:      p.UserID,
		Pid:         p.Pid,
		Name:        p.Name,
		Description: p.Description,
		NotifyURL:   p.NotifyURL,
		ReturnURL:   p.ReturnURL,
		FeeRate:     p.FeeRate,
		Status:      string(p.Status),
		Keytype:     p.Keytype,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
	if includeSecret {
		out.Pkey = p.Pkey
	}
	return out
}

// Sentinel errors.
var (
	ErrNameRequired         = errors.New("product name is required")
	ErrNotOwned             = errors.New("product does not belong to this user")
	ErrInvalidFeeRate       = errors.New("fee rate must be between 0 and 1")
	ErrConflictingFeeUpdate = errors.New("clear_fee and fee_rate are mutually exclusive")
)
