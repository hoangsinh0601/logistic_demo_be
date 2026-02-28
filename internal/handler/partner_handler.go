package handler

import (
	"net/http"
	"strconv"

	"backend/internal/middleware"
	"backend/internal/service"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type PartnerHandler struct {
	partnerService service.PartnerService
}

func NewPartnerHandler(partnerService service.PartnerService) *PartnerHandler {
	return &PartnerHandler{partnerService: partnerService}
}

func (h *PartnerHandler) RegisterRoutes(router *gin.RouterGroup) {
	partners := router.Group("/api/partners")
	{
		partners.GET("", middleware.RequirePermission("partners.read"), h.ListPartners)
		partners.POST("", middleware.RequirePermission("partners.write"), h.CreatePartner)
		partners.PUT("/:id", middleware.RequirePermission("partners.write"), h.UpdatePartner)
		partners.DELETE("/:id", middleware.RequirePermission("partners.write"), h.DeletePartner)
	}
}

// ListPartners returns paginated partners with optional type/search filter
// @Summary      List partners
// @Tags         partners
// @Security     BearerAuth
// @Produce      json
// @Param        page    query     int     false  "Page number (default: 1)"
// @Param        limit   query     int     false  "Items per page (default: 20)"
// @Param        type    query     string  false  "Filter by type: CUSTOMER, SUPPLIER, BOTH"
// @Param        search  query     string  false  "Search by name, company, phone, email"
// @Success      200     {object}  response.Response
// @Router       /api/partners [get]
func (h *PartnerHandler) ListPartners(c *gin.Context) {
	page := 1
	limit := 20
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	partnerType := c.Query("type")
	search := c.Query("search")

	partners, total, err := h.partnerService.GetPartners(c.Request.Context(), partnerType, search, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.SuccessWithPagination(http.StatusOK, partners, page, limit, total))
}

// CreatePartner creates a new partner
// @Summary      Create partner
// @Tags         partners
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        payload  body  service.CreatePartnerRequest  true  "Partner payload"
// @Success      201  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Router       /api/partners [post]
func (h *PartnerHandler) CreatePartner(c *gin.Context) {
	var req service.CreatePartnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	partner, err := h.partnerService.CreatePartner(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, partner))
}

// UpdatePartner updates an existing partner
// @Summary      Update partner
// @Tags         partners
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id       path  string                        true  "Partner ID"
// @Param        payload  body  service.UpdatePartnerRequest   true  "Update payload"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Router       /api/partners/{id} [put]
func (h *PartnerHandler) UpdatePartner(c *gin.Context) {
	id := c.Param("id")

	var req service.UpdatePartnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	partner, err := h.partnerService.UpdatePartner(c.Request.Context(), id, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, partner))
}

// DeletePartner deletes a partner (soft delete)
// @Summary      Delete partner
// @Tags         partners
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Partner ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Router       /api/partners/{id} [delete]
func (h *PartnerHandler) DeletePartner(c *gin.Context) {
	id := c.Param("id")

	if err := h.partnerService.DeletePartner(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, gin.H{"message": "Partner deleted successfully"}))
}
