package easypay

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"epay/ent"
	"epay/ent/merchant"
	"epay/ent/order"
	"epay/ent/withdraw"

	"github.com/gin-gonic/gin"
)

// HandleAPI is the entry point for /api.php. It supports two routing modes:
//
//   - GET (or POST) with `act=<query|settle|order|orders|refund>` —
//     the classic rainbow EasyPay API. MD5/plaintext key auth.
//   - POST with `s=class/func` — the API_INIT mode, RSA-signed JSON-style
//     endpoints. Routes to sRouter (see api_s_router.go).
//
// When both `s` and `act` are present, `s` wins (matches rainbow's behavior
// where ApiHelper::load_api short-circuits before the act dispatcher).
func (h *Handler) HandleAPI(c *gin.Context) {
	if s := strings.TrimSpace(c.Query("s")); s != "" {
		h.sRouter(c, s)
		return
	}
	act := strings.TrimSpace(c.Query("act"))
	switch act {
	case "query":
		h.actQuery(c)
	case "settle":
		h.actSettle(c)
	case "order":
		h.actOrder(c)
	case "orders":
		h.actOrders(c)
	case "refund":
		h.actRefund(c)
	case "refundquery":
		h.actRefundQuery(c)
	default:
		c.JSON(http.StatusOK, gin.H{"code": -5, "msg": "No Act!"})
	}
}

// ----- shared helpers for act endpoints -----------------------------------

// authMerchantByKey validates the legacy pid+key plaintext credentials used
// by rainbow's GET act endpoints. Returns nil merchant + (code, msg) on
// failure suitable for direct response. Also enforces the keytype=1 → RSA
// rule documented in the rainbow protocol ("该商户只能使用RSA签名类型").
func (h *Handler) authMerchantByKey(ctx context.Context, c *gin.Context) (*ent.Merchant, int, string) {
	pidStr := c.Query("pid")
	if pidStr == "" {
		pidStr = c.PostForm("pid")
	}
	key := c.Query("key")
	if key == "" {
		key = c.PostForm("key")
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return nil, -3, "商户ID不存在"
	}
	m, err := h.client.Merchant.Query().Where(merchant.Pid(pid)).First(ctx)
	if err != nil {
		return nil, -3, "商户ID不存在"
	}
	if m.Pkey != key {
		return nil, -3, "商户密钥错误"
	}
	if m.Keytype == 1 {
		return nil, -3, "该商户只能使用RSA签名类型"
	}
	return m, 0, ""
}

func clampLimitOffset(c *gin.Context) (limit, offset int) {
	limit, _ = strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	offset, _ = strconv.Atoi(c.Query("offset"))
	if offset < 0 {
		offset = 0
	}
	return
}

// ----- act=query ----------------------------------------------------------

// actQuery returns merchant overview info — balance, total orders, today and
// yesterday order counts. Compatible with rainbow `act=query` response shape.
func (h *Handler) actQuery(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	m, code, msg := h.authMerchantByKey(ctx, c)
	if code != 0 {
		c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg})
		return
	}

	// Balance is read from the Settlement ledger (1:1 with merchant).
	var balance float64
	if s, err := h.client.Settlement.Query().Where().All(ctx); err == nil {
		for _, item := range s {
			if item.MerchantID == m.ID {
				balance = item.Balance
				break
			}
		}
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	totalOrders, _ := h.client.Order.Query().Where(order.MerchantID(m.ID)).Count(ctx)
	ordersToday, _ := h.client.Order.Query().
		Where(order.MerchantID(m.ID),
			order.StatusIn(order.StatusPAID, order.StatusSETTLED),
			order.CreatedAtGTE(parseDate(today)),
			order.CreatedAtLT(parseDate(today).Add(24*time.Hour)),
		).Count(ctx)
	ordersYesterday, _ := h.client.Order.Query().
		Where(order.MerchantID(m.ID),
			order.StatusIn(order.StatusPAID, order.StatusSETTLED),
			order.CreatedAtGTE(parseDate(yesterday)),
			order.CreatedAtLT(parseDate(yesterday).Add(24*time.Hour)),
		).Count(ctx)

	c.JSON(http.StatusOK, gin.H{
		"code":           1,
		"pid":            m.Pid,
		"key":            m.Pkey,
		"active":         boolTo01(m.Status == "active"),
		"money":          formatMoneyTwo(balance),
		"type":           0, // settle_id placeholder; unused in single-channel epay
		"account":        "",
		"username":       m.Name,
		"orders":         totalOrders,
		"orders_today":   ordersToday,
		"orders_lastday": ordersYesterday,
	})
}

