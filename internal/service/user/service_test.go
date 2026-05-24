package user

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"epay/ent"
	"epay/ent/enttest"
	"epay/ent/settlement"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
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

func TestRegisterNormalizesEmailAndCreatesSettlement(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	svc := NewService(client)
	feeRate := 0.015

	info, err := svc.Register(ctx, " Alice@Example.COM ", "secret123", "Alice", &feeRate)
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if info.Email != "alice@example.com" {
		t.Fatalf("email = %q, want normalized alice@example.com", info.Email)
	}
	if info.Name != "Alice" || info.FeeRate != feeRate || info.Status != "active" {
		t.Fatalf("unexpected user info: %+v", info)
	}

	sett := client.Settlement.Query().Where(settlement.UserIDEQ(info.ID)).OnlyX(ctx)
	if sett.Balance != 0 || sett.Frozen != 0 || sett.TotalIncome != 0 || sett.TotalWithdrawn != 0 {
		t.Fatalf("new settlement = %+v, want zeroed balance fields", sett)
	}
}

func TestRegisterUsesSchemaDefaultFeeRateWhenUnset(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	svc := NewService(client)

	info, err := svc.Register(ctx, "default@example.com", "secret123", "Default", nil)
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if info.FeeRate != 0.006 {
		t.Fatalf("default fee_rate = %.4f, want schema default 0.006", info.FeeRate)
	}
}

func TestRegisterAndUpdateRejectInvalidFeeRate(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	svc := NewService(client)
	invalidRate := -0.01
	if _, err := svc.Register(ctx, "bad@example.com", "secret123", "Bad", &invalidRate); !errors.Is(err, ErrInvalidFeeRate) {
		t.Fatalf("Register invalid fee rate error = %v, want ErrInvalidFeeRate", err)
	}

	info, err := svc.Register(ctx, "good@example.com", "secret123", "Good", nil)
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	tooLargeRate := 1.01
	if _, err := svc.Update(ctx, info.ID, nil, &tooLargeRate, nil); !errors.Is(err, ErrInvalidFeeRate) {
		t.Fatalf("Update invalid fee rate error = %v, want ErrInvalidFeeRate", err)
	}
}

func TestRegisterRejectsMissingFieldsAndDuplicateEmail(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	svc := NewService(client)

	if _, err := svc.Register(ctx, "", "secret123", "Alice", nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register missing email error = %v, want ErrInvalidInput", err)
	}
	feeRate := 0.01
	if _, err := svc.Register(ctx, "alice@example.com", "secret123", "Alice", &feeRate); err != nil {
		t.Fatalf("first Register returned error: %v", err)
	}
	otherRate := 0.02
	if _, err := svc.Register(ctx, " ALICE@example.com ", "secret123", "Alice 2", &otherRate); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate Register error = %v, want ErrEmailTaken", err)
	}
}

func TestVerifyCredentialRejectsWrongPasswordAndDisabledUser(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	svc := NewService(client)

	feeRate := 0.01
	info, err := svc.Register(ctx, "alice@example.com", "secret123", "Alice", &feeRate)
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if _, err := svc.VerifyCredential(ctx, "alice@example.com", "wrong"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("wrong password error = %v, want ErrInvalidCredential", err)
	}
	verified, err := svc.VerifyCredential(ctx, " ALICE@example.com ", "secret123")
	if err != nil {
		t.Fatalf("VerifyCredential returned error: %v", err)
	}
	if verified.ID != info.ID {
		t.Fatalf("verified ID = %s, want %s", verified.ID, info.ID)
	}

	disabled := "disabled"
	if _, err := svc.Update(ctx, info.ID, nil, nil, &disabled); err != nil {
		t.Fatalf("Update disabled returned error: %v", err)
	}
	if _, err := svc.VerifyCredential(ctx, "alice@example.com", "secret123"); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("disabled user auth error = %v, want ErrUserDisabled", err)
	}
}

func TestUpdateChangePasswordAndList(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	svc := NewService(client)

	firstRate := 0.01
	first, err := svc.Register(ctx, "alice@example.com", "secret123", "Alice", &firstRate)
	if err != nil {
		t.Fatalf("Register first returned error: %v", err)
	}
	secondRate := 0.02
	if _, err := svc.Register(ctx, "bob@example.com", "secret123", "Bob", &secondRate); err != nil {
		t.Fatalf("Register second returned error: %v", err)
	}

	badStatus := "suspended"
	if _, err := svc.Update(ctx, first.ID, nil, nil, &badStatus); err == nil {
		t.Fatalf("Update invalid status returned nil error")
	}
	name := "Alice Renamed"
	rate := 0.025
	updated, err := svc.Update(ctx, first.ID, &name, &rate, nil)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.Name != name || updated.FeeRate != rate {
		t.Fatalf("updated info = %+v, want name %q fee %.3f", updated, name, rate)
	}

	if err := svc.ChangePassword(ctx, first.ID, "wrong", "newsecret"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("ChangePassword wrong current error = %v, want ErrInvalidCredential", err)
	}
	if err := svc.ChangePassword(ctx, first.ID, "secret123", "newsecret"); err != nil {
		t.Fatalf("ChangePassword returned error: %v", err)
	}
	if _, err := svc.VerifyCredential(ctx, "alice@example.com", "newsecret"); err != nil {
		t.Fatalf("VerifyCredential after password change returned error: %v", err)
	}

	listed, err := svc.List(ctx, 0, 999, "active")
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if listed.Total != 2 || listed.Page != 1 || listed.Limit != 20 || listed.TotalPages != 1 {
		t.Fatalf("list result = %+v, want total=2 page=1 limit=20 total_pages=1", listed)
	}
}

func TestGetAndCredentialErrorBranches(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	svc := NewService(client)

	feeRate := 0.03
	info, err := svc.Register(ctx, "carol@example.com", "secret123", "Carol", &feeRate)
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	got, err := svc.Get(ctx, info.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Email != info.Email || got.ID != info.ID {
		t.Fatalf("Get = %+v, want %+v", got, info)
	}
	if _, err := svc.VerifyCredential(ctx, "missing@example.com", "secret123"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("VerifyCredential missing user error = %v, want ErrInvalidCredential", err)
	}
	if err := svc.ChangePassword(ctx, info.ID, "secret123", ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ChangePassword empty next error = %v, want ErrInvalidInput", err)
	}
}
