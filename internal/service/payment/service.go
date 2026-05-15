package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"epay/ent"
	"epay/ent/merchant"
	"epay/ent/order"
	"epay/internal/config"
	"epay/internal/provider"

	goredis "github.com/redis/go-redis/v9"
)

const (
	callbackIDTTL        = 24 * time.Hour
	amountTolerance      = 0.01
	defaultOrderExpireIn = 30 * time.Minute
)

var merchantNotifyRetryIntervals = []time.Duration{
	10 * time.Second,
	60 * time.Second,
	10 * time.Minute,
	30 * time.Minute,
	60 * time.Minute,
}

type PaymentService struct {
	ent *ent.Client
	rdb *goredis.Client
	cfg *config.Config
}

func NewPaymentService(ent *ent.Client, rdb *goredis.Client, cfg *config.Config) *PaymentService {
	return &PaymentService{ent: ent, rdb: rdb, cfg: cfg}
}

type CreateOrderRequest struct {
	PID         int
	OrderNo     string
	Type        provider.PaymentType
	Amount      float64
	Subject     string
	NotifyURL   string
	ReturnURL   string
	ClientIP    string
	IsMobile    bool
	ExpireAfter time.Duration
}

type CreateOrderResponse struct {
	Order    *ent.Order
	Provider *provider.CreatePaymentResponse
}

type providerSnapshot struct {
	ProviderKey string               `json:"provider_key"`
	InstanceID  string               `json:"instance_id"`
	PaymentType provider.PaymentType `json:"payment_type"`
	Config      map[string]string    `json:"config"`
	CreatedAt   time.Time            `json:"created_at"`
}

func (s *PaymentService) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, error) {
	if s == nil || s.ent == nil {
		return nil, fmt.Errorf("payment service ent client is nil")
	}
	if req.PID == 0 {
		return nil, fmt.Errorf("pid is required")
	}
	req.OrderNo = strings.TrimSpace(req.OrderNo)
	if req.OrderNo == "" {
		return nil, fmt.Errorf("order_no is required")
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	if strings.TrimSpace(req.NotifyURL) == "" {
		return nil, fmt.Errorf("notify_url is required")
	}
	if req.Subject == "" {
		req.Subject = req.OrderNo
	}

	m, err := s.ent.Merchant.Query().Where(merchant.PidEQ(req.PID)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("load merchant by pid: %w", err)
	}
	if m.Status != merchant.StatusActive {
		return nil, fmt.Errorf("merchant is not active")
	}

	providerKey, entType, err := providerKeyForType(req.Type)
	if err != nil {
		return nil, err
	}
	configMap, err := s.providerConfig(providerKey)
	if err != nil {
		return nil, err
	}
	payProvider, snap, err := s.newProvider(providerKey, req.Type, configMap)
	if err != nil {
		return nil, err
	}
	snapshotJSON, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("marshal provider snapshot: %w", err)
	}

	amount := formatAmount(req.Amount)
	createResp, err := payProvider.CreatePayment(ctx, provider.CreatePaymentRequest{
		OrderID:     req.OrderNo,
		Amount:      amount,
		PaymentType: req.Type,
		Subject:     req.Subject,
		NotifyURL:   callbackURLForProvider(configMap, req.NotifyURL),
		ReturnURL:   req.ReturnURL,
		ClientIP:    req.ClientIP,
		IsMobile:    req.IsMobile,
	})
	if err != nil {
		return nil, fmt.Errorf("provider create payment: %w", err)
	}

	tradeNo := ""
	if createResp != nil {
		tradeNo = createResp.TradeNo
	}
	orderRow, err := s.ent.Order.Create().
		SetOrderNo(req.OrderNo).
		SetMerchantID(m.ID).
		SetType(entType).
		SetAmount(req.Amount).
		SetStatus(order.StatusPENDING).
		SetNotifyURL(req.NotifyURL).
		SetProviderSnapshot(string(snapshotJSON)).
		SetTradeNo(tradeNo).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create order row: %w", err)
	}

	return &CreateOrderResponse{Order: orderRow, Provider: createResp}, nil
}

