// Package settlement provides the SettlementService for handling user
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

// SettlementService manages user withdrawal operations, coordinating
// between the Settlement balance and Withdraw records.
type SettlementService struct {
	ent *ent.Client
	rdb *redis.Client
}

// New creates a new SettlementService with the given ent client and Redis client.
func New(ent *ent.Client, rdb *redis.Client) *SettlementService {
	return &SettlementService{ent: ent, rdb: rdb}
}

// RequestWithdraw initiates a withdrawal request for a user.
// It locks the user, validates the balance, updates the settlement
// (balance -= amount, frozen += amount), and creates a PENDING Withdraw record.
func (s *SettlementService) RequestWithdraw(ctx context.Context, userID uuid.UUID, amount float64, accountInfo string) (*ent.Withdraw, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("withdraw amount must be positive, got %.2f", amount)
	}

	lockKey := fmt.Sprintf("withdraw:user:%s", userID.String())
	ok, err := redisutil.AcquireLock(ctx, s.rdb, lockKey, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("withdraw request already in progress for user %s", userID)
	}
	defer func() {
		_ = redisutil.ReleaseLock(ctx, s.rdb, lockKey)
	}()

	sett, err := s.ent.Settlement.Query().
		Where(settlement.UserIDEQ(userID)).
		First(ctx)
	if err != nil {
		return nil, fmt.Errorf("load settlement for user %s: %w", userID, err)
	}
	if sett.Balance < amount {
		return nil, fmt.Errorf("insufficient balance: have %.2f, need %.2f", sett.Balance, amount)
	}

	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = tx.Settlement.Update().
		Where(settlement.UserIDEQ(userID)).
		AddBalance(-amount).
		AddFrozen(amount).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update settlement balance: %w", err)
	}

	wd, err := tx.Withdraw.Create().
		SetUserID(userID).
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

// ApproveWithdraw updates a PENDING withdrawal request status to APPROVED.
func (s *SettlementService) ApproveWithdraw(ctx context.Context, withdrawID uuid.UUID) error {
	unlock, err := s.lockWithdraw(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("approve withdraw %s: %w", withdrawID, err)
	}
	defer unlock()

	wd, err := s.ent.Withdraw.Get(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("approve withdraw %s: %w", withdrawID, err)
	}
	if wd.Status != withdraw.StatusPENDING {
		return fmt.Errorf("withdraw %s is not in PENDING status (current: %s)", withdrawID, wd.Status)
	}
	affected, err := s.ent.Withdraw.Update().
		Where(withdraw.IDEQ(withdrawID), withdraw.StatusEQ(withdraw.StatusPENDING)).
		SetStatus(withdraw.StatusAPPROVED).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("approve withdraw %s: %w", withdrawID, err)
	}
	if affected != 1 {
		return fmt.Errorf("withdraw %s is not in PENDING status", withdrawID)
	}
	return nil
}

// ConfirmWithdraw finalizes an APPROVED withdrawal: marks it PAID and
// updates the settlement (frozen -= amount, total_withdrawn += amount).
func (s *SettlementService) ConfirmWithdraw(ctx context.Context, withdrawID uuid.UUID) error {
	unlock, err := s.lockWithdraw(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("confirm withdraw %s: %w", withdrawID, err)
	}
	defer unlock()

	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	wd, err := tx.Withdraw.Get(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("load withdraw %s: %w", withdrawID, err)
	}
	if wd.Status != withdraw.StatusAPPROVED {
		return fmt.Errorf("withdraw %s is not in APPROVED status (current: %s)", withdrawID, wd.Status)
	}

	affected, err := tx.Withdraw.Update().
		Where(withdraw.IDEQ(withdrawID), withdraw.StatusEQ(withdraw.StatusAPPROVED)).
		SetStatus(withdraw.StatusPAID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update withdraw status to PAID: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("withdraw %s is not in APPROVED status", withdrawID)
	}

	_, err = tx.Settlement.Update().
		Where(settlement.UserIDEQ(wd.UserID)).
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
// refunds the frozen amount back to the user's balance.
func (s *SettlementService) RejectWithdraw(ctx context.Context, withdrawID uuid.UUID, remark string) error {
	unlock, err := s.lockWithdraw(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("reject withdraw %s: %w", withdrawID, err)
	}
	defer unlock()

	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	wd, err := tx.Withdraw.Get(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("load withdraw %s: %w", withdrawID, err)
	}
	if wd.Status != withdraw.StatusPENDING {
		return fmt.Errorf("withdraw %s is not in PENDING status (current: %s)", withdrawID, wd.Status)
	}

	affected, err := tx.Withdraw.Update().
		Where(withdraw.IDEQ(withdrawID), withdraw.StatusEQ(withdraw.StatusPENDING)).
		SetStatus(withdraw.StatusREJECTED).
		SetRemark(remark).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update withdraw status to REJECTED: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("withdraw %s is not in PENDING status", withdrawID)
	}

	_, err = tx.Settlement.Update().
		Where(settlement.UserIDEQ(wd.UserID)).
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

func (s *SettlementService) lockWithdraw(ctx context.Context, withdrawID uuid.UUID) (func(), error) {
	lockKey := fmt.Sprintf("withdraw:id:%s", withdrawID.String())
	ok, err := redisutil.AcquireLock(ctx, s.rdb, lockKey, 10*time.Second)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("withdraw %s is already being processed", withdrawID)
	}
	return func() {
		_ = redisutil.ReleaseLock(ctx, s.rdb, lockKey)
	}, nil
}

// ListWithdraws returns a paginated list of withdrawal records, optionally
// filtered by user_id and status. Returns the records, total count, and any error.
func (s *SettlementService) ListWithdraws(ctx context.Context, userID uuid.UUID, statusVal withdraw.Status, page, limit int) ([]*ent.Withdraw, int, error) {
	q := s.ent.Withdraw.Query()

	if userID != uuid.Nil {
		q.Where(withdraw.UserIDEQ(userID))
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
	if limit <= 0 || limit > 100 {
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
