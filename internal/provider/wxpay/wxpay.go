package wxpay

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"epay/internal/provider"
)

const (
	wxpayProviderKey  = "wxpay"
	wxpayProviderName = "WeChat Pay"
	wxpayCurrency     = "CNY"
	wxpayNativePath   = "/v3/pay/transactions/native"
	wxpayH5Path       = "/v3/pay/transactions/h5"
	wxpayQueryPrefix  = "/v3/pay/transactions/out-trade-no/"
	wxpayNotifyEvent  = "TRANSACTION.SUCCESS"
	wxpaySuccess      = "SUCCESS"
	wxpayNotPay       = "NOTPAY"
	wxpayAccept       = "ACCEPT"
	wxpayClosed       = "CLOSED"
	wxpayPayError     = "PAYERROR"
	wxpayGCMTagSize   = 16
)

type wxpayConfig struct {
	appId       string
	mchId       string
	privateKey  string
	apiV3Key    string
	publicKey   string
	publicKeyId string
	serialNo    string
	notifyUrl   string
}

type WxpayProvider struct {
	instanceID string
	config     wxpayConfig
	httpClient *http.Client
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	apiBaseURL string
	mu         sync.Mutex
}

func init() {
	provider.Register(wxpayProviderKey, func(instanceID string, config map[string]string) (provider.Provider, error) {
		return NewWxpay(instanceID, config)
	})
}

func NewWxpay(instanceID string, config map[string]string) (*WxpayProvider, error) {
	cfg := wxpayConfig{
		appId:       strings.TrimSpace(config["appId"]),
		mchId:       strings.TrimSpace(config["mchId"]),
		privateKey:  strings.TrimSpace(config["privateKey"]),
		apiV3Key:    strings.TrimSpace(config["apiV3Key"]),
		publicKey:   strings.TrimSpace(config["publicKey"]),
		publicKeyId: strings.TrimSpace(config["publicKeyId"]),
		serialNo:    strings.TrimSpace(config["serialNo"]),
		notifyUrl:   strings.TrimSpace(config["notifyUrl"]),
	}
	for _, field := range []struct{ name, value string }{
		{"appId", cfg.appId}, {"mchId", cfg.mchId}, {"privateKey", cfg.privateKey}, {"apiV3Key", cfg.apiV3Key},
		{"publicKey", cfg.publicKey}, {"publicKeyId", cfg.publicKeyId}, {"serialNo", cfg.serialNo}, {"notifyUrl", cfg.notifyUrl},
	} {
		if field.value == "" {
			return nil, fmt.Errorf("wxpay config %s is required", field.name)
		}
	}
	if len(cfg.apiV3Key) != 32 {
		return nil, fmt.Errorf("wxpay apiV3Key must be 32 bytes")
	}
	privateKey, err := parsePrivateKey(cfg.privateKey)
	if err != nil {
		return nil, fmt.Errorf("wxpay privateKey: %w", err)
	}
	publicKey, err := parsePublicKey(cfg.publicKey)
	if err != nil {
		return nil, fmt.Errorf("wxpay publicKey: %w", err)
	}
	return &WxpayProvider{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		privateKey: privateKey,
		publicKey:  publicKey,
		apiBaseURL: "https://api.mch.weixin.qq.com",
	}, nil
}

func (p *WxpayProvider) Name() string { return wxpayProviderName }

func (p *WxpayProvider) ProviderKey() string { return wxpayProviderKey }

func (p *WxpayProvider) SupportedTypes() []provider.PaymentType {
	return []provider.PaymentType{provider.TypeWxpay}
}

func (p *WxpayProvider) CreatePayment(ctx context.Context, req provider.CreatePaymentRequest) (*provider.CreatePaymentResponse, error) {
	if req.OrderID == "" {
		return nil, fmt.Errorf("out_trade_no is required")
	}
	if req.Amount == "" {
		return nil, fmt.Errorf("amount is required")
	}
	tradeURL := p.endpointForRequest(req)
	amountFen, err := yuanToFen(req.Amount)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"appid":        p.config.appId,
		"mchid":        p.config.mchId,
		"out_trade_no": req.OrderID,
		"description":  req.Subject,
		"notify_url":   firstNonEmpty(req.NotifyURL, p.config.notifyUrl),
		"amount": map[string]any{
			"total":    amountFen,
			"currency": wxpayCurrency,
		},
	}
	if req.IsMobile {
		body["scene_info"] = map[string]any{
			"payer_client_ip": req.ClientIP,
			"h5_info": map[string]any{
				"type": "Wap",
			},
		}
		if req.ReturnURL != "" {
			body["scene_info"].(map[string]any)["h5_info"].(map[string]any)["app_url"] = req.ReturnURL
		}
	}
	if req.IsMobile {
		body["scene_info"] = map[string]any{
			"payer_client_ip": req.ClientIP,
			"h5_info": map[string]any{
				"type":     "Wap",
				"app_name": "",
				"app_url":  "",
			},
		}
	}
	if req.IsMobile {
		scene := body["scene_info"].(map[string]any)
		h5info := scene["h5_info"].(map[string]any)
		if req.ReturnURL != "" {
			h5info["app_url"] = req.ReturnURL
		}
		if req.Subject != "" {
			h5info["app_name"] = req.Subject
		}
	}
	if req.IsMobile {
		// H5 route
		h5Resp := struct {
			H5URL    string `json:"h5_url"`
			PrepayID string `json:"prepay_id"`
		}{}
		if err := p.doRequest(ctx, http.MethodPost, tradeURL, body, &h5Resp); err != nil {
			if isNoAuth(err) {
				return nil, err
			}
			return nil, err
		}
		return &provider.CreatePaymentResponse{TradeNo: h5Resp.PrepayID, PayURL: h5Resp.H5URL}, nil
	}
	resp := struct {
		CodeURL  string `json:"code_url"`
		PrepayID string `json:"prepay_id"`
	}{}
	if err := p.doRequest(ctx, http.MethodPost, tradeURL, body, &resp); err != nil {
		return nil, err
	}
	return &provider.CreatePaymentResponse{TradeNo: resp.PrepayID, QRCode: resp.CodeURL}, nil
}

