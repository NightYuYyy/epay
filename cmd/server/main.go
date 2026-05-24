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
	demo "epay/internal/handler/demo"
	"epay/internal/handler/easypay"
	"epay/internal/handler/middleware"
	userHandler "epay/internal/handler/user"
	"epay/internal/provider"
	_ "epay/internal/provider/alipay"
	_ "epay/internal/provider/wxpay"
	redisutil "epay/internal/redis"
	adminSvc "epay/internal/service/admin"
	feeSvc "epay/internal/service/fee"
	paymentSvc "epay/internal/service/payment"
	productSvc "epay/internal/service/product"
	settlementSvc "epay/internal/service/settlement"
	userSvc "epay/internal/service/user"

	"github.com/gin-gonic/gin"
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
	userService := userSvc.NewService(dbClient)
	productService := productSvc.NewService(dbClient)

	// Initialize Redis
	rdb, err := redisutil.NewClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		log.Printf("[main] Redis not available (non-fatal): %v", err)
		rdb = nil
	} else {
		defer rdb.Close()
		log.Println("[main] redis connected")
	}

	// Initialize settlement and payment services
	settlementService := settlementSvc.New(dbClient, rdb)
	feeService := feeSvc.New(dbClient, rdb)
	paymentService := paymentSvc.NewPaymentService(dbClient, rdb, cfg, paymentSvc.WithSettlementApplier(feeService))
	paymentCtx, stopPaymentWorkers := context.WithCancel(context.Background())
	defer stopPaymentWorkers()
	paymentService.StartExpiryScanner(paymentCtx)

	// Initialize admin handler
	adminHandler := admin.NewHandler(dbClient, adminService, userService, productService, settlementService)

	// Initialize user handler (self-service portal)
	usrHandler := userHandler.NewHandler(dbClient, userService, productService, settlementService, cfg)

	// Initialize Gin
	r := gin.Default()

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	demo.RegisterRoutes(r, demo.NewNotifyStore(200))
	// Demo: manual sync endpoint — queries Alipay TradeQuery API and syncs order status
	r.GET("/demo/sync", func(c *gin.Context) {
		outTradeNo := strings.TrimSpace(c.Query("out_trade_no"))
		if outTradeNo == "" {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "out_trade_no is required"})
			return
		}
		ord, err := paymentService.QueryOrder(c.Request.Context(), outTradeNo)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": -1, "msg": "sync failed: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": gin.H{
			"status":  string(ord.Status),
			"paid_at": ord.PaidAt,
		}})
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
		adminGroup.GET("/users", adminHandler.ListUsers)
		adminGroup.POST("/users", adminHandler.CreateUser)
		adminGroup.PUT("/users/:id", adminHandler.UpdateUser)
		adminGroup.GET("/products", adminHandler.ListProducts)
		adminGroup.POST("/products", adminHandler.CreateProduct)
		adminGroup.PUT("/products/:id", adminHandler.UpdateProduct)
		adminGroup.POST("/products/:id/regenerate-pkey", adminHandler.RegenerateProductPkey)
		adminGroup.GET("/orders", adminHandler.ListOrders)
		adminGroup.GET("/withdraws", adminHandler.ListWithdraws)
		adminGroup.POST("/withdraws/:id/approve", adminHandler.ApproveWithdraw)
		adminGroup.POST("/withdraws/:id/reject", adminHandler.RejectWithdraw)
		adminGroup.POST("/withdraws/:id/confirm", adminHandler.ConfirmWithdraw)
		adminGroup.GET("/configs", adminHandler.GetConfigs)
		adminGroup.PUT("/configs", adminHandler.UpdateConfigs)
	}

	// ========================
	// User self-service portal
	// ========================
	userSelfGroup := r.Group("/api/user")
	{
		userSelfGroup.POST("/login", usrHandler.Login)
		userSelfGroup.POST("/register", usrHandler.Register)
	}
	userAuthGroup := userSelfGroup.Group("")
	userAuthGroup.Use(middleware.UserAuth(cfg))
	{
		userAuthGroup.GET("/profile", usrHandler.Profile)
		userAuthGroup.GET("/balance", usrHandler.Balance)
		userAuthGroup.GET("/orders", usrHandler.ListOrders)
		userAuthGroup.GET("/withdraws", usrHandler.ListWithdraws)
		userAuthGroup.POST("/withdraws", usrHandler.RequestWithdraw)
		userAuthGroup.GET("/products", usrHandler.ListProducts)
		userAuthGroup.POST("/products", usrHandler.CreateProduct)
		userAuthGroup.GET("/products/:id/secret", usrHandler.GetProductSecret)
		userAuthGroup.PUT("/products/:id", usrHandler.UpdateProduct)
		userAuthGroup.POST("/products/:id/regenerate-pkey", usrHandler.RegenerateProductPkey)
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
		// Root serves the public SPA landing page with user registration/login entry points.
		r.GET("/", func(c *gin.Context) {
			c.File(indexHTML)
		})
		// Catch-all: serve index.html for unmatched browser routes (deep links
		// like /admin/dashboard, /user/login). API 404s stay JSON; asset
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
			if fp == spaDir || strings.HasPrefix(fp, spaDir+string(filepath.Separator)) {
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
