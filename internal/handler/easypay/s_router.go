package easypay

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"epay/ent"
	"epay/ent/order"

	"github.com/gin-gonic/gin"
)

// sRouter implements rainbow-epay's `ApiHelper::load_api` semantics for the
// API_INIT mode: POST /api.php?s=<class>/<func>.
//
// All endpoints under this router enforce:
//   - sign_type == "RSA"
//   - timestamp present and within ±300s of server time
//   - signature verifies against the merchant's stored public_key
//
// Successful responses are wrapped with `{code, ..., timestamp, sign_type,
// sign}` where `sign` is RSA-signed by the platform private key.
func (h *Handler) sRouter(c *gin.Context, s string) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		h.sError(c, -5, "URL Error!")
		return
	}
	switch parts[0] + "/" + parts[1] {
	case "pay/create":
		h.sPayCreate(c)
	case "pay/query":
		h.sPayQuery(c)
	case "pay/refund":
		h.actRefund(c)
	case "pay/refundquery":
		h.actRefundQuery(c)
	default:
		h.sError(c, -5, "接口方法不存在")
	}
}

// sError returns an unsigned error envelope.
func (h *Handler) sError(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg})
}

// sSuccess returns an RSA-signed success envelope. Adds timestamp + sign_type
// + sign computed over all other fields with the platform private key.
func (h *Handler) sSuccess(c *gin.Context, payload gin.H) {
	if payload == nil {
		payload = gin.H{}
	}
	payload["timestamp"] = strconv.FormatInt(time.Now().Unix(), 10)
	payload["sign_type"] = SignTypeRSA
	signParams := make(map[string]string, len(payload))
	for k, v := range payload {
		signParams[k] = anyToString(v)
	}
	if h.platformPrivKey != "" {
		if sig, err := SignRSA(signParams, h.platformPrivKey); err == nil {
			payload["sign"] = sig
		}
	}
	c.JSON(http.StatusOK, payload)
}

func anyToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

// sPayCreate — RSA-signed equivalent of /mapi.php.
func (h *Handler) sPayCreate(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	r := extractMerchantRequest(c)
	m, code, msg := h.resolveAndAuthMerchant(ctx, r, true)
	if code != 0 {
		h.sError(c, code, msg)
		return
	}
	if code, msg := validateCreateRequest(r, true); code != 0 {
		h.sError(c, code, msg)
		return
	}
	if h.paymentCreator != nil {
		resp, code, msg := h.createRealPayment(ctx, m, r, 1)
		if code != 0 {
			h.sError(c, code, msg)
			return
		}
		h.sSuccess(c, gin.H{
			"code":     0,
			"trade_no": resp.TradeNo,
			"pay_type": "jump",
			"pay_info": firstNonEmptyString(resp.PayURL, resp.QRCode),
			"qrcode":   firstNonEmptyString(resp.QRCode, resp.PayURL),
		})
		return
	}
	ord, code, msg := h.findOrCreateOrder(ctx, m, r, 1)
	if code != 0 {
		h.sError(c, code, msg)
		return
	}
	payURL := buildMockPayURL(ord.OrderNo, string(ord.Type))
	h.sSuccess(c, gin.H{
		"code":     0,
		"trade_no": ord.TradeNo,
		"pay_type": "jump",
		"pay_info": payURL,
	})
}

// sPayQuery — RSA-signed equivalent of act=order, scoped to the authenticated
// merchant (no cross-merchant SYS_KEY mode here — that's reserved for the
// platform's GET act=order endpoint).
func (h *Handler) sPayQuery(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	r := extractMerchantRequest(c)
	m, code, msg := h.resolveAndAuthMerchant(ctx, r, true)
	if code != 0 {
		h.sError(c, code, msg)
		return
	}
	outTradeNo := r.OutTradeNo
	tradeNo := r.Raw["trade_no"]
	if outTradeNo == "" && tradeNo == "" {
		h.sError(c, -4, "订单号不能为空")
		return
	}
	var ord *ent.Order
	var err error
	if outTradeNo != "" {
		ord, err = h.client.Order.Query().
			Where(order.ProductID(m.ID), order.OrderNo(outTradeNo)).First(ctx)
	} else {
		ord, err = h.client.Order.Query().
			Where(order.ProductID(m.ID), order.TradeNo(tradeNo)).First(ctx)
	}
	if err != nil || ord == nil {
		h.sError(c, -1, "订单号不存在")
		return
	}
	payload := h.orderToFullResponse(ctx, ord)
	payload["code"] = 0
	delete(payload, "msg")
	h.sSuccess(c, payload)
}
