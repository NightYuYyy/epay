package easypay

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"epay/ent"
	"epay/ent/merchant"
	"epay/ent/order"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler implements the rainbow-EasyPay-compatible HTTP endpoints:
//
//   - POST /mapi.php — create order, return JSON pay info
//   - GET|POST /submit.php — same as mapi but 302-redirect to payurl
//   - GET /api.php?act=… — query / settle / order / orders
//   - POST /api.php?act=refund — refund (POST form-encoded)
//   - POST /api.php?s=class/func — RSA `s=path` API_INIT routes
type Handler struct {
	client          *ent.Client
	platformPrivKey string
	platformPubKey  string
	sysKey          string
	userRefundOn    bool
}

// HandlerOption applies optional configuration to a Handler.
type HandlerOption func(*Handler)

// WithPlatformKeys configures platform RSA keypair + system key.
func WithPlatformKeys(privKey, pubKey, sysKey string) HandlerOption {
	return func(h *Handler) {
		h.platformPrivKey = privKey
		h.platformPubKey = pubKey
		h.sysKey = sysKey
	}
}

// WithUserRefund enables/disables the merchant self-service refund endpoint.
func WithUserRefund(enabled bool) HandlerOption {
	return func(h *Handler) { h.userRefundOn = enabled }
}

// NewHandler creates a new EasyPay protocol handler.
func NewHandler(client *ent.Client, opts ...HandlerOption) *Handler {
	h := &Handler{client: client}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// easyPayResponse is the standard JSON envelope returned by mapi/api endpoints.
// Optional fields are omitted via `omitempty`.
type easyPayResponse struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg,omitempty"`
	TradeNo string `json:"trade_no,omitempty"`
	PayURL  string `json:"payurl,omitempty"`
	QRCode  string `json:"qrcode,omitempty"`
	HTML    string `json:"html,omitempty"`
	URLScheme string `json:"urlscheme,omitempty"`
}

// ----- Request parsing ----------------------------------------------------

// merchantRequest captures the full set of EasyPay create-order parameters,
// aligned with rainbow-epay's mapi.php / submit.php inputs.
type merchantRequest struct {
	Pid         string
	Type        string
	OutTradeNo  string
	NotifyURL   string
	ReturnURL   string
	Name        string
	Money       string
	ClientIP    string
	Device      string
	Method      string
	Param       string
	Sitename    string
	SubOpenID   string
	SubAppID    string
	AuthCode    string
	Timestamp   string
	Sign        string
	SignType    string

	// Raw map used for signature verification (drops empty values per protocol).
	Raw map[string]string
}

// merchantParamKeys defines all keys we accept from inbound merchant requests.
// They are also the keys included in the signature canonicalization (after
// removing sign / sign_type / empty values).
var merchantParamKeys = []string{
	"pid", "type", "out_trade_no", "notify_url", "return_url",
	"name", "money", "clientip", "device", "method",
	"param", "sitename", "sub_openid", "sub_appid", "auth_code",
	"timestamp", "sign", "sign_type",
}

// extractMerchantRequest reads parameters from both query and form
// (matches rainbow's lenient parser that accepts either $_GET or $_POST).
// Empty values are kept out of the Raw map so signature canonicalization
// behaves correctly.
func extractMerchantRequest(c *gin.Context) *merchantRequest {
	_ = c.Request.ParseForm()
	raw := make(map[string]string, len(merchantParamKeys))
	for _, k := range merchantParamKeys {
		if v := pickFirstNonEmpty(c.Request.PostForm[k], c.Request.Form[k]); v != "" {
			raw[k] = v
		}
	}
	r := &merchantRequest{
		Pid:        raw["pid"],
		Type:       raw["type"],
		OutTradeNo: raw["out_trade_no"],
		NotifyURL:  raw["notify_url"],
		ReturnURL:  raw["return_url"],
		Name:       raw["name"],
		Money:      raw["money"],
		ClientIP:   raw["clientip"],
		Device:     raw["device"],
		Method:     raw["method"],
		Param:      raw["param"],
		Sitename:   raw["sitename"],
		SubOpenID:  raw["sub_openid"],
		SubAppID:   raw["sub_appid"],
		AuthCode:   raw["auth_code"],
		Timestamp:  raw["timestamp"],
		Sign:       raw["sign"],
		SignType:   raw["sign_type"],
		Raw:        raw,
	}
	if r.Device == "" {
		r.Device = "pc"
	}
	return r
}

func pickFirstNonEmpty(slices ...[]string) string {
	for _, s := range slices {
		for _, v := range s {
			if strings.TrimSpace(v) != "" {
				return v
			}
		}
	}
	return ""
}

// ----- Common validation pipeline ------------------------------------------

// outTradeNoPattern matches the rainbow regex `/^[a-zA-Z0-9.\_\-|]+$/`.
var outTradeNoPattern = regexp.MustCompile(`^[a-zA-Z0-9._\-|]+$`)

