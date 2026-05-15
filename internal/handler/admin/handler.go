// Package admin provides HTTP handlers for admin API endpoints,
// including login, dashboard, merchant/order/withdraw management, and config.
package admin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"epay/ent"
	"epay/ent/order"
	entcfg "epay/ent/platformconfig"
	"epay/ent/withdraw"
	adminSvc "epay/internal/service/admin"
	merchantSvc "epay/internal/service/merchant"
	settlementSvc "epay/internal/service/settlement"

	"entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminHandler consolidates all admin API endpoint handlers.
type AdminHandler struct {
	merchantSvc   *merchantSvc.Service
	adminSvc      *adminSvc.Service
	settlementSvc *settlementSvc.SettlementService
	ent           *ent.Client
}

// NewHandler creates an AdminHandler with the required services.
func NewHandler(entClient *ent.Client, as *adminSvc.Service, ms *merchantSvc.Service, ss *settlementSvc.SettlementService) *AdminHandler {
	return &AdminHandler{
		merchantSvc:   ms,
		adminSvc:      as,
		settlementSvc: ss,
		ent:           entClient,
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

// ---- Login (no auth) ----

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login authenticates an admin and returns a JWT token.
func (h *AdminHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	result, err := h.adminSvc.Login(ctx, req.Username, req.Password)
	if err != nil {
		respFail(c, 401, err.Error())
		return
	}

	respOK(c, gin.H{
		"token":    result.Token,
		"username": req.Username,
	})
}

// ---- ChangePassword ----

type changePwdReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ChangePassword validates the old password and sets a new one.
func (h *AdminHandler) ChangePassword(c *gin.Context) {
	var req changePwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}

	adminID, _ := c.Get("admin_id")
	id, ok := adminID.(uuid.UUID)
	if !ok {
		respErr(c, "invalid admin session")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	if err := h.adminSvc.ChangePassword(ctx, id, req.OldPassword, req.NewPassword); err != nil {
		respFail(c, 400, err.Error())
		return
	}

	respOK(c, nil)
}

// ---- Dashboard ----

type dashboardResp struct {
	TodayOrderCount     int     `json:"today_order_count"`
	TodayRevenue        float64 `json:"today_revenue"`
	PendingWithdrawCount int    `json:"pending_withdraw_count"`
}

// Dashboard returns summary statistics for today's orders and pending withdraws.
func (h *AdminHandler) Dashboard(c *gin.Context) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	// Count and sum today's PENDING/PAID/SETTLED orders
	todayOrders, err := h.ent.Order.Query().
		Where(
			order.StatusIn(order.StatusPENDING, order.StatusPAID, order.StatusSETTLED),
			order.CreatedAtGTE(todayStart),
		).All(ctx)
	if err != nil {
		respErr(c, "query orders: "+err.Error())
		return
	}

	var totalRevenue float64
	for _, o := range todayOrders {
		totalRevenue += o.Amount
	}

	// Count pending withdraws
	pendingCount, err := h.ent.Withdraw.Query().
		Where(withdraw.StatusEQ(withdraw.StatusPENDING)).
		Count(ctx)
	if err != nil {
		respErr(c, "query withdraws: "+err.Error())
		return
	}

	respOK(c, dashboardResp{
		TodayOrderCount:     len(todayOrders),
		TodayRevenue:        totalRevenue,
		PendingWithdrawCount: pendingCount,
	})
}

// ---- Merchants ----

// ListMerchants returns a paginated list of merchants, optionally filtered by status.
func (h *AdminHandler) ListMerchants(c *gin.Context) {
	page, limit := paramPageLimit(c)
	status := c.Query("status")

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	result, err := h.merchantSvc.ListMerchants(ctx, page, limit, status)
	if err != nil {
		respErr(c, err.Error())
		return
	}

	respOK(c, result)
}

type createMerchantReq struct {
	Name      string  `json:"name"`
	FeeRate   float64 `json:"fee_rate"`
	NotifyURL string  `json:"notify_url"`
}

// CreateMerchant creates a new merchant with generated pid and pkey.
func (h *AdminHandler) CreateMerchant(c *gin.Context) {
	var req createMerchantReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		respErr(c, "name is required")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	m, err := h.merchantSvc.CreateMerchant(ctx, req.Name, req.FeeRate, req.NotifyURL)
	if err != nil {
		respErr(c, err.Error())
		return
	}

	respOK(c, m)
}

type updateMerchantReq struct {
	Name      *string  `json:"name"`
	FeeRate   *float64 `json:"fee_rate"`
	Status    *string  `json:"status"`
	NotifyURL *string  `json:"notify_url"`
}

// UpdateMerchant updates a merchant's mutable fields by ID.
func (h *AdminHandler) UpdateMerchant(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respErr(c, "invalid id")
		return
	}

	var req updateMerchantReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	m, err := h.merchantSvc.UpdateMerchant(ctx, id, req.Name, req.FeeRate, req.Status, req.NotifyURL)
	if err != nil {
		respErr(c, err.Error())
		return
	}

	respOK(c, m)
}

