// Package user provides HTTP handlers for the user self-service portal:
// login, register, profile, balance, orders, withdraws, and product CRUD.
package user

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
	productSvc "epay/internal/service/product"
	settlementSvc "epay/internal/service/settlement"
	userSvc "epay/internal/service/user"

	"entgo.io/ent/dialect/sql"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Handler bundles every user-facing HTTP endpoint.
type Handler struct {
	ent           *ent.Client
	userSvc       *userSvc.Service
	productSvc    *productSvc.Service
	settlementSvc *settlementSvc.SettlementService
	cfg           *config.Config
}

// NewHandler constructs a Handler with the required services.
func NewHandler(entClient *ent.Client, us *userSvc.Service, ps *productSvc.Service, ss *settlementSvc.SettlementService, cfg *config.Config) *Handler {
	return &Handler{
		ent:           entClient,
		userSvc:       us,
		productSvc:    ps,
		settlementSvc: ss,
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
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return page, limit
}

func timeoutCtx(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), 10*time.Second)
}

func (h *Handler) currentUserID(c *gin.Context) (uuid.UUID, bool) {
	value, ok := c.Get("user_id")
	if !ok {
		respErr(c, "invalid user session")
		return uuid.Nil, false
	}
	uid, ok := value.(uuid.UUID)
	if !ok || uid == uuid.Nil {
		respErr(c, "invalid user session")
		return uuid.Nil, false
	}
	return uid, true
}

// ---- JWT claims ----

// Claims is the JWT payload for user sessions.
type Claims struct {
	jwt.RegisteredClaims
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
}

func (h *Handler) issueToken(u *userSvc.Info) (string, time.Time, error) {
	expireHours := h.cfg.JWT.ExpireHour
	if expireHours <= 0 {
		expireHours = 24
	}
	expiresAt := time.Now().Add(time.Duration(expireHours) * time.Hour)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
		UserID: u.ID,
		Email:  u.Email,
		Role:   "user",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(h.cfg.JWT.Secret))
	return signed, expiresAt, err
}

// ---- Login / Register (no auth) ----

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login authenticates a user by email + password and returns a JWT token.
func (h *Handler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	u, err := h.userSvc.VerifyCredential(ctx, req.Email, req.Password)
	if err != nil {
		respFail(c, -1, err.Error())
		return
	}
	token, expiresAt, err := h.issueToken(u)
	if err != nil {
		respErr(c, "issue token failed")
		return
	}
	respOK(c, gin.H{
		"token":     token,
		"expire_at": expiresAt,
		"user":      u,
	})
}

type registerReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Register creates a self-service user account.
func (h *Handler) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	u, err := h.userSvc.Register(ctx, req.Email, req.Password, req.Name, nil)
	if err != nil {
		respErr(c, err.Error())
		return
	}
	respOK(c, u)
}

// ---- Profile / Balance ----

