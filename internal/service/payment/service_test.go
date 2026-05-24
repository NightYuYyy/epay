package payment

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"epay/ent"
	"epay/ent/enttest"
	"epay/ent/order"
	"epay/ent/settlement"
	"epay/internal/config"
	"epay/internal/provider"
	feesvc "epay/internal/service/fee"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

type fakeProvider struct {
	createCalls  *int
	createResp   *provider.CreatePaymentResponse
	queryResp    *provider.QueryOrderResponse
	notification *provider.PaymentNotification
}

func (f fakeProvider) Name() string        { return "fake" }
func (f fakeProvider) ProviderKey() string { return "alipay" }
func (f fakeProvider) SupportedTypes() []provider.PaymentType {
	return []provider.PaymentType{provider.TypeAlipay}
}
func (f fakeProvider) CreatePayment(context.Context, provider.CreatePaymentRequest) (*provider.CreatePaymentResponse, error) {
	if f.createCalls != nil {
		(*f.createCalls)++
	}
	return f.createResp, nil
}
func (f fakeProvider) QueryOrder(context.Context, string) (*provider.QueryOrderResponse, error) {
	return f.queryResp, nil
}
func (f fakeProvider) VerifyNotification(context.Context, string, map[string]string) (*provider.PaymentNotification, error) {
	return f.notification, nil
}
func (f fakeProvider) Refund(context.Context, provider.RefundRequest) (*provider.RefundResponse, error) {
	return nil, nil
}

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

func testConfig() *config.Config {
	return &config.Config{
		Alipay: config.AlipayConfig{AppID: "app", PrivateKey: "private", PublicKey: "public"},
		JWT:    config.JWTConfig{Secret: "secret", ExpireHour: 24},
	}
}

func snapshotJSON(t *testing.T) string {
	t.Helper()
	raw, err := json.Marshal(providerSnapshot{
		ProviderKey: "alipay",
		InstanceID:  "alipay:test",
		PaymentType: provider.TypeAlipay,
		Config:      map[string]string{"appId": "app", "privateKey": "private", "publicKey": "public"},
		CreatedAt:   time.Now(),
	})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return string(raw)
}

func createMerchant(t *testing.T, client *ent.Client) *ent.Merchant {
	t.Helper()
	return client.Merchant.Create().
		SetPid(1001).
		SetPkey("merchant-key").
		SetName("merchant").
		SetFeeRate(2).
		SaveX(context.Background())
}

func TestHandleCallbackAppliesSettlementBeforeReturningSuccess(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	merchant := createMerchant(t, client)
	ord := client.Order.Create().
		SetMerchantID(merchant.ID).
		SetOrderNo("ORDER-CALLBACK").
		SetTradeNo("TRADE-CALLBACK").
		SetType(order.TypeAlipay).
		SetAmount(100).
		SetNotifyURL("https://merchant.example/notify").
		SetReturnURL("https://merchant.example/return").
		SetName("callback order").
		SetProviderSnapshot(snapshotJSON(t)).
		SaveX(ctx)

	provider.Register("alipay", func(string, map[string]string) (provider.Provider, error) {
		return fakeProvider{notification: &provider.PaymentNotification{
			TradeNo: "TRADE-CALLBACK",
			OrderID: "ORDER-CALLBACK",
			Amount:  100,
			Status:  provider.ProviderStatusSuccess,
		}}, nil
	})

	notifyCalls := 0
	service := NewPaymentService(client, nil, testConfig(), WithSettlementApplier(feesvc.New(client, nil)), WithNotifyDispatcher(func(context.Context, *ent.Order, *provider.PaymentNotification) {
		notifyCalls++
	}))
	resp, err := service.HandleCallback(ctx, provider.TypeAlipay, "provider-body", map[string]string{})
	if err != nil {
		t.Fatalf("HandleCallback returned error: %v", err)
	}
	if resp != "success" {
		t.Fatalf("HandleCallback response = %q, want success", resp)
	}

	updated := client.Order.GetX(ctx, ord.ID)
	if updated.Status != order.StatusSETTLED {
		t.Fatalf("order status = %s, want SETTLED", updated.Status)
	}
	if updated.FeePlatform != 2 || updated.NetAmount != 98 {
		t.Fatalf("settlement fields fee_platform=%.2f net_amount=%.2f, want 2.00 and 98.00", updated.FeePlatform, updated.NetAmount)
	}
	sett := client.Settlement.Query().Where(settlement.MerchantIDEQ(merchant.ID)).OnlyX(ctx)
	if sett.Balance != 98 || sett.TotalIncome != 100 {
		t.Fatalf("settlement balance=%.2f total_income=%.2f, want 98.00 and 100.00", sett.Balance, sett.TotalIncome)
	}
	if notifyCalls != 1 {
		t.Fatalf("notify calls = %d, want 1", notifyCalls)
	}
}