func (s *PaymentService) HandleCallback(ctx context.Context, paymentType provider.PaymentType, rawBody string, headers map[string]string) (string, error) {
	providerKey, _, err := providerKeyForType(paymentType)
	if err != nil {
		return "", err
	}
	configMap, err := s.providerConfig(providerKey)
	if err != nil {
		return "", err
	}
	payProvider, _, err := s.newProvider(providerKey, paymentType, configMap)
	if err != nil {
		return "", err
	}
	notification, err := payProvider.VerifyNotification(ctx, rawBody, headers)
	if err != nil {
		return "", fmt.Errorf("verify notification: %w", err)
	}
	if notification == nil {
		return "", fmt.Errorf("empty notification")
	}
	outTradeNo := strings.TrimSpace(notification.OrderID)
	if outTradeNo == "" {
		return "", fmt.Errorf("notification out_trade_no is empty")
	}
	if !isPaidProviderStatus(notification.Status) {
		return "success", nil
	}

	if s.rdb != nil {
		ok, err := s.rdb.SetNX(ctx, "callback:"+outTradeNo, 1, callbackIDTTL).Result()
		if err != nil {
			return "", fmt.Errorf("callback idempotency setnx: %w", err)
		}
		if !ok {
			return "success", nil
		}
	}

	ord, err := s.ent.Order.Query().Where(order.OrderNoEQ(outTradeNo)).Only(ctx)
	if err != nil {
		return "", fmt.Errorf("load order: %w", err)
	}
	if ord.Status == order.StatusPAID || ord.Status == order.StatusSETTLED {
		return "success", nil
	}
	if ord.Status != order.StatusPENDING {
		return "", fmt.Errorf("order status is %s, not payable", ord.Status)
	}
	if err := verifySnapshot(ord.ProviderSnapshot, providerKey, paymentType); err != nil {
		return "", err
	}
	if math.Abs(notification.Amount-ord.Amount) > amountTolerance {
		return "", fmt.Errorf("amount mismatch: paid %.2f expected %.2f", notification.Amount, ord.Amount)
	}

	paidAt := time.Now()
	updated, err := s.ent.Order.UpdateOneID(ord.ID).
		SetStatus(order.StatusPAID).
		SetTradeNo(firstNonEmpty(notification.TradeNo, ord.TradeNo)).
		SetPaidAt(paidAt).
		Save(ctx)
	if err != nil {
		return "", fmt.Errorf("mark order paid: %w", err)
	}
	go s.forwardMerchantCallback(context.Background(), updated, notification)
	return "success", nil
}

func (s *PaymentService) QueryOrder(ctx context.Context, orderNo string) (*ent.Order, error) {
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return nil, fmt.Errorf("order_no is required")
	}
	ord, err := s.ent.Order.Query().Where(order.OrderNoEQ(orderNo)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("load order: %w", err)
	}
	if ord.Status != order.StatusPENDING {
		return ord, nil
	}
	return s.syncPendingOrder(ctx, ord, false)
}

func (s *PaymentService) StartExpiryScanner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.scanExpiredOrders(ctx)
			}
		}
	}()
}

func (s *PaymentService) scanExpiredOrders(ctx context.Context) {
	deadline := time.Now().Add(-defaultOrderExpireIn)
	orders, err := s.ent.Order.Query().
		Where(order.StatusEQ(order.StatusPENDING), order.CreatedAtLTE(deadline)).
		Limit(100).
		All(ctx)
	if err != nil {
		log.Printf("[payment] scan expired orders: %v", err)
		return
	}
	for _, ord := range orders {
		if _, err := s.syncPendingOrder(ctx, ord, true); err != nil {
			log.Printf("[payment] sync expired order %s: %v", ord.OrderNo, err)
		}
	}
}

