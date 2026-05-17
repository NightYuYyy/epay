package easypay

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"epay/ent"
	"epay/ent/order"
)

// fakeOrderAndMerchant builds a minimal Order + Merchant for notify tests.
// Bypasses the database so the test stays a pure unit test.
func fakeOrderAndMerchant() (*ent.Order, *ent.Merchant) {
	ord := &ent.Order{
		OrderNo:   "20250101001",
		TradeNo:   "TR000001",
		Type:      order.TypeAlipay,
		Amount:    100.00,
		Name:      "hello",
		Param:     "biz=xyz",
		NotifyURL: "https://shop.example.com/notify",
		Version:   0,
	}
	merch := &ent.Merchant{
		Pid:  1001,
		Pkey: "secret-key",
	}
	return ord, merch
}

func TestBuildNotifyURL_MD5(t *testing.T) {
	ord, merch := fakeOrderAndMerchant()
	raw, err := BuildNotifyURL(ord, merch, "")
	if err != nil {
		t.Fatalf("BuildNotifyURL: %v", err)
	}
	if !strings.HasPrefix(raw, "https://shop.example.com/notify?") {
		t.Fatalf("notify_url not preserved: %s", raw)
	}
	q, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	params := q.Query()
	// Required field set
	for _, k := range []string{"pid", "trade_no", "out_trade_no", "type",
		"name", "money", "trade_status", "sign", "sign_type"} {
		if params.Get(k) == "" {
			t.Errorf("missing required field %s in: %s", k, raw)
		}
	}
	if params.Get("sign_type") != "MD5" {
		t.Errorf("expected sign_type=MD5, got %s", params.Get("sign_type"))
	}
	if params.Get("trade_status") != "TRADE_SUCCESS" {
		t.Errorf("expected trade_status=TRADE_SUCCESS, got %s", params.Get("trade_status"))
	}
	if params.Get("param") != "biz=xyz" {
		t.Errorf("expected param=biz=xyz, got %s", params.Get("param"))
	}
	// Recompute the signature without using the helper to ensure the algorithm
	// path is invariant to URL encoding (rainbow's verifier uses the decoded
	// values from $_GET).
	sigParams := map[string]string{}
	for k := range params {
		if k == "sign" || k == "sign_type" {
			continue
		}
		sigParams[k] = params.Get(k)
	}
	if !VerifyMD5(sigParams, merch.Pkey, params.Get("sign")) {
		t.Errorf("notify signature does not verify with merchant pkey")
	}
}

func TestBuildNotifyURL_RSA(t *testing.T) {
	priv, pub := mustGenerateRSAKeyPair(t)
	ord, merch := fakeOrderAndMerchant()
	ord.Version = 1
	raw, err := BuildNotifyURL(ord, merch, priv)
	if err != nil {
		t.Fatalf("BuildNotifyURL: %v", err)
	}
	q, _ := url.Parse(raw)
	params := q.Query()
	if params.Get("sign_type") != "RSA" {
		t.Fatalf("expected sign_type=RSA, got %s", params.Get("sign_type"))
	}
	if params.Get("timestamp") == "" {
		t.Fatal("RSA notify must include timestamp")
	}
	// Verify with the published platform public key
	sigParams := map[string]string{}
	for k := range params {
		if k == "sign" || k == "sign_type" {
			continue
		}
		sigParams[k] = params.Get(k)
	}
	if err := VerifyRSA(sigParams, pub, params.Get("sign")); err != nil {
		t.Errorf("RSA notify signature did not verify: %v", err)
	}
	// Timestamp must be within 5 seconds of "now"
	ts, _ := strconv.ParseInt(params.Get("timestamp"), 10, 64)
	if drift := time.Now().Unix() - ts; drift < -2 || drift > 2 {
		t.Errorf("timestamp drift too large: %ds", drift)
	}
}

func TestBuildNotifyURL_OptionalFieldsOmittedWhenEmpty(t *testing.T) {
	ord, merch := fakeOrderAndMerchant()
	ord.Param = ""
	ord.APITradeNo = ""
	ord.Buyer = ""
	raw, err := BuildNotifyURL(ord, merch, "")
	if err != nil {
		t.Fatalf("BuildNotifyURL: %v", err)
	}
	q, _ := url.Parse(raw)
	params := q.Query()
	for _, k := range []string{"param", "api_trade_no", "buyer"} {
		if _, exists := params[k]; exists {
			t.Errorf("optional field %s must be absent when empty, got: %s", k, raw)
		}
	}
}

func TestBuildNotifyURL_AppendsToExistingQueryString(t *testing.T) {
	ord, merch := fakeOrderAndMerchant()
	ord.NotifyURL = "https://shop.example.com/notify?fixed=1"
	raw, err := BuildNotifyURL(ord, merch, "")
	if err != nil {
		t.Fatalf("BuildNotifyURL: %v", err)
	}
	if !strings.Contains(raw, "fixed=1&") {
		t.Fatalf("existing query param dropped: %s", raw)
	}
}
