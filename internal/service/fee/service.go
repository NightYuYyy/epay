package fee

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"epay/ent"
	"epay/ent/merchant"
	"epay/ent/order"
	"epay/ent/platformconfig"
	"epay/ent/settlement"
	"epay/internal/redis"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

// FeeResult holds the calculated fee breakdown for a payment.
type FeeResult struct {
	FeeOfficial  float64
	FeePlatform  float64
	NetAmount    float64
	OfficialRate float64
	PlatformRate float64
}

// FeeService calculates fees and applies settlements to merchant balances.
type FeeService struct {
	ent *ent.Client
	rdb *goredis.Client
}

// New creates a new FeeService with the given ent and redis clients.
func New(client *ent.Client, rdb *goredis.Client) *FeeService {
	return &FeeService{ent: client, rdb: rdb}
}

// CalculateFee computes the official fee, platform fee, and net amount for a payment.
//
//   - officialRate is loaded from PlatformConfig with key "official_{paymentType}_rate"
//     (stored as a decimal string, e.g. "0.006" for 0.6%)
//   - platformRate falls back to config key "default_platform_rate" if the merchant
//     has no fee_rate set (default 1.0 means 1%, applied as rate/100)
func (s *FeeService) CalculateFee(ctx context.Context, paymentType string, amount float64, merchantID uuid.UUID) (*FeeResult, error) {
	// Load merchant for platform fee_rate
	m, err := s.ent.Merchant.Query().
		Where(merchant.IDEQ(merchantID)).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("calculate fee: query merchant %s: %w", merchantID, err)
	}

	platformRate := m.FeeRate
	if platformRate == 0 {
		// Fallback to global default platform rate
		cfg, err := s.ent.PlatformConfig.Query().
			Where(platformconfig.KeyEQ("default_platform_rate")).
			Only(ctx)
		if err == nil {
			if v, parseErr := strconv.ParseFloat(cfg.Value, 64); parseErr == nil {
				platformRate = v
			}
		}
	}

	// Load official rate from global config
	configKey := fmt.Sprintf("official_%s_rate", paymentType)
	var officialRate float64
	cfg, err := s.ent.PlatformConfig.Query().
		Where(platformconfig.KeyEQ(configKey)).
		Only(ctx)
	if err == nil {
		if v, parseErr := strconv.ParseFloat(cfg.Value, 64); parseErr == nil {
			officialRate = v
		}
	}

	feeOfficial := amount * officialRate
	feePlatform := amount * platformRate / 100
	netAmount := amount - feePlatform

	return &FeeResult{
		FeeOfficial:  feeOfficial,
		FeePlatform:  feePlatform,
		NetAmount:    netAmount,
		OfficialRate: officialRate,
		PlatformRate: platformRate,
	}, nil
}

// ApplySettlement finalizes a PAID order by computing fees and crediting the
// merchant's settlement balance. Uses a distributed Redis lock to prevent
// concurrent settlement on the same order.
func (s *FeeService) ApplySettlement(ctx context.Context, orderID uuid.UUID) error {
	lockKey := fmt.Sprintf("settle:%s", orderID)

	acquired, err := redis.AcquireLock(ctx, s.rdb, lockKey, 30*time.Second)
	if err != nil {
		return fmt.Errorf("apply settlement: acquire lock: %w", err)
	}
	if !acquired {
		return fmt.Errorf("apply settlement: order %s is already being settled", orderID)
	}
	defer redis.ReleaseLock(ctx, s.rdb, lockKey)

	// Load order — must be PAID
	o, err := s.ent.Order.Query().
		Where(order.IDEQ(orderID)).
		Only(ctx)
	if err != nil {
		return fmt.Errorf("apply settlement: query order %s: %w", orderID, err)
	}
	if o.Status != order.StatusPAID {
		return fmt.Errorf("apply settlement: order %s has status %s, expected PAID", orderID, o.Status)
	}

	// Calculate fees
	result, err := s.CalculateFee(ctx, string(o.Type), o.Amount, o.MerchantID)
	if err != nil {
		return fmt.Errorf("apply settlement: %w", err)
	}

	// Run order update and settlement upsert in a transaction
	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return fmt.Errorf("apply settlement: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Update order with settlement data
	err = tx.Order.UpdateOneID(o.ID).
		SetFeeOfficial(result.FeeOfficial).
		SetFeePlatform(result.FeePlatform).
		SetNetAmount(result.NetAmount).
		SetStatus(order.StatusSETTLED).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("apply settlement: update order: %w", err)
	}

	// Upsert merchant settlement record
	err = s.upsertSettlement(ctx, tx, o.MerchantID, result.NetAmount, o.Amount)
	if err != nil {
		return fmt.Errorf("apply settlement: upsert settlement: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("apply settlement: commit tx: %w", err)
	}

	return nil
}

// upsertSettlement adds net_amount to the merchant's balance and total income.
// Creates a new settlement record if one does not exist.
func (s *FeeService) upsertSettlement(ctx context.Context, tx *ent.Tx, merchantID uuid.UUID, netAmount, amount float64) error {
	existing, err := tx.Settlement.Query().
		Where(settlement.MerchantIDEQ(merchantID)).
		Only(ctx)

	if ent.IsNotFound(err) {
		// Create new settlement record
		_, err := tx.Settlement.Create().
			SetMerchantID(merchantID).
			SetBalance(netAmount).
			SetTotalIncome(amount).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("create settlement: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("query settlement: %w", err)
	}

	// Update existing settlement with atomic increment
	_, err = tx.Settlement.UpdateOneID(existing.ID).
		AddBalance(netAmount).
		AddTotalIncome(amount).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update settlement: %w", err)
	}
	return nil
}

// GetMerchantBalance returns the merchant's current settlement balance.
// Returns 0 if no settlement record exists.
func (s *FeeService) GetMerchantBalance(ctx context.Context, merchantID uuid.UUID) (float64, error) {
	stl, err := s.ent.Settlement.Query().
		Where(settlement.MerchantIDEQ(merchantID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get merchant balance: query settlement: %w", err)
	}
	return stl.Balance, nil
}
