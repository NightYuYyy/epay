// Package admin provides HTTP handlers for admin API endpoints,
// including login, dashboard, user/product/order/withdraw management, and config.
package admin

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"epay/ent"
	"epay/ent/order"
	entcfg "epay/ent/platformconfig"
	"epay/ent/withdraw"
	adminSvc "epay/internal/service/admin"
	productSvc "epay/internal/service/product"
	settlementSvc "epay/internal/service/settlement"
	userSvc "epay/internal/service/user"

	"entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminHandler consolidates all admin API endpoint handlers.
type AdminHandler struct {
	userSvc       *userSvc.Service
	productSvc    *productSvc.Service
	adminSvc      *adminSvc.Service
	settlementSvc *settlementSvc.SettlementService
	ent           *ent.Client
}

// NewHandler creates an AdminHandler with the required services.
func NewHandler(entClient *ent.Client, as *adminSvc.Service, us *userSvc.Service, ps *productSvc.Service, ss *settlementSvc.SettlementService) *AdminHandler {
	return &AdminHandler{
		userSvc:       us,
		productSvc:    ps,
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

// ---- Login ----

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
	respOK(c, gin.H{"token": result.Token, "username": req.Username})
}

// ---- ChangePassword ----

type changePwdReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

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
	TodayOrderCount      int     `json:"today_order_count"`
	TodayRevenue         float64 `json:"today_revenue"`
	PendingWithdrawCount int     `json:"pending_withdraw_count"`
}

func (h *AdminHandler) Dashboard(c *gin.Context) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	ctx, cancel := timeoutCtx(c)
	defer cancel()
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
	pendingCount, err := h.ent.Withdraw.Query().
		Where(withdraw.StatusEQ(withdraw.StatusPENDING)).
		Count(ctx)
	if err != nil {
		respErr(c, "query withdraws: "+err.Error())
		return
	}
	respOK(c, dashboardResp{TodayOrderCount: len(todayOrders), TodayRevenue: totalRevenue, PendingWithdrawCount: pendingCount})
}

// ---- Users ----

// ListUsers returns a paginated list of users.
func (h *AdminHandler) ListUsers(c *gin.Context) {
	page, limit := paramPageLimit(c)
	status := c.Query("status")
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	result, err := h.userSvc.List(ctx, page, limit, status)
	if err != nil {
		respErr(c, err.Error())
		return
	}
	respOK(c, result)
}

type adminCreateUserReq struct {
	Email    string   `json:"email"`
	Password string   `json:"password"`
	Name     string   `json:"name"`
	FeeRate  *float64 `json:"fee_rate"`
}

// CreateUser admin-creates a user.
func (h *AdminHandler) CreateUser(c *gin.Context) {
	var req adminCreateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	u, err := h.userSvc.Register(ctx, req.Email, req.Password, req.Name, req.FeeRate)
	if err != nil {
		respErr(c, err.Error())
		return
	}
	respOK(c, u)
}

type updateUserReq struct {
	Name    *string  `json:"name"`
	FeeRate *float64 `json:"fee_rate"`
	Status  *string  `json:"status"`
}

// UpdateUser updates a user's mutable fields by id.
func (h *AdminHandler) UpdateUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respErr(c, "invalid id")
		return
	}
	var req updateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	u, err := h.userSvc.Update(ctx, id, req.Name, req.FeeRate, req.Status)
	if err != nil {
		respErr(c, err.Error())
		return
	}
	respOK(c, u)
}

// ---- Products ----

// ListProducts returns all products platform-wide.
func (h *AdminHandler) ListProducts(c *gin.Context) {
	page, limit := paramPageLimit(c)
	status := c.Query("status")
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	result, err := h.productSvc.ListAll(ctx, page, limit, status)
	if err != nil {
		respErr(c, err.Error())
		return
	}
	respOK(c, result)
}

