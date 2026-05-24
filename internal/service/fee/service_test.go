package fee

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"

	"epay/ent"
	"epay/ent/enttest"
	"epay/ent/order"
	"epay/ent/settlement"

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

var pidSeq int32

func createUserAndProduct(t *testing.T, client *ent.Client, userRate float64, productRate *float64) (*ent.User, *ent.Product) {
	t.Helper()
	ctx := context.Background()
	usr := client.User.Create().
		SetEmail(uuid.NewString() + "@example.com").
		SetPasswordHash("hash").
		SetName("user").
		SetFeeRate(userRate).
		SaveX(ctx)
	pid := int(atomic.AddInt32(&pidSeq, 1)) + 1000
	create := client.Product.Create().
		SetUserID(usr.ID).
		SetPid(pid).
		SetPkey("product-key").
		SetName("product")
	if productRate != nil {
		create.SetFeeRate(*productRate)
	}
	prod := create.SaveX(ctx)
	return usr, prod
}

func TestCalculateFeeUsesProductFeeBeforeUserAndConfigFallback(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	productRate := 0.04
	usr, prod := createUserAndProduct(t, client, 0.02, &productRate)
	client.PlatformConfig.Create().SetKey("official_alipay_rate").SetValue("0.006").SaveX(ctx)
	client.PlatformConfig.Create().SetKey("default_platform_rate").SetValue("0.03").SaveX(ctx)
	svc := New(client, nil)

	got, err := svc.CalculateFee(ctx, "alipay", 100, usr.ID, prod.ID)
	if err != nil {
		t.Fatalf("CalculateFee product override returned error: %v", err)
	}
	if got.PlatformRate != 0.04 || got.FeePlatform != 4 || got.OfficialRate != 0.006 || got.FeeOfficial != 0.6 || got.NetAmount != 96 {
		t.Fatalf("product override fee = %+v, want platform 4%% and official 0.6%%", got)
	}

	client.Product.UpdateOneID(prod.ID).ClearFeeRate().ExecX(ctx)
	got, err = svc.CalculateFee(ctx, "alipay", 100, usr.ID, prod.ID)
	if err != nil {
		t.Fatalf("CalculateFee user fallback returned error: %v", err)
	}
	if got.PlatformRate != 0.02 || got.FeePlatform != 2 || got.NetAmount != 98 {
		t.Fatalf("user fallback fee = %+v, want platform 2%%", got)
	}

	client.User.UpdateOneID(usr.ID).SetFeeRate(0).ExecX(ctx)
	got, err = svc.CalculateFee(ctx, "alipay", 100, usr.ID, prod.ID)
	if err != nil {
		t.Fatalf("CalculateFee explicit zero user rate returned error: %v", err)
	}
	if got.PlatformRate != 0 || got.FeePlatform != 0 || got.NetAmount != 100 {
		t.Fatalf("explicit zero user rate fee = %+v, want no platform fee", got)
	}
}

func TestCalculateFeeNormalizesLegacyPercentConfigAndHonorsZeroProductRate(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	zeroProductRate := 0.0
	usr, prod := createUserAndProduct(t, client, 0.02, &zeroProductRate)
	client.PlatformConfig.Create().SetKey("official_alipay_rate").SetValue("1.0").SaveX(ctx)
	svc := New(client, nil)

	got, err := svc.CalculateFee(ctx, "alipay", 100, usr.ID, prod.ID)
	if err != nil {
		t.Fatalf("CalculateFee returned error: %v", err)
	}
	if got.PlatformRate != 0 || got.FeePlatform != 0 || got.OfficialRate != 0.01 || got.FeeOfficial != 1 || got.NetAmount != 100 {
		t.Fatalf("fee with zero product rate and legacy official config = %+v, want zero platform and 1%% official", got)
	}
}

func TestCalculateFeeUsesDefaultConfigWhenNoScopedRate(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	client.PlatformConfig.Create().SetKey("default_platform_rate").SetValue("3.0").SaveX(ctx)
	svc := New(client, nil)

	got, err := svc.CalculateFee(ctx, "alipay", 100, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("CalculateFee returned error: %v", err)
	}
	if got.PlatformRate != 0.03 || got.FeePlatform != 3 || got.NetAmount != 97 {
		t.Fatalf("default config fee = %+v, want normalized 3%% platform fee", got)
	}
}

