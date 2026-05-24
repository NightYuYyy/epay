package product

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"epay/ent"
	"epay/ent/enttest"
	entproduct "epay/ent/product"

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

func createUser(t *testing.T, client *ent.Client, email string) *ent.User {
	t.Helper()
	return client.User.Create().
		SetEmail(email).
		SetPasswordHash("hash").
		SetName(email).
		SetFeeRate(0.01).
		SaveX(context.Background())
}

func createProduct(t *testing.T, client *ent.Client, userID uuid.UUID, pid int, name string) *ent.Product {
	t.Helper()
	return client.Product.Create().
		SetUserID(userID).
		SetPid(pid).
		SetPkey("old-secret").
		SetName(name).
		SaveX(context.Background())
}

func TestCreateReturnsSecretAndListMasksSecret(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	owner := createUser(t, client, "owner@example.com")
	svc := NewService(client)

	feeRate := 0.03
	created, err := svc.Create(ctx, CreateParams{
		UserID:      owner.ID,
		Name:        " Shop ",
		Description: "  desc  ",
		NotifyURL:   " https://merchant.example/notify ",
		ReturnURL:   " https://merchant.example/return ",
		FeeRate:     &feeRate,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.Pid < 1000 || created.Pid > 9999 {
		t.Fatalf("pid = %d, want 1000..9999", created.Pid)
	}
	if len(created.Pkey) != 32 {
		t.Fatalf("pkey length = %d, want 32", len(created.Pkey))
	}
	if created.Description != "desc" || created.NotifyURL != "https://merchant.example/notify" || created.ReturnURL != "https://merchant.example/return" {
		t.Fatalf("created URLs/description were not trimmed: %+v", created)
	}

	listed, err := svc.ListByUser(ctx, owner.ID, 1, 20)
	if err != nil {
		t.Fatalf("ListByUser returned error: %v", err)
	}
	if listed.Total != 1 || len(listed.Items) != 1 {
		t.Fatalf("ListByUser result = %+v, want exactly one item", listed)
	}
	if listed.Items[0].Pkey != "" {
		t.Fatalf("listed product leaked pkey %q", listed.Items[0].Pkey)
	}

	secret, err := svc.Get(ctx, created.ID, true)
	if err != nil {
		t.Fatalf("Get include secret returned error: %v", err)
	}
	masked, err := svc.GetByPid(ctx, created.Pid, false)
	if err != nil {
		t.Fatalf("GetByPid masked returned error: %v", err)
	}
	if secret.Pkey != created.Pkey || masked.Pkey != "" {
		t.Fatalf("secret=%q masked=%q, want secret returned only when requested", secret.Pkey, masked.Pkey)
	}
	owned, err := svc.GetForUser(ctx, created.ID, owner.ID, true)
	if err != nil {
		t.Fatalf("GetForUser owner returned error: %v", err)
	}
	if owned.Pkey != created.Pkey {
		t.Fatalf("GetForUser pkey = %q, want created secret", owned.Pkey)
	}
	other := createUser(t, client, "other@example.com")
	if _, err := svc.GetForUser(ctx, created.ID, other.ID, true); err == nil {
		t.Fatalf("GetForUser with wrong owner returned nil error")
	}
}

func TestCreateRequiresName(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	owner := createUser(t, client, "owner@example.com")
	svc := NewService(client)

	if _, err := svc.Create(ctx, CreateParams{UserID: owner.ID}); !errors.Is(err, ErrNameRequired) {
		t.Fatalf("Create empty name error = %v, want ErrNameRequired", err)
	}
}

func TestUpdateRequiresOwnerAndHandlesStatusAndFee(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	owner := createUser(t, client, "owner@example.com")
	other := createUser(t, client, "other@example.com")
	prod := createProduct(t, client, owner.ID, 2001, "old")
	svc := NewService(client)

	if _, err := svc.Update(ctx, prod.ID, &other.ID, UpdateParams{}); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("Update with wrong owner error = %v, want ErrNotOwned", err)
	}
	badStatus := "archived"
	if _, err := svc.Update(ctx, prod.ID, nil, UpdateParams{Status: &badStatus}); err == nil {
		t.Fatalf("Update invalid status returned nil error")
	}
	tooLargeRate := 2.0
	if _, err := svc.Update(ctx, prod.ID, &owner.ID, UpdateParams{FeeRate: &tooLargeRate}); !errors.Is(err, ErrInvalidFeeRate) {
		t.Fatalf("Update invalid fee rate error = %v, want ErrInvalidFeeRate", err)
	}
	conflictRate := 0.05
	if _, err := svc.Update(ctx, prod.ID, &owner.ID, UpdateParams{ClearFee: true, FeeRate: &conflictRate}); !errors.Is(err, ErrConflictingFeeUpdate) {
		t.Fatalf("Update conflicting fee fields error = %v, want ErrConflictingFeeUpdate", err)
	}

	name := "new name"
	desc := "new desc"
	notifyURL := "https://notify.example/path"
	returnURL := "https://return.example/path"
	feeRate := 0.05
	status := "disabled"
	updated, err := svc.Update(ctx, prod.ID, &owner.ID, UpdateParams{
		Name:        &name,
		Description: &desc,
		NotifyURL:   &notifyURL,
		ReturnURL:   &returnURL,
		FeeRate:     &feeRate,
		Status:      &status,
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.Name != name || updated.Description != desc || updated.Status != string(entproduct.StatusDisabled) || updated.FeeRate == nil || *updated.FeeRate != feeRate {
		t.Fatalf("updated product = %+v, want updated fields", updated)
	}

	cleared, err := svc.Update(ctx, prod.ID, &owner.ID, UpdateParams{ClearFee: true})
	if err != nil {
		t.Fatalf("Update clear fee returned error: %v", err)
	}
	if cleared.FeeRate != nil {
		t.Fatalf("fee rate after ClearFee = %v, want nil", *cleared.FeeRate)
	}
}

func TestRegeneratePkeyRequiresOwnerAndChangesSecret(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	owner := createUser(t, client, "owner@example.com")
	other := createUser(t, client, "other@example.com")
	prod := createProduct(t, client, owner.ID, 2002, "product")
	svc := NewService(client)

	if _, err := svc.RegeneratePkey(ctx, prod.ID, &other.ID); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("RegeneratePkey with wrong owner error = %v, want ErrNotOwned", err)
	}
	newKey, err := svc.RegeneratePkey(ctx, prod.ID, &owner.ID)
	if err != nil {
		t.Fatalf("RegeneratePkey returned error: %v", err)
	}
	if len(newKey) != 32 || newKey == "old-secret" {
		t.Fatalf("new pkey = %q, want 32-char key different from old-secret", newKey)
	}
	reloaded := client.Product.GetX(ctx, prod.ID)
	if reloaded.Pkey != newKey {
		t.Fatalf("stored pkey = %q, want regenerated key %q", reloaded.Pkey, newKey)
	}
}

func TestListAllFiltersAndNormalizesPagination(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	owner := createUser(t, client, "owner@example.com")
	active := createProduct(t, client, owner.ID, 2101, "active")
	disabled := createProduct(t, client, owner.ID, 2102, "disabled")
	client.Product.UpdateOneID(disabled.ID).SetStatus(entproduct.StatusDisabled).ExecX(ctx)
	svc := NewService(client)

	listed, err := svc.ListAll(ctx, 0, 999, "active")
	if err != nil {
		t.Fatalf("ListAll returned error: %v", err)
	}
	if listed.Total != 1 || listed.Limit != 20 || listed.Page != 1 || listed.Items[0].ID != active.ID {
		t.Fatalf("ListAll result = %+v, want only active product with normalized pagination", listed)
	}
}
