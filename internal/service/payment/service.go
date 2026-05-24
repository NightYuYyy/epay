package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"epay/ent"
	"epay/ent/order"
	"epay/ent/product"
	"epay/ent/user"
	"epay/internal/config"
	easypay "epay/internal/handler/easypay"
	"epay/internal/provider"
	redisutil "epay/internal/redis"

	"github.com/google/uuid"

	goredis "github.com/redis/go-redis/v9"
)

const (
	callbackIDTTL        = 24 * time.Hour
	amountTolerance      = 0.01
	defaultOrderExpireIn = 30 * time.Minute
)

func amountDiffAtOrAboveTolerance(a, b float64) bool {
	return math.Abs(a-b) >= amountTolerance
}

type settlementApplier interface {
	ApplySettlement(ctx context.Context, orderID uuid.UUID) error
}

// NotifyDispatcher is the hook used in tests; production code uses the
// built-in forwardMerchantCallback path.
type NotifyDispatcher func(ctx context.Context, ord *ent.Order, notification *provider.PaymentNotification)

type Option func(*PaymentService)

type PaymentService struct {
	ent        *ent.Client
	rdb        *goredis.Client
	cfg        *config.Config
	settlement settlementApplier
	notify     NotifyDispatcher
}

func WithSettlementApplier(applier settlementApplier) Option {
	return func(s *PaymentService) {
		s.settlement = applier
	}
}

func WithNotifyDispatcher(dispatcher NotifyDispatcher) Option {
	return func(s *PaymentService) {
		s.notify = dispatcher
	}
}

func NewPaymentService(ent *ent.Client, rdb *goredis.Client, cfg *config.Config, opts ...Option) *PaymentService {
	s := &PaymentService{ent: ent, rdb: rdb, cfg: cfg}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
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
	Param       string
	Device      string
	Method      string
	SubOpenID   string
	SubAppID    string
	AuthCode    string
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

	lockKey := "payment:create:" + req.OrderNo
	acquired, err := redisutil.AcquireLock(ctx, s.rdb, lockKey, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("acquire payment creation lock: %w", err)
	}
	if !acquired {
		return nil, fmt.Errorf("payment creation already in progress for order_no %s", req.OrderNo)
	}
	defer func() {
		_ = redisutil.ReleaseLock(ctx, s.rdb, lockKey)
	}()

	prod, err := s.ent.Product.Query().Where(product.PidEQ(req.PID)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("load product by pid: %w", err)
	}
	if prod.Status != product.StatusActive {
		return nil, fmt.Errorf("product is not active")
	}
	usr, err := s.ent.User.Query().Where(user.IDEQ(prod.UserID)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("load product owner: %w", err)
	}
	if usr.Status != user.StatusActive {
		return nil, fmt.Errorf("product owner is not active")
	}

	providerKey, entType, err := providerKeyForType(req.Type)
	if err != nil {
		return nil, err
	}
	if existing, err := s.ent.Order.Query().Where(order.OrderNoEQ(req.OrderNo)).Only(ctx); err == nil {
		return s.recreatePendingPayment(ctx, existing, prod, req, providerKey, entType)
	} else if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("load existing order: %w", err)
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

	createResp, err := payProvider.CreatePayment(ctx, buildProviderPaymentRequest(configMap, req))
	if err != nil {
		return nil, fmt.Errorf("provider create payment: %w", err)
	}

	tradeNo := ""
	if createResp != nil {
		tradeNo = createResp.TradeNo
	}
	orderRow, err := s.ent.Order.Create().
		SetOrderNo(req.OrderNo).
		SetProductID(prod.ID).
		SetUserID(prod.UserID).
		SetType(entType).
		SetAmount(req.Amount).
		SetStatus(order.StatusPENDING).
		SetNotifyURL(req.NotifyURL).
		SetReturnURL(req.ReturnURL).
		SetName(req.Subject).
		SetParam(req.Param).
		SetClientip(req.ClientIP).
		SetDevice(firstNonEmpty(req.Device, "pc")).
		SetMethod(req.Method).
		SetSubOpenid(req.SubOpenID).
		SetSubAppid(req.SubAppID).
		SetAuthCode(req.AuthCode).
		SetProviderSnapshot(string(snapshotJSON)).
		SetTradeNo(tradeNo).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create order row: %w", err)
	}

	return &CreateOrderResponse{Order: orderRow, Provider: createResp}, nil
}

