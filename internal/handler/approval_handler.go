package handler

import (
	"net/http"
	"strconv"

	"backend/internal/middleware"
	"backend/internal/service"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type ApprovalHandler struct {
	approvalService service.ApprovalService
}

func NewApprovalHandler(approvalService service.ApprovalService) *ApprovalHandler {
	return &ApprovalHandler{approvalService: approvalService}
}

func (h *ApprovalHandler) RegisterRoutes(router *gin.RouterGroup) {
	approvals := router.Group("/api/approvals")
	{
		approvals.GET("", middleware.RequirePermission("approvals.read"), h.ListApprovalRequests)
		approvals.PUT("/:id/approve", middleware.RequirePermission("APPROVE_INVOICE"), h.ApproveRequest)
		approvals.PUT("/:id/reject", middleware.RequirePermission("APPROVE_INVOICE"), h.RejectRequest)
	}
}

// ListApprovalRequests returns approval requests, optionally filtered by status
func (h *ApprovalHandler) ListApprovalRequests(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	filter := service.ApprovalFilter{
		Status: c.Query("status"),
		Page:   page,
		Limit:  limit,
	}

	approvals, total, err := h.approvalService.ListApprovalRequests(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data":   approvals,
		"total":  total,
		"page":   page,
		"limit":  limit,
	})
}

// ApproveRequest approves a pending approval request
func (h *ApprovalHandler) ApproveRequest(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")
	userIDStr, _ := userID.(string)

	result, err := h.approvalService.ApproveRequest(c.Request.Context(), id, userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, result))
}

// RejectRequest rejects a pending approval request
func (h *ApprovalHandler) RejectRequest(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("userID")
	userIDStr, _ := userID.(string)

	var req service.RejectRequestDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body — reason is optional
		req.Reason = ""
	}

	result, err := h.approvalService.RejectRequest(c.Request.Context(), id, userIDStr, req.Reason)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, result))
}
