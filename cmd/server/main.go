package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"epay/internal/config"
	"epay/internal/database"
	admin "epay/internal/handler/admin"
	"epay/internal/handler/easypay"
	merchant "epay/internal/handler/merchant"
	"epay/internal/handler/middleware"
	"epay/internal/provider"
	_ "epay/internal/provider/alipay"
	_ "epay/internal/provider/wxpay"
	redisutil "epay/internal/redis"
	adminSvc "epay/internal/service/admin"
	merchantSvc "epay/internal/service/merchant"
	paymentSvc "epay/internal/service/payment"
	settlementSvc "epay/internal/service/settlement"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func main() {
	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("[main] config error: %v", err)
	}

	// Initialize database
	dsn := cfg.Database.DSN()
	dbClient, err := database.NewClient(dsn)
	if err != nil {
		log.Fatalf("[main] failed to connect database: %v", err)
	}
	defer dbClient.Close()
	log.Println("[main] database connected")

	// Seed default admin
	adminService := adminSvc.NewService(dbClient, cfg)
	created, err := adminService.SeedDefaultAdmin(context.Background())
	if err != nil {
		log.Fatalf("[main] seed admin error: %v", err)
	}
	if created {
		log.Println("[main] default admin user created (username: admin)")
	}

	// Initialize services
	merchantService := merchantSvc.NewService(dbClient)

	// Initialize Redis
	rdb, err := redisutil.NewClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		log.Printf("[main] Redis not available (non-fatal): %v", err)
		rdb = nil
	} else {
		defer rdb.Close()
		log.Println("[main] redis connected")
	}

	// Initialize settlement service
	settlementService := settlementSvc.New(dbClient, rdb)
	paymentService := paymentSvc.NewPaymentService(dbClient, rdb, cfg)

	// Initialize admin handler
	adminHandler := admin.NewHandler(dbClient, adminService, merchantService, settlementService)

	// Initialize merchant handler
	merchantHandler := merchant.NewHandler(dbClient, merchantService, settlementService, cfg)

	// Initialize Gin
	r := gin.Default()

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// ========================
	// Public routes (no auth)
	// ========================

	// EasyPay protocol endpoints
	easypayHandler := easypay.NewHandler(
		dbClient,
		easypay.WithPlatformKeys(
			cfg.Platform.RSAPrivateKey,
			cfg.Platform.RSAPublicKey,
			cfg.Platform.SysKey,
		),
		easypay.WithUserRefund(cfg.Platform.UserRefundEnabled),
		easypay.WithPaymentCreator(func(ctx context.Context, req easypay.PaymentCreateRequest) (*easypay.PaymentCreateResponse, error) {
			createResp, err := paymentService.CreateOrder(ctx, paymentSvc.CreateOrderRequest{
				PID:         req.PID,
				OrderNo:     req.OrderNo,
				Type:        provider.PaymentType(req.Type),
				Amount:      req.Amount,
				Subject:     req.Subject,
				NotifyURL:   req.NotifyURL,
				ReturnURL:   req.ReturnURL,
				ClientIP:    req.ClientIP,
				IsMobile:    req.IsMobile,
				Param:       req.Param,
				Device:      req.Device,
				Method:      req.Method,
				SubOpenID:   req.SubOpenID,
				SubAppID:    req.SubAppID,
				AuthCode:    req.AuthCode,
				ExpireAfter: 30 * time.Minute,
			})
			if err != nil {
				return nil, err
			}
			if createResp == nil || createResp.Provider == nil {
				return nil, fmt.Errorf("empty payment response")
			}
			return &easypay.PaymentCreateResponse{
				TradeNo: createResp.Provider.TradeNo,
				PayURL:  createResp.Provider.PayURL,
				QRCode:  createResp.Provider.QRCode,
			}, nil
		}),
	)
	r.POST("/mapi.php", easypayHandler.HandleMapi)
	r.GET("/submit.php", easypayHandler.HandleSubmit)
	r.POST("/submit.php", easypayHandler.HandleSubmit)
	r.GET("/api.php", easypayHandler.HandleAPI)
	r.POST("/api.php", easypayHandler.HandleAPI)
	r.POST("/api/alipay/notify", func(c *gin.Context) {
		rawBody, _ := io.ReadAll(c.Request.Body)
		headers := make(map[string]string, len(c.Request.Header))
		for k, values := range c.Request.Header {
			headers[k] = strings.Join(values, ",")
		}
		resp, err := paymentService.HandleCallback(c.Request.Context(), provider.TypeAlipay, string(rawBody), headers)
		if err != nil {
			log.Printf("[main] alipay notify error: %v", err)
			c.String(http.StatusOK, "failure")
			return
		}
		c.String(http.StatusOK, resp)
	})
	r.POST("/api/wxpay/notify", func(c *gin.Context) {
		rawBody, _ := io.ReadAll(c.Request.Body)
		headers := make(map[string]string, len(c.Request.Header))
		for k, values := range c.Request.Header {
			headers[k] = strings.Join(values, ",")
		}
		resp, err := paymentService.HandleCallback(c.Request.Context(), provider.TypeWxpay, string(rawBody), headers)
		if err != nil {
			log.Printf("[main] wxpay notify error: %v", err)
			c.String(http.StatusOK, "failure")
			return
		}
		c.String(http.StatusOK, resp)
	})

	// Admin login (no auth)
	r.POST("/api/admin/login", adminHandler.Login)

	// ========================
	// Authenticated routes
	// ========================

	adminAuth := middleware.AdminAuth(cfg)

	// Admin API (authenticated)
	adminGroup := r.Group("/api/admin")
	adminGroup.Use(adminAuth)
	{
		adminGroup.POST("/change-password", adminHandler.ChangePassword)
		adminGroup.GET("/dashboard", adminHandler.Dashboard)
		adminGroup.GET("/merchants", adminHandler.ListMerchants)
		adminGroup.POST("/merchants", adminHandler.CreateMerchant)
		adminGroup.PUT("/merchants/:id", adminHandler.UpdateMerchant)
		adminGroup.POST("/merchants/:id/regenerate-key", adminHandler.RegeneratePkey)
		adminGroup.GET("/orders", adminHandler.ListOrders)
		adminGroup.GET("/withdraws", adminHandler.ListWithdraws)
		adminGroup.POST("/withdraws/:id/approve", adminHandler.ApproveWithdraw)
		adminGroup.POST("/withdraws/:id/reject", adminHandler.RejectWithdraw)
		adminGroup.POST("/withdraws/:id/confirm", adminHandler.ConfirmWithdraw)
		adminGroup.GET("/configs", adminHandler.GetConfigs)
		adminGroup.PUT("/configs", adminHandler.UpdateConfigs)
	}

	// Merchant management (admin only)
	merchantGroup := r.Group("/api/merchants")
	merchantGroup.Use(adminAuth)

	// Create merchant
	merchantGroup.POST("", func(c *gin.Context) {
		var req struct {
			Name      string  `json:"name"`
			FeeRate   float64 `json:"fee_rate"`
			NotifyURL string  `json:"notify_url"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "name is required"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		m, err := merchantService.CreateMerchant(ctx, req.Name, req.FeeRate, req.NotifyURL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{"code": 0, "msg": "ok", "data": m})
	})

	// Get merchant by ID
	merchantGroup.GET("/:id", func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid id"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		m, err := merchantService.GetMerchant(ctx, id, false)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": m})
	})

	// Update merchant
	merchantGroup.PUT("/:id", func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid id"})
			return
		}

		var req struct {
			Name      *string  `json:"name"`
			FeeRate   *float64 `json:"fee_rate"`
			Status    *string  `json:"status"`
			NotifyURL *string  `json:"notify_url"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid request"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		m, err := merchantService.UpdateMerchant(ctx, id, req.Name, req.FeeRate, req.Status, req.NotifyURL)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": m})
	})

	// List merchants
	merchantGroup.GET("", func(c *gin.Context) {
		page := 1
		if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
			page = p
		}
		limit := 20
		if l, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil && l > 0 && l <= 100 {
			limit = l
		}
		status := c.Query("status")

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		result, err := merchantService.ListMerchants(ctx, page, limit, status)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": result})
	})

	// Regenerate pkey
	merchantGroup.POST("/:id/regenerate-pkey", func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid id"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		newPkey, err := merchantService.RegeneratePkey(ctx, id)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "pkey": newPkey})
	})

	// ========================
	// Merchant API (self-service)
	// ========================

	merchantSelfGroup := r.Group("/api/merchant")
	{
		merchantSelfGroup.POST("/login", merchantHandler.Login)
		merchantSelfGroup.POST("/register", merchantHandler.Register)
	}

	merchantAuthGroup := merchantSelfGroup.Group("")
	merchantAuthGroup.Use(middleware.MerchantAuth(cfg))
	{
		merchantAuthGroup.GET("/profile", merchantHandler.Profile)
		merchantAuthGroup.GET("/balance", merchantHandler.Balance)
		merchantAuthGroup.GET("/orders", merchantHandler.ListOrders)
		merchantAuthGroup.GET("/withdraws", merchantHandler.ListWithdraws)
		merchantAuthGroup.POST("/withdraws", merchantHandler.RequestWithdraw)
		merchantAuthGroup.GET("/api-key", merchantHandler.GetAPIKey)
		merchantAuthGroup.PUT("/notify-url", merchantHandler.UpdateNotifyURL)
	}

	// ========================
	// Static assets + SPA fallback
	// ========================
	// Serves the Vue 3 SPA built by `npm run build` into frontend/dist.
	// SPA_DIR env var can point the binary at a non-default location.
	spaDir := os.Getenv("SPA_DIR")
	if spaDir == "" {
		spaDir = "frontend/dist"
	}
	if abs, err := filepath.Abs(spaDir); err == nil {
		spaDir = abs
	}
	indexHTML := filepath.Join(spaDir, "index.html")
	if _, err := os.Stat(indexHTML); err == nil {
		log.Printf("[main] serving SPA from %s", spaDir)
		// Concrete asset/static paths handled directly.
		r.Static("/assets", filepath.Join(spaDir, "assets"))
		// Map common favicon paths if the corresponding file exists.
		for _, fav := range []string{"favicon.ico", "favicon.svg", "favicon.png"} {
			path := filepath.Join(spaDir, fav)
			if _, err := os.Stat(path); err == nil {
				r.StaticFile("/"+fav, path)
			}
		}
		// Root → admin login (matches existing Vue router default).
		r.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusFound, "/admin/login")
		})
		// Catch-all: serve index.html for unmatched browser routes (deep links
		// like /admin/dashboard, /merchant/login). API 404s stay JSON; asset
		// 404s (paths with file extensions) return JSON 404 too so the browser
		// doesn't get HTML for a missing image / script.
		r.NoRoute(func(c *gin.Context) {
			p := c.Request.URL.Path
			if strings.HasPrefix(p, "/api/") ||
				p == "/mapi.php" || p == "/submit.php" || p == "/api.php" {
				c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "not found"})
				return
			}
			fp := filepath.Join(spaDir, filepath.Clean(p))
			if strings.HasPrefix(fp, spaDir) {
				if info, err := os.Stat(fp); err == nil && !info.IsDir() {
					c.File(fp)
					return
				}
			}
			if ext := filepath.Ext(p); ext != "" && ext != ".html" {
				c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "asset not found"})
				return
			}
			c.File(indexHTML)
		})
	} else {
		log.Printf("[main] SPA dist not found at %s; UI routes will 404. Build with: cd frontend && npm run build", spaDir)
	}

	// Determine port: env override > config > default 8080
	port := "8080"
	if cfg.Server.Port > 0 {
		port = strconv.Itoa(cfg.Server.Port)
	}
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	log.Printf("Server starting on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
