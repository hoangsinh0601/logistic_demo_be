package main

import (
	swaggerDocs "backend/api/swagger" // swagger docs
	"backend/internal/database"
	"backend/internal/handler"
	"backend/internal/middleware"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/websocket"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

// @title           User Management API
// @version         1.0
// @description     This is an API for managing users (CRUD) with Clean Architecture.
// @host            localhost:8080
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {
	// 1. Setup Environment
	if os.Getenv("RENDER") == "" {
		if err := godotenv.Load("configs/.env"); err != nil {
			log.Println("Note: No configs/.env file found, using system environment variables")
		}
	}

	port := getEnv("PORT", "8080")

	// 2. Initialize Gin Router
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()

	// 3. Configure CORS (uses CORS_ORIGINS env variable)
	corsConfig := cors.DefaultConfig()
	origins := []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
	}
	if corsOrigins := os.Getenv("CORS_ORIGINS"); corsOrigins != "" {
		for _, origin := range strings.Split(corsOrigins, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				origins = append(origins, origin)
			}
		}
	}
	if feURL := os.Getenv("FRONTEND_URL"); feURL != "" {
		origins = append(origins, feURL)
	}
	corsConfig.AllowOrigins = origins
	corsConfig.AllowCredentials = true
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "Accept"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}
	corsConfig.ExposeHeaders = []string{"Content-Length", "Content-Type"}
	corsConfig.MaxAge = 12 * time.Hour
	router.Use(cors.New(corsConfig))

	// 4. Swagger Configuration
	if externalURL := os.Getenv("RENDER_EXTERNAL_URL"); externalURL != "" {
		swaggerDocs.SwaggerInfo.Host = strings.TrimPrefix(externalURL, "https://")
	} else {
		swaggerDocs.SwaggerInfo.Host = getEnv("SWAGGER_HOST", "localhost:"+port)
	}

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "UP",
			"message": "Server is healthy and connected to database",
		})
	})
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 5. Database Initialization (synchronous with retry)
	dsn := buildDSN()
	db, err := initDatabase(dsn)
	if err != nil {
		log.Fatalf("CRITICAL: Failed to connect to database after 5 attempts: %v", err)
	}
	log.Println("Connected to Database successfully.")

	// 6. Initialize Repositories
	txManager := repository.NewTransactionManager(db)
	userRepo := repository.NewUserRepository(db)
	productRepo := repository.NewProductRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	taxRuleRepo := repository.NewTaxRuleRepository(db)
	expenseRepo := repository.NewExpenseRepository(db)
	invoiceRepo := repository.NewInvoiceRepository(db)
	approvalRepo := repository.NewApprovalRepository(db)
	roleRepo := repository.NewRoleRepository(db)
	invTxRepo := repository.NewInventoryTxRepository(db)
	statsRepo := repository.NewStatisticsRepository(db)
	revenueRepo := repository.NewRevenueRepository(db)
	partnerRepo := repository.NewPartnerRepository(db)

	// 7. Initialize Services & Handlers
	wsHub := websocket.NewHub()
	go wsHub.Run()

	userService := service.NewUserService(userRepo)
	inventoryService := service.NewInventoryService(productRepo, orderRepo, approvalRepo, auditRepo, partnerRepo, txManager, wsHub)
	auditService := service.NewAuditService(auditRepo)
	statisticsService := service.NewStatisticsService(statsRepo)
	taxService := service.NewTaxService(taxRuleRepo, auditRepo)
	expenseService := service.NewExpenseService(expenseRepo, auditRepo, approvalRepo, txManager, taxService)
	roleService := service.NewRoleService(roleRepo, txManager)
	invoiceService := service.NewInvoiceService(invoiceRepo, taxRuleRepo, orderRepo, expenseRepo, partnerRepo, txManager)
	revenueService := service.NewRevenueService(revenueRepo)
	approvalService := service.NewApprovalService(approvalRepo, auditRepo, orderRepo, productRepo, expenseRepo, invoiceRepo, taxRuleRepo, invTxRepo, partnerRepo, txManager)
	partnerService := service.NewPartnerService(partnerRepo, txManager)

	// Seed default roles and permissions
	if seedErr := roleService.SeedDefaultRolesAndPermissions(context.Background()); seedErr != nil {
		log.Printf("WARNING: Failed to seed roles/permissions: %v", seedErr)
	}

	// Init permission middleware with DB for RequirePermission
	middleware.InitPermissionMiddleware(db)

	userHandler := handler.NewUserHandler(userService)
	inventoryHandler := handler.NewInventoryHandler(inventoryService)
	auditHandler := handler.NewAuditHandler(auditService)
	statisticsHandler := handler.NewStatisticsHandler(statisticsService)
	taxHandler := handler.NewTaxHandler(taxService)
	expenseHandler := handler.NewExpenseHandler(expenseService)
	roleHandler := handler.NewRoleHandler(roleService)
	invoiceHandler := handler.NewInvoiceHandler(invoiceService, revenueService)
	approvalHandler := handler.NewApprovalHandler(approvalService)
	partnerHandler := handler.NewPartnerHandler(partnerService)

	// 8. Register API Routes (synchronous — guaranteed available before serving)
	apiGroup := router.Group("")
	userHandler.RegisterRoutes(apiGroup)
	inventoryHandler.RegisterRoutes(apiGroup)
	auditHandler.RegisterRoutes(apiGroup)
	statisticsHandler.RegisterRoutes(apiGroup)
	taxHandler.RegisterRoutes(apiGroup)
	expenseHandler.RegisterRoutes(apiGroup)
	roleHandler.RegisterRoutes(apiGroup)
	invoiceHandler.RegisterRoutes(apiGroup)
	approvalHandler.RegisterRoutes(apiGroup)
	partnerHandler.RegisterRoutes(apiGroup)

	// WebSocket endpoint
	router.GET("/ws", func(c *gin.Context) {
		websocket.ServeWs(wsHub, c, middleware.GetJWTSecret())
	})

	log.Println("All routes registered successfully.")

	// 9. Graceful Shutdown with http.Server
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server is listening on port %s...", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal (SIGINT or SIGTERM)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Give outstanding requests 10 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited gracefully.")
}

// initDatabase connects to the database with retry logic (5 attempts)
func initDatabase(dsn string) (*gorm.DB, error) {
	var db *gorm.DB
	var err error

	for i := 1; i <= 5; i++ {
		log.Printf("Connecting to Database (Attempt %d/5)...", i)
		db, err = database.NewConnection(dsn)
		if err == nil {
			return db, nil
		}
		log.Printf("Database connection failed: %v. Retrying in %d seconds...", err, i*2)
		time.Sleep(time.Duration(i*2) * time.Second)
	}

	return nil, err
}

// buildDSN constructs the connection string support DATABASE_URL or individual variables
func buildDSN() string {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dbHost := getEnv("DB_HOST", "localhost")
		dbPort := getEnv("DB_PORT", "5432")
		dbUser := getEnv("DB_USER", "postgres")
		dbPassword := getEnv("DB_PASSWORD", "postgres")
		dbName := getEnv("DB_NAME", "postgres")
		dbSslMode := getEnv("DB_SSLMODE", "disable")

		dsn = "postgres://" + dbUser + ":" + dbPassword + "@" + dbHost + ":" + dbPort + "/" + dbName + "?sslmode=" + dbSslMode
	} else {
		if !strings.Contains(dsn, "sslmode=") {
			if strings.Contains(dsn, "?") {
				dsn += "&sslmode=require"
			} else {
				dsn += "?sslmode=require"
			}
		}
	}
	return dsn
}

// getEnv retrieves env with fallback
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
