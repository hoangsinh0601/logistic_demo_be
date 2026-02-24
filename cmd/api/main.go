package main

import (
	swaggerDocs "backend/api/swagger" // swagger docs
	"backend/internal/database"
	"backend/internal/handler"
	"backend/internal/middleware"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/websocket"
	"log"
	"net/http"
	"os"
	"strings"
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
var (
	isReady = false
)

func main() {
	// 1. Setup Environment
	if os.Getenv("RENDER") == "" {
		if err := godotenv.Load("configs/.env"); err != nil {
			log.Println("Note: No configs/.env file found, using system environment variables")
		}
	}

	port := getEnv("PORT", "8080")

	// 2. Initialize Gin Router Early
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()

	// 3. Configure CORS (Cleanup empty FRONTEND_URL)
	corsConfig := cors.DefaultConfig()
	origins := []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
		"https://logistic-demo-fe.onrender.com",
	}
	if feURL := os.Getenv("FRONTEND_URL"); feURL != "" {
		origins = append(origins, feURL)
	}
	corsConfig.AllowOrigins = origins
	corsConfig.AllowCredentials = true
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "Accept"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	router.Use(cors.New(corsConfig))

	// 4. Immediate Routes (Health & Swagger)
	if externalURL := os.Getenv("RENDER_EXTERNAL_URL"); externalURL != "" {
		swaggerDocs.SwaggerInfo.Host = strings.TrimPrefix(externalURL, "https://")
	} else {
		swaggerDocs.SwaggerInfo.Host = getEnv("SWAGGER_HOST", "localhost:"+port)
	}

	router.GET("/health", func(c *gin.Context) {
		if !isReady {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "starting",
				"message": "Server is initializing database connection",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":  "UP",
			"message": "Server is healthy and connected to database",
		})
	})
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 5. Background Initialization (Non-blocking)
	go func() {
		dsn := buildDSN()
		var db *gorm.DB
		var err error

		// Retry logic: 5 attempts
		for i := 1; i <= 5; i++ {
			log.Printf("Connecting to Database (Attempt %d/5)...", i)
			db, err = database.NewConnection(dsn)
			if err == nil {
				break
			}
			log.Printf("Database connection failed: %v. Retrying in %d seconds...", err, i*2)
			time.Sleep(time.Duration(i*2) * time.Second)
		}

		if err != nil {
			log.Printf("CRITICAL: Failed to connect to database after 5 attempts. Background services will not be available.")
			return
		}
		log.Println("Connected to Database successfully.")

		// Init Services & Handlers inside goroutine
		wsHub := websocket.NewHub()
		go wsHub.Run()

		userRepo := repository.NewUserRepository(db)
		userService := service.NewUserService(userRepo)
		inventoryService := service.NewInventoryService(db, wsHub)
		auditService := service.NewAuditService(db)
		statisticsService := service.NewStatisticsService(db)
		taxService := service.NewTaxService(db)
		expenseService := service.NewExpenseService(db, taxService)

		userHandler := handler.NewUserHandler(userService)
		inventoryHandler := handler.NewInventoryHandler(inventoryService)
		auditHandler := handler.NewAuditHandler(auditService)
		statisticsHandler := handler.NewStatisticsHandler(statisticsService)
		taxHandler := handler.NewTaxHandler(taxService)
		expenseHandler := handler.NewExpenseHandler(expenseService)

		// Register API Routes
		apiGroup := router.Group("")
		userHandler.RegisterRoutes(apiGroup)
		inventoryHandler.RegisterRoutes(apiGroup)
		auditHandler.RegisterRoutes(apiGroup)
		statisticsHandler.RegisterRoutes(apiGroup)
		taxHandler.RegisterRoutes(apiGroup)
		expenseHandler.RegisterRoutes(apiGroup)

		// Create WebSocket endpoint
		router.GET("/ws", func(c *gin.Context) {
			websocket.ServeWs(wsHub, c, middleware.GetJWTSecret())
		})

		isReady = true
		log.Println("Background initialization completed. All routes registered.")
	}()

	// 6. Start Server (Must be last and non-blocking relative to DB)
	log.Printf("Server is starting and listening on port %s...", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
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
