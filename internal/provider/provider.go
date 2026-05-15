package provider

import "context"

// Provider defines the behavior all payment providers must implement.
type Provider interface {
	// Name returns a human-readable provider name.
	Name() string
	// ProviderKey returns the unique key for the provider implementation.
	ProviderKey() string
	// SupportedTypes returns the payment types handled by this provider.
	SupportedTypes() []PaymentType
	// CreatePayment initiates a payment and returns normalized provider output.
	CreatePayment(ctx context.Context, req CreatePaymentRequest) (*CreatePaymentResponse, error)
	// QueryOrder queries the provider for a payment's current status.
	QueryOrder(ctx context.Context, tradeNo string) (*QueryOrderResponse, error)
	// VerifyNotification verifies and parses a provider notification body.
	VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*PaymentNotification, error)
	// Refund requests a refund from the provider. Concrete providers may return an
	// unsupported error in v1 if upstream refund is not enabled.
	Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error)
}

// CancelableProvider extends Provider with upstream payment cancellation.
type CancelableProvider interface {
	Provider
	CancelPayment(ctx context.Context, tradeNo string) error
}