func TestHandleCallbackForAlreadyPaidOrderSettlesAndNotifies(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	merchant := createMerchant(t, client)
	ord := client.Order.Create().
		SetMerchantID(merchant.ID).
		SetOrderNo("ORDER-PAID-CALLBACK").
		SetTradeNo("TRADE-PAID-CALLBACK").
		SetType(order.TypeAlipay).
		SetAmount(20).
		SetStatus(order.StatusPAID).
		SetNotifyURL("https://merchant.example/notify").
		SetReturnURL("https://merchant.example/return").
		SetName("already paid callback order").
		SetProviderSnapshot(snapshotJSON(t)).
		SetPaidAt(time.Now().Add(-time.Minute)).
		SaveX(ctx)

	provider.Register("alipay", func(string, map[string]string) (provider.Provider, error) {
		return fakeProvider{notification: &provider.PaymentNotification{
			TradeNo: ord.TradeNo,
			OrderID: ord.OrderNo,
			Amount:  ord.Amount,
			Status:  provider.ProviderStatusSuccess,
		}}, nil
	})
	notifyCalls := 0
	service := NewPaymentService(client, nil, testConfig(), WithSettlementApplier(feesvc.New(client, nil)), WithNotifyDispatcher(func(context.Context, *ent.Order, *provider.PaymentNotification) {
		notifyCalls++
	}))
	resp, err := service.HandleCallback(ctx, provider.TypeAlipay, "provider-body", map[string]string{})
	if err != nil {
		t.Fatalf("HandleCallback returned error: %v", err)
	}
	if resp != "success" {
		t.Fatalf("HandleCallback response = %q, want success", resp)
	}
	updated := client.Order.GetX(ctx, ord.ID)
	if updated.Status != order.StatusSETTLED {
		t.Fatalf("order status = %s, want SETTLED", updated.Status)
	}
	if notifyCalls != 1 {
		t.Fatalf("notify calls = %d, want 1", notifyCalls)
	}
}

func TestCreateOrderReusesPendingOrderWithoutCreatingDuplicate(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	merchant := createMerchant(t, client)
	existing := client.Order.Create().
		SetMerchantID(merchant.ID).
		SetOrderNo("ORDER-IDEMPOTENT").
		SetTradeNo("OLD-PREPAY").
		SetType(order.TypeAlipay).
		SetAmount(12.34).
		SetNotifyURL("https://merchant.example/notify").
		SetReturnURL("https://merchant.example/return").
		SetName("idempotent order").
		SetParam("keep-me").
		SetClientip("203.0.113.10").
		SetDevice("pc").
		SetProviderSnapshot(snapshotJSON(t)).
		SaveX(ctx)

	createCalls := 0
	provider.Register("alipay", func(string, map[string]string) (provider.Provider, error) {
		return fakeProvider{
			createCalls: &createCalls,
			createResp:  &provider.CreatePaymentResponse{TradeNo: "NEW-PREPAY", PayURL: "https://pay.example/reused"},
		}, nil
	})

	service := NewPaymentService(client, nil, testConfig())
	resp, err := service.CreateOrder(ctx, CreateOrderRequest{
		PID:       merchant.Pid,
		OrderNo:   existing.OrderNo,
		Type:      provider.TypeAlipay,
		Amount:    existing.Amount,
		Subject:   existing.Name,
		NotifyURL: existing.NotifyURL,
		ReturnURL: existing.ReturnURL,
		ClientIP:  existing.Clientip,
		Device:    existing.Device,
		Param:     existing.Param,
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error for idempotent pending order: %v", err)
	}
	if resp.Order.ID != existing.ID {
		t.Fatalf("CreateOrder returned order %s, want existing %s", resp.Order.ID, existing.ID)
	}
	if resp.Provider.PayURL != "https://pay.example/reused" {
		t.Fatalf("provider pay URL = %q", resp.Provider.PayURL)
	}
	if createCalls != 1 {
		t.Fatalf("provider create calls = %d, want 1", createCalls)
	}
	count := client.Order.Query().Where(order.OrderNoEQ(existing.OrderNo)).CountX(ctx)
	if count != 1 {
		t.Fatalf("orders with reused order_no = %d, want 1", count)
	}
	updated := client.Order.GetX(ctx, existing.ID)
	if updated.TradeNo != "NEW-PREPAY" {
		t.Fatalf("existing trade_no = %q, want provider trade_no", updated.TradeNo)
	}
}

func TestScanExpiredOrdersSettlesPaidOrdersLeftUnsettled(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)
	merchant := createMerchant(t, client)
	paidAt := time.Now().Add(-10 * time.Minute)
	ord := client.Order.Create().
		SetMerchantID(merchant.ID).
		SetOrderNo("ORDER-PAID-UNSETTLED").
		SetTradeNo("TRADE-PAID-UNSETTLED").
		SetType(order.TypeAlipay).
		SetAmount(50).
		SetStatus(order.StatusPAID).
		SetNotifyURL("https://merchant.example/notify").
		SetReturnURL("https://merchant.example/return").
		SetName("paid unsettled order").
		SetProviderSnapshot(snapshotJSON(t)).
		SetPaidAt(paidAt).
		SaveX(ctx)

	notifyCalls := 0
	service := NewPaymentService(client, nil, testConfig(), WithSettlementApplier(feesvc.New(client, nil)), WithNotifyDispatcher(func(context.Context, *ent.Order, *provider.PaymentNotification) {
		notifyCalls++
	}))
	service.scanExpiredOrders(ctx)

	updated := client.Order.GetX(ctx, ord.ID)
	if updated.Status != order.StatusSETTLED {
		t.Fatalf("order status = %s, want SETTLED", updated.Status)
	}
	sett := client.Settlement.Query().Where(settlement.MerchantIDEQ(merchant.ID)).OnlyX(ctx)
	if sett.Balance != 49 || sett.TotalIncome != 50 {
		t.Fatalf("settlement balance=%.2f total_income=%.2f, want 49.00 and 50.00", sett.Balance, sett.TotalIncome)
	}
	if notifyCalls != 1 {
		t.Fatalf("notify calls = %d, want 1", notifyCalls)
	}
}