func parseDate(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", s, time.Local)
	return t
}

func boolTo01(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ----- act=settle ---------------------------------------------------------

// actSettle returns paginated withdrawal/settlement records for the merchant.
// Rainbow's pre_settle equivalent.
func (h *Handler) actSettle(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	m, code, msg := h.authMerchantByKey(ctx, c)
	if code != 0 {
		c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg})
		return
	}
	limit, offset := clampLimitOffset(c)
	rows, err := h.client.Withdraw.Query().
		Where(withdraw.MerchantID(m.ID)).
		Order(ent.Desc("created_at")).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "查询结算记录失败！"})
		return
	}
	data := make([]gin.H, 0, len(rows))
	for _, w := range rows {
		data = append(data, gin.H{
			"id":          w.ID.String(),
			"uid":         m.Pid,
			"money":       formatMoneyTwo(w.Amount),
			"status":      withdrawStatusToInt(w.Status),
			"account":     w.AccountInfo,
			"remark":      w.Remark,
			"addtime":     w.CreatedAt.Format("2006-01-02 15:04:05"),
			"endtime":     w.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "查询结算记录成功！",
		"data": data,
	})
}

func withdrawStatusToInt(s withdraw.Status) int {
	switch s {
	case withdraw.StatusPAID:
		return 1
	case withdraw.StatusREJECTED:
		return 2
	default:
		return 0
	}
}

// ----- act=order ----------------------------------------------------------

// actOrder returns a single order's full details. Authentication has two modes:
//
//   - Platform mode: ?sign=...&trade_no=... where
//     sign == md5(SYS_KEY + trade_no + SYS_KEY). Allows cross-merchant query.
//   - Merchant mode: ?pid=&key=&[out_trade_no=|trade_no=]. Per-merchant scope.
func (h *Handler) actOrder(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	tradeNo := strings.TrimSpace(c.Query("trade_no"))
	if tradeNo == "" {
		tradeNo = strings.TrimSpace(c.PostForm("trade_no"))
	}
	outTradeNo := strings.TrimSpace(c.Query("out_trade_no"))
	if outTradeNo == "" {
		outTradeNo = strings.TrimSpace(c.PostForm("out_trade_no"))
	}
	sign := strings.TrimSpace(c.Query("sign"))

	var ord *ent.Order
	var err error
	switch {
	case sign != "" && tradeNo != "":
		if h.sysKey == "" {
			c.JSON(http.StatusOK, gin.H{"code": -3, "msg": "verify sign failed"})
			return
		}
		expected := md5Hex(h.sysKey + tradeNo + h.sysKey)
		if expected != sign {
			c.JSON(http.StatusOK, gin.H{"code": -3, "msg": "verify sign failed"})
			return
		}
		ord, err = h.client.Order.Query().Where(order.TradeNo(tradeNo)).First(ctx)
	default:
		m, code, msg := h.authMerchantByKey(ctx, c)
		if code != 0 {
			c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg})
			return
		}
		switch {
		case outTradeNo != "":
			ord, err = h.client.Order.Query().
				Where(order.MerchantID(m.ID), order.OrderNo(outTradeNo)).
				First(ctx)
		case tradeNo != "":
			ord, err = h.client.Order.Query().
				Where(order.MerchantID(m.ID), order.TradeNo(tradeNo)).
				First(ctx)
		default:
			c.JSON(http.StatusOK, gin.H{"code": -4, "msg": "订单号不能为空"})
			return
		}
	}
	if err != nil || ord == nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "订单号不存在"})
		return
	}
	c.JSON(http.StatusOK, h.orderToFullResponse(ctx, ord))
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// orderToFullResponse converts an Order entity to the full rainbow act=order
// JSON shape (all fields present, populated when available).
func (h *Handler) orderToFullResponse(ctx context.Context, ord *ent.Order) gin.H {
	endTime := ""
	if ord.PaidAt != nil {
		endTime = ord.PaidAt.Format("2006-01-02 15:04:05")
	}
	merch, _ := h.client.Merchant.Query().Where(merchant.ID(ord.MerchantID)).First(ctx)
	pid := 0
	if merch != nil {
		pid = merch.Pid
	}
	return gin.H{
		"code":         1,
		"msg":          "succ",
		"trade_no":     ord.TradeNo,
		"out_trade_no": ord.OrderNo,
		"api_trade_no": ord.APITradeNo,
		"type":         string(ord.Type),
		"pid":          pid,
		"addtime":      ord.CreatedAt.Format("2006-01-02 15:04:05"),
		"endtime":      endTime,
		"name":         ord.Name,
		"money":        formatMoneyTwo(ord.Amount),
		"param":        ord.Param,
		"buyer":        ord.Buyer,
		"clientip":     ord.Clientip,
		"status":       orderStatusToInt(ord.Status),
		"payurl":       buildMockPayURL(ord.OrderNo, string(ord.Type)),
		"refundmoney":  formatMoneyTwo(ord.RefundMoney),
	}
}