func (s *PaymentService) recreatePendingPayment(ctx context.Context, existing *ent.Order, prod *ent.Product, req CreateOrderRequest, providerKey string, entType order.Type) (*CreateOrderResponse, error) {
	if existing.ProductID != prod.ID {
		return nil, fmt.Errorf("order_no is already used by another product")
	}
	switch existing.Status {
	case order.StatusPAID, order.StatusSETTLED:
		return nil, fmt.Errorf("order %s is already paid", existing.OrderNo)
	case order.StatusPENDING:
		// Recreate the upstream payment below using the immutable provider snapshot.
	default:
		return nil, fmt.Errorf("order %s has status %s and cannot be paid", existing.OrderNo, existing.Status)
	}
	if err := sameCreateOrderRequest(existing, req, entType); err != nil {
		return nil, err
	}
	if err := verifySnapshot(existing.ProviderSnapshot, providerKey, req.Type); err != nil {
		return nil, err
	}
	snap, err := parseSnapshot(existing.ProviderSnapshot)
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
	createResp, err := payProvider.CreatePayment(ctx, buildProviderPaymentRequest(snap.Config, req))
	if err != nil {
		return nil, fmt.Errorf("provider recreate payment: %w", err)
	}
	if createResp != nil && strings.TrimSpace(createResp.TradeNo) != "" && createResp.TradeNo != existing.TradeNo {
		updated, err := s.ent.Order.UpdateOneID(existing.ID).SetTradeNo(createResp.TradeNo).Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("update existing order trade_no: %w", err)
		}
		existing = updated
	}
	return &CreateOrderResponse{Order: existing, Provider: createResp}, nil
}

func sameCreateOrderRequest(existing *ent.Order, req CreateOrderRequest, entType order.Type) error {
	if existing.Type != entType ||
		amountDiffAtOrAboveTolerance(existing.Amount, req.Amount) ||
		existing.Name != req.Subject ||
		existing.NotifyURL != req.NotifyURL ||
		existing.ReturnURL != req.ReturnURL ||
		existing.Param != req.Param ||
		existing.Clientip != req.ClientIP ||
		existing.Device != firstNonEmpty(req.Device, "pc") ||
		existing.Method != req.Method ||
		existing.SubOpenid != req.SubOpenID ||
		existing.SubAppid != req.SubAppID ||
		existing.AuthCode != req.AuthCode {
		return fmt.Errorf("order %s payment parameters changed", existing.OrderNo)
	}
	return nil
}

func buildProviderPaymentRequest(configMap map[string]string, req CreateOrderRequest) provider.CreatePaymentRequest {
	return provider.CreatePaymentRequest{
		OrderID:     req.OrderNo,
		Amount:      formatAmount(req.Amount),
		PaymentType: req.Type,
		Subject:     req.Subject,
		NotifyURL:   callbackURLForProvider(configMap, req.NotifyURL),
		ReturnURL:   req.ReturnURL,
		ClientIP:    req.ClientIP,
		IsMobile:    req.IsMobile,
	}
}

