package handler

import (
	"net/http"

	"backend/internal/middleware"
	"backend/internal/service"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type ExpenseHandler struct {
	expenseService service.ExpenseService
}

func NewExpenseHandler(expenseService service.ExpenseService) *ExpenseHandler {
	return &ExpenseHandler{expenseService: expenseService}
}

func (h *ExpenseHandler) RegisterRoutes(router *gin.RouterGroup) {
	expenses := router.Group("/api/expenses")
	{
		expenses.GET("", middleware.RequirePermission("expenses.read"), h.GetExpenses)
		expenses.POST("", middleware.RequirePermission("expenses.write"), h.CreateExpense)
	}
}

// GetExpenses returns all expense entries
func (h *ExpenseHandler) GetExpenses(c *gin.Context) {
	expenses, err := h.expenseService.GetExpenses(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, expenses))
}

// CreateExpense handles expense creation with currency conversion, FCT, and deductibility
func (h *ExpenseHandler) CreateExpense(c *gin.Context) {
	var req service.CreateExpenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	expense, err := h.expenseService.CreateExpense(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, expense))
}