type adminCreateProductReq struct {
	UserID      string   `json:"user_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	NotifyURL   string   `json:"notify_url"`
	ReturnURL   string   `json:"return_url"`
	FeeRate     *float64 `json:"fee_rate"`
}

// CreateProduct admin-creates a product for a user.
func (h *AdminHandler) CreateProduct(c *gin.Context) {
	var req adminCreateProductReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}
	uid, err := uuid.Parse(req.UserID)
	if err != nil {
		respErr(c, "invalid user_id")
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	info, err := h.productSvc.Create(ctx, productSvc.CreateParams{
		UserID:      uid,
		Name:        req.Name,
		Description: req.Description,
		NotifyURL:   req.NotifyURL,
		ReturnURL:   req.ReturnURL,
		FeeRate:     req.FeeRate,
	})
	if err != nil {
		respErr(c, err.Error())
		return
	}
	respOK(c, info)
}

type updateProductReq struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	NotifyURL   *string  `json:"notify_url"`
	ReturnURL   *string  `json:"return_url"`
	FeeRate     *float64 `json:"fee_rate"`
	ClearFee    bool     `json:"clear_fee"`
	Status      *string  `json:"status"`
}

// UpdateProduct admin-updates any product.
func (h *AdminHandler) UpdateProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respErr(c, "invalid id")
		return
	}
	var req updateProductReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	info, err := h.productSvc.Update(ctx, id, nil, productSvc.UpdateParams{
		Name:        req.Name,
		Description: req.Description,
		NotifyURL:   req.NotifyURL,
		ReturnURL:   req.ReturnURL,
		FeeRate:     req.FeeRate,
		ClearFee:    req.ClearFee,
		Status:      req.Status,
	})
	if err != nil {
		respErr(c, err.Error())
		return
	}
	respOK(c, info)
}

// RegenerateProductPkey rolls a product's pkey (admin override).
func (h *AdminHandler) RegenerateProductPkey(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respErr(c, "invalid id")
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	newKey, err := h.productSvc.RegeneratePkey(ctx, id, nil)
	if err != nil {
		respErr(c, err.Error())
		return
	}
	respOK(c, gin.H{"pkey": newKey})
}

// ---- Orders ----

type orderItem struct {
	ID          string     `json:"id"`
	OrderNo     string     `json:"order_no"`
	UserName    string     `json:"user_name"`
	ProductName string     `json:"product_name"`
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

// ListOrders returns a paginated list of all orders, optionally filtered by
// status, user_id, or product_id.
func (h *AdminHandler) ListOrders(c *gin.Context) {
	page, limit := paramPageLimit(c)
	status := c.Query("status")
	userIDStr := c.Query("user_id")
	productIDStr := c.Query("product_id")

	q := h.ent.Order.Query().WithUser().WithProduct()
	if status != "" {
		q.Where(order.StatusEQ(order.Status(status)))
	}
	if userIDStr != "" {
		uid, err := uuid.Parse(userIDStr)
		if err != nil {
			respErr(c, "invalid user_id")
			return
		}
		q.Where(order.UserIDEQ(uid))
	}
	if productIDStr != "" {
		pid, err := uuid.Parse(productIDStr)
		if err != nil {
			respErr(c, "invalid product_id")
			return
		}
		q.Where(order.ProductIDEQ(pid))
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
		userName, productName := "", ""
		if o.Edges.User != nil {
			userName = o.Edges.User.Name
		}
		if o.Edges.Product != nil {
			productName = o.Edges.Product.Name
		}
		items = append(items, orderItem{
			ID:          o.ID.String(),
			OrderNo:     o.OrderNo,
			UserName:    userName,
			ProductName: productName,
			Type:        string(o.Type),
			Amount:      o.Amount,
			FeeOfficial: o.FeeOfficial,
			FeePlatform: o.FeePlatform,
			NetAmount:   o.NetAmount,
			TradeNo:     o.TradeNo,
			Status:      string(o.Status),
			NotifyURL:   o.NotifyURL,
			PaidAt:      o.PaidAt,
			CreatedAt:   o.CreatedAt,
		})
	}
	totalPages := (total + limit - 1) / limit
	respOK(c, orderListResp{Items: items, Total: total, Page: page, Limit: limit, TotalPages: totalPages})
}

// ---- Withdraws ----

type withdrawItem struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	UserName    string    `json:"user_name"`
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

// ListWithdraws returns a paginated list of withdrawal records with user names.
func (h *AdminHandler) ListWithdraws(c *gin.Context) {
	page, limit := paramPageLimit(c)
	status := c.Query("status")
	q := h.ent.Withdraw.Query().WithUser()
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
		userName := ""
		if w.Edges.User != nil {
			userName = w.Edges.User.Name
		}
		items = append(items, withdrawItem{
			ID:          w.ID.String(),
			UserID:      w.UserID.String(),
			UserName:    userName,
			Amount:      w.Amount,
			AccountInfo: w.AccountInfo,
			Status:      string(w.Status),
			Remark:      w.Remark,
			CreatedAt:   w.CreatedAt,
		})
	}
	totalPages := (total + limit - 1) / limit
	respOK(c, withdrawListResp{Items: items, Total: total, Page: page, Limit: limit, TotalPages: totalPages})
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

// GetConfigs returns all platform configuration entries as a flat key→value map.
func (h *AdminHandler) GetConfigs(c *gin.Context) {
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	configs, err := h.ent.PlatformConfig.Query().All(ctx)
	if err != nil {
		respErr(c, err.Error())
		return
	}
	result := make(map[string]string, len(configs))
	for _, cfg := range configs {
		result[cfg.Key] = cfg.Value
	}
	respOK(c, result)
}

// UpdateConfigs upserts platform configuration entries.
func (h *AdminHandler) UpdateConfigs(c *gin.Context) {
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request: "+err.Error())
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	for key, raw := range req {
		value := stringifyConfigValue(raw)
		existing, err := h.ent.PlatformConfig.Query().
			Where(entcfg.KeyEQ(key)).
			First(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
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

// stringifyConfigValue converts a JSON-decoded value to a string for storage.
func stringifyConfigValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", x)
	}
}
