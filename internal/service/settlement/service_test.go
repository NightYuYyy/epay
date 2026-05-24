package settlement

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"epay/ent"
	"epay/ent/enttest"
	entsettlement "epay/ent/settlement"
	"epay/ent/withdraw"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func newTestClient(t *testing.T) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(drv)))
	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
	})
	return client
}

func createUser(t *testing.T, client *ent.Client, balance float64) *ent.User {
	t.Helper()
	ctx := context.Background()
	usr := client.User.Create().
		SetEmail(uuid.NewString() + "@example.com").
		SetPasswordHash("hash").
		SetName("user").
		SetFeeRate(0.01).
		SaveX(ctx)
	client.Settlement.Create().
		SetUserID(usr.ID).
		SetBalance(balance).
		SetFrozen(0).
		SetTotalIncome(balance).
		SetTotalWithdrawn(0).
		SaveX(ctx)
	return usr
}

func TestRequestWithdrawMovesBalanceToFrozenAndCreatesPending(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	usr := createUser(t, client, 100)
	svc := New(client, nil)

	wd, err := svc.RequestWithdraw(ctx, usr.ID, 40, "alipay:alice")
	if err != nil {
		t.Fatalf("RequestWithdraw returned error: %v", err)
	}
	if wd.UserID != usr.ID || wd.Amount != 40 || wd.AccountInfo != "alipay:alice" || wd.Status != withdraw.StatusPENDING {
		t.Fatalf("withdraw = %+v, want pending user withdrawal", wd)
	}
	sett := client.Settlement.Query().Where(entsettlement.UserIDEQ(usr.ID)).OnlyX(ctx)
	if sett.Balance != 60 || sett.Frozen != 40 {
		t.Fatalf("settlement = %+v, want balance 60 frozen 40", sett)
	}
}

func TestRequestWithdrawRejectsInvalidAmountAndInsufficientBalance(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	usr := createUser(t, client, 20)
	svc := New(client, nil)

	if _, err := svc.RequestWithdraw(ctx, usr.ID, 0, "account"); err == nil {
		t.Fatalf("RequestWithdraw zero amount returned nil error")
	}
	if _, err := svc.RequestWithdraw(ctx, usr.ID, 30, "account"); err == nil {
		t.Fatalf("RequestWithdraw insufficient balance returned nil error")
	}
	sett := client.Settlement.Query().Where(entsettlement.UserIDEQ(usr.ID)).OnlyX(ctx)
	if sett.Balance != 20 || sett.Frozen != 0 {
		t.Fatalf("settlement after rejected withdrawals = %+v, want unchanged", sett)
	}
}

func TestRejectWithdrawRestoresFrozenBalanceAndStoresRemark(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	usr := createUser(t, client, 100)
	svc := New(client, nil)
	wd, err := svc.RequestWithdraw(ctx, usr.ID, 25, "account")
	if err != nil {
		t.Fatalf("RequestWithdraw returned error: %v", err)
	}

	if err := svc.RejectWithdraw(ctx, wd.ID, "bad account"); err != nil {
		t.Fatalf("RejectWithdraw returned error: %v", err)
	}
	reloaded := client.Withdraw.GetX(ctx, wd.ID)
	if reloaded.Status != withdraw.StatusREJECTED || reloaded.Remark != "bad account" {
		t.Fatalf("withdraw after rejection = %+v, want rejected with remark", reloaded)
	}
	sett := client.Settlement.Query().Where(entsettlement.UserIDEQ(usr.ID)).OnlyX(ctx)
	if sett.Balance != 100 || sett.Frozen != 0 {
		t.Fatalf("settlement after rejection = %+v, want restored balance", sett)
	}
}

func TestConfirmWithdrawMovesFrozenToTotalWithdrawn(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	usr := createUser(t, client, 100)
	svc := New(client, nil)
	wd, err := svc.RequestWithdraw(ctx, usr.ID, 25, "account")
	if err != nil {
		t.Fatalf("RequestWithdraw returned error: %v", err)
	}
	if err := svc.ConfirmWithdraw(ctx, wd.ID); err == nil {
		t.Fatalf("ConfirmWithdraw on pending withdrawal returned nil error")
	}
	if err := svc.ApproveWithdraw(ctx, wd.ID); err != nil {
		t.Fatalf("ApproveWithdraw returned error: %v", err)
	}
	if err := svc.ConfirmWithdraw(ctx, wd.ID); err != nil {
		t.Fatalf("ConfirmWithdraw returned error: %v", err)
	}
	reloaded := client.Withdraw.GetX(ctx, wd.ID)
	if reloaded.Status != withdraw.StatusPAID {
		t.Fatalf("withdraw status = %s, want PAID", reloaded.Status)
	}
	sett := client.Settlement.Query().Where(entsettlement.UserIDEQ(usr.ID)).OnlyX(ctx)
	if sett.Balance != 75 || sett.Frozen != 0 || sett.TotalWithdrawn != 25 {
		t.Fatalf("settlement after confirmation = %+v, want balance 75 frozen 0 withdrawn 25", sett)
	}
}