func TestCalculateFeeRejectsInvalidRateConfig(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	client.PlatformConfig.Create().SetKey("official_alipay_rate").SetValue("not-a-rate").SaveX(ctx)
	usr, prod := createUserAndProduct(t, client, 0.02, nil)
	svc := New(client, nil)

	if _, err := svc.CalculateFee(ctx, "alipay", 100, usr.ID, prod.ID); err == nil {
		t.Fatalf("CalculateFee with invalid rate config returned nil error")
	}
}

func TestCalculateFeeReturnsProductLookupError(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	usr, _ := createUserAndProduct(t, client, 0.02, nil)
	svc := New(client, nil)

	if _, err := svc.CalculateFee(ctx, "alipay", 100, usr.ID, uuid.New()); err == nil {
		t.Fatalf("CalculateFee with missing product returned nil error")
	}
}

func TestApplySettlementCreatesAndUpdatesUserSettlement(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	usr, prod := createUserAndProduct(t, client, 0.02, nil)
	client.PlatformConfig.Create().SetKey("official_alipay_rate").SetValue("0.01").SaveX(ctx)
	svc := New(client, nil)

	first := createPaidOrder(t, client, usr.ID, prod.ID, "ORDER-1", 100)
	if err := svc.ApplySettlement(ctx, first.ID); err != nil {
		t.Fatalf("ApplySettlement first returned error: %v", err)
	}
	updated := client.Order.GetX(ctx, first.ID)
	if updated.Status != order.StatusSETTLED || updated.FeePlatform != 2 || updated.FeeOfficial != 1 || updated.NetAmount != 98 {
		t.Fatalf("settled first order = %+v, want fees and SETTLED", updated)
	}
	sett := client.Settlement.Query().Where(settlement.UserIDEQ(usr.ID)).OnlyX(ctx)
	if sett.Balance != 98 || sett.TotalIncome != 100 {
		t.Fatalf("settlement after first = %+v, want balance 98 total 100", sett)
	}

	second := createPaidOrder(t, client, usr.ID, prod.ID, "ORDER-2", 50)
	if err := svc.ApplySettlement(ctx, second.ID); err != nil {
		t.Fatalf("ApplySettlement second returned error: %v", err)
	}
	sett = client.Settlement.Query().Where(settlement.UserIDEQ(usr.ID)).OnlyX(ctx)
	if sett.Balance != 147 || sett.TotalIncome != 150 {
		t.Fatalf("settlement after second = %+v, want balance 147 total 150", sett)
	}
}

func TestApplySettlementRejectsNonPaidAndGetUserBalanceMissing(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	usr, prod := createUserAndProduct(t, client, 0.02, nil)
	svc := New(client, nil)

	pending := client.Order.Create().
		SetProductID(prod.ID).
		SetUserID(usr.ID).
		SetOrderNo("PENDING-ORDER").
		SetType(order.TypeAlipay).
		SetAmount(10).
		SetNotifyURL("https://notify.example").
		SaveX(ctx)
	if err := svc.ApplySettlement(ctx, pending.ID); err == nil {
		t.Fatalf("ApplySettlement pending order returned nil error")
	}
	balance, err := svc.GetUserBalance(ctx, usr.ID)
	if err != nil {
		t.Fatalf("GetUserBalance returned error: %v", err)
	}
	if balance != 0 {
		t.Fatalf("balance = %.2f, want 0 for missing settlement", balance)
	}
}

func createPaidOrder(t *testing.T, client *ent.Client, userID, productID uuid.UUID, orderNo string, amount float64) *ent.Order {
	t.Helper()
	return client.Order.Create().
		SetProductID(productID).
		SetUserID(userID).
		SetOrderNo(orderNo).
		SetTradeNo(orderNo + "-TRADE").
		SetType(order.TypeAlipay).
		SetAmount(amount).
		SetStatus(order.StatusPAID).
		SetNotifyURL("https://notify.example").
		SetName("paid order").
		SaveX(context.Background())
}

func TestGetUserBalanceReturnsExistingSettlement(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	usr, _ := createUserAndProduct(t, client, 0.02, nil)
	client.Settlement.Create().
		SetUserID(usr.ID).
		SetBalance(42).
		SetFrozen(1).
		SetTotalIncome(43).
		SaveX(ctx)
	svc := New(client, nil)

	balance, err := svc.GetUserBalance(ctx, usr.ID)
	if err != nil {
		t.Fatalf("GetUserBalance returned error: %v", err)
	}
	if balance != 42 {
		t.Fatalf("balance = %.2f, want 42", balance)
	}
}

func TestApplySettlementReturnsMissingOrderError(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	svc := New(client, nil)

	if err := svc.ApplySettlement(ctx, uuid.New()); err == nil {
		t.Fatalf("ApplySettlement missing order returned nil error")
	}
}
