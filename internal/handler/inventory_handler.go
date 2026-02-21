package handler

import (
	"net/http"

	"backend/internal/middleware"
	"backend/internal/service"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type InventoryHandler struct {
	inventoryService service.InventoryService
}

func NewInventoryHandler(inventoryService service.InventoryService) *InventoryHandler {
	return &InventoryHandler{inventoryService: inventoryService}
}

func (h *InventoryHandler) RegisterRoutes(router *gin.RouterGroup) {
	inventory := router.Group("/api")
	inventory.Use(middleware.RequireRole("admin", "staff")) // Protect all inventory routes
	{
		inventory.GET("/products", h.GetProducts)
		inventory.POST("/products", h.CreateProduct)
		inventory.PUT("/products/:id", h.UpdateProduct)
		inventory.DELETE("/products/:id", h.DeleteProduct)
		inventory.POST("/orders", h.CreateOrder)
	}
}

// GetProducts handles retrieving inventory statuses
// @Summary      Get products
// @Description  Retrieves list of products with current stock
// @Tags         inventory
// @Security     BearerAuth
// @Produce      json
// @Success      200    {object}  response.Response{data=[]service.ProductResponse}
// @Failure      500    {object}  response.Response
// @Router       /api/products [get]
func (h *InventoryHandler) GetProducts(c *gin.Context) {
	products, err := h.inventoryService.GetProducts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, "Failed to retrieve products: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, products))
}

// CreateProduct creates a new inventory product entry
func (h *InventoryHandler) CreateProduct(c *gin.Context) {
	var req service.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	product, err := h.inventoryService.CreateProduct(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, product))
}

// UpdateProduct updates an existing product's metadata
func (h *InventoryHandler) UpdateProduct(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Product ID is missing"))
		return
	}

	var req service.UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	product, err := h.inventoryService.UpdateProduct(c.Request.Context(), id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, product))
}

// DeleteProduct removes a product entry softly
func (h *InventoryHandler) DeleteProduct(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Product ID is missing"))
		return
	}

	err := h.inventoryService.DeleteProduct(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, "Product deleted successfully"))
}

// CreateOrder handles EXPORT/IMPORT order creation with DB Transactions
// @Summary      Create inventory order
// @Description  Creates an EXPORT or IMPORT order manipulating stock via strict ACID transactions and broadcasting WS updates
// @Tags         inventory
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        payload  body      service.CreateOrderRequest  true  "Create Order Payload"
// @Success      201      {object}  response.Response
// @Failure      400      {object}  response.Response
// @Failure      500      {object}  response.Response
// @Router       /api/orders [post]
func (h *InventoryHandler) CreateOrder(c *gin.Context) {
	var req service.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	// Strictly invoke Service layer inside WS+Tx orchestrator
	err := h.inventoryService.CreateOrder(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, "Order created successfully"))
}
