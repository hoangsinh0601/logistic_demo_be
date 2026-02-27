package handler

import (
	"net/http"
	"strconv"

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

// GetExpenses returns a paginated list of expense entries
// @Summary      Get expenses
// @Description  Retrieves a paginated list of expense entries
// @Tags         expenses
// @Security     BearerAuth
// @Produce      json
// @Param        page   query     int  false  "Page number (default 1)"
// @Param        limit  query     int  false  "Number of items per page (default 20)"
// @Success      200    {object}  response.Response{data=object}
// @Failure      500    {object}  response.Response
// @Router       /api/expenses [get]
func (h *ExpenseHandler) GetExpenses(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	expenses, total, err := h.expenseService.GetExpenses(c.Request.Context(), page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, map[string]interface{}{
		"expenses": expenses,
		"total":    total,
		"page":     page,
		"limit":    limit,
	}))
}

// CreateExpense handles expense creation with currency conversion, FCT, and deductibility
// @Summary      Create expense
// @Description  Creates a new expense entry with currency conversion, FCT, VAT calculations, and deductibility logic
// @Tags         expenses
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        payload  body      service.CreateExpenseRequest  true  "Create Expense Payload"
// @Success      201      {object}  response.Response{data=service.ExpenseResponse}
// @Failure      400      {object}  response.Response
// @Router       /api/expenses [post]
func (h *ExpenseHandler) CreateExpense(c *gin.Context) {
	var req service.CreateExpenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	userID, _ := c.Get("userID")
	userIDStr, _ := userID.(string)

	expense, err := h.expenseService.CreateExpense(c.Request.Context(), userIDStr, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, expense))
}
