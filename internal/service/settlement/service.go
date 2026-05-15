// Package settlement provides the SettlementService for handling merchant
// withdrawal requests, approval, confirmation, and rejection.
package settlement

import (
	"context"
	"fmt"
	"time"

	"epay/ent"
	"epay/ent/settlement"
	"epay/ent/withdraw"
	redisutil "epay/internal/redis"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// SettlementService manages merchant withdrawal operations, coordinating
// between the Settlement balance and Withdraw records.
type SettlementService struct {
	ent *ent.Client
	rdb *redis.Client
}

// New creates a new SettlementService with the given ent client and Redis client.
func New(ent *ent.Client, rdb *redis.Client) *SettlementService {
	return &SettlementService{ent: ent, rdb: rdb}
}

// RequestWithdraw initiates a withdrawal request for a merchant.
// It locks the merchant, validates the balance, updates the settlement
// (balance -= amount, frozen += amount), and creates a PENDING Withdraw record.
func (s *SettlementService) RequestWithdraw(ctx context.Context, merchantID uuid.UUID, amount float64, accountInfo string) (*ent.Withdraw, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("withdraw amount must be positive, got %.2f", amount)
	}

	lockKey := fmt.Sprintf("withdraw:%s", merchantID.String())
	ok, err := redisutil.AcquireLock(ctx, s.rdb, lockKey, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("withdraw request already in progress for merchant %s", merchantID)
	}
	defer func() {
		_ = redisutil.ReleaseLock(ctx, s.rdb, lockKey)
	}()

	// Load settlement and validate balance
	sett, err := s.ent.Settlement.Query().
		Where(settlement.MerchantIDEQ(merchantID)).
		First(ctx)
	if err != nil {
		return nil, fmt.Errorf("load settlement for merchant %s: %w", merchantID, err)
	}
	if sett.Balance < amount {
		return nil, fmt.Errorf("insufficient balance: have %.2f, need %.2f", sett.Balance, amount)
	}

	// Transaction: update settlement + create withdraw
	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = tx.Settlement.Update().
		Where(settlement.MerchantIDEQ(merchantID)).
		AddBalance(-amount).
		AddFrozen(amount).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update settlement balance: %w", err)
	}

	wd, err := tx.Withdraw.Create().
		SetMerchantID(merchantID).
		SetAmount(amount).
		SetAccountInfo(accountInfo).
		SetStatus(withdraw.StatusPENDING).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create withdraw record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return wd, nil
}

// ApproveWithdraw updates a withdrawal request status to APPROVED.
func (s *SettlementService) ApproveWithdraw(ctx context.Context, withdrawID uuid.UUID) error {
	_, err := s.ent.Withdraw.UpdateOneID(withdrawID).
		SetStatus(withdraw.StatusAPPROVED).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("approve withdraw %s: %w", withdrawID, err)
	}
	return nil
}

// ConfirmWithdraw finalizes an APPROVED withdrawal: marks it PAID and
// updates the settlement (frozen -= amount, total_withdrawn += amount).
func (s *SettlementService) ConfirmWithdraw(ctx context.Context, withdrawID uuid.UUID) error {
	wd, err := s.ent.Withdraw.Get(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("load withdraw %s: %w", withdrawID, err)
	}
	if wd.Status != withdraw.StatusAPPROVED {
		return fmt.Errorf("withdraw %s is not in APPROVED status (current: %s)", withdrawID, wd.Status)
	}

	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = tx.Withdraw.UpdateOneID(withdrawID).
		SetStatus(withdraw.StatusPAID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update withdraw status to PAID: %w", err)
	}

	_, err = tx.Settlement.Update().
		Where(settlement.MerchantIDEQ(wd.MerchantID)).
		AddFrozen(-wd.Amount).
		AddTotalWithdrawn(wd.Amount).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update settlement for confirmation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// RejectWithdraw rejects a PENDING withdrawal: marks it REJECTED and
// refunds the frozen amount back to the merchant's balance
// (frozen -= amount, balance += amount).
func (s *SettlementService) RejectWithdraw(ctx context.Context, withdrawID uuid.UUID, remark string) error {
	wd, err := s.ent.Withdraw.Get(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("load withdraw %s: %w", withdrawID, err)
	}
	if wd.Status != withdraw.StatusPENDING {
		return fmt.Errorf("withdraw %s is not in PENDING status (current: %s)", withdrawID, wd.Status)
	}

	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = tx.Withdraw.UpdateOneID(withdrawID).
		SetStatus(withdraw.StatusREJECTED).
		SetRemark(remark).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update withdraw status to REJECTED: %w", err)
	}

	// Refund the frozen amount
	_, err = tx.Settlement.Update().
		Where(settlement.MerchantIDEQ(wd.MerchantID)).
		AddFrozen(-wd.Amount).
		AddBalance(wd.Amount).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update settlement for rejection refund: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// ListWithdraws returns a paginated list of withdrawal records, optionally
// filtered by merchant_id and status. Returns the records, total count, and any error.
func (s *SettlementService) ListWithdraws(ctx context.Context, merchantID uuid.UUID, statusVal withdraw.Status, page, limit int) ([]*ent.Withdraw, int, error) {
	q := s.ent.Withdraw.Query()

	if merchantID != uuid.Nil {
		q.Where(withdraw.MerchantIDEQ(merchantID))
	}
	if statusVal != "" {
		q.Where(withdraw.StatusEQ(statusVal))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count withdraws: %w", err)
	}

	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	records, err := q.
		Order(withdraw.ByCreatedAt(sql.OrderDesc())).
		Limit(limit).
		Offset((page - 1) * limit).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list withdraws: %w", err)
	}

	return records, total, nil
}
