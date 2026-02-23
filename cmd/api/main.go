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
	if err := godotenv.Load("configs/.env"); err != nil {
		log.Println("No configs/.env file found or error loading it")
	}

	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbSslMode := os.Getenv("DB_SSLMODE")

	if dbHost == "" {
		dbHost = "localhost"
	}
	if dbPort == "" {
		dbPort = "5432"
	}
	if dbUser == "" {
		dbUser = "postgres"
	}
	if dbPassword == "" {
		dbPassword = "postgres"
	}
	if dbName == "" {
		dbName = "postgres"
	}
	if dbSslMode == "" {
		dbSslMode = "disable"
	}

	dsn := "postgres://" + dbUser + ":" + dbPassword + "@" + dbHost + ":" + dbPort + "/" + dbName + "?sslmode=" + dbSslMode

	db, err := database.NewConnection(dsn)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	log.Println("Connected to PostgreSQL successfully.")

	// Set up WebSocket Hub
	wsHub := websocket.NewHub()
	go wsHub.Run()

	// Set up dependencies (Repository -> Service -> Handler)
	userRepo := repository.NewUserRepository(db)
	userService := service.NewUserService(userRepo)
	inventoryService := service.NewInventoryService(db, wsHub)
	auditService := service.NewAuditService(db)
	statisticsService := service.NewStatisticsService(db)

	// Initialize Handlers
	userHandler := handler.NewUserHandler(userService)
	inventoryHandler := handler.NewInventoryHandler(inventoryService)
	auditHandler := handler.NewAuditHandler(auditService)
	statisticsHandler := handler.NewStatisticsHandler(statisticsService)

	// Set up Gin Router
	router := gin.Default()

	// CORS configuration
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:5174"} // Frontend URL
	corsConfig.AllowCredentials = true
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "Accept"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	router.Use(cors.New(corsConfig))

	// Swagger route
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "OK"})
	})

	// WebSocket endpoint
	router.GET("/ws", func(c *gin.Context) {
		websocket.ServeWs(wsHub, c, middleware.GetJWTSecret())
	})

	// Register API Routes
	// API Routing
	userHandler.RegisterRoutes(router.Group(""))
	inventoryHandler.RegisterRoutes(router.Group(""))
	auditHandler.RegisterRoutes(router.Group(""))
	statisticsHandler.RegisterRoutes(router.Group(""))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server listening on :%s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
