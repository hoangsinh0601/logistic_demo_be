package handler

import (
	"net/http"
	"strconv"

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
	{
		inventory.GET("/products", middleware.RequirePermission("inventory.read"), h.GetProducts)
		inventory.POST("/products", middleware.RequirePermission("inventory.write"), h.CreateProduct)
		inventory.PUT("/products/:id", middleware.RequirePermission("inventory.write"), h.UpdateProduct)
		inventory.DELETE("/products/:id", middleware.RequirePermission("inventory.write"), h.DeleteProduct)
		inventory.POST("/orders", middleware.RequirePermission("inventory.write"), h.CreateOrder)
	}
}

// GetProducts handles retrieving paginated inventory statuses
// @Summary      Get products
// @Description  Retrieves a paginated list of products with current stock
// @Tags         inventory
// @Security     BearerAuth
// @Produce      json
// @Param        page    query     int     false  "Page number (default 1)"
// @Param        limit   query     int     false  "Number of items per page (default 20)"
// @Param        search  query     string  false  "Search by product name"
// @Success      200    {object}  response.Response{data=object}
// @Failure      500    {object}  response.Response
// @Router       /api/products [get]
func (h *InventoryHandler) GetProducts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	search := c.Query("search")

	products, total, err := h.inventoryService.GetProducts(c.Request.Context(), page, limit, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, "Failed to retrieve products: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, map[string]interface{}{
		"products": products,
		"total":    total,
		"page":     page,
		"limit":    limit,
	}))
}

// CreateProduct creates a new inventory product entry
// @Summary      Create product
// @Description  Creates a new product entry in the inventory system
// @Tags         inventory
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        payload  body      service.CreateProductRequest  true  "Create Product Payload"
// @Success      201      {object}  response.Response{data=service.ProductResponse}
// @Failure      400      {object}  response.Response
// @Failure      500      {object}  response.Response
// @Router       /api/products [post]
func (h *InventoryHandler) CreateProduct(c *gin.Context) {
	var req service.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	userID := c.GetString("userID")

	product, err := h.inventoryService.CreateProduct(c.Request.Context(), userID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, product))
}

// UpdateProduct updates an existing product's metadata
// @Summary      Update product
// @Description  Updates an existing product's details by ID
// @Tags         inventory
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id       path      string                        true  "Product ID"
// @Param        payload  body      service.UpdateProductRequest  true  "Update Product Payload"
// @Success      200      {object}  response.Response{data=service.ProductResponse}
// @Failure      400      {object}  response.Response
// @Failure      500      {object}  response.Response
// @Router       /api/products/{id} [put]
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

	userID := c.GetString("userID")

	product, err := h.inventoryService.UpdateProduct(c.Request.Context(), userID, id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, product))
}

// DeleteProduct removes a product entry softly
// @Summary      Delete product
// @Description  Soft deletes a product by ID
// @Tags         inventory
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Product ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /api/products/{id} [delete]
func (h *InventoryHandler) DeleteProduct(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Product ID is missing"))
		return
	}

	userID := c.GetString("userID")

	err := h.inventoryService.DeleteProduct(c.Request.Context(), userID, id)
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
	userID := c.GetString("userID")
	err := h.inventoryService.CreateOrder(c.Request.Context(), userID, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, "Order created successfully"))
}
