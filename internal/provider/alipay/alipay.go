package alipay

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"epay/internal/provider"

	"github.com/smartwalle/alipay/v3"
)

// Alipay product codes.
const (
	alipayProductCodePreCreate = "FACE_TO_FACE_PAYMENT"
	alipayProductCodeWapPay    = "QUICK_WAP_WAY"
	alipayProductCodePagePay   = "FAST_INSTANT_TRADE_PAY"
)

// Alipay response constants.
const alipayErrTradeNotExist = "ACQ.TRADE_NOT_EXIST"

var (
	alipayTradeWapPay = func(client *alipay.Client, param alipay.TradeWapPay) (*url.URL, error) {
		return client.TradeWapPay(param)
	}
	alipayTradePreCreate = func(ctx context.Context, client *alipay.Client, param alipay.TradePreCreate) (*alipay.TradePreCreateRsp, error) {
		return client.TradePreCreate(ctx, param)
	}
	alipayTradePagePay = func(client *alipay.Client, param alipay.TradePagePay) (*url.URL, error) {
		return client.TradePagePay(param)
	}
)

// AlipayProvider implements provider.Provider and provider.CancelableProvider
// using the smartwalle/alipay/v3 SDK.
type AlipayProvider struct {
	instanceID string
	config     map[string]string // appId, privateKey, publicKey/alipayPublicKey, notifyUrl, returnUrl

	mu     sync.Mutex
	client *alipay.Client
}

func init() {
	provider.Register("alipay", func(instanceID string, config map[string]string) (provider.Provider, error) {
		return NewAlipay(instanceID, config)
	})
}

// NewAlipay creates a lazy-initialized Alipay provider instance.
func NewAlipay(instanceID string, config map[string]string) (*AlipayProvider, error) {
	if config == nil {
		return nil, errors.New("alipay config is nil")
	}
	for _, key := range []string{"appId", "privateKey"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("alipay config missing required key: %s", key)
		}
	}
	return &AlipayProvider{
		instanceID: instanceID,
		config:     config,
	}, nil
}

func (a *AlipayProvider) getClient() (*alipay.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil {
		return a.client, nil
	}

	production := false
	if raw := strings.TrimSpace(a.config["production"]); raw != "" {
		if parsed, perr := strconv.ParseBool(raw); perr == nil {
			production = parsed
		}
	}
	client, err := alipay.New(a.config["appId"], a.config["privateKey"], production)
	if err != nil {
		return nil, fmt.Errorf("alipay init client: %w", err)
	}

	publicKey := a.config["publicKey"]
	if strings.TrimSpace(publicKey) == "" {
		publicKey = a.config["alipayPublicKey"]
	}
	if strings.TrimSpace(publicKey) == "" {
		return nil, errors.New("alipay config missing required key: publicKey (or alipayPublicKey)")
	}
	if err := client.LoadAliPayPublicKey(publicKey); err != nil {
		return nil, fmt.Errorf("alipay load public key: %w", err)
	}

	a.client = client
	return a.client, nil
}

// Name returns the human-readable provider name.
func (a *AlipayProvider) Name() string { return "Alipay" }

// ProviderKey returns the provider registry key.
func (a *AlipayProvider) ProviderKey() string { return "alipay" }

// SupportedTypes returns payment types supported by AlipayProvider.
func (a *AlipayProvider) SupportedTypes() []string {
	return []string{provider.TypeAlipay}
}

// CreatePayment creates an Alipay payment using mobile WAP pay or desktop
// precreate/page-pay fallback routing.
func (a *AlipayProvider) CreatePayment(ctx context.Context, req provider.CreatePaymentRequest) (*provider.CreatePaymentResponse, error) {
	client, err := a.getClient()
	if err != nil {
		return nil, err
	}

	notifyURL := a.config["notifyUrl"]
	if req.NotifyURL != "" {
		notifyURL = req.NotifyURL
	}
	returnURL := a.config["returnUrl"]
	if req.ReturnURL != "" {
		returnURL = req.ReturnURL
	}

	if req.IsMobile {
		return a.createWapTrade(client, req, notifyURL, returnURL)
	}
	return a.createDesktopTrade(ctx, client, req, notifyURL, returnURL)
}

func (a *AlipayProvider) createWapTrade(client *alipay.Client, req provider.CreatePaymentRequest, notifyURL, returnURL string) (*provider.CreatePaymentResponse, error) {
	param := alipay.TradeWapPay{}
	param.OutTradeNo = req.OrderID
	param.TotalAmount = req.Amount
	param.Subject = req.Subject
	param.ProductCode = alipayProductCodeWapPay
	param.NotifyURL = notifyURL
	param.ReturnURL = returnURL

	payURL, err := alipayTradeWapPay(client, param)
	if err != nil {
		return nil, fmt.Errorf("alipay TradeWapPay: %w", err)
	}

	return &provider.CreatePaymentResponse{
		TradeNo: req.OrderID,
		PayURL:  payURL.String(),
	}, nil
}

func (a *AlipayProvider) createDesktopTrade(ctx context.Context, client *alipay.Client, req provider.CreatePaymentRequest, notifyURL, returnURL string) (*provider.CreatePaymentResponse, error) {
	resp, precreateErr := a.createPrecreateTrade(ctx, client, req, notifyURL)
	if precreateErr == nil {
		return resp, nil
	}

	resp, pagePayErr := a.createPagePayTrade(client, req, notifyURL, returnURL)
	if pagePayErr == nil {
		return resp, nil
	}

	return nil, fmt.Errorf("alipay desktop payment failed: precreate=%v; pagepay=%w", precreateErr, pagePayErr)
}