// moneyPattern matches positive decimal numbers `/^[0-9.]+$/`.
var moneyPattern = regexp.MustCompile(`^[0-9.]+$`)

// resolveAndAuthMerchant looks up the merchant by pid and verifies the
// signature. Returns the merchant on success, or a (code, msg) error suitable
// for direct response.
func (h *Handler) resolveAndAuthMerchant(ctx context.Context, r *merchantRequest, apiInit bool) (*ent.Merchant, int, string) {
	if r.Pid == "" {
		return nil, -1, "商户ID不能为空"
	}
	pid, err := strconv.Atoi(r.Pid)
	if err != nil || pid <= 0 {
		return nil, -1, "商户ID不能为空"
	}
	m, err := h.client.Merchant.Query().Where(merchant.Pid(pid)).First(ctx)
	if err != nil {
		log.Printf("[easypay] merchant pid=%d lookup: %v", pid, err)
		return nil, -1, "商户不存在！"
	}
	if m.Status != "active" {
		return nil, -1, "商户已被封禁，无法支付！"
	}

	// RSA / MD5 sign-type policy alignment with rainbow:
	//   keytype=1 → merchant must use RSA
	//   API_INIT mode (s=path) → always force RSA + require timestamp
	if m.Keytype == 1 && !strings.EqualFold(r.SignType, SignTypeRSA) {
		return nil, -3, "该商户只能使用RSA签名类型"
	}
	if apiInit {
		if !strings.EqualFold(r.SignType, SignTypeRSA) {
			return nil, -3, "该接口只能使用RSA签名类型"
		}
		if r.Timestamp == "" {
			return nil, -3, "时间戳(timestamp)字段不能为空"
		}
		if ts, terr := strconv.ParseInt(r.Timestamp, 10, 64); terr != nil || absInt64(time.Now().Unix()-ts) > 300 {
			return nil, -3, "时间戳字段不正确，请检查服务器时间"
		}
	}

	if err := VerifyParamSign(r.Raw, m.Pkey, m.PublicKey); err != nil {
		if errors.Is(err, ErrInvalidSignType) {
			return nil, -3, "签名类型无效"
		}
		return nil, -3, err.Error()
	}
	return m, 0, ""
}

// validateCreateRequest applies the rainbow `Pay::create/submit` validation
// sequence in identical order. The first failure is returned.
func validateCreateRequest(r *merchantRequest, requireClientIP bool) (int, string) {
	if r.OutTradeNo == "" {
		return -1, "订单号(out_trade_no)不能为空"
	}
	if r.NotifyURL == "" {
		return -1, "通知地址(notify_url)不能为空"
	}
	if r.ReturnURL == "" {
		return -1, "回调地址(return_url)不能为空"
	}
	if r.Name == "" {
		return -1, "商品名称(name)不能为空"
	}
	if r.Money == "" {
		return -1, "金额(money)不能为空"
	}
	if r.Type == "" && r.Method != "scan" {
		return -1, "支付方式(type)不能为空"
	}
	if requireClientIP && r.ClientIP == "" {
		return -1, "用户IP地址(clientip)不能为空"
	}
	moneyF, err := strconv.ParseFloat(r.Money, 64)
	if err != nil || moneyF <= 0 || !moneyPattern.MatchString(r.Money) {
		return -1, "金额不合法"
	}
	if !outTradeNoPattern.MatchString(r.OutTradeNo) {
		return -1, "订单号(out_trade_no)格式不正确"
	}
	if msg := ValidateNotifyURL(r.NotifyURL); msg != "" {
		return -1, msg
	}
	if r.Method == "jsapi" && r.SubOpenID == "" {
		return -1, "jsapi支付时参数(sub_openid)不能为空"
	}
	if r.Method == "jsapi" && r.Type == "wxpay" && r.SubAppID == "" {
		return -1, "jsapi支付时参数(sub_appid)不能为空"
	}
	if r.Method == "scan" && r.AuthCode == "" {
		return -1, "付款码支付时授权码(auth_code)不能为空"
	}
	return 0, ""
}

// ----- Order create / idempotency -----------------------------------------

