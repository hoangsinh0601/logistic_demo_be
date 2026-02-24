package handler

import (
	"net/http"
	"time"

	"backend/internal/middleware"
	"backend/internal/service"

	"github.com/gin-gonic/gin"
)

type StatisticsHandler struct {
	statisticsService service.StatisticsService
}

func NewStatisticsHandler(statisticsService service.StatisticsService) *StatisticsHandler {
	return &StatisticsHandler{statisticsService: statisticsService}
}

func (h *StatisticsHandler) RegisterRoutes(router *gin.RouterGroup) {
	statsGroup := router.Group("/api/statistics")
	{
		statsGroup.GET("", middleware.RequirePermission("dashboard.read"), h.GetStatistics)
	}
}

// @Summary      Get Dashboard Statistics
// @Description  Get import/export totals, profit and top ranked items bounded by time
// @Tags         Statistics
// @Accept       json
// @Produce      json
// @Param        start_date query string false "Start Date (RFC3339)"
// @Param        end_date   query string false "End Date (RFC3339)"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} map[string]interface{} "Invalid date format"
// @Failure      401 {object} map[string]interface{} "Unauthorized"
// @Failure      500 {object} map[string]interface{} "Internal server error"
// @Security     BearerAuth
// @Router       /api/statistics [get]
func (h *StatisticsHandler) GetStatistics(c *gin.Context) {
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	var startDate, endDate time.Time
	var err error

	// Default to current month if no dates are provided
	now := time.Now()
	if startDateStr == "" {
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	} else {
		startDate, err = time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_date format, expected RFC3339"})
			return
		}
	}

	if endDateStr == "" {
		endDate = now
	} else {
		endDate, err = time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end_date format, expected RFC3339"})
			return
		}
	}

	stats, err := h.statisticsService.GetStatistics(c.Request.Context(), startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    stats,
	})
}
