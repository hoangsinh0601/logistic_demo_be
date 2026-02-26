package handler

import (
	"net/http"
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
		invoices.PUT("/:id/approve", middleware.RequirePermission("APPROVE_INVOICE"), h.ApproveInvoice)
		invoices.PUT("/:id/reject", middleware.RequirePermission("APPROVE_INVOICE"), h.RejectInvoice)
	}

	// Revenue statistics — separate route group
	stats := router.Group("/api/statistics")
	{
		stats.GET("/revenue", middleware.RequirePermission("VIEW_FINANCE_REPORT"), h.GetRevenueStatistics)
	}
}

// CreateInvoice creates a new invoice from an order or expense
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

// ListInvoices returns invoices, optionally filtered by approval_status
func (h *InvoiceHandler) ListInvoices(c *gin.Context) {
	filter := service.InvoiceFilter{
		ApprovalStatus: c.Query("status"),
	}

	invoices, err := h.invoiceService.ListInvoices(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, invoices))
}

// ApproveInvoice approves a pending invoice
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
