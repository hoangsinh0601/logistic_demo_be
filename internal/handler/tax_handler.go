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
	{
		tax.GET("", middleware.RequirePermission("tax_rules.read"), h.GetTaxRules)
		tax.GET("/active", middleware.RequirePermission("tax_rules.read"), h.GetActiveTaxRate)
		tax.POST("", middleware.RequirePermission("tax_rules.write"), h.CreateTaxRule)
		tax.PUT("/:id", middleware.RequirePermission("tax_rules.write"), h.UpdateTaxRule)
		tax.DELETE("/:id", middleware.RequirePermission("tax_rules.write"), h.DeleteTaxRule)
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

// GetActiveTaxRate returns the currently active tax rate for a given type
func (h *TaxHandler) GetActiveTaxRate(c *gin.Context) {
	taxType := c.Query("type")
	if taxType == "" {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "query parameter 'type' is required (VAT_INLAND, VAT_INTL, FCT)"))
		return
	}

	rate, err := h.taxService.GetActiveTaxRate(c.Request.Context(), taxType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	if rate == nil {
		c.JSON(http.StatusNotFound, response.Error(http.StatusNotFound, "no active tax rule found for type '"+taxType+"'"))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, rate))
}

// CreateTaxRule creates a new tax rule entry
func (h *TaxHandler) CreateTaxRule(c *gin.Context) {
	var req service.CreateTaxRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	userID, _ := c.Get("userID")
	userIDStr, _ := userID.(string)

	rule, err := h.taxService.CreateTaxRule(c.Request.Context(), req, userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, rule))
}

// UpdateTaxRule updates an existing tax rule
func (h *TaxHandler) UpdateTaxRule(c *gin.Context) {
	id := c.Param("id")

	var req service.UpdateTaxRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	userID, _ := c.Get("userID")
	userIDStr, _ := userID.(string)

	rule, err := h.taxService.UpdateTaxRule(c.Request.Context(), id, req, userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, rule))
}

// DeleteTaxRule deletes a tax rule
func (h *TaxHandler) DeleteTaxRule(c *gin.Context) {
	id := c.Param("id")

	userID, _ := c.Get("userID")
	userIDStr, _ := userID.(string)

	if err := h.taxService.DeleteTaxRule(c.Request.Context(), id, userIDStr); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, gin.H{"message": "Tax rule deleted successfully"}))
}
