package provider

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
)

type mockProvider struct{}

func (mockProvider) Name() string { return "Mock Provider" }
func (mockProvider) ProviderKey() string { return "mock" }
func (mockProvider) SupportedTypes() []PaymentType { return []PaymentType{TypeAlipay} }
func (mockProvider) CreatePayment(context.Context, CreatePaymentRequest) (*CreatePaymentResponse, error) {
	return nil, nil
}
func (mockProvider) QueryOrder(context.Context, string) (*QueryOrderResponse, error) { return nil, nil }
func (mockProvider) VerifyNotification(context.Context, string, map[string]string) (*PaymentNotification, error) {
	return nil, nil
}
func (mockProvider) Refund(context.Context, RefundRequest) (*RefundResponse, error) { return nil, nil }

func TestRegistryRegisterGetAndList(t *testing.T) {
	key := "mock-registry-test"
	Register(key, func(instanceID string, config map[string]string) (Provider, error) {
		if instanceID != "inst_1" {
			t.Fatalf("unexpected instanceID: %s", instanceID)
		}
		if config["app_id"] != "app_1" {
			t.Fatalf("unexpected config: %#v", config)
		}
		return mockProvider{}, nil
	})

	factory, ok := Get(key)
	if !ok {
		t.Fatalf("expected factory for key %q", key)
	}

	provider, err := factory("inst_1", map[string]string{"app_id": "app_1"})
	if err != nil {
		t.Fatalf("factory returned error: %v", err)
	}
	if provider.ProviderKey() != "mock" {
		t.Fatalf("unexpected provider key: %s", provider.ProviderKey())
	}

	if !slices.Contains(List(), key) {
		t.Fatalf("registered key %q not found in List()", key)
	}
}

func TestCreatePaymentResponseJSONTagsMatchEasyPay(t *testing.T) {
	payload, err := json.Marshal(CreatePaymentResponse{
		TradeNo:    "T202605140001",
		PayURL:     "https://pay.example.com/pay/T202605140001",
		QRCode:     "https://qr.example.com/T202605140001",
		ResultType: "qrcode",
	})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	want := map[string]string{
		"trade_no":    "T202605140001",
		"payurl":      "https://pay.example.com/pay/T202605140001",
		"qrcode":      "https://qr.example.com/T202605140001",
		"result_type": "qrcode",
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("JSON field %q = %q, want %q; payload=%s", key, got[key], wantValue, string(payload))
		}
	}
}
