// Package fee computes per-order fees and applies settlements to user balances.
package fee

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"epay/ent"
	"epay/ent/order"
	"epay/ent/platformconfig"
	"epay/ent/product"
	"epay/ent/settlement"
	"epay/ent/user"
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

// FeeService calculates fees and applies settlements to user balances.
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
//   - platformRate is product.fee_rate when set; otherwise user.fee_rate.
//   - Rate values are decimal fractions (0.006 = 0.6%). Legacy percent-style
//     config values >= 1 are normalized to fractions to avoid 100x fees.
func (s *FeeService) CalculateFee(ctx context.Context, paymentType string, amount float64, userID, productID uuid.UUID) (*FeeResult, error) {
	platformRate := 0.0
	platformRateSet := false

	if productID != uuid.Nil {
		p, err := s.ent.Product.Query().Where(product.IDEQ(productID)).Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("calculate fee: query product %s: %w", productID, err)
		}
		if p.FeeRate != nil {
			platformRate = normalizeRate(*p.FeeRate)
			platformRateSet = true
		}
	}

	if !platformRateSet && userID != uuid.Nil {
		u, err := s.ent.User.Query().Where(user.IDEQ(userID)).Only(ctx)
		if err != nil {
			return nil, fmt.Errorf("calculate fee: query user %s: %w", userID, err)
		}
		platformRate = normalizeRate(u.FeeRate)
		platformRateSet = true
	}

	if !platformRateSet {
		v, ok, err := s.loadRateConfig(ctx, "default_platform_rate")
		if err != nil {
			return nil, err
		}
		if ok {
			platformRate = v
		}
	}

	// Load official rate from global config.
	configKey := fmt.Sprintf("official_%s_rate", paymentType)
	officialRate, _, err := s.loadRateConfig(ctx, configKey)
	if err != nil {
		return nil, err
	}

	feeOfficial := amount * officialRate
	feePlatform := amount * platformRate
	netAmount := amount - feePlatform

	return &FeeResult{
		FeeOfficial:  feeOfficial,
		FeePlatform:  feePlatform,
		NetAmount:    netAmount,
		OfficialRate: officialRate,
		PlatformRate: platformRate,
	}, nil
}

func normalizeRate(rate float64) float64 {
	if rate >= 1 {
		return rate / 100
	}
	return rate
}

func (s *FeeService) loadRateConfig(ctx context.Context, key string) (float64, bool, error) {
	cfg, err := s.ent.PlatformConfig.Query().
		Where(platformconfig.KeyEQ(key)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("calculate fee: query config %s: %w", key, err)
	}
	v, err := strconv.ParseFloat(cfg.Value, 64)
	if err != nil {
		log.Printf("[fee] invalid rate config %s=%q: %v", key, cfg.Value, err)
		return 0, false, fmt.Errorf("calculate fee: invalid rate config %s", key)
	}
	return normalizeRate(v), true, nil
}

// ApplySettlement finalizes a PAID order by computing fees and crediting the
// user's settlement balance. Uses a distributed Redis lock to prevent
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

	o, err := s.ent.Order.Query().
		Where(order.IDEQ(orderID)).
		Only(ctx)
	if err != nil {
		return fmt.Errorf("apply settlement: query order %s: %w", orderID, err)
	}
	if o.Status != order.StatusPAID {
		return fmt.Errorf("apply settlement: order %s has status %s, expected PAID", orderID, o.Status)
	}

	result, err := s.CalculateFee(ctx, string(o.Type), o.Amount, o.UserID, o.ProductID)
	if err != nil {
		return fmt.Errorf("apply settlement: %w", err)
	}

	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return fmt.Errorf("apply settlement: begin tx: %w", err)
	}
	defer tx.Rollback()

	err = tx.Order.UpdateOneID(o.ID).
		SetFeeOfficial(result.FeeOfficial).
		SetFeePlatform(result.FeePlatform).
		SetNetAmount(result.NetAmount).
		SetStatus(order.StatusSETTLED).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("apply settlement: update order: %w", err)
	}

	err = s.upsertSettlement(ctx, tx, o.UserID, result.NetAmount, o.Amount)
	if err != nil {
		return fmt.Errorf("apply settlement: upsert settlement: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("apply settlement: commit tx: %w", err)
	}

	return nil
}

// upsertSettlement adds net_amount to the user's balance and total income.
// Creates a new settlement record if one does not exist.
func (s *FeeService) upsertSettlement(ctx context.Context, tx *ent.Tx, userID uuid.UUID, netAmount, amount float64) error {
	_, err := tx.Settlement.Query().
		Where(settlement.UserIDEQ(userID)).
		Only(ctx)

	if ent.IsNotFound(err) {
		_, err := tx.Settlement.Create().
			SetUserID(userID).
			SetBalance(netAmount).
			SetTotalIncome(amount).
			Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				return s.addSettlementAmounts(ctx, tx, userID, netAmount, amount)
			}
			return fmt.Errorf("create settlement: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("query settlement: %w", err)
	}

	if err := s.addSettlementAmounts(ctx, tx, userID, netAmount, amount); err != nil {
		return err
	}
	return nil
}

func (s *FeeService) addSettlementAmounts(ctx context.Context, tx *ent.Tx, userID uuid.UUID, netAmount, amount float64) error {
	_, err := tx.Settlement.Update().
		Where(settlement.UserIDEQ(userID)).
		AddBalance(netAmount).
		AddTotalIncome(amount).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update settlement: %w", err)
	}
	return nil
}

// GetUserBalance returns the user's current settlement balance. Returns 0
// when no settlement record exists.
func (s *FeeService) GetUserBalance(ctx context.Context, userID uuid.UUID) (float64, error) {
	stl, err := s.ent.Settlement.Query().
		Where(settlement.UserIDEQ(userID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get user balance: query settlement: %w", err)
	}
	return stl.Balance, nil
}