// RegeneratePkey generates a new pkey for the given merchant.
func (h *AdminHandler) RegeneratePkey(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respErr(c, "invalid id")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	newPkey, err := h.merchantSvc.RegeneratePkey(ctx, id)
	if err != nil {
		respErr(c, err.Error())
		return
	}

	respOK(c, gin.H{"pkey": newPkey})
}

// ---- Orders ----

type orderItem struct {
	ID               string    `json:"id"`
	OrderNo          string    `json:"order_no"`
	MerchantName     string    `json:"merchant_name"`
	Type             string    `json:"type"`
	Amount           float64   `json:"amount"`
	FeeOfficial      float64   `json:"fee_official"`
	FeePlatform      float64   `json:"fee_platform"`
	NetAmount        float64   `json:"net_amount"`
	TradeNo          string    `json:"trade_no"`
	Status           string    `json:"status"`
	NotifyURL        string    `json:"notify_url"`
	PaidAt           *time.Time `json:"paid_at"`
	CreatedAt        time.Time `json:"created_at"`
}

type orderListResp struct {
	Items      []orderItem `json:"items"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	TotalPages int         `json:"total_pages"`
}

// ListOrders returns a paginated list of orders with merchant names,
// optionally filtered by status and merchant_id.
func (h *AdminHandler) ListOrders(c *gin.Context) {
	page, limit := paramPageLimit(c)
	status := c.Query("status")
	merchantIDStr := c.Query("merchant_id")

	q := h.ent.Order.Query().
		WithMerchant()

	if status != "" {
		q.Where(order.StatusEQ(order.Status(status)))
	}
	if merchantIDStr != "" {
		mid, err := uuid.Parse(merchantIDStr)
		if err != nil {
			respErr(c, "invalid merchant_id")
			return
		}
		q.Where(order.MerchantIDEQ(mid))
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
		merchantName := ""
		if o.Edges.Merchant != nil {
			merchantName = o.Edges.Merchant.Name
		}
		tradeNo := ""
		if o.TradeNo != "" {
			tradeNo = o.TradeNo
		}
		items = append(items, orderItem{
			ID:           o.ID.String(),
			OrderNo:      o.OrderNo,
			MerchantName: merchantName,
			Type:         string(o.Type),
			Amount:       o.Amount,
			FeeOfficial:  o.FeeOfficial,
			FeePlatform:  o.FeePlatform,
			NetAmount:    o.NetAmount,
			TradeNo:      tradeNo,
			Status:       string(o.Status),
			NotifyURL:    o.NotifyURL,
			PaidAt:       o.PaidAt,
			CreatedAt:    o.CreatedAt,
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

// ---- Withdraws ----

type withdrawItem struct {
	ID            string    `json:"id"`
	MerchantID    string    `json:"merchant_id"`
	MerchantName  string    `json:"merchant_name"`
	Amount        float64   `json:"amount"`
	AccountInfo   string    `json:"account_info"`
	Status        string    `json:"status"`
	Remark        string    `json:"remark"`
	CreatedAt     time.Time `json:"created_at"`
}

type withdrawListResp struct {
	Items      []withdrawItem `json:"items"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	TotalPages int            `json:"total_pages"`
}

