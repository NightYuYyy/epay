package provider

// PaymentType represents a supported upstream payment method.
type PaymentType = string

// PaymentType constants supported by the first version of epay.
const (
	TypeAlipay       PaymentType = "alipay"
	TypeWxpay        PaymentType = "wxpay"
	TypeAlipayDirect PaymentType = "alipay_direct"
	TypeWxpayDirect  PaymentType = "wxpay_direct"
)

// Order status constants shared between provider and service layers.
const (
	OrderStatusPending   = "PENDING"
	OrderStatusPaid      = "PAID"
	OrderStatusSettled   = "SETTLED"
	OrderStatusExpired   = "EXPIRED"
	OrderStatusCancelled = "CANCELLED"
)

// Provider-level status constants parsed from provider notifications.
const (
	ProviderStatusPending  = "pending"
	ProviderStatusPaid     = "paid"
	ProviderStatusSuccess  = "success"
	ProviderStatusFailed   = "failed"
	ProviderStatusRefunded = "refunded"
)

// CreatePaymentRequest holds the provider-level parameters for initiating a payment.
type CreatePaymentRequest struct {
	OrderID     string      `json:"out_trade_no"`
	Amount      string      `json:"money"`
	PaymentType PaymentType `json:"type"`
	Subject     string      `json:"name"`
	NotifyURL   string      `json:"notify_url"`
	ReturnURL   string      `json:"return_url,omitempty"`
	ClientIP    string      `json:"client_ip,omitempty"`
	IsMobile    bool        `json:"is_mobile,omitempty"`
}

// CreatePaymentResponse is returned after a provider accepts a payment request.
type CreatePaymentResponse struct {
	TradeNo    string `json:"trade_no"`
	PayURL     string `json:"payurl,omitempty"`
	QRCode     string `json:"qrcode,omitempty"`
	ResultType string `json:"result_type,omitempty"`
}

// QueryOrderResponse describes a provider order query result.
type QueryOrderResponse struct {
	TradeNo  string            `json:"trade_no"`
	Status   string            `json:"status"`
	Amount   float64           `json:"money"`
	PaidAt   string            `json:"paid_at,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PaymentNotification is the verified, normalized representation of a provider callback.
type PaymentNotification struct {
	TradeNo  string            `json:"trade_no"`
	OrderID  string            `json:"out_trade_no"`
	Amount   float64           `json:"money"`
	Status   string            `json:"status"`
	RawData  string            `json:"raw_data,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RefundRequest contains the provider-level parameters for requesting a refund.
type RefundRequest struct {
	TradeNo string `json:"trade_no"`
	OrderID string `json:"out_trade_no"`
	Amount  string `json:"money"`
	Reason  string `json:"reason,omitempty"`
}

// RefundResponse is returned after a provider accepts or rejects a refund request.
type RefundResponse struct {
	RefundID string `json:"refund_id"`
	Status   string `json:"status"`
}
