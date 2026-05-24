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

// fakeOrderAndProduct builds a minimal Order + Product for notify tests.
// Bypasses the database so the test stays a pure unit test.
func fakeOrderAndProduct() (*ent.Order, *ent.Product) {
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
	prod := &ent.Product{
		Pid:  1001,
		Pkey: "secret-key",
	}
	return ord, prod
}

func TestBuildNotifyURL_MD5(t *testing.T) {
	ord, prod := fakeOrderAndProduct()
	raw, err := BuildNotifyURL(ord, prod, "")
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
	sigParams := map[string]string{}
	for k := range params {
		if k == "sign" || k == "sign_type" {
			continue
		}
		sigParams[k] = params.Get(k)
	}
	if !VerifyMD5(sigParams, prod.Pkey, params.Get("sign")) {
		t.Errorf("notify signature does not verify with product pkey")
	}
}

func TestBuildNotifyURL_RSA(t *testing.T) {
	priv, pub := mustGenerateRSAKeyPair(t)
	ord, prod := fakeOrderAndProduct()
	ord.Version = 1
	raw, err := BuildNotifyURL(ord, prod, priv)
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
	ts, _ := strconv.ParseInt(params.Get("timestamp"), 10, 64)
	if drift := time.Now().Unix() - ts; drift < -2 || drift > 2 {
		t.Errorf("timestamp drift too large: %ds", drift)
	}
}

func TestBuildNotifyURL_OptionalFieldsOmittedWhenEmpty(t *testing.T) {
	ord, prod := fakeOrderAndProduct()
	ord.Param = ""
	ord.APITradeNo = ""
	ord.Buyer = ""
	raw, err := BuildNotifyURL(ord, prod, "")
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
	ord, prod := fakeOrderAndProduct()
	ord.NotifyURL = "https://shop.example.com/notify?fixed=1"
	raw, err := BuildNotifyURL(ord, prod, "")
	if err != nil {
		t.Fatalf("BuildNotifyURL: %v", err)
	}
	if !strings.Contains(raw, "fixed=1&") {
		t.Fatalf("existing query param dropped: %s", raw)
	}
}