func (s *PaymentService) HandleCallback(ctx context.Context, paymentType provider.PaymentType, rawBody string, headers map[string]string) (response string, err error) {
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

	ord, err := s.ent.Order.Query().Where(order.OrderNoEQ(outTradeNo)).Only(ctx)
	if err != nil {
		return "", fmt.Errorf("load order: %w", err)
	}
	if ord.Status == order.StatusSETTLED {
		return "success", nil
	}
	if ord.Status == order.StatusPAID {
		settled, err := s.settlePaidOrder(ctx, ord)
		if err != nil {
			return "", err
		}
		s.dispatchMerchantCallback(context.Background(), settled, notification)
		return "success", nil
	}
	if ord.Status != order.StatusPENDING {
		return "", fmt.Errorf("order status is %s, not payable", ord.Status)
	}
	if err := verifySnapshot(ord.ProviderSnapshot, providerKey, paymentType); err != nil {
		return "", err
	}
	if amountDiffAtOrAboveTolerance(notification.Amount, ord.Amount) {
		return "", fmt.Errorf("amount mismatch: paid %.2f expected %.2f", notification.Amount, ord.Amount)
	}

	callbackKey := "callback:" + outTradeNo
	callbackLocked := false
	defer func() {
		if err != nil && callbackLocked && s.rdb != nil {
			_ = s.rdb.Del(context.Background(), callbackKey).Err()
		}
	}()
	if s.rdb != nil {
		ok, rerr := s.rdb.SetNX(ctx, callbackKey, 1, callbackIDTTL).Result()
		if rerr != nil {
			return "", fmt.Errorf("callback idempotency setnx: %w", rerr)
		}
		if !ok {
			return "success", nil
		}
		callbackLocked = true
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
	settled, err := s.settlePaidOrder(ctx, updated)
	if err != nil {
		return "", err
	}
	s.dispatchMerchantCallback(context.Background(), settled, notification)
	return "success", nil
}

func (s *PaymentService) settlePaidOrder(ctx context.Context, ord *ent.Order) (*ent.Order, error) {
	if ord == nil {
		return nil, fmt.Errorf("order is nil")
	}
	if s.settlement == nil || ord.Status == order.StatusSETTLED {
		return ord, nil
	}
	if ord.Status != order.StatusPAID {
		return nil, fmt.Errorf("order %s has status %s, not PAID", ord.OrderNo, ord.Status)
	}
	if err := s.settlement.ApplySettlement(ctx, ord.ID); err != nil {
		return nil, fmt.Errorf("apply settlement for order %s: %w", ord.OrderNo, err)
	}
	updated, err := s.ent.Order.Get(ctx, ord.ID)
	if err != nil {
		return nil, fmt.Errorf("reload settled order %s: %w", ord.OrderNo, err)
	}
	return updated, nil
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
	s.scanPaidOrders(ctx)
}

func (s *PaymentService) scanPaidOrders(ctx context.Context) {
	deadline := time.Now().Add(-5 * time.Minute)
	orders, err := s.ent.Order.Query().
		Where(order.StatusEQ(order.StatusPAID), order.PaidAtLTE(deadline)).
		Limit(100).
		All(ctx)
	if err != nil {
		log.Printf("[payment] scan paid orders: %v", err)
		return
	}
	for _, ord := range orders {
		settled, err := s.settlePaidOrder(ctx, ord)
		if err != nil {
			log.Printf("[payment] settle paid order %s: %v", ord.OrderNo, err)
			continue
		}
		s.dispatchMerchantCallback(context.Background(), settled, &provider.PaymentNotification{
			TradeNo: settled.TradeNo,
			OrderID: settled.OrderNo,
			Amount:  settled.Amount,
			Status:  provider.ProviderStatusSuccess,
		})
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
		if queryResp.Amount > 0 && amountDiffAtOrAboveTolerance(queryResp.Amount, ord.Amount) {
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
		settled, err := s.settlePaidOrder(ctx, updated)
		if err != nil {
			return nil, err
		}
		s.dispatchMerchantCallback(context.Background(), settled, &provider.PaymentNotification{
			TradeNo: queryResp.TradeNo,
			OrderID: ord.OrderNo,
			Amount:  ord.Amount,
			Status:  provider.ProviderStatusSuccess,
		})
		return settled, nil
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

func (s *PaymentService) dispatchMerchantCallback(ctx context.Context, ord *ent.Order, notification *provider.PaymentNotification) {
	if s.notify != nil {
		s.notify(ctx, ord, notification)
		return
	}
	go s.forwardMerchantCallback(ctx, ord, notification)
}

// forwardMerchantCallback dispatches an asynchronous notification to the
// caller's notify_url using the rainbow-EasyPay GET + signed-form contract.
// Signing algorithm is selected by ord.Version (0 → MD5 with product.Pkey,
// 1 → RSA with the platform private key).
func (s *PaymentService) forwardMerchantCallback(ctx context.Context, ord *ent.Order, _ *provider.PaymentNotification) {
	if ord == nil || strings.TrimSpace(ord.NotifyURL) == "" {
		return
	}
	prod, err := s.ent.Product.Query().Where(product.ID(ord.ProductID)).First(ctx)
	if err != nil {
		log.Printf("[payment] notify lookup product %s: %v", ord.OrderNo, err)
		return
	}
	platformPriv := ""
	if s.cfg != nil {
		platformPriv = s.cfg.Platform.RSAPrivateKey
	}
	fullURL, err := easypay.BuildNotifyURL(ord, prod, platformPriv)
	if err != nil {
		log.Printf("[payment] notify build url %s: %v", ord.OrderNo, err)
		return
	}
	easypay.DispatchNotify(ctx, nil, fullURL, ord.OrderNo)
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
	entries := map[string]string{}
	if s.ent != nil {
		rows, err := s.ent.PlatformConfig.Query().All(context.Background())
		if err == nil {
			for _, row := range rows {
				entries[row.Key] = row.Value
			}
		}
	}
	switch providerKey {
	case "alipay":
		return map[string]string{
			"appId":                firstNonEmpty(entries["alipay_app_id"], s.cfg.Alipay.AppID),
			"privateKey":           firstNonEmpty(entries["alipay_private_key"], s.cfg.Alipay.PrivateKey),
			"publicKey":            firstNonEmpty(entries["alipay_public_key"], s.cfg.Alipay.PublicKey),
			"alipayPublicKey":      firstNonEmpty(entries["alipay_public_key"], s.cfg.Alipay.PublicKey),
			"notifyUrl":            firstNonEmpty(entries["alipay_notify_url"], s.cfg.Alipay.NotifyURL),
			"returnUrl":            firstNonEmpty(entries["alipay_return_url"], s.cfg.Alipay.ReturnURL),
			"production":           firstNonEmpty(entries["alipay_production"], "false"),
			"sandboxLegacyGateway": firstNonEmpty(entries["alipay_sandbox_legacy_gateway"], "false"),
		}, nil
	case "wxpay":
		return map[string]string{
			"appId":       firstNonEmpty(entries["wxpay_app_id"], s.cfg.Wxpay.AppID),
			"mchId":       firstNonEmpty(entries["wxpay_mch_id"], s.cfg.Wxpay.MchID),
			"privateKey":  firstNonEmpty(entries["wxpay_private_key"], s.cfg.Wxpay.PrivateKey),
			"apiV3Key":    firstNonEmpty(entries["wxpay_api_v3_key"], s.cfg.Wxpay.APIv3Key),
			"publicKey":   firstNonEmpty(entries["wxpay_public_key"], s.cfg.Wxpay.PublicKey),
			"publicKeyId": firstNonEmpty(entries["wxpay_public_key_id"], s.cfg.Wxpay.PublicKeyID),
			"serialNo":    firstNonEmpty(entries["wxpay_serial_no"], s.cfg.Wxpay.SerialNo),
			"notifyUrl":   firstNonEmpty(entries["wxpay_notify_url"], s.cfg.Wxpay.NotifyURL),
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
