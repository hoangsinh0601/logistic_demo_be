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
		approvals.GET("/:id", middleware.RequirePermission("approvals.read"), h.GetApprovalRequest)
		approvals.PUT("/:id/approve", middleware.RequirePermission("approvals.approve"), h.ApproveRequest)
		approvals.PUT("/:id/reject", middleware.RequirePermission("approvals.approve"), h.RejectRequest)
	}
}

// ListApprovalRequests returns approval requests, optionally filtered by status
// @Summary      List approval requests
// @Description  Retrieves a paginated list of approval requests, optionally filtered by status
// @Tags         approvals
// @Security     BearerAuth
// @Produce      json
// @Param        status  query     string  false  "Filter by status (PENDING, APPROVED, REJECTED)"
// @Param        page    query     int     false  "Page number (default 1)"
// @Param        limit   query     int     false  "Number of items per page (default 20)"
// @Success      200     {object}  response.Response{data=object}
// @Failure      500     {object}  response.Response
// @Router       /api/approvals [get]
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

	c.JSON(http.StatusOK, response.Success(http.StatusOK, map[string]interface{}{
		"data":  approvals,
		"total": total,
		"page":  page,
		"limit": limit,
	}))
}

// GetApprovalRequest returns a single approval request by ID
// @Summary      Get approval request detail
// @Description  Retrieves a single approval request by ID with full details
// @Tags         approvals
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Approval Request ID"
// @Success      200  {object}  response.Response{data=service.ApprovalRequestResponse}
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /api/approvals/{id} [get]
func (h *ApprovalHandler) GetApprovalRequest(c *gin.Context) {
	id := c.Param("id")

	result, err := h.approvalService.GetApprovalRequest(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, response.Error(http.StatusNotFound, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, result))
}

// ApproveRequest approves a pending approval request
// @Summary      Approve request
// @Description  Approves a pending approval request by ID, executing post-approval actions
// @Tags         approvals
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Approval Request ID"
// @Success      200  {object}  response.Response{data=service.ApprovalRequestResponse}
// @Failure      400  {object}  response.Response
// @Router       /api/approvals/{id}/approve [put]
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
// @Summary      Reject request
// @Description  Rejects a pending approval request by ID with an optional reason
// @Tags         approvals
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id       path      string                      true   "Approval Request ID"
// @Param        payload  body      service.RejectRequestDTO    false  "Rejection reason"
// @Success      200      {object}  response.Response{data=service.ApprovalRequestResponse}
// @Failure      400      {object}  response.Response
// @Router       /api/approvals/{id}/reject [put]
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
