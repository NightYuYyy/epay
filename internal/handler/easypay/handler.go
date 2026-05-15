package easypay

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"epay/ent"
	"epay/ent/merchant"
	"epay/ent/order"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler implements the server-side EasyPay protocol endpoints.
// Merchants call these endpoints to interact with the payment gateway.
type Handler struct {
	client *ent.Client
}

// NewHandler creates a new EasyPay protocol handler.
func NewHandler(client *ent.Client) *Handler {
	return &Handler{client: client}
}

// easyPayResponse is the standard JSON response for EasyPay endpoints.
type easyPayResponse struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg,omitempty"`
	TradeNo string `json:"trade_no,omitempty"`
	PayURL  string `json:"payurl,omitempty"`
	QRCode  string `json:"qrcode,omitempty"`
	Status  int    `json:"status,omitempty"`
	Money   string `json:"money,omitempty"`
}

// HandleMapi handles POST /mapi.php — create a payment order.
//
// Parameters (form-encoded):
//
//	pid          — merchant ID (int)
//	type         — payment type: alipay | wxpay
//	out_trade_no — merchant order number (unique)
//	notify_url   — callback URL (no query params, no private IPs)
//	return_url   — return URL after payment
//	name         — order subject/description
//	money        — order amount (string, e.g. "100.00")
//	clientip     — client IP address (optional)
//	sign         — MD5 signature
//	sign_type    — signature type (always "MD5")
//	device       — device type (optional, "mobile")
//	cid          — custom channel ID (optional)
func (h *Handler) HandleMapi(c *gin.Context) {
	params := make(map[string]string)
	for _, key := range []string{"pid", "type", "out_trade_no", "notify_url",
		"return_url", "name", "money", "clientip", "sign", "sign_type",
		"device", "cid"} {
		if v := c.PostForm(key); v != "" {
			params[key] = v
		}
	}
	// Preserve optional fields even when empty so sign excludes them correctly.
	// PostForm returns "" for missing keys; we only add non-empty values above
	// to match the EasyPay convention (empty values are excluded from sign).

	pidStr := params["pid"]
	if pidStr == "" {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "pid不能为空"})
		return
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "pid格式无效"})
		return
	}

	// Lookup merchant by pid
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	m, err := h.client.Merchant.Query().
		Where(merchant.Pid(pid)).
		First(ctx)
	if err != nil {
		log.Printf("[easypay] merchant pid=%d not found: %v", pid, err)
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "商户不存在"})
		return
	}

	if m.Status != "active" {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "商户已禁用"})
		return
	}

	// Verify signature
	sign, ok := params["sign"]
	if !ok {
		c.JSON(http.StatusOK, easyPayResponse{Code: -3, Msg: "签名错误"})
		return
	}
	if !EasyPayVerifySign(params, m.Pkey, sign) {
		c.JSON(http.StatusOK, easyPayResponse{Code: -3, Msg: "签名错误"})
		return
	}

	// Validate notify_url
	notifyURL := params["notify_url"]
	if errMsg := ValidateNotifyURL(notifyURL); errMsg != "" {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: errMsg})
		return
	}

	outTradeNo := params["out_trade_no"]
	if outTradeNo == "" {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "out_trade_no不能为空"})
		return
	}

	// Check duplicate: same out_trade_no must have consistent params
	existing, err := h.client.Order.Query().
		Where(order.OrderNo(outTradeNo)).
		First(ctx)
	if err == nil {
		// Order exists — verify params match
		amountStr := params["money"]
		payType := params["type"]
		if existing.Amount != parseFloatOrZero(amountStr) ||
			string(existing.Type) != payType {
			c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "订单已存在且参数不一致"})
			return
		}
		// Same params — return existing (idempotent)
		c.JSON(http.StatusOK, easyPayResponse{
			Code:    1,
			TradeNo: existing.TradeNo,
			PayURL:  buildMockPayURL(outTradeNo, payType),
		})
		c.Abort()
		return
	}

	// Stub: create order (real payment service integration comes in Task 9)
	tradeNo := uuid.New().String()

	// Log for debugging
	log.Printf("[easypay] creating order: pid=%d out_trade_no=%s type=%s money=%s trade_no=%s",
		pid, outTradeNo, params["type"], params["money"], tradeNo)

	// Build mock response
	payURL := buildMockPayURL(outTradeNo, params["type"])
	qrcode := payURL // In real implementation, QR code may differ from payurl

	c.JSON(http.StatusOK, easyPayResponse{
		Code:    1,
		TradeNo: tradeNo,
		PayURL:  payURL,
		QRCode:  qrcode,
	})
}

