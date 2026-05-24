package easypay

import (
	"context"
	"crypto/rand"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"epay/ent"
	"epay/ent/order"
	"epay/ent/refund"

	"github.com/gin-gonic/gin"
)

// actRefund handles POST /api.php?act=refund.
//
// Rainbow-compatible parameter shape:
//
//	pid             — merchant id
//	key             — merchant pkey (plaintext)
//	trade_no        — platform trade number (one of trade_no / out_trade_no required)
//	out_trade_no    — merchant order number
//	money           — refund amount (decimal string)
//	out_refund_no   — optional idempotency key (merchant-supplied)
//
// Behaviour (current scope — protocol-only):
//   - Validates auth & order ownership
//   - Records a PENDING Refund row (idempotent on out_refund_no when supplied)
//   - Immediately marks it FAILED and returns
//     `{"code":-3,"msg":"暂不支持自动退款，请联系平台手动处理"}`
//     so merchants get a deterministic, signed protocol response while the
//     real provider integration is pending.
func (h *Handler) actRefund(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if !h.userRefundOn {
		c.JSON(http.StatusOK, gin.H{"code": -4, "msg": "未开启商户后台自助退款"})
		return
	}
	m, code, msg := h.authMerchantByKey(ctx, c)
	if code != 0 {
		c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg})
		return
	}
	if !m.RefundEnabled {
		c.JSON(http.StatusOK, gin.H{"code": -2, "msg": "商户未开启订单退款API接口"})
		return
	}

	moneyStr := strings.TrimSpace(c.PostForm("money"))
	if moneyStr == "" {
		moneyStr = strings.TrimSpace(c.Query("money"))
	}
	if !moneyPattern.MatchString(moneyStr) {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "金额输入错误"})
		return
	}
	moneyF, _ := strconv.ParseFloat(moneyStr, 64)
	if moneyF <= 0 {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "金额输入错误"})
		return
	}

	tradeNo := strings.TrimSpace(c.PostForm("trade_no"))
	if tradeNo == "" {
		tradeNo = strings.TrimSpace(c.Query("trade_no"))
	}
	outTradeNo := strings.TrimSpace(c.PostForm("out_trade_no"))
	if outTradeNo == "" {
		outTradeNo = strings.TrimSpace(c.Query("out_trade_no"))
	}
	outRefundNo := strings.TrimSpace(c.PostForm("out_refund_no"))
	if outRefundNo == "" {
		outRefundNo = strings.TrimSpace(c.Query("out_refund_no"))
	}

	// Resolve trade_no from out_trade_no if needed, then load the order.
	var ord *ent.Order
	var err error
	switch {
	case tradeNo != "":
		ord, err = h.client.Order.Query().
			Where(order.ProductID(m.ID), order.TradeNo(tradeNo)).
			First(ctx)
	case outTradeNo != "":
		ord, err = h.client.Order.Query().
			Where(order.ProductID(m.ID), order.OrderNo(outTradeNo)).
			First(ctx)
		if err == nil {
			tradeNo = ord.TradeNo
		}
	default:
		c.JSON(http.StatusOK, gin.H{"code": -4, "msg": "订单号不能为空"})
		return
	}
	if err != nil || ord == nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "当前订单不存在！"})
		return
	}

	// Idempotency on out_refund_no.
	if outRefundNo != "" {
		existing, err := h.client.Refund.Query().
			Where(refund.UserID(m.UserID), refund.OutRefundNo(outRefundNo)).
			First(ctx)
		if err == nil && existing != nil {
			h.writeRefundRecordResponse(c, m.Pid, existing)
			return
		}
	}

	refundNo := generateRefundNo()
	_, ierr := h.client.Refund.Create().
		SetUserID(m.UserID).
		SetRefundNo(refundNo).
		SetOutRefundNo(outRefundNo).
		SetTradeNo(tradeNo).
		SetMoney(moneyF).
		SetReduceMoney(0).
		SetStatus(refund.StatusFAILED).
		SetMessage("upstream refund integration pending").
		SetFinishedAt(time.Now()).
		Save(ctx)
	if ierr != nil {
		if outRefundNo != "" && ent.IsConstraintError(ierr) {
			existing, err := h.client.Refund.Query().
				Where(refund.UserID(m.UserID), refund.OutRefundNo(outRefundNo)).
				First(ctx)
			if err == nil && existing != nil {
				h.writeRefundRecordResponse(c, m.Pid, existing)
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"code": -2, "msg": "退款记录创建失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":      -3,
		"msg":       "暂不支持自动退款，请联系平台手动处理",
		"refund_no": refundNo,
		"trade_no":  tradeNo,
		"money":     formatMoneyTwo(moneyF),
	})
}