func (p *WxpayProvider) QueryOrder(ctx context.Context, tradeNo string) (*provider.QueryOrderResponse, error) {
	if tradeNo == "" {
		return nil, fmt.Errorf("tradeNo is required")
	}
	path := p.apiBaseURL + wxpayQueryPrefix + url.PathEscape(tradeNo) + "?mchid=" + url.QueryEscape(p.config.mchId)
	var resp struct {
		TransactionID string `json:"transaction_id"`
		OutTradeNo    string `json:"out_trade_no"`
		TradeState    string `json:"trade_state"`
		SuccessTime   string `json:"success_time"`
		Amount        struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
	}
	if err := p.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	status := mapTradeState(resp.TradeState)
	metadata := map[string]string{}
	if resp.OutTradeNo != "" {
		metadata["out_trade_no"] = resp.OutTradeNo
	}
	if resp.TradeState != "" {
		metadata["trade_state"] = resp.TradeState
	}
	if resp.Amount.Currency != "" {
		metadata["currency"] = resp.Amount.Currency
	}
	return &provider.QueryOrderResponse{
		TradeNo:  firstNonEmpty(resp.TransactionID, tradeNo),
		Status:   status,
		Amount:   fenToYuan(resp.Amount.Total),
		PaidAt:   resp.SuccessTime,
		Metadata: metadata,
	}, nil
}

func (p *WxpayProvider) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*provider.PaymentNotification, error) {
	msg, err := buildNotificationMessage(headers, rawBody)
	if err != nil {
		return nil, err
	}
	if err := verifyNotificationSignature(p.publicKey, headers, msg); err != nil {
		return nil, err
	}
	var envelope struct {
		Resource struct {
			Algorithm      string `json:"algorithm"`
			Ciphertext     string `json:"ciphertext"`
			Nonce          string `json:"nonce"`
			AssociatedData string `json:"associated_data"`
		} `json:"resource"`
	}
	if err := json.Unmarshal([]byte(rawBody), &envelope); err != nil {
		return nil, fmt.Errorf("wxpay parse notification: %w", err)
	}
	plain, err := decryptResource(p.config.apiV3Key, envelope.Resource)
	if err != nil {
		return nil, err
	}
	var tx struct {
		TransactionID string `json:"transaction_id"`
		OutTradeNo    string `json:"out_trade_no"`
		TradeState    string `json:"trade_state"`
		Amount        struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
	}
	if err := json.Unmarshal(plain, &tx); err != nil {
		return nil, fmt.Errorf("wxpay decode transaction: %w", err)
	}
	status := provider.ProviderStatusFailed
	if tx.TradeState == wxpaySuccess {
		status = provider.ProviderStatusSuccess
	}
	return &provider.PaymentNotification{
		TradeNo:  tx.TransactionID,
		OrderID:  tx.OutTradeNo,
		Amount:   fenToYuan(tx.Amount.Total),
		Status:   status,
		RawData:  rawBody,
		Metadata: map[string]string{"trade_state": tx.TradeState, "currency": tx.Amount.Currency},
	}, nil
}

func (p *WxpayProvider) Refund(context.Context, provider.RefundRequest) (*provider.RefundResponse, error) {
	return nil, errors.New("refund not implemented")
}

func (p *WxpayProvider) authHeader(method, path string, body []byte) (string, string, string, string, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomString(16)
	message := method + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + string(body) + "\n"
	h := sha256.Sum256([]byte(message))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.privateKey, crypto.SHA256, h[:])
	if err != nil {
		return "", "", "", "", err
	}
	auth := fmt.Sprintf(`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",timestamp="%s",serial_no="%s",signature="%s"`,
		p.config.mchId, nonce, timestamp, p.config.serialNo, base64.StdEncoding.EncodeToString(sig))
	return auth, timestamp, nonce, string(sig), nil
}