// findOrCreateOrder mirrors rainbow's idempotency check:
//   - Look up by out_trade_no
//   - If exists & age < 10 days & status > 0 → already paid
//   - If exists & params drift → reject
//   - Otherwise reuse existing PENDING order
//   - Else insert new order with full field set
func (h *Handler) findOrCreateOrder(ctx context.Context, m *ent.Merchant, r *merchantRequest, version int) (*ent.Order, int, string) {
	moneyF, _ := strconv.ParseFloat(r.Money, 64)
	existing, err := h.client.Order.Query().Where(order.OrderNo(r.OutTradeNo)).First(ctx)
	if err == nil {
		// Order with the same out_trade_no exists.
		if existing.MerchantID != m.ID {
			return nil, -1, "该订单号已被其他商户使用"
		}
		if time.Since(existing.CreatedAt) < 10*24*time.Hour {
			if existing.Status == order.StatusPAID || existing.Status == order.StatusSETTLED {
				return nil, -1, fmt.Sprintf("该订单(%s)已完成支付，请勿重复发起支付", r.OutTradeNo)
			}
			if !roughlyEqual(existing.Amount, moneyF) ||
				existing.Name != r.Name ||
				existing.NotifyURL != r.NotifyURL ||
				existing.ReturnURL != r.ReturnURL ||
				existing.Param != r.Param {
				return nil, -1, fmt.Sprintf("该订单(%s)支付参数有变化，请更换订单号重新发起支付", r.OutTradeNo)
			}
			return existing, 0, ""
		}
		// Old order: fall through to create a fresh trade_no with the same
		// out_trade_no will violate the unique constraint; we treat as
		// already-settled stale state.
		return nil, -1, fmt.Sprintf("该订单(%s)已过期，请更换订单号重新发起", r.OutTradeNo)
	}

	tradeNo := generateTradeNo()
	ordType, terr := mapType(r.Type)
	if terr != nil {
		return nil, -1, terr.Error()
	}
	created, cerr := h.client.Order.Create().
		SetMerchantID(m.ID).
		SetOrderNo(r.OutTradeNo).
		SetTradeNo(tradeNo).
		SetType(ordType).
		SetAmount(moneyF).
		SetNotifyURL(r.NotifyURL).
		SetReturnURL(r.ReturnURL).
		SetName(r.Name).
		SetParam(r.Param).
		SetClientip(r.ClientIP).
		SetDevice(r.Device).
		SetMethod(r.Method).
		SetSubOpenid(r.SubOpenID).
		SetSubAppid(r.SubAppID).
		SetAuthCode(r.AuthCode).
		SetVersion(version).
		Save(ctx)
	if cerr != nil {
		log.Printf("[easypay] create order pid=%d out_trade_no=%s: %v", m.Pid, r.OutTradeNo, cerr)
		return nil, -1, "创建订单失败，请返回重试！"
	}
	return created, 0, ""
}

// generateTradeNo mirrors rainbow `date("YmdHis").rand(11111,99999)` shape
// (17 digits + 5 digits) — keeps interop with merchant log greps.
func generateTradeNo() string {
	now := time.Now()
	return now.Format("20060102150405") + fmt.Sprintf("%05d", uuid.New().ID()%90000+10000)
}

func mapType(t string) (order.Type, error) {
	switch strings.ToLower(t) {
	case "alipay":
		return order.TypeAlipay, nil
	case "wxpay":
		return order.TypeWxpay, nil
	case "":
		return order.TypeAlipay, fmt.Errorf("支付方式(type)不能为空")
	default:
		return order.TypeAlipay, fmt.Errorf("不支持的支付方式: %s", t)
	}
}

func roughlyEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.005
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// ----- HTTP endpoints -----------------------------------------------------

// HandleMapi processes POST /mapi.php. Returns JSON payment info.
func (h *Handler) HandleMapi(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	r := extractMerchantRequest(c)
	m, code, msg := h.resolveAndAuthMerchant(ctx, r, false)
	if code != 0 {
		c.JSON(http.StatusOK, easyPayResponse{Code: code, Msg: msg})
		return
	}
	if code, msg := validateCreateRequest(r, false); code != 0 {
		c.JSON(http.StatusOK, easyPayResponse{Code: code, Msg: msg})
		return
	}
	ord, code, msg := h.findOrCreateOrder(ctx, m, r, 0)
	if code != 0 {
		c.JSON(http.StatusOK, easyPayResponse{Code: code, Msg: msg})
		return
	}
	payURL := buildMockPayURL(ord.OrderNo, string(ord.Type))
	c.JSON(http.StatusOK, easyPayResponse{
		Code:    1,
		TradeNo: ord.TradeNo,
		PayURL:  payURL,
		QRCode:  payURL,
	})
}

// HandleSubmit processes GET|POST /submit.php. Returns 302 redirect on success.
func (h *Handler) HandleSubmit(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	r := extractMerchantRequest(c)
	m, code, msg := h.resolveAndAuthMerchant(ctx, r, false)
	if code != 0 {
		c.JSON(http.StatusOK, easyPayResponse{Code: code, Msg: msg})
		return
	}
	if code, msg := validateCreateRequest(r, false); code != 0 {
		c.JSON(http.StatusOK, easyPayResponse{Code: code, Msg: msg})
		return
	}
	ord, code, msg := h.findOrCreateOrder(ctx, m, r, 0)
	if code != 0 {
		c.JSON(http.StatusOK, easyPayResponse{Code: code, Msg: msg})
		return
	}
	payURL := buildMockPayURL(ord.OrderNo, string(ord.Type))
	c.Redirect(http.StatusFound, payURL)
}

// buildMockPayURL is a stub kept until the real payment provider integration
// is wired up. Real implementations return alipay/wxpay redirect URLs.
func buildMockPayURL(outTradeNo, payType string) string {
	return "https://pay.example.com/" + payType + "?order=" + outTradeNo
}
