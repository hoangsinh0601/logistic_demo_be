package handler

import (
	"net/http"

	"backend/internal/middleware"
	"backend/internal/service"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type TaxHandler struct {
	taxService service.TaxService
}

func NewTaxHandler(taxService service.TaxService) *TaxHandler {
	return &TaxHandler{taxService: taxService}
}

func (h *TaxHandler) RegisterRoutes(router *gin.RouterGroup) {
	tax := router.Group("/api/tax-rules")
	tax.Use(middleware.RequireRole("admin", "manager", "staff"))
	{
		tax.GET("", h.GetTaxRules)
		tax.POST("", h.CreateTaxRule)
	}
}

// GetTaxRules returns all tax rules ordered by effective_from DESC
func (h *TaxHandler) GetTaxRules(c *gin.Context) {
	rules, err := h.taxService.GetTaxRules(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, rules))
}

// CreateTaxRule creates a new tax rule entry
func (h *TaxHandler) CreateTaxRule(c *gin.Context) {
	var req service.CreateTaxRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	rule, err := h.taxService.CreateTaxRule(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, rule))
}