// ListWithdraws returns a paginated list of withdrawal records with merchant names.
func (h *AdminHandler) ListWithdraws(c *gin.Context) {
	page, limit := paramPageLimit(c)
	status := c.Query("status")

	q := h.ent.Withdraw.Query().
		WithMerchant()

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
	withdraws, err := q.
		Order(withdraw.ByCreatedAt(sql.OrderDesc())).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		respErr(c, err.Error())
		return
	}

	items := make([]withdrawItem, 0, len(withdraws))
	for _, w := range withdraws {
		merchantName := ""
		if w.Edges.Merchant != nil {
			merchantName = w.Edges.Merchant.Name
		}
		items = append(items, withdrawItem{
			ID:           w.ID.String(),
			MerchantID:   w.MerchantID.String(),
			MerchantName: merchantName,
			Amount:       w.Amount,
			AccountInfo:  w.AccountInfo,
			Status:       string(w.Status),
			Remark:       w.Remark,
			CreatedAt:    w.CreatedAt,
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

// ApproveWithdraw sets a withdrawal request status to APPROVED.
func (h *AdminHandler) ApproveWithdraw(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respErr(c, "invalid id")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	if err := h.settlementSvc.ApproveWithdraw(ctx, id); err != nil {
		respErr(c, err.Error())
		return
	}

	respOK(c, nil)
}

type rejectWithdrawReq struct {
	Remark string `json:"remark"`
}

// RejectWithdraw rejects a PENDING withdrawal and refunds the frozen amount.
func (h *AdminHandler) RejectWithdraw(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respErr(c, "invalid id")
		return
	}

	var req rejectWithdrawReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	if err := h.settlementSvc.RejectWithdraw(ctx, id, req.Remark); err != nil {
		respErr(c, err.Error())
		return
	}

	respOK(c, nil)
}

// ConfirmWithdraw finalizes an APPROVED withdrawal as PAID.
func (h *AdminHandler) ConfirmWithdraw(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respErr(c, "invalid id")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	if err := h.settlementSvc.ConfirmWithdraw(ctx, id); err != nil {
		respErr(c, err.Error())
		return
	}

	respOK(c, nil)
}

// ---- Configs ----

type configItem struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// GetConfigs returns all platform configuration entries.
func (h *AdminHandler) GetConfigs(c *gin.Context) {
	ctx, cancel := timeoutCtx(c)
	defer cancel()

	configs, err := h.ent.PlatformConfig.Query().All(ctx)
	if err != nil {
		respErr(c, err.Error())
		return
	}

	items := make([]configItem, 0, len(configs))
	for _, cfg := range configs {
		items = append(items, configItem{
			Key:         cfg.Key,
			Value:       cfg.Value,
			Description: cfg.Description,
		})
	}

	respOK(c, items)
}

// UpdateConfigs upserts platform configuration entries from a map of key-value pairs.
func (h *AdminHandler) UpdateConfigs(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}

	ctx, cancel := timeoutCtx(c)
	defer cancel()

	for key, value := range req {
		// Check if config already exists
		existing, err := h.ent.PlatformConfig.Query().
			Where(entcfg.KeyEQ(key)).
			First(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				// Create new config entry
				_, err := h.ent.PlatformConfig.Create().
					SetKey(key).
					SetValue(value).
					Save(ctx)
				if err != nil {
					respErr(c, "failed to create config "+key+": "+err.Error())
					return
				}
				continue
			}
			respErr(c, err.Error())
			return
		}

		// Update existing config
		_, err = h.ent.PlatformConfig.UpdateOneID(existing.ID).
			SetValue(value).
			Save(ctx)
		if err != nil {
			respErr(c, "failed to update config "+key+": "+err.Error())
			return
		}
	}

	respOK(c, nil)
}