func (p *WxpayProvider) doRequest(ctx context.Context, method, fullURL string, payload any, out any) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	parsed, err := url.Parse(fullURL)
	if err != nil {
		return err
	}
	path := parsed.Path
	if parsed.RawQuery != "" {
		path += "?" + parsed.RawQuery
	}
	auth, _, _, _, err := p.authHeader(method, path, body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("wxpay request failed: %s", string(respBody))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return err
	}
	return nil
}

func (p *WxpayProvider) endpointForRequest(req provider.CreatePaymentRequest) string {
	if req.IsMobile {
		return p.apiBaseURL + wxpayH5Path
	}
	return p.apiBaseURL + wxpayNativePath
}

func mapTradeState(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case wxpaySuccess:
		return provider.ProviderStatusPaid
	case wxpayNotPay, wxpayAccept, "":
		return provider.ProviderStatusPending
	case wxpayClosed, wxpayPayError:
		return provider.ProviderStatusFailed
	default:
		return provider.ProviderStatusPending
	}
}

func parsePrivateKey(pemText string) (*rsa.PrivateKey, error) {
	blk, _ := pem.Decode([]byte(pemText))
	if blk == nil {
		return nil, fmt.Errorf("invalid PEM")
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		keyAny, err = x509.ParsePKCS1PrivateKey(blk.Bytes)
		if err != nil {
			return nil, err
		}
	}
	key, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not RSA private key")
	}
	return key, nil
}

func parsePublicKey(pemText string) (*rsa.PublicKey, error) {
	blk, _ := pem.Decode([]byte(pemText))
	if blk == nil {
		return nil, fmt.Errorf("invalid PEM")
	}
	keyAny, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := keyAny.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not RSA public key")
	}
	return key, nil
}

func verifyNotificationSignature(pub *rsa.PublicKey, headers map[string]string, msg string) error {
	timestamp := firstHeader(headers, "Wechatpay-Timestamp", "Timestamp")
	nonce := firstHeader(headers, "Wechatpay-Nonce", "Nonce")
	signature := firstHeader(headers, "Wechatpay-Signature", "Signature")
	if timestamp == "" || nonce == "" || signature == "" {
		return fmt.Errorf("missing notification headers")
	}
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("decode notification signature: %w", err)
	}
	h := sha256.Sum256([]byte(msg))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], sigBytes); err != nil {
		return fmt.Errorf("wxpay notification signature invalid: %w", err)
	}
	return nil
}

func buildNotificationMessage(headers map[string]string, body string) (string, error) {
	timestamp := firstHeader(headers, "Wechatpay-Timestamp", "Timestamp")
	nonce := firstHeader(headers, "Wechatpay-Nonce", "Nonce")
	if timestamp == "" || nonce == "" {
		return "", fmt.Errorf("missing notification headers")
	}
	return timestamp + "\n" + nonce + "\n" + body + "\n", nil
}

func decryptResource(apiV3Key string, resource struct {
	Algorithm      string `json:"algorithm"`
	Ciphertext     string `json:"ciphertext"`
	Nonce          string `json:"nonce"`
	AssociatedData string `json:"associated_data"`
}) ([]byte, error) {
	if strings.TrimSpace(resource.Algorithm) != "AEAD_AES_256_GCM" {
		return nil, fmt.Errorf("unsupported algorithm %s", resource.Algorithm)
	}
	block, err := aes.NewCipher([]byte(apiV3Key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(resource.Ciphertext)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, []byte(resource.Nonce), ciphertext, []byte(resource.AssociatedData))
	if err != nil {
		return nil, err
	}
	return plain, nil
}

func yuanToFen(amount string) (int64, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return 0, fmt.Errorf("amount is required")
	}
	parts := strings.SplitN(amount, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	fen := whole * 100
	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) > 2 {
			frac = frac[:2]
		}
		for len(frac) < 2 {
			frac += "0"
		}
		f, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, err
		}
		fen += f
	}
	return fen, nil
}

func fenToYuan(fen int64) float64 { return float64(fen) / 100 }

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	for i := range buf {
		buf[i] = letters[int(buf[i])%len(letters)]
	}
	return string(buf)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstHeader(headers map[string]string, names ...string) string {
	for _, name := range names {
		for k, v := range headers {
			if strings.EqualFold(k, name) {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func isNoAuth(err error) bool { return err != nil && strings.Contains(err.Error(), "NO_AUTH") }

func (p *WxpayProvider) ensureSortedHeaders(headers map[string]string) []string {
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (p *WxpayProvider) buildPathForURL(fullURL string) (string, error) {
	u, err := url.Parse(fullURL)
	if err != nil {
		return "", err
	}
	path := u.Path
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return path, nil
}

var _ provider.Provider = (*WxpayProvider)(nil)
