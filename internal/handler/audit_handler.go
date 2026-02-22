package handler

import (
	"net/http"
	"strconv"

	"backend/internal/middleware"
	"backend/internal/service"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type AuditHandler struct {
	auditService service.AuditService
}

func NewAuditHandler(auditService service.AuditService) *AuditHandler {
	return &AuditHandler{auditService: auditService}
}

func (h *AuditHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/api/audit-logs")
	group.Use(middleware.RequireRole("admin", "manager")) // Protect history logs
	{
		group.GET("", h.GetAuditLogs)
	}
}

// GetAuditLogs retrieves strictly paginated records with Users pre-loaded joining details
// @Summary      Get audit logs
// @Description  Retrieves list of audit logs securely mapping User interaction history
// @Tags         audit
// @Security     BearerAuth
// @Produce      json
// @Param        page   query     int  false  "Page number (default 1)"
// @Param        limit  query     int  false  "Number of items per page (default 20)"
// @Success      200    {object}  response.Response{data=object}
// @Router       /api/audit-logs [get]
func (h *AuditHandler) GetAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	logs, total, err := h.auditService.GetAuditLogs(c.Request.Context(), page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, "Failed to retrieve audit logs: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, map[string]interface{}{
		"logs":  logs,
		"total": total,
		"page":  page,
		"limit": limit,
	}))
}
