package main

import (
	_ "backend/api/swagger" // swagger docs
	"backend/internal/database"
	"backend/internal/handler"
	"backend/internal/middleware"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/websocket"
	"log"
	"os"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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
	// 1. Chỉ load .env nếu chạy ở local, trên Render sẽ bỏ qua bước này
	if os.Getenv("RENDER") == "" {
		if err := godotenv.Load("configs/.env"); err != nil {
			log.Println("Note: No configs/.env file found, using system environment variables")
		}
	}

	// 2. Cấu hình Database DSN
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		// Fallback build DSN thủ công (thường dùng cho local)
		dbHost := getEnv("DB_HOST", "localhost")
		dbPort := getEnv("DB_PORT", "5432")
		dbUser := getEnv("DB_USER", "postgres")
		dbPassword := getEnv("DB_PASSWORD", "postgres")
		dbName := getEnv("DB_NAME", "postgres")
		dbSslMode := getEnv("DB_SSLMODE", "disable")

		dsn = "postgres://" + dbUser + ":" + dbPassword + "@" + dbHost + ":" + dbPort + "/" + dbName + "?sslmode=" + dbSslMode
		log.Println("Constructed DSN from individual variables.")
	} else {
		// Fix lỗi nếu Render đưa link postgres:// nhưng thư viện Go yêu cầu sslmode
		if !strings.Contains(dsn, "sslmode=") {
			if strings.Contains(dsn, "?") {
				dsn += "&sslmode=require"
			} else {
				dsn += "?sslmode=require"
			}
		}
		log.Println("Using DATABASE_URL from environment.")
	}

	// 3. Kết nối Database (Bước này dễ gây crash làm Render báo lỗi Port)
	log.Println("Connecting to Database...")
	db, err := database.NewConnection(dsn)
	if err != nil {
		log.Printf("CRITICAL: Database connection failed: %v", err)
		// Thay vì Fatalf ngay, ta có thể log lỗi để Render không bị loop restart quá nhanh
		os.Exit(1)
	}
	log.Println("Connected to Database successfully.")

	// 4. Khởi tạo Services & Handlers
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

	// 5. Cấu hình Gin
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()

	// CORS nâng cao (Thêm domain của frontend trên Render vào đây)
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
		os.Getenv("FRONTEND_URL"), // Add your Render frontend URL here
	}
	// Xóa các entry trống nếu FRONTEND_URL chưa set
	corsConfig.AllowCredentials = true
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "Accept"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	router.Use(cors.New(corsConfig))

	// Routes
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "UP", "message": "Server is healthy"})
	})
	router.GET("/ws", func(c *gin.Context) {
		websocket.ServeWs(wsHub, c, middleware.GetJWTSecret())
	})

	userHandler.RegisterRoutes(router.Group(""))
	inventoryHandler.RegisterRoutes(router.Group(""))
	auditHandler.RegisterRoutes(router.Group(""))
	statisticsHandler.RegisterRoutes(router.Group(""))
	taxHandler.RegisterRoutes(router.Group(""))
	expenseHandler.RegisterRoutes(router.Group(""))

	// 6. Chạy Server với PORT từ Render
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server is starting and listening on port %s...", port)
	// Render yêu cầu bind vào 0.0.0.0
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// Hàm hỗ trợ lấy env với giá trị mặc định
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