func (h *Handler) writeRefundRecordResponse(c *gin.Context, pid int, row *ent.Refund) {
	statusCode := -3
	msg := "已有相同退款单号，退款失败：" + firstNonEmptyString(row.Message, "暂不支持自动退款，请联系平台手动处理")
	switch row.Status {
	case refund.StatusSUCCESS:
		statusCode = 0
		msg = "已存在相同退款单号！退款金额￥" + formatMoneyTwo(row.Money)
	case refund.StatusPENDING:
		statusCode = 1
		msg = "已存在相同退款单号，退款处理中"
	}
	c.JSON(http.StatusOK, gin.H{
		"code":          statusCode,
		"refund_no":     row.RefundNo,
		"out_refund_no": row.OutRefundNo,
		"trade_no":      row.TradeNo,
		"uid":           pid,
		"money":         formatMoneyTwo(row.Money),
		"reducemoney":   formatMoneyTwo(row.ReduceMoney),
		"msg":           msg,
	})
}

// actRefundQuery handles POST /api.php?act=refundquery.
func (h *Handler) actRefundQuery(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if !h.userRefundOn {
		c.JSON(http.StatusOK, gin.H{"code": -4, "msg": "未开启商户后台自助退款"})
		return
	}
	m, code, msg := h.authMerchantByKey(ctx, c)
	if code != 0 {
		c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg})
		return
	}
	refundNo := strings.TrimSpace(c.Query("refund_no"))
	outRefundNo := strings.TrimSpace(c.Query("out_refund_no"))
	if refundNo == "" && outRefundNo == "" {
		c.JSON(http.StatusOK, gin.H{"code": -4, "msg": "商户退款单号不能为空"})
		return
	}

	q := h.client.Refund.Query().Where(refund.UserID(m.UserID))
	switch {
	case refundNo != "":
		q = q.Where(refund.RefundNo(refundNo))
	case outRefundNo != "":
		q = q.Where(refund.OutRefundNo(outRefundNo))
	}
	row, err := q.First(ctx)
	if err != nil || row == nil {
		c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "退款记录不存在"})
		return
	}
	// Resolve out_trade_no via trade_no for the response.
	ord, _ := h.client.Order.Query().Where(order.UserIDEQ(m.UserID), order.TradeNo(row.TradeNo)).First(ctx)
	outTradeNo := ""
	if ord != nil {
		outTradeNo = ord.OrderNo
	}

	statusInt := 0
	switch row.Status {
	case refund.StatusSUCCESS:
		statusInt = 1
	case refund.StatusFAILED:
		statusInt = 2
	}
	endTime := ""
	if row.FinishedAt != nil {
		endTime = row.FinishedAt.Format("2006-01-02 15:04:05")
	}
	c.JSON(http.StatusOK, gin.H{
		"code":          0,
		"refund_no":     row.RefundNo,
		"out_refund_no": row.OutRefundNo,
		"trade_no":      row.TradeNo,
		"out_trade_no":  outTradeNo,
		"uid":           m.Pid,
		"money":         formatMoneyTwo(row.Money),
		"reducemoney":   formatMoneyTwo(row.ReduceMoney),
		"status":        statusInt,
		"addtime":       row.CreatedAt.Format("2006-01-02 15:04:05"),
		"endtime":       endTime,
	})
}

func generateRefundNo() string {
	return "RF" + time.Now().Format("20060102150405") + randomDigits(5)
}

func randomDigits(n int) string {
	const digits = "0123456789"
	b := make([]byte, n)
	for i := range b {
		v, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			b[i] = '0'
			continue
		}
		b[i] = digits[v.Int64()]
	}
	return string(b)
}