// orderStatusToInt maps the internal lifecycle enum to rainbow's 0/1/2 codes.
func orderStatusToInt(s order.Status) int {
	switch s {
	case order.StatusPAID, order.StatusSETTLED:
		return 1
	case order.StatusCANCELLED:
		return 2
	default:
		return 0
	}
}

// ----- act=orders ---------------------------------------------------------

// actOrders returns a paginated list of orders for the authenticated merchant.
// Supports filtering by status (0=pending, 1=paid).
func (h *Handler) actOrders(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	m, code, msg := h.authMerchantByKey(ctx, c)
	if code != 0 {
		c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg})
		return
	}
	limit, offset := clampLimitOffset(c)
	q := h.client.Order.Query().Where(order.MerchantID(m.ID))
	if s := c.Query("status"); s != "" {
		if status, err := strconv.Atoi(s); err == nil {
			switch status {
			case 1:
				q = q.Where(order.StatusIn(order.StatusPAID, order.StatusSETTLED))
			case 0:
				q = q.Where(order.StatusIn(order.StatusPENDING, order.StatusEXPIRED))
			case 2:
				q = q.Where(order.StatusEQ(order.StatusCANCELLED))
			}
		}
	}
	rows, err := q.Order(ent.Desc("created_at")).Limit(limit).Offset(offset).All(ctx)
	if err != nil {
		log.Printf("[easypay] act=orders: %v", err)
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "查询订单记录失败！"})
		return
	}
	data := make([]gin.H, 0, len(rows))
	for _, o := range rows {
		endTime := ""
		if o.PaidAt != nil {
			endTime = o.PaidAt.Format("2006-01-02 15:04:05")
		}
		data = append(data, gin.H{
			"trade_no":     o.TradeNo,
			"out_trade_no": o.OrderNo,
			"type":         string(o.Type),
			"pid":          m.Pid,
			"addtime":      o.CreatedAt.Format("2006-01-02 15:04:05"),
			"endtime":      endTime,
			"name":         o.Name,
			"money":        formatMoneyTwo(o.Amount),
			"param":        o.Param,
			"buyer":        o.Buyer,
			"status":       orderStatusToInt(o.Status),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"code":  1,
		"msg":   "查询订单记录成功！",
		"count": len(data),
		"data":  data,
	})
}
