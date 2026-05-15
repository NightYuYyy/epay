package wxpay

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"epay/internal/provider"
)

func TestWxpayProviderMetadataAndRegistration(t *testing.T) {
	factory, ok := provider.Get("wxpay")
	if !ok {
		t.Fatal("wxpay provider should be registered")
	}

	privatePEM, publicPEM := generateRSAKeyPair(t)
	p, err := factory("inst_wx", validWxpayConfig(privatePEM, publicPEM))
	if err != nil {
		t.Fatalf("factory returned error: %v", err)
	}
	if p.Name() != "WeChat Pay" {
		t.Fatalf("Name() = %q", p.Name())
	}
	if p.ProviderKey() != "wxpay" {
		t.Fatalf("ProviderKey() = %q", p.ProviderKey())
	}
	gotTypes := p.SupportedTypes()
	if len(gotTypes) != 1 || gotTypes[0] != provider.TypeWxpay {
		t.Fatalf("SupportedTypes() = %#v", gotTypes)
	}
}

func TestCreatePaymentUsesNativeForDesktopAndH5ForMobile(t *testing.T) {
	privatePEM, publicPEM := generateRSAKeyPair(t)
	var seen []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") == "" || !strings.Contains(r.Header.Get("Authorization"), `mchid="mch_123"`) {
			t.Fatalf("missing or invalid Authorization header: %s", r.Header.Get("Authorization"))
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if payload["appid"] != "wx_app_123" || payload["mchid"] != "mch_123" || payload["out_trade_no"] == "" {
			t.Fatalf("unexpected prepay payload: %#v", payload)
		}
		amount := payload["amount"].(map[string]any)
		if amount["total"] != float64(1234) || amount["currency"] != "CNY" {
			t.Fatalf("unexpected amount payload: %#v", amount)
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v3/pay/transactions/native":
			_, _ = io.WriteString(w, `{"code_url":"weixin://wxpay/bizpayurl?pr=desktop","prepay_id":"wx_native_prepay"}`)
		case "/v3/pay/transactions/h5":
			_, _ = io.WriteString(w, `{"h5_url":"https://wx.tenpay.com/h5pay/prepay?token=mobile","prepay_id":"wx_h5_prepay"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p, err := NewWxpay("inst_wx", validWxpayConfig(privatePEM, publicPEM))
	if err != nil {
		t.Fatalf("NewWxpay: %v", err)
	}
	p.apiBaseURL = server.URL

	desktop, err := p.CreatePayment(context.Background(), provider.CreatePaymentRequest{
		OrderID:     "order_native",
		Amount:      "12.34",
		PaymentType: provider.TypeWxpay,
		Subject:     "Desktop order",
	})
	if err != nil {
		t.Fatalf("CreatePayment desktop: %v", err)
	}
	if desktop.TradeNo != "wx_native_prepay" || desktop.QRCode != "weixin://wxpay/bizpayurl?pr=desktop" || desktop.PayURL != "" {
		t.Fatalf("desktop response = %#v", desktop)
	}

	mobile, err := p.CreatePayment(context.Background(), provider.CreatePaymentRequest{
		OrderID:     "order_h5",
		Amount:      "12.34",
		PaymentType: provider.TypeWxpay,
		Subject:     "Mobile order",
		ClientIP:    "203.0.113.8",
		IsMobile:    true,
	})
	if err != nil {
		t.Fatalf("CreatePayment mobile: %v", err)
	}
	if mobile.TradeNo != "wx_h5_prepay" || mobile.PayURL != "https://wx.tenpay.com/h5pay/prepay?token=mobile" || mobile.QRCode != "" {
		t.Fatalf("mobile response = %#v", mobile)
	}
	if strings.Join(seen, ";") != "POST /v3/pay/transactions/native;POST /v3/pay/transactions/h5" {
		t.Fatalf("unexpected endpoints: %#v", seen)
	}
}

func TestQueryOrderMapsWeChatTradeState(t *testing.T) {
	privatePEM, publicPEM := generateRSAKeyPair(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v3/pay/transactions/out-trade-no/order_query" {
			t.Fatalf("unexpected query request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("mchid") != "mch_123" {
			t.Fatalf("mchid query = %q", r.URL.Query().Get("mchid"))
		}
		_, _ = io.WriteString(w, `{"transaction_id":"wx_tx_1","out_trade_no":"order_query","trade_state":"SUCCESS","success_time":"2026-05-14T12:00:00+08:00","amount":{"total":2500,"currency":"CNY"}}`)
	}))
	defer server.Close()

	p, err := NewWxpay("inst_wx", validWxpayConfig(privatePEM, publicPEM))
	if err != nil {
		t.Fatalf("NewWxpay: %v", err)
	}
	p.apiBaseURL = server.URL

	got, err := p.QueryOrder(context.Background(), "order_query")
	if err != nil {
		t.Fatalf("QueryOrder: %v", err)
	}
	if got.TradeNo != "wx_tx_1" || got.Status != provider.ProviderStatusPaid || got.Amount != 25 || got.PaidAt == "" {
		t.Fatalf("query response = %#v", got)
	}
	if got.Metadata["trade_state"] != "SUCCESS" || got.Metadata["out_trade_no"] != "order_query" {
		t.Fatalf("query metadata = %#v", got.Metadata)
	}
}

func TestVerifyNotificationVerifiesSignatureAndDecryptsResource(t *testing.T) {
	privatePEM, publicPEM, privateKey := generateRSAKeyPairWithKey(t)
	p, err := NewWxpay("inst_wx", validWxpayConfig(privatePEM, publicPEM))
	if err != nil {
		t.Fatalf("NewWxpay: %v", err)
	}

	plaintext := `{"transaction_id":"wx_tx_notify","out_trade_no":"order_notify","trade_state":"SUCCESS","amount":{"total":990,"currency":"CNY"}}`
	body := signedEncryptedNotification(t, p.config.apiV3Key, plaintext)
	headers := signWxpayHeaders(t, privateKey, body, "1715688000", "nonce-notify")

	got, err := p.VerifyNotification(context.Background(), body, headers)
	if err != nil {
		t.Fatalf("VerifyNotification: %v", err)
	}
	if got.TradeNo != "wx_tx_notify" || got.OrderID != "order_notify" || got.Status != provider.ProviderStatusSuccess || got.Amount != 9.90 {
		t.Fatalf("notification = %#v", got)
	}
	if got.RawData != body || got.Metadata["trade_state"] != "SUCCESS" {
		t.Fatalf("notification metadata/raw = %#v raw=%q", got.Metadata, got.RawData)
	}

	headers["Wechatpay-Signature"] = "tampered"
	if _, err := p.VerifyNotification(context.Background(), body, headers); err == nil {
		t.Fatal("VerifyNotification should reject tampered signature")
	}
}

func validWxpayConfig(privatePEM, publicPEM string) map[string]string {
	return map[string]string{
		"appId":       "wx_app_123",
		"mchId":       "mch_123",
		"privateKey":  privatePEM,
		"apiV3Key":    "12345678901234567890123456789012",
		"publicKey":   publicPEM,
		"publicKeyId": "PUB_KEY_ID_123",
		"serialNo":    "SERIAL_123",
		"notifyUrl":   "https://merchant.example.com/wxpay/notify",
	}
}

func generateRSAKeyPair(t *testing.T) (string, string) {
	privatePEM, publicPEM, _ := generateRSAKeyPairWithKey(t)
	return privatePEM, publicPEM
}

func generateRSAKeyPairWithKey(t *testing.T) (string, string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})), key
}

func signedEncryptedNotification(t *testing.T, apiV3Key string, plaintext string) string {
	t.Helper()
	nonce := []byte("gcmnonce1234")
	additionalData := []byte("transaction")
	block, err := aes.NewCipher([]byte(apiV3Key))
	if err != nil {
		t.Fatalf("aes cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("gcm: %v", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), additionalData)
	body := map[string]any{
		"id":            "evt_1",
		"create_time":   "2026-05-14T12:00:00+08:00",
		"event_type":    "TRANSACTION.SUCCESS",
		"resource_type": "encrypt-resource",
		"resource": map[string]string{
			"algorithm":       "AEAD_AES_256_GCM",
			"ciphertext":      base64.StdEncoding.EncodeToString(ciphertext),
			"nonce":           string(nonce),
			"associated_data": string(additionalData),
		},
		"summary": "success",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return string(raw)
}

func signWxpayHeaders(t *testing.T, key *rsa.PrivateKey, body, timestamp, nonce string) map[string]string {
	t.Helper()
	message := fmt.Sprintf("%s\n%s\n%s\n", timestamp, nonce, body)
	digest := sha256.Sum256([]byte(message))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, cryptoHashSHA256, digest[:])
	if err != nil {
		t.Fatalf("sign notification: %v", err)
	}
	return map[string]string{
		"Wechatpay-Timestamp": timestamp,
		"Wechatpay-Nonce":     nonce,
		"Wechatpay-Signature": base64.StdEncoding.EncodeToString(sig),
		"Wechatpay-Serial":    "PUB_KEY_ID_123",
	}
}

const cryptoHashSHA256 = 5 // crypto.SHA256 without importing crypto only for this constant.
