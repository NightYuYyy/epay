// Package merchant provides HTTP handlers for merchant API endpoints,
// including login, register, profile, balance, orders, withdraws, and settings.
package merchant

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"epay/ent"
	"epay/ent/order"
	"epay/ent/settlement"
	"epay/ent/withdraw"
	"epay/internal/config"
	merchantSvc "epay/internal/service/merchant"
	settlementSvc "epay/internal/service/settlement"

	"entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// MerchantHandler consolidates all merchant API endpoint handlers.
type MerchantHandler struct {
	merchantSvc   *merchantSvc.Service
	settlementSvc *settlementSvc.SettlementService
	ent           *ent.Client
	cfg           *config.Config
}

// NewHandler creates a MerchantHandler with the required services.
func NewHandler(entClient *ent.Client, ms *merchantSvc.Service, ss *settlementSvc.SettlementService, cfg *config.Config) *MerchantHandler {
	return &MerchantHandler{
		merchantSvc:   ms,
		settlementSvc: ss,
		ent:           entClient,
		cfg:           cfg,
	}
}

// ---- helpers ----

func respOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": data})
}

func respFail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, gin.H{"code": code, "msg": msg})
}

func respErr(c *gin.Context, msg string) {
	respFail(c, -1, msg)
}

func paramPageLimit(c *gin.Context) (int, int) {
	page := 1
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = p
	}
	limit := 20
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	return page, limit
}

func timeoutCtx(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), 5*time.Second)
}

// ---- JWT claims ----

type merchantClaims struct {
	jwt.RegisteredClaims
	AdminID uuid.UUID `json:"admin_id"`
	Role    string    `json:"role"`
}

// ---- Login (no auth) ----

type loginReq struct {
	Pid      string `json:"pid"`
	Password string `json:"password"`
}

// Login authenticates a merchant by pid and password (pkey), and returns a JWT token.
func (h *MerchantHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}

	pid, err := strconv.Atoi(req.Pid)
	if err != nil {
		respErr(c, "invalid pid")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	m, err := h.merchantSvc.VerifyCredential(ctx, pid, req.Password)
	if err != nil {
		respFail(c, 401, err.Error())
		return
	}

	expireHours := h.cfg.JWT.ExpireHour
	if expireHours <= 0 {
		expireHours = 24
	}
	expiresAt := time.Now().Add(time.Duration(expireHours) * time.Hour)

	claims := merchantClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Subject:   m.ID.String(),
		},
		AdminID: m.ID,
		Role:    "merchant",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.cfg.JWT.Secret))
	if err != nil {
		respErr(c, "failed to generate token")
		return
	}

	respOK(c, gin.H{
		"token": tokenString,
		"pid":   m.Pid,
	})
}

// ---- Register (no auth) ----

type registerReq struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// Register creates a new merchant account and returns the generated pid and pkey.
func (h *MerchantHandler) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.Password == "" {
		respErr(c, "name and password are required")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	// Use default platform rate from config
	feeRate := h.cfg.Default.DefaultPlatformRate
	m, err := h.merchantSvc.CreateMerchant(ctx, req.Name, feeRate, "")
	if err != nil {
		respErr(c, err.Error())
		return
	}

	respOK(c, gin.H{
		"pid":  m.Pid,
		"pkey": m.Pkey,
	})
}

// ---- Profile (authenticated) ----