func (s *PaymentService) syncPendingOrder(ctx context.Context, ord *ent.Order, expireIfUnpaid bool) (*ent.Order, error) {
	if ord == nil {
		return nil, fmt.Errorf("order is nil")
	}
	snap, err := parseSnapshot(ord.ProviderSnapshot)
	if err != nil {
		return nil, err
	}
	factory, ok := provider.Get(snap.ProviderKey)
	if !ok {
		return nil, fmt.Errorf("provider factory not found: %s", snap.ProviderKey)
	}
	payProvider, err := factory(snap.InstanceID, snap.Config)
	if err != nil {
		return nil, fmt.Errorf("new provider from snapshot: %w", err)
	}
	queryNo := firstNonEmpty(ord.OrderNo, ord.TradeNo)
	queryResp, err := payProvider.QueryOrder(ctx, queryNo)
	if err != nil {
		return nil, fmt.Errorf("provider query order: %w", err)
	}
	if queryResp == nil {
		return ord, nil
	}
	if isPaidProviderStatus(queryResp.Status) {
		if queryResp.Amount > 0 && math.Abs(queryResp.Amount-ord.Amount) > amountTolerance {
			return nil, fmt.Errorf("amount mismatch: queried %.2f expected %.2f", queryResp.Amount, ord.Amount)
		}
		paidAt := parseProviderTime(queryResp.PaidAt)
		upd := s.ent.Order.UpdateOneID(ord.ID).
			SetStatus(order.StatusPAID).
			SetTradeNo(firstNonEmpty(queryResp.TradeNo, ord.TradeNo))
		if !paidAt.IsZero() {
			upd.SetPaidAt(paidAt)
		} else {
			upd.SetPaidAt(time.Now())
		}
		updated, err := upd.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("sync mark paid: %w", err)
		}
		go s.forwardMerchantCallback(context.Background(), updated, &provider.PaymentNotification{
			TradeNo: queryResp.TradeNo,
			OrderID: ord.OrderNo,
			Amount:  ord.Amount,
			Status:  provider.ProviderStatusSuccess,
		})
		return updated, nil
	}
	if expireIfUnpaid {
		updated, err := s.ent.Order.UpdateOneID(ord.ID).SetStatus(order.StatusEXPIRED).Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("mark expired: %w", err)
		}
		return updated, nil
	}
	return ord, nil
}

func (s *PaymentService) forwardMerchantCallback(ctx context.Context, ord *ent.Order, notification *provider.PaymentNotification) {
	if ord == nil || strings.TrimSpace(ord.NotifyURL) == "" {
		return
	}
	payload := map[string]any{
		"out_trade_no": ord.OrderNo,
		"trade_no":     firstNonEmpty(ord.TradeNo, notificationTradeNo(notification)),
		"money":        formatAmount(ord.Amount),
		"status":       provider.ProviderStatusSuccess,
		"paid_at":      time.Now().Format(time.RFC3339),
	}
	if ord.PaidAt != nil {
		payload["paid_at"] = ord.PaidAt.Format(time.RFC3339)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[payment] marshal merchant notify payload: %v", err)
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	for attempt := 0; attempt <= len(merchantNotifyRetryIntervals); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(merchantNotifyRetryIntervals[attempt-1]):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, ord.NotifyURL, bytes.NewReader(body))
		if err != nil {
			log.Printf("[payment] build merchant notify request %s: %v", ord.OrderNo, err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 && strings.TrimSpace(string(respBody)) == "success" {
				return
			}
			log.Printf("[payment] merchant notify %s attempt %d failed: status=%d body=%q", ord.OrderNo, attempt+1, resp.StatusCode, string(respBody))
			continue
		}
		log.Printf("[payment] merchant notify %s attempt %d error: %v", ord.OrderNo, attempt+1, err)
	}
}

func (s *PaymentService) newProvider(providerKey string, paymentType provider.PaymentType, configMap map[string]string) (provider.Provider, providerSnapshot, error) {
	factory, ok := provider.Get(providerKey)
	if !ok {
		return nil, providerSnapshot{}, fmt.Errorf("provider factory not found: %s", providerKey)
	}
	snap := providerSnapshot{
		ProviderKey: providerKey,
		InstanceID:  providerKey + ":default",
		PaymentType: paymentType,
		Config:      cloneStringMap(configMap),
		CreatedAt:   time.Now(),
	}
	payProvider, err := factory(snap.InstanceID, snap.Config)
	if err != nil {
		return nil, providerSnapshot{}, fmt.Errorf("new provider %s: %w", providerKey, err)
	}
	return payProvider, snap, nil
}