func TestListWithdrawsFiltersByUserAndStatus(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	first := createUser(t, client, 100)
	second := createUser(t, client, 100)
	svc := New(client, nil)
	firstWithdraw, err := svc.RequestWithdraw(ctx, first.ID, 10, "first")
	if err != nil {
		t.Fatalf("first RequestWithdraw returned error: %v", err)
	}
	if _, err := svc.RequestWithdraw(ctx, second.ID, 20, "second"); err != nil {
		t.Fatalf("second RequestWithdraw returned error: %v", err)
	}
	if err := svc.ApproveWithdraw(ctx, firstWithdraw.ID); err != nil {
		t.Fatalf("ApproveWithdraw returned error: %v", err)
	}

	rows, total, err := svc.ListWithdraws(ctx, first.ID, withdraw.StatusAPPROVED, 0, 0)
	if err != nil {
		t.Fatalf("ListWithdraws returned error: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != firstWithdraw.ID {
		t.Fatalf("filtered withdraws total=%d rows=%+v, want first approved withdraw", total, rows)
	}
}

func TestApproveConfirmRejectErrorBranches(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	usr := createUser(t, client, 100)
	svc := New(client, nil)

	missingID := uuid.New()
	if err := svc.ApproveWithdraw(ctx, missingID); err == nil {
		t.Fatalf("ApproveWithdraw missing id returned nil error")
	}
	if err := svc.ConfirmWithdraw(ctx, missingID); err == nil {
		t.Fatalf("ConfirmWithdraw missing id returned nil error")
	}
	if err := svc.RejectWithdraw(ctx, missingID, "missing"); err == nil {
		t.Fatalf("RejectWithdraw missing id returned nil error")
	}

	wd, err := svc.RequestWithdraw(ctx, usr.ID, 10, "account")
	if err != nil {
		t.Fatalf("RequestWithdraw returned error: %v", err)
	}
	if err := svc.ApproveWithdraw(ctx, wd.ID); err != nil {
		t.Fatalf("ApproveWithdraw returned error: %v", err)
	}
	if err := svc.ApproveWithdraw(ctx, wd.ID); err == nil {
		t.Fatalf("ApproveWithdraw approved withdrawal returned nil error")
	}
	if err := svc.RejectWithdraw(ctx, wd.ID, "too late"); err == nil {
		t.Fatalf("RejectWithdraw approved withdrawal returned nil error")
	}
}

func TestApproveRejectsTerminalWithdrawals(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	usr := createUser(t, client, 100)
	svc := New(client, nil)

	rejected, err := svc.RequestWithdraw(ctx, usr.ID, 10, "account")
	if err != nil {
		t.Fatalf("RequestWithdraw rejected setup returned error: %v", err)
	}
	if err := svc.RejectWithdraw(ctx, rejected.ID, "bad account"); err != nil {
		t.Fatalf("RejectWithdraw setup returned error: %v", err)
	}
	if err := svc.ApproveWithdraw(ctx, rejected.ID); err == nil {
		t.Fatalf("ApproveWithdraw rejected withdrawal returned nil error")
	}

	paid, err := svc.RequestWithdraw(ctx, usr.ID, 10, "account")
	if err != nil {
		t.Fatalf("RequestWithdraw paid setup returned error: %v", err)
	}
	if err := svc.ApproveWithdraw(ctx, paid.ID); err != nil {
		t.Fatalf("ApproveWithdraw paid setup returned error: %v", err)
	}
	if err := svc.ConfirmWithdraw(ctx, paid.ID); err != nil {
		t.Fatalf("ConfirmWithdraw setup returned error: %v", err)
	}
	if err := svc.ApproveWithdraw(ctx, paid.ID); err == nil {
		t.Fatalf("ApproveWithdraw paid withdrawal returned nil error")
	}

	sett := client.Settlement.Query().Where(entsettlement.UserIDEQ(usr.ID)).OnlyX(ctx)
	if sett.Balance != 90 || sett.Frozen != 0 || sett.TotalWithdrawn != 10 {
		t.Fatalf("settlement after rejected approve attempts = %+v, want balance 90 frozen 0 withdrawn 10", sett)
	}
}

func TestConcurrentConfirmWithdrawOnlyAppliesOnce(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	usr := createUser(t, client, 100)
	svc := New(client, nil)
	wd, err := svc.RequestWithdraw(ctx, usr.ID, 25, "account")
	if err != nil {
		t.Fatalf("RequestWithdraw returned error: %v", err)
	}
	if err := svc.ApproveWithdraw(ctx, wd.ID); err != nil {
		t.Fatalf("ApproveWithdraw returned error: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- svc.ConfirmWithdraw(ctx, wd.ID)
		}()
	}
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent confirmations = %d, want exactly 1", successes)
	}
	sett := client.Settlement.Query().Where(entsettlement.UserIDEQ(usr.ID)).OnlyX(ctx)
	if sett.Balance != 75 || sett.Frozen != 0 || sett.TotalWithdrawn != 25 {
		t.Fatalf("settlement after concurrent confirmations = %+v, want single debit", sett)
	}
}