func (a *AlipayProvider) createPrecreateTrade(ctx context.Context, client *alipay.Client, req provider.CreatePaymentRequest, notifyURL string) (*provider.CreatePaymentResponse, error) {
	param := alipay.TradePreCreate{}
	param.OutTradeNo = req.OrderID
	param.TotalAmount = req.Amount
	param.Subject = req.Subject
	param.ProductCode = alipayProductCodePreCreate
	param.NotifyURL = notifyURL

	rsp, err := alipayTradePreCreate(ctx, client, param)
	if err != nil {
		return nil, fmt.Errorf("alipay TradePreCreate: %w", err)
	}
	if rsp == nil {
		return nil, errors.New("alipay TradePreCreate: empty response")
	}
	if rsp.IsFailure() {
		return nil, fmt.Errorf("alipay TradePreCreate failed: %s", rsp.Error.Error())
	}
	if strings.TrimSpace(rsp.QRCode) == "" {
		return nil, errors.New("alipay TradePreCreate: empty qr_code")
	}

	return &provider.CreatePaymentResponse{
		TradeNo: req.OrderID,
		QRCode:  rsp.QRCode,
	}, nil
}

func (a *AlipayProvider) createPagePayTrade(client *alipay.Client, req provider.CreatePaymentRequest, notifyURL, returnURL string) (*provider.CreatePaymentResponse, error) {
	param := alipay.TradePagePay{}
	param.OutTradeNo = req.OrderID
	param.TotalAmount = req.Amount
	param.Subject = req.Subject
	param.ProductCode = alipayProductCodePagePay
	param.NotifyURL = notifyURL
	param.ReturnURL = returnURL

	payURL, err := alipayTradePagePay(client, param)
	if err != nil {
		return nil, fmt.Errorf("alipay TradePagePay: %w", err)
	}

	return &provider.CreatePaymentResponse{
		TradeNo: req.OrderID,
		PayURL:  payURL.String(),
		QRCode:  payURL.String(),
	}, nil
}

// QueryOrder queries Alipay for a trade status and returns normalized status.
func (a *AlipayProvider) QueryOrder(ctx context.Context, tradeNo string) (*provider.QueryOrderResponse, error) {
	client, err := a.getClient()
	if err != nil {
		return nil, err
	}

	result, err := client.TradeQuery(ctx, alipay.TradeQuery{OutTradeNo: tradeNo})
	if err != nil {
		if isTradeNotExist(err) {
			return &provider.QueryOrderResponse{TradeNo: tradeNo, Status: provider.ProviderStatusPending}, nil
		}
		return nil, fmt.Errorf("alipay TradeQuery: %w", err)
	}

	status := provider.ProviderStatusPending
	switch result.TradeStatus {
	case alipay.TradeStatusSuccess, alipay.TradeStatusFinished:
		status = provider.ProviderStatusPaid
	case alipay.TradeStatusClosed:
		status = provider.ProviderStatusFailed
	}

	amount, err := parseAlipayAmount(result.TotalAmount, result.ReceiptAmount, result.BuyerPayAmount, result.InvoiceAmount)
	if err != nil {
		return nil, fmt.Errorf("alipay parse amount: %w", err)
	}

	return &provider.QueryOrderResponse{
		TradeNo: result.TradeNo,
		Status:  status,
		Amount:  amount,
		PaidAt:  result.SendPayDate,
	}, nil
}

// VerifyNotification decodes and verifies an Alipay async notification.
func (a *AlipayProvider) VerifyNotification(ctx context.Context, rawBody string, _ map[string]string) (*provider.PaymentNotification, error) {
	client, err := a.getClient()
	if err != nil {
		return nil, err
	}

	values, err := url.ParseQuery(rawBody)
	if err != nil {
		return nil, fmt.Errorf("alipay parse notification: %w", err)
	}

	notification, err := client.DecodeNotification(ctx, values)
	if err != nil {
		return nil, fmt.Errorf("alipay verify notification: %w", err)
	}

	status := provider.ProviderStatusFailed
	if notification.TradeStatus == alipay.TradeStatusSuccess || notification.TradeStatus == alipay.TradeStatusFinished {
		status = provider.ProviderStatusSuccess
	}

	amount, err := parseAlipayAmount(notification.TotalAmount, notification.ReceiptAmount, notification.BuyerPayAmount)
	if err != nil {
		return nil, fmt.Errorf("alipay parse notification amount: %w", err)
	}

	return &provider.PaymentNotification{
		TradeNo: notification.TradeNo,
		OrderID: notification.OutTradeNo,
		Amount:  amount,
		Status:  status,
		RawData: rawBody,
	}, nil
}

// Refund returns unsupported because refunds are outside this provider task.
func (a *AlipayProvider) Refund(ctx context.Context, req provider.RefundRequest) (*provider.RefundResponse, error) {
	return nil, errors.New("alipay refund is unsupported")
}

// CancelPayment closes a pending trade on Alipay.
func (a *AlipayProvider) CancelPayment(ctx context.Context, tradeNo string) error {
	client, err := a.getClient()
	if err != nil {
		return err
	}

	_, err = client.TradeClose(ctx, alipay.TradeClose{OutTradeNo: tradeNo})
	if err != nil {
		if isTradeNotExist(err) {
			return nil
		}
		return fmt.Errorf("alipay TradeClose: %w", err)
	}
	return nil
}

func isTradeNotExist(err error) bool {
	return err != nil && strings.Contains(err.Error(), alipayErrTradeNotExist)
}

func parseAlipayAmount(values ...string) (float64, error) {
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		amount, err := strconv.ParseFloat(raw, 64)
		if err == nil {
			return amount, nil
		}
	}
	return 0, errors.New("no valid amount field")
}

var (
	_ provider.Provider           = (*AlipayProvider)(nil)
	_ provider.CancelableProvider = (*AlipayProvider)(nil)
)