func (s *PaymentService) providerConfig(providerKey string) (map[string]string, error) {
	if s == nil || s.cfg == nil {
		return nil, fmt.Errorf("payment service config is nil")
	}
	switch providerKey {
	case "alipay":
		return map[string]string{
			"appId":           s.cfg.Alipay.AppID,
			"privateKey":      s.cfg.Alipay.PrivateKey,
			"publicKey":       s.cfg.Alipay.PublicKey,
			"alipayPublicKey": s.cfg.Alipay.PublicKey,
			"notifyUrl":       s.cfg.Alipay.NotifyURL,
			"returnUrl":       s.cfg.Alipay.ReturnURL,
		}, nil
	case "wxpay":
		return map[string]string{
			"appId":       s.cfg.Wxpay.AppID,
			"mchId":       s.cfg.Wxpay.MchID,
			"privateKey":  s.cfg.Wxpay.PrivateKey,
			"apiV3Key":    s.cfg.Wxpay.APIv3Key,
			"publicKey":   s.cfg.Wxpay.PublicKey,
			"publicKeyId": s.cfg.Wxpay.PublicKeyID,
			"serialNo":    s.cfg.Wxpay.SerialNo,
			"notifyUrl":   s.cfg.Wxpay.NotifyURL,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported provider key: %s", providerKey)
	}
}

func providerKeyForType(paymentType provider.PaymentType) (string, order.Type, error) {
	switch paymentType {
	case provider.TypeAlipay, provider.TypeAlipayDirect:
		return "alipay", order.TypeAlipay, nil
	case provider.TypeWxpay, provider.TypeWxpayDirect:
		return "wxpay", order.TypeWxpay, nil
	default:
		return "", "", fmt.Errorf("unsupported payment type: %s", paymentType)
	}
}

func verifySnapshot(raw, providerKey string, paymentType provider.PaymentType) error {
	snap, err := parseSnapshot(raw)
	if err != nil {
		return err
	}
	if snap.ProviderKey != providerKey {
		return fmt.Errorf("provider snapshot mismatch: got %s want %s", snap.ProviderKey, providerKey)
	}
	expectedKey, _, err := providerKeyForType(snap.PaymentType)
	if err != nil {
		return err
	}
	actualKey, _, err := providerKeyForType(paymentType)
	if err != nil {
		return err
	}
	if expectedKey != actualKey {
		return fmt.Errorf("payment type snapshot mismatch: got %s want %s", paymentType, snap.PaymentType)
	}
	return nil
}

func parseSnapshot(raw string) (providerSnapshot, error) {
	var snap providerSnapshot
	if strings.TrimSpace(raw) == "" {
		return snap, fmt.Errorf("provider snapshot is empty")
	}
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return snap, fmt.Errorf("parse provider snapshot: %w", err)
	}
	if snap.ProviderKey == "" || snap.InstanceID == "" || len(snap.Config) == 0 {
		return snap, fmt.Errorf("provider snapshot is incomplete")
	}
	return snap, nil
}

func callbackURLForProvider(configMap map[string]string, merchantNotifyURL string) string {
	if notifyURL := strings.TrimSpace(configMap["notifyUrl"]); notifyURL != "" {
		return notifyURL
	}
	return strings.TrimSpace(merchantNotifyURL)
}

func isPaidProviderStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case provider.ProviderStatusPaid, provider.ProviderStatusSuccess:
		return true
	default:
		return false
	}
}

func formatAmount(amount float64) string {
	return strconv.FormatFloat(math.Round(amount*100)/100, 'f', 2, 64)
}

func parseProviderTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func notificationTradeNo(n *provider.PaymentNotification) string {
	if n == nil {
		return ""
	}
	return n.TradeNo
}
