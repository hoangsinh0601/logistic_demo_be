package handler

import (
	"net/http"
	"strconv"
	"time"

	"backend/internal/middleware"
	"backend/internal/service"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type InvoiceHandler struct {
	invoiceService service.InvoiceService
	revenueService service.RevenueService
}

func NewInvoiceHandler(invoiceService service.InvoiceService, revenueService service.RevenueService) *InvoiceHandler {
	return &InvoiceHandler{
		invoiceService: invoiceService,
		revenueService: revenueService,
	}
}

func (h *InvoiceHandler) RegisterRoutes(router *gin.RouterGroup) {
	invoices := router.Group("/api/invoices")
	{
		invoices.POST("", middleware.RequirePermission("invoices.write"), h.CreateInvoice)
		invoices.GET("", middleware.RequirePermission("invoices.read"), h.ListInvoices)
		invoices.PUT("/:id/approve", middleware.RequirePermission("approvals.approve"), h.ApproveInvoice)
		invoices.PUT("/:id/reject", middleware.RequirePermission("approvals.approve"), h.RejectInvoice)
	}

	// Revenue statistics — separate route group
	stats := router.Group("/api/statistics")
	{
		stats.GET("/revenue", middleware.RequirePermission("finance.read"), h.GetRevenueStatistics)
	}
}

// CreateInvoice creates a new invoice from an order or expense
// @Summary      Create invoice
// @Description  Creates a new invoice from an order or expense reference
// @Tags         invoices
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        payload  body      service.CreateInvoiceRequest  true  "Create Invoice Payload"
// @Success      201      {object}  response.Response{data=service.InvoiceResponse}
// @Failure      400      {object}  response.Response
// @Router       /api/invoices [post]
func (h *InvoiceHandler) CreateInvoice(c *gin.Context) {
	var req service.CreateInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	invoice, err := h.invoiceService.CreateInvoice(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, invoice))
}

// ListInvoices returns a paginated list of invoices, optionally filtered by approval_status
// @Summary      List invoices
// @Description  Retrieves a paginated list of invoices, optionally filtered by approval status
// @Tags         invoices
// @Security     BearerAuth
// @Produce      json
// @Param        status  query     string  false  "Filter by approval status (PENDING, APPROVED, REJECTED)"
// @Param        page    query     int     false  "Page number (default 1)"
// @Param        limit   query     int     false  "Number of items per page (default 20)"
// @Success      200     {object}  response.Response{data=object}
// @Failure      500     {object}  response.Response
// @Router       /api/invoices [get]
func (h *InvoiceHandler) ListInvoices(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	filter := service.InvoiceFilter{
		ApprovalStatus: c.Query("status"),
		Page:           page,
		Limit:          limit,
	}

	invoices, total, err := h.invoiceService.ListInvoices(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, map[string]interface{}{
		"invoices": invoices,
		"total":    total,
		"page":     page,
		"limit":    limit,
	}))
}

// ApproveInvoice approves a pending invoice
// @Summary      Approve invoice
// @Description  Approves a pending invoice by ID
// @Tags         invoices
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Invoice ID"
// @Success      200  {object}  response.Response{data=service.InvoiceResponse}
// @Failure      400  {object}  response.Response
// @Router       /api/invoices/{id}/approve [put]
func (h *InvoiceHandler) ApproveInvoice(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")
	userIDStr, _ := userID.(string)

	invoice, err := h.invoiceService.ApproveInvoice(c.Request.Context(), id, userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, invoice))
}

// RejectInvoice rejects a pending invoice
// @Summary      Reject invoice
// @Description  Rejects a pending invoice by ID
// @Tags         invoices
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Invoice ID"
// @Success      200  {object}  response.Response{data=service.InvoiceResponse}
// @Failure      400  {object}  response.Response
// @Router       /api/invoices/{id}/reject [put]
func (h *InvoiceHandler) RejectInvoice(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")
	userIDStr, _ := userID.(string)

	invoice, err := h.invoiceService.RejectInvoice(c.Request.Context(), id, userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, invoice))
}

// GetRevenueStatistics returns revenue data grouped by period (week/month/quarter)
// @Summary      Get revenue statistics
// @Description  Returns revenue, expense, and tax data grouped by time period
// @Tags         statistics
// @Security     BearerAuth
// @Produce      json
// @Param        group_by    query     string  false  "Group by period: week, month, quarter, year (default: month)"
// @Param        start_date  query     string  false  "Start date (RFC3339)"
// @Param        end_date    query     string  false  "End date (RFC3339)"
// @Success      200         {object}  response.Response{data=[]service.RevenueDataPoint}
// @Failure      500         {object}  response.Response
// @Router       /api/statistics/revenue [get]
func (h *InvoiceHandler) GetRevenueStatistics(c *gin.Context) {
	groupBy := c.DefaultQuery("group_by", "month")
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	// Default to current month
	now := time.Now()
	if startDateStr == "" {
		startDateStr = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format(time.RFC3339)
	}
	if endDateStr == "" {
		endDateStr = now.Format(time.RFC3339)
	}

	filter := service.RevenueFilter{
		GroupBy:   groupBy,
		StartDate: startDateStr,
		EndDate:   endDateStr,
	}

	data, err := h.revenueService.GetRevenueStatistics(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, data))
}