// HandleSubmit handles GET /submit.php — redirect to payment page.
//
// Same parameters as /mapi.php but via query string (GET).
// On success, performs a 302 redirect to the pay URL.
func (h *Handler) HandleSubmit(c *gin.Context) {
	params := make(map[string]string)
	for _, key := range []string{"pid", "type", "out_trade_no", "notify_url",
		"return_url", "name", "money", "clientip", "sign", "sign_type",
		"device", "cid"} {
		if v := c.Query(key); v != "" {
			params[key] = v
		}
	}

	pidStr := params["pid"]
	if pidStr == "" {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "pid不能为空"})
		return
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "pid格式无效"})
		return
	}

	// Lookup merchant
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	m, err := h.client.Merchant.Query().
		Where(merchant.Pid(pid)).
		First(ctx)
	if err != nil {
		log.Printf("[easypay] submit: merchant pid=%d not found: %v", pid, err)
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "商户不存在"})
		return
	}

	if m.Status != "active" {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "商户已禁用"})
		return
	}

	// Verify signature
	sign, ok := params["sign"]
	if !ok {
		c.JSON(http.StatusOK, easyPayResponse{Code: -3, Msg: "签名错误"})
		return
	}
	if !EasyPayVerifySign(params, m.Pkey, sign) {
		c.JSON(http.StatusOK, easyPayResponse{Code: -3, Msg: "签名错误"})
		return
	}

	// Validate notify_url (same rules as mapi.php)
	notifyURL := params["notify_url"]
	if errMsg := ValidateNotifyURL(notifyURL); errMsg != "" {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: errMsg})
		return
	}

	outTradeNo := params["out_trade_no"]
	if outTradeNo == "" {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "out_trade_no不能为空"})
		return
	}

	// Check duplicate
	existing, err := h.client.Order.Query().
		Where(order.OrderNo(outTradeNo)).
		First(ctx)
	if err == nil {
		amountStr := params["money"]
		payType := params["type"]
		if existing.Amount != parseFloatOrZero(amountStr) ||
			string(existing.Type) != payType {
			c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "订单已存在且参数不一致"})
			return
		}
		// Redirect to existing pay URL
		payURL := buildMockPayURL(outTradeNo, payType)
		c.Redirect(http.StatusFound, payURL)
		c.Abort()
		return
	}

	// Stub: create order
	tradeNo := uuid.New().String()
	log.Printf("[easypay] submit: creating order pid=%d out_trade_no=%s type=%s money=%s trade_no=%s",
		pid, outTradeNo, params["type"], params["money"], tradeNo)

	payURL := buildMockPayURL(outTradeNo, params["type"])
	c.Redirect(http.StatusFound, payURL)
}

// HandleAPI handles POST /api.php — query order by act parameter.
//
// Parameters (form-encoded):
//
//	act          — action: "order" (query order)
//	pid          — merchant ID
//	out_trade_no — merchant order number (takes priority) OR
//	trade_no     — platform trade number (fallback)
//	key          — merchant pkey (sent in plaintext per EasyPay protocol)
func (h *Handler) HandleAPI(c *gin.Context) {
	act := c.Query("act")
	if act != "order" {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "不支持的act"})
		return
	}

	pidStr := c.PostForm("pid")
	key := c.PostForm("key")
	outTradeNo := c.PostForm("out_trade_no")
	tradeNo := c.PostForm("trade_no")

	if pidStr == "" {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "pid不能为空"})
		return
	}
	if key == "" {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "key不能为空"})
		return
	}
	if outTradeNo == "" && tradeNo == "" {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "out_trade_no和trade_no不能同时为空"})
		return
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "pid格式无效"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Verify pid + key match (plaintext pkey comparison per EasyPay protocol)
	m, err := h.client.Merchant.Query().
		Where(merchant.Pid(pid)).
		First(ctx)
	if err != nil {
		log.Printf("[easypay] api query: merchant pid=%d not found: %v", pid, err)
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "商户不存在"})
		return
	}

	if m.Pkey != key {
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "key不正确"})
		return
	}

	// Query order by out_trade_no or trade_no
	var o *ent.Order
	if outTradeNo != "" {
		o, err = h.client.Order.Query().
			Where(order.OrderNo(outTradeNo)).
			First(ctx)
	} else {
		o, err = h.client.Order.Query().
			Where(order.TradeNo(tradeNo)).
			First(ctx)
	}

	if err != nil {
		log.Printf("[easypay] api query: order not found: out_trade_no=%s trade_no=%s err=%v",
			outTradeNo, tradeNo, err)
		c.JSON(http.StatusOK, easyPayResponse{Code: -1, Msg: "订单不存在"})
		return
	}

	// Map internal status to EasyPay status
	// 0 = unpaid/pending, 1 = paid
	status := 0
	switch o.Status {
	case "PAID", "SETTLED":
		status = 1
	}

	c.JSON(http.StatusOK, easyPayResponse{
		Code:   1,
		Status: status,
		Money:  formatMoney(o.Amount),
	})
}

// buildMockPayURL generates a mock payment URL for stub responses.
// This will be replaced with real payment provider URLs in Task 9.
func buildMockPayURL(outTradeNo, payType string) string {
	return "https://pay.example.com/" + payType + "?order=" + outTradeNo
}

// parseFloatOrZero parses a string to float64, returning 0 on error.
func parseFloatOrZero(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// formatMoney formats a float64 amount as a string with 2 decimal places
// matching the EasyPay protocol format.
func formatMoney(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}