// Profile returns the authenticated user's profile information.
func (h *Handler) Profile(c *gin.Context) {
	uid, ok := h.currentUserID(c)
	if !ok {
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	u, err := h.userSvc.Get(ctx, uid)
	if err != nil {
		respErr(c, "user not found")
		return
	}
	respOK(c, u)
}

// Balance returns the user's settlement balance.
func (h *Handler) Balance(c *gin.Context) {
	uid, ok := h.currentUserID(c)
	if !ok {
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	sett, err := h.ent.Settlement.Query().Where(settlement.UserIDEQ(uid)).First(ctx)
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

// ---- Orders ----

type orderItem struct {
	ID          string     `json:"id"`
	OrderNo     string     `json:"order_no"`
	ProductID   string     `json:"product_id"`
	ProductPid  int        `json:"product_pid"`
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

// ListOrders returns a paginated list of the authenticated user's orders.
func (h *Handler) ListOrders(c *gin.Context) {
	uid, ok := h.currentUserID(c)
	if !ok {
		return
	}
	page, limit := paramPageLimit(c)
	status := c.Query("status")
	productIDStr := c.Query("product_id")

	q := h.ent.Order.Query().Where(order.UserIDEQ(uid))
	if status != "" {
		q.Where(order.StatusEQ(order.Status(status)))
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
		WithProduct().
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
		productPid := 0
		if o.Edges.Product != nil {
			productPid = o.Edges.Product.Pid
		}
		items = append(items, orderItem{
			ID:          o.ID.String(),
			OrderNo:     o.OrderNo,
			ProductID:   o.ProductID.String(),
			ProductPid:  productPid,
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

// ListWithdraws returns a paginated list of the user's withdrawal records.
func (h *Handler) ListWithdraws(c *gin.Context) {
	uid, ok := h.currentUserID(c)
	if !ok {
		return
	}
	page, limit := paramPageLimit(c)
	status := c.Query("status")
	q := h.ent.Withdraw.Query().Where(withdraw.UserIDEQ(uid))
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
	respOK(c, withdrawListResp{Items: items, Total: total, Page: page, Limit: limit, TotalPages: totalPages})
}

type requestWithdrawReq struct {
	Amount      float64 `json:"amount"`
	AccountInfo string  `json:"account_info"`
}

// RequestWithdraw creates a new withdrawal request for the user.
func (h *Handler) RequestWithdraw(c *gin.Context) {
	uid, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req requestWithdrawReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	wd, err := h.settlementSvc.RequestWithdraw(ctx, uid, req.Amount, req.AccountInfo)
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

// ---- Products (self-service CRUD) ----

type productCreateReq struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	NotifyURL   string   `json:"notify_url"`
	ReturnURL   string   `json:"return_url"`
	FeeRate     *float64 `json:"fee_rate"`
}

// ListProducts returns the user's products.
func (h *Handler) ListProducts(c *gin.Context) {
	uid, ok := h.currentUserID(c)
	if !ok {
		return
	}
	page, limit := paramPageLimit(c)
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	res, err := h.productSvc.ListByUser(ctx, uid, page, limit)
	if err != nil {
		respErr(c, err.Error())
		return
	}
	respOK(c, res)
}

// CreateProduct creates a new product owned by the authenticated user.
// The response includes the freshly minted pkey — record it now, the API
// list endpoint will mask it.
func (h *Handler) CreateProduct(c *gin.Context) {
	uid, ok := h.currentUserID(c)
	if !ok {
		return
	}
	var req productCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
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

// GetProductSecret returns pid + plaintext pkey for the user's own product.
func (h *Handler) GetProductSecret(c *gin.Context) {
	uid, ok := h.currentUserID(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respErr(c, "invalid product id")
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	info, err := h.productSvc.GetForUser(ctx, id, uid, true)
	if err != nil {
		respErr(c, "product not found")
		return
	}
	respOK(c, info)
}

type productUpdateReq struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	NotifyURL   *string  `json:"notify_url"`
	ReturnURL   *string  `json:"return_url"`
	FeeRate     *float64 `json:"fee_rate"`
	ClearFee    bool     `json:"clear_fee"`
	Status      *string  `json:"status"`
}

// UpdateProduct mutates a user's own product.
func (h *Handler) UpdateProduct(c *gin.Context) {
	uid, ok := h.currentUserID(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respErr(c, "invalid product id")
		return
	}
	var req productUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respErr(c, "invalid request")
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	info, err := h.productSvc.Update(ctx, id, &uid, productSvc.UpdateParams{
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

// RegenerateProductPkey rolls the pkey for the user's product and returns the new value once.
func (h *Handler) RegenerateProductPkey(c *gin.Context) {
	uid, ok := h.currentUserID(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respErr(c, "invalid product id")
		return
	}
	ctx, cancel := timeoutCtx(c)
	defer cancel()
	newKey, err := h.productSvc.RegeneratePkey(ctx, id, &uid)
	if err != nil {
		respErr(c, err.Error())
		return
	}
	respOK(c, gin.H{"pkey": newKey})
}