// Profile returns the authenticated merchant's profile information.
func (h *MerchantHandler) Profile(c *gin.Context) {
	mid := c.GetString("merchant_id")
	merchantID, err := uuid.Parse(mid)
	if err != nil {
		respErr(c, "invalid merchant session")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	m, err := h.merchantSvc.GetMerchant(ctx, merchantID, false)
	if err != nil {
		respErr(c, "merchant not found")
		return
	}

	respOK(c, m)
}

// ---- Balance (authenticated) ----

// Balance returns the merchant's settlement balance.
func (h *MerchantHandler) Balance(c *gin.Context) {
	mid := c.GetString("merchant_id")
	merchantID, err := uuid.Parse(mid)
	if err != nil {
		respErr(c, "invalid merchant session")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	sett, err := h.ent.Settlement.Query().
		Where(settlement.MerchantIDEQ(merchantID)).
		First(ctx)
	if err != nil {
		respErr(c, "query balance failed")
		return
	}

	respOK(c, gin.H{
		"balance":         sett.Balance,
		"frozen":          sett.Frozen,
		"total_income":    sett.TotalIncome,
		"total_withdrawn": sett.TotalWithdrawn,
	})
}

// ---- Orders (authenticated) ----

type orderItem struct {
	ID          string     `json:"id"`
	OrderNo     string     `json:"order_no"`
	Type        string     `json:"type"`
	Amount      float64    `json:"amount"`
	FeeOfficial float64    `json:"fee_official"`
	FeePlatform float64    `json:"fee_platform"`
	NetAmount   float64    `json:"net_amount"`
	TradeNo     string     `json:"trade_no"`
	Status      string     `json:"status"`
	NotifyURL   string     `json:"notify_url"`
	PaidAt      *time.Time `json:"paid_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type orderListResp struct {
	Items      []orderItem `json:"items"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	TotalPages int         `json:"total_pages"`
}

// ListOrders returns a paginated list of the merchant's orders,
// optionally filtered by status.
func (h *MerchantHandler) ListOrders(c *gin.Context) {
	mid := c.GetString("merchant_id")
	merchantID, err := uuid.Parse(mid)
	if err != nil {
		respErr(c, "invalid merchant session")
		return
	}

	page, limit := paramPageLimit(c)
	status := c.Query("status")

	q := h.ent.Order.Query().
		Where(order.MerchantIDEQ(merchantID))

	if status != "" {
		q.Where(order.StatusEQ(order.Status(status)))
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	total, err := q.Count(ctx)
	if err != nil {
		respErr(c, err.Error())
		return
	}

	offset := (page - 1) * limit
	orders, err := q.
		Order(order.ByCreatedAt(sql.OrderDesc())).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		respErr(c, err.Error())
		return
	}

	items := make([]orderItem, 0, len(orders))
	for _, o := range orders {
		tradeNo := ""
		if o.TradeNo != "" {
			tradeNo = o.TradeNo
		}
		items = append(items, orderItem{
			ID:          o.ID.String(),
			OrderNo:     o.OrderNo,
			Type:        string(o.Type),
			Amount:      o.Amount,
			FeeOfficial: o.FeeOfficial,
			FeePlatform: o.FeePlatform,
			NetAmount:   o.NetAmount,
			TradeNo:     tradeNo,
			Status:      string(o.Status),
			NotifyURL:   o.NotifyURL,
			PaidAt:      o.PaidAt,
			CreatedAt:   o.CreatedAt,
		})
	}

	totalPages := (total + limit - 1) / limit
	respOK(c, orderListResp{
		Items:      items,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// ---- Withdraws (authenticated) ----

type withdrawItem struct {
	ID          string    `json:"id"`
	Amount      float64   `json:"amount"`
	AccountInfo string    `json:"account_info"`
	Status      string    `json:"status"`
	Remark      string    `json:"remark"`
	CreatedAt   time.Time `json:"created_at"`
}

type withdrawListResp struct {
	Items      []withdrawItem `json:"items"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	TotalPages int            `json:"total_pages"`
}

// ListWithdraws returns a paginated list of the merchant's withdrawal records.
func (h *MerchantHandler) ListWithdraws(c *gin.Context) {
	mid := c.GetString("merchant_id")
	merchantID, err := uuid.Parse(mid)
	if err != nil {
		respErr(c, "invalid merchant session")
		return
	}

	page, limit := paramPageLimit(c)
	status := c.Query("status")

	q := h.ent.Withdraw.Query().
		Where(withdraw.MerchantIDEQ(merchantID))

	if status != "" {
		q.Where(withdraw.StatusEQ(withdraw.Status(status)))
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	total, err := q.Count(ctx)
	if err != nil {
		respErr(c, err.Error())
		return
	}

	offset := (page - 1) * limit
	records, err := q.
		Order(withdraw.ByCreatedAt(sql.OrderDesc())).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		respErr(c, err.Error())
		return
	}

	items := make([]withdrawItem, 0, len(records))
	for _, w := range records {
		items = append(items, withdrawItem{
			ID:          w.ID.String(),
			Amount:      w.Amount,
			AccountInfo: w.AccountInfo,
			Status:      string(w.Status),
			Remark:      w.Remark,
			CreatedAt:   w.CreatedAt,
		})
	}

	totalPages := (total + limit - 1) / limit
	respOK(c, withdrawListResp{
		Items:      items,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// ---- RequestWithdraw (authenticated) ----

type requestWithdrawReq struct {
	Amount      float64 `json:"amount"`
	AccountInfo string  `json:"account_info"`
}

// RequestWithdraw creates a new withdrawal request for the merchant.
func (h *MerchantHandler) RequestWithdraw(c *gin.Context) {
	mid := c.GetString("merchant_id")
	merchantID, err := uuid.Parse(mid)
	if err != nil {
		respErr(c, "invalid merchant session")
		return
	}

	var req requestWithdrawReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	wd, err := h.settlementSvc.RequestWithdraw(ctx, merchantID, req.Amount, req.AccountInfo)
	if err != nil {
		respErr(c, err.Error())
		return
	}

	respOK(c, gin.H{
		"id":           wd.ID.String(),
		"amount":       wd.Amount,
		"account_info": wd.AccountInfo,
		"status":       string(wd.Status),
		"created_at":   wd.CreatedAt,
	})
}

// ---- GetAPIKey (authenticated) ----

// GetAPIKey returns the merchant's pid and full pkey.
func (h *MerchantHandler) GetAPIKey(c *gin.Context) {
	mid := c.GetString("merchant_id")
	merchantID, err := uuid.Parse(mid)
	if err != nil {
		respErr(c, "invalid merchant session")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	m, err := h.merchantSvc.GetMerchant(ctx, merchantID, false)
	if err != nil {
		respErr(c, "merchant not found")
		return
	}

	respOK(c, gin.H{
		"pid":  m.Pid,
		"pkey": m.Pkey,
	})
}

// ---- UpdateNotifyURL (authenticated) ----

type updateNotifyURLReq struct {
	NotifyURL string `json:"notify_url"`
}

// UpdateNotifyURL updates the merchant's notification URL.
func (h *MerchantHandler) UpdateNotifyURL(c *gin.Context) {
	mid := c.GetString("merchant_id")
	merchantID, err := uuid.Parse(mid)
	if err != nil {
		respErr(c, "invalid merchant session")
		return
	}

	var req updateNotifyURLReq
	if err := c.ShouldBindJSON(&req); err != nil || req.NotifyURL == "" {
		respErr(c, "notify_url is required")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	m, err := h.merchantSvc.UpdateMerchant(ctx, merchantID, nil, nil, nil, &req.NotifyURL)
	if err != nil {
		respErr(c, err.Error())
		return
	}

	respOK(c, m)
}
